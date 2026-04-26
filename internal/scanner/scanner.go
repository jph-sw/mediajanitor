package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jph-sw/mediajanitor/internal/arr"
	"github.com/jph-sw/mediajanitor/internal/torrent"
)

// ProgressReporter receives incremental progress updates during a scan.
type ProgressReporter interface {
	// SetTotal sets the estimated total number of files (may be approximate).
	SetTotal(n int64)
	// Add increments the processed count by n.
	Add(n int64)
	// Finish marks the progress display as complete.
	Finish()
}

// nopProgress discards all progress reports.
type nopProgress struct{}

func (nopProgress) SetTotal(_ int64) {}
func (nopProgress) Add(_ int64)      {}
func (nopProgress) Finish()          {}

// FileResult holds classification data for one file found during the walk.
type FileResult struct {
	Path       string
	SizeBytes  int64
	Inode      uint64
	Mtime      time.Time
	Category   Category
	LibSource  string // "sonarr" | "radarr" | ""
	LibRef     string // series/movie title
	TorrentRef string // torrent name
}

// ScanResult is returned by Scanner.Scan.
type ScanResult struct {
	ScanID        int64
	DownloadsRoot string
	StartedAt     time.Time
	FinishedAt    time.Time
	TotalFiles    int64
	TotalBytes    int64
	ByCategory    map[Category]CategoryStats
	Files         []FileResult
}

// CategoryStats holds aggregate numbers for one category.
type CategoryStats struct {
	Count int64
	Bytes int64
}

// Scanner orchestrates fetching data from *arr and torrent clients, walking
// the downloads directory, and classifying every file.
type Scanner struct {
	sonarr   *arr.SonarrClient
	radarr   *arr.RadarrClient
	torrents []torrent.Client
	store    Store
}

// Store is the minimal persistence interface the scanner needs.
// The full implementation lives in internal/store.
type Store interface {
	CreateScan(ctx context.Context, root string) (int64, error)
	SaveFiles(ctx context.Context, scanID int64, files []FileResult) error
	FinalizeScan(ctx context.Context, scanID int64, status string, totalFiles, totalBytes int64, finishedAt time.Time) error
}

// New constructs a Scanner.
func New(sonarr *arr.SonarrClient, radarr *arr.RadarrClient, clients []torrent.Client, store Store) *Scanner {
	return &Scanner{
		sonarr:   sonarr,
		radarr:   radarr,
		torrents: clients,
		store:    store,
	}
}

