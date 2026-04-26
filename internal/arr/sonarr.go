package arr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SonarrConfig holds connection parameters for a Sonarr instance.
type SonarrConfig struct {
	URL    string
	APIKey string
}

// SonarrClient fetches file data from Sonarr's v3 API.
type SonarrClient struct {
	cfg  SonarrConfig
	http *http.Client
	base string
}

// NewSonarr constructs a Sonarr client.
func NewSonarr(cfg SonarrConfig) *SonarrClient {
	return &SonarrClient{
		cfg:  cfg,
		base: strings.TrimRight(cfg.URL, "/"),
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *SonarrClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sonarr GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("sonarr: invalid API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sonarr GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// HealthCheck pings Sonarr's system/status endpoint.
func (c *SonarrClient) HealthCheck(ctx context.Context) error {
	var status struct {
		AppName string `json:"appName"`
	}
	return c.get(ctx, "/api/v3/system/status", &status)
}

// ListSeries returns all series tracked by Sonarr.
func (c *SonarrClient) ListSeries(ctx context.Context) ([]Series, error) {
	var series []Series
	if err := c.get(ctx, "/api/v3/series", &series); err != nil {
		return nil, err
	}
	return series, nil
}

// ListEpisodeFiles returns all episode files across all series.
// Sonarr's /api/v3/episodefile requires a seriesId parameter, so we fetch
// one page per series and concatenate.
func (c *SonarrClient) ListEpisodeFiles(ctx context.Context) ([]EpisodeFile, error) {
	series, err := c.ListSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching series for file annotation: %w", err)
	}

	var all []EpisodeFile
	for _, s := range series {
		var files []EpisodeFile
		if err := c.get(ctx, fmt.Sprintf("/api/v3/episodefile?seriesId=%d", s.ID), &files); err != nil {
			return nil, fmt.Errorf("fetching episode files for series %d (%s): %w", s.ID, s.Title, err)
		}
		for i := range files {
			files[i].SeriesTitle = s.Title
		}
		all = append(all, files...)
	}
	return all, nil
}
