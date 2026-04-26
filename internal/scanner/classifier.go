package scanner

// Category classifies a file's relationship to the library and torrent client.
type Category string

const (
	// CategoryLibrarySeeding — inode matches a tracked *arr file AND held by an active torrent.
	CategoryLibrarySeeding Category = "library_seeding"
	// CategoryLibraryOnly — inode matches a tracked *arr file, not seeding.
	CategoryLibraryOnly Category = "library_only"
	// CategorySeedingOnly — held by torrent client, not in any *arr library.
	CategorySeedingOnly Category = "seeding_only"
	// CategoryOrphan — neither in a library nor seeding.
	CategoryOrphan Category = "orphan"
)

// LibraryRef records which *arr application and media title owns an inode.
type LibraryRef struct {
	Source string // "sonarr" or "radarr"
	Title  string // series or movie title
}

// TorrentRef records which torrent owns a path.
type TorrentRef struct {
	Client string // torrent client name
	Name   string // torrent name
}

// Classifier holds the lookup tables built from *arr and torrent data.
type Classifier struct {
	// inodes maps inode number → library reference.
	// Only populated on Linux; empty on other platforms.
	inodes map[uint64]LibraryRef

	// paths maps absolute file path → library reference, used as fallback
	// when inode data is not available (or as additional lookup).
	libraryPaths map[string]LibraryRef

	// torrentPaths maps absolute file path → torrent reference.
	torrentPaths map[string]TorrentRef
}

// NewClassifier constructs a Classifier from pre-built lookup tables.
func NewClassifier(
	inodes map[uint64]LibraryRef,
	libraryPaths map[string]LibraryRef,
	torrentPaths map[string]TorrentRef,
) *Classifier {
	return &Classifier{
		inodes:       inodes,
		libraryPaths: libraryPaths,
		torrentPaths: torrentPaths,
	}
}

// ClassifyResult is the output of classifying a single file.
type ClassifyResult struct {
	Category   Category
	LibraryRef *LibraryRef
	TorrentRef *TorrentRef
}

// Classify determines the category of a file given its path and inode number.
// inodeNum == 0 means inode data is unavailable (non-Linux).
func (c *Classifier) Classify(path string, inodeNum uint64) ClassifyResult {
	var libRef *LibraryRef

	// Prefer inode-based library matching (survives renames, hardlinks).
	if inodeNum != 0 {
		if ref, ok := c.inodes[inodeNum]; ok {
			libRef = &ref
		}
	}
	// Fall back to path-based matching.
	if libRef == nil {
		if ref, ok := c.libraryPaths[path]; ok {
			libRef = &ref
		}
	}

	var torRef *TorrentRef
	if ref, ok := c.torrentPaths[path]; ok {
		torRef = &ref
	}

	return ClassifyResult{
		Category:   determineCategory(libRef, torRef),
		LibraryRef: libRef,
		TorrentRef: torRef,
	}
}

func determineCategory(lib *LibraryRef, tor *TorrentRef) Category {
	switch {
	case lib != nil && tor != nil:
		return CategoryLibrarySeeding
	case lib != nil:
		return CategoryLibraryOnly
	case tor != nil:
		return CategorySeedingOnly
	default:
		return CategoryOrphan
	}
}
