package arr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RadarrConfig holds connection parameters for a Radarr instance.
type RadarrConfig struct {
	URL    string
	APIKey string
}

// RadarrClient fetches file data from Radarr's v3 API.
type RadarrClient struct {
	cfg  RadarrConfig
	http *http.Client
	base string
}

// NewRadarr constructs a Radarr client.
func NewRadarr(cfg RadarrConfig) *RadarrClient {
	return &RadarrClient{
		cfg:  cfg,
		base: strings.TrimRight(cfg.URL, "/"),
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *RadarrClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("radarr GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("radarr: invalid API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("radarr GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// HealthCheck pings Radarr's system/status endpoint.
func (c *RadarrClient) HealthCheck(ctx context.Context) error {
	var status struct {
		AppName string `json:"appName"`
	}
	return c.get(ctx, "/api/v3/system/status", &status)
}

// ListMovies returns all movies tracked by Radarr.
func (c *RadarrClient) ListMovies(ctx context.Context) ([]Movie, error) {
	var movies []Movie
	if err := c.get(ctx, "/api/v3/movie", &movies); err != nil {
		return nil, err
	}
	return movies, nil
}

// ListMovieFiles returns all movie files tracked by Radarr.
// It fetches the movie list first to annotate each file with its title.
func (c *RadarrClient) ListMovieFiles(ctx context.Context) ([]MovieFile, error) {
	movies, err := c.ListMovies(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching movies for file annotation: %w", err)
	}

	var all []MovieFile
	for _, m := range movies {
		var files []MovieFile
		if err := c.get(ctx, fmt.Sprintf("/api/v3/moviefile?movieId=%d", m.ID), &files); err != nil {
			return nil, fmt.Errorf("fetching movie files for movie %d (%s): %w", m.ID, m.Title, err)
		}
		for i := range files {
			files[i].MovieTitle = m.Title
		}
		all = append(all, files...)
	}
	return all, nil
}

// radarrMovieFileResponse mirrors Radarr's extended movie file response shape.
type radarrMovieFileResponse struct {
	ID           int    `json:"id"`
	MovieID      int    `json:"movieId"`
	RelativePath string `json:"relativePath"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
}

// ListMovieFilesRaw fetches raw movie file records without title annotation.
// Useful when you already have a title index.
func (c *RadarrClient) ListMovieFilesRaw(ctx context.Context) ([]radarrMovieFileResponse, error) {
	var files []radarrMovieFileResponse
	if err := c.get(ctx, "/api/v3/moviefile", &files); err != nil {
		return nil, err
	}
	return files, nil
}
