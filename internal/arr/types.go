package arr

// EpisodeFile represents a single episode file as returned by Sonarr.
type EpisodeFile struct {
	ID           int    `json:"id"`
	SeriesID     int    `json:"seriesId"`
	SeriesTitle  string `json:"-"` // populated by client from series index
	RelativePath string `json:"relativePath"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
}

// MovieFile represents a single movie file as returned by Radarr.
type MovieFile struct {
	ID           int    `json:"id"`
	MovieID      int    `json:"movieId"`
	MovieTitle   string `json:"-"` // populated by client from movie index
	RelativePath string `json:"relativePath"`
	Path         string `json:"path"`
	Size         int64  `json:"size"`
}

// Series is a minimal Sonarr series record.
type Series struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// Movie is a minimal Radarr movie record.
type Movie struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}