// Scan runs a full scan and returns the result. Pass nil for progress to
// suppress progress reporting.
func (s *Scanner) Scan(ctx context.Context, downloadsRoot string, progress ProgressReporter) (*ScanResult, error) {
	if progress == nil {
		progress = nopProgress{}
	}

	if !hardlinkDetectionAvailable {
		slog.Warn("hardlink detection unavailable on this platform; library matching falls back to path comparison")
	}

	startedAt := time.Now()

	scanID, err := s.store.CreateScan(ctx, downloadsRoot)
	if err != nil {
		return nil, fmt.Errorf("creating scan record: %w", err)
	}

	// Fetch library and torrent data concurrently.
	var (
		episodeFiles []arr.EpisodeFile
		movieFiles   []arr.MovieFile
		torrentFiles []torrent.File
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		if s.sonarr != nil {
			episodeFiles, err = s.sonarr.ListEpisodeFiles(gctx)
			if err != nil {
				return fmt.Errorf("sonarr: %w", err)
			}
		}
		return nil
	})

	g.Go(func() error {
		var err error
		if s.radarr != nil {
			movieFiles, err = s.radarr.ListMovieFiles(gctx)
			if err != nil {
				return fmt.Errorf("radarr: %w", err)
			}
		}
		return nil
	})

	for _, client := range s.torrents {
		client := client
		g.Go(func() error {
			qb, ok := client.(interface {
				ListTorrentsWithFiles(context.Context) ([]torrent.File, error)
			})
			if !ok {
				return fmt.Errorf("client %s does not support ListTorrentsWithFiles", client.Name())
			}
			files, err := qb.ListTorrentsWithFiles(gctx)
			if err != nil {
				return fmt.Errorf("torrent client %s: %w", client.Name(), err)
			}
			torrentFiles = append(torrentFiles, files...)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		_ = s.store.FinalizeScan(ctx, scanID, "failed", 0, 0, time.Now())
		return nil, err
	}

	// Build lookup tables.
	inodeMap, libPathMap := buildLibraryMaps(episodeFiles, movieFiles)
	torrentPathMap := buildTorrentMap(torrentFiles)

	classifier := NewClassifier(inodeMap, libPathMap, torrentPathMap)

	// Walk the downloads directory.
	var results []FileResult
	err = filepath.WalkDir(downloadsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk error", "path", path, "err", err)
			return nil
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil // skip dirs and symlinks
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		fi, err := d.Info()
		if err != nil {
			slog.Warn("stat error", "path", path, "err", err)
			progress.Add(1)
			return nil
		}

		inodeNum, _ := inode(fi)
		result := classifier.Classify(path, inodeNum)

		fr := FileResult{
			Path:      path,
			SizeBytes: fi.Size(),
			Inode:     inodeNum,
			Mtime:     fi.ModTime(),
			Category:  result.Category,
		}
		if result.LibraryRef != nil {
			fr.LibSource = result.LibraryRef.Source
			fr.LibRef = result.LibraryRef.Title
		}
		if result.TorrentRef != nil {
			fr.TorrentRef = result.TorrentRef.Name
		}

		results = append(results, fr)
		progress.Add(1)
		return nil
	})

	status := "completed"
	if err != nil {
		if ctx.Err() != nil {
			status = "failed"
		} else {
			slog.Warn("walk completed with errors", "err", err)
		}
	}

	var totalBytes int64
	for _, r := range results {
		totalBytes += r.SizeBytes
	}

	finishedAt := time.Now()
	progress.Finish()

	if saveErr := s.store.SaveFiles(ctx, scanID, results); saveErr != nil {
		slog.Warn("saving files to store", "err", saveErr)
	}
	if finalErr := s.store.FinalizeScan(ctx, scanID, status, int64(len(results)), totalBytes, finishedAt); finalErr != nil {
		slog.Warn("finalizing scan", "err", finalErr)
	}

	byCategory := make(map[Category]CategoryStats)
	for _, r := range results {
		s := byCategory[r.Category]
		s.Count++
		s.Bytes += r.SizeBytes
		byCategory[r.Category] = s
	}

	return &ScanResult{
		ScanID:        scanID,
		DownloadsRoot: downloadsRoot,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		TotalFiles:    int64(len(results)),
		TotalBytes:    totalBytes,
		ByCategory:    byCategory,
		Files:         results,
	}, nil
}

func buildLibraryMaps(
	episodes []arr.EpisodeFile,
	movies []arr.MovieFile,
) (inodes map[uint64]LibraryRef, paths map[string]LibraryRef) {
	inodes = make(map[uint64]LibraryRef)
	paths = make(map[string]LibraryRef, len(episodes)+len(movies))

	for _, ef := range episodes {
		ref := LibraryRef{Source: "sonarr", Title: ef.SeriesTitle}
		paths[ef.Path] = ref
		if fi, err := os.Stat(ef.Path); err == nil {
			if ino, ok := inode(fi); ok {
				inodes[ino] = ref
			}
		}
	}

	for _, mf := range movies {
		ref := LibraryRef{Source: "radarr", Title: mf.MovieTitle}
		paths[mf.Path] = ref
		if fi, err := os.Stat(mf.Path); err == nil {
			if ino, ok := inode(fi); ok {
				inodes[ino] = ref
			}
		}
	}

	return inodes, paths
}

func buildTorrentMap(files []torrent.File) map[string]TorrentRef {
	m := make(map[string]TorrentRef, len(files))
	for _, f := range files {
		m[f.Path] = TorrentRef{Name: f.Path}
	}
	return m
}
