package torrent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// QBittorrentConfig holds connection parameters for a qBittorrent instance.
type QBittorrentConfig struct {
	URL      string
	Username string
	Password string
	Timeout  time.Duration
}

// qBittorrentClient implements Client for qBittorrent Web API v2.
type qBittorrentClient struct {
	cfg    QBittorrentConfig
	http   *http.Client
	baseURL string
}

// NewQBittorrent constructs a qBittorrent client. Call HealthCheck or any list
// method first — it authenticates lazily on the first request.
func NewQBittorrent(cfg QBittorrentConfig) (Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	base := strings.TrimRight(cfg.URL, "/")

	return &qBittorrentClient{
		cfg:     cfg,
		baseURL: base,
		http: &http.Client{
			Timeout: timeout,
			Jar:     jar,
		},
	}, nil
}

func (c *qBittorrentClient) Name() string { return "qbittorrent" }

func (c *qBittorrentClient) HealthCheck(ctx context.Context) error {
	if err := c.login(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/app/version", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent health check: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent health check: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (c *qBittorrentClient) login(ctx context.Context) error {
	form := url.Values{
		"username": {c.cfg.Username},
		"password": {c.cfg.Password},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/auth/login",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("building login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.baseURL)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("qbittorrent login: IP banned or too many failed attempts")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent login: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// qbTorrent mirrors the JSON shape returned by /api/v2/torrents/info.
type qbTorrent struct {
	Hash     string  `json:"hash"`
	Name     string  `json:"name"`
	SavePath string  `json:"save_path"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"`
}

func (c *qBittorrentClient) ListTorrents(ctx context.Context) ([]Torrent, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v2/torrents/info", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing torrents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing torrents: status %d", resp.StatusCode)
	}

	var raw []qbTorrent
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding torrents: %w", err)
	}

	out := make([]Torrent, 0, len(raw))
	for _, t := range raw {
		out = append(out, Torrent{
			ID:       t.Hash,
			Name:     t.Name,
			SavePath: t.SavePath,
			State:    mapQbState(t.State),
			Progress: t.Progress,
		})
	}
	return out, nil
}

// qbFile mirrors the JSON shape returned by /api/v2/torrents/files.
type qbFile struct {
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
}

func (c *qBittorrentClient) ListFiles(ctx context.Context, torrentID string) ([]File, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}

	u := c.baseURL + "/api/v2/torrents/files?hash=" + url.QueryEscape(torrentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing files for %s: %w", torrentID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing files for %s: status %d", torrentID, resp.StatusCode)
	}

	var raw []qbFile
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding files: %w", err)
	}

	// We need the save_path from the torrent to build absolute paths.
	// ListFiles is called after ListTorrents, so the caller must pass the
	// torrent's SavePath when constructing File.Path. Here we return relative
	// paths in File.Path and let the scanner resolve them.
	// Actually, to keep the interface clean we need the save_path; callers
	// should use ListTorrentsWithFiles instead for bulk work.
	out := make([]File, 0, len(raw))
	for _, f := range raw {
		out = append(out, File{
			Path:     f.Name, // relative; caller must prepend SavePath
			Size:     f.Size,
			Progress: f.Progress,
		})
	}
	return out, nil
}

// ListTorrentsWithFiles fetches all torrents and their files in one pass,
// returning File.Path as absolute paths. This is the method the scanner uses.
func (c *qBittorrentClient) ListTorrentsWithFiles(ctx context.Context) ([]File, error) {
	torrents, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}

	var all []File
	for _, t := range torrents {
		files, err := c.ListFiles(ctx, t.ID)
		if err != nil {
			return nil, fmt.Errorf("torrent %s (%s): %w", t.Name, t.ID, err)
		}
		savePath := strings.TrimRight(t.SavePath, "/")
		for _, f := range files {
			abs := filepath.Join(savePath, f.Path)
			all = append(all, File{
				Path:     abs,
				Size:     f.Size,
				Progress: f.Progress,
			})
		}
	}
	return all, nil
}

func mapQbState(s string) State {
	switch s {
	case "uploading", "stalledUP", "forcedUP", "queuedUP":
		return StateSeeding
	case "downloading", "stalledDL", "forcedDL", "queuedDL",
		"metaDL", "checkingDL", "allocating":
		return StateDownloading
	case "pausedUP", "pausedDL":
		return StatePaused
	case "error", "missingFiles", "unknown":
		return StateError
	default:
		return StateOther
	}
}

// TODO: implement Deluge client
// TODO: implement Transmission client
