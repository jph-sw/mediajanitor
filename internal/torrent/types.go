package torrent

// State represents the current state of a torrent.
type State string

const (
	StateSeeding     State = "seeding"
	StateDownloading State = "downloading"
	StatePaused      State = "paused"
	StateError       State = "error"
	StateOther       State = "other"
)

// Torrent is a client-agnostic representation of a torrent.
type Torrent struct {
	ID       string // client-specific identifier (hash for qbit)
	Name     string
	SavePath string
	State    State
	Progress float64
}

// File is a file within a torrent.
type File struct {
	Path     string // absolute path (SavePath + relative path from client)
	Size     int64
	Progress float64
}
