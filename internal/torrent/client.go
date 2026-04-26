package torrent

import "context"

// Client is the interface all torrent backend implementations must satisfy.
// Adding a new client (Deluge, Transmission, rTorrent) means implementing this
// interface and registering it in the config loader — the scanner never needs
// to know which concrete type it is talking to.
type Client interface {
	// Name returns a short identifier used in logs and config ("qbittorrent").
	Name() string

	// ListTorrents returns all torrents known to the client.
	ListTorrents(ctx context.Context) ([]Torrent, error)

	// ListFiles returns the files belonging to a specific torrent.
	// torrentID is the value from Torrent.ID.
	ListFiles(ctx context.Context, torrentID string) ([]File, error)

	// HealthCheck verifies the client is reachable and authenticated.
	HealthCheck(ctx context.Context) error
}
