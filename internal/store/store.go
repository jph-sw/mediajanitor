package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/jph-sw/mediajanitor/internal/scanner"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, running any pending migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateScan inserts a new scan record with status "running" and returns its ID.
func (s *Store) CreateScan(ctx context.Context, root string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO scans (started_at, downloads_root, status) VALUES (?, ?, 'running')`,
		time.Now().UTC(), root)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinalizeScan updates the scan record with final stats and status.
func (s *Store) FinalizeScan(ctx context.Context, scanID int64, status string, totalFiles, totalBytes int64, finishedAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scans SET status=?, total_files=?, total_bytes=?, finished_at=? WHERE id=?`,
		status, totalFiles, totalBytes, finishedAt.UTC(), scanID)
	return err
}

// SaveFiles bulk-inserts file records for a scan.
func (s *Store) SaveFiles(ctx context.Context, scanID int64, files []scanner.FileResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if rErr := tx.Rollback(); rErr != nil {
				slog.Warn("rolling back file save", "err", rErr)
			}
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO files
			(scan_id, path, size_bytes, inode, mtime, category, library_source, library_ref, torrent_ref)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range files {
		var inodeVal any
		if f.Inode != 0 {
			inodeVal = f.Inode
		}
		_, err = stmt.ExecContext(ctx,
			scanID, f.Path, f.SizeBytes, inodeVal, f.Mtime.UTC(),
			string(f.Category), nullStr(f.LibSource), nullStr(f.LibRef), nullStr(f.TorrentRef))
		if err != nil {
			return fmt.Errorf("inserting file %s: %w", f.Path, err)
		}
	}

	return tx.Commit()
}

// ScanRow holds a single scan record from the database.
type ScanRow struct {
	ID            int64
	StartedAt     time.Time
	FinishedAt    *time.Time
	DownloadsRoot string
	TotalFiles    int64
	TotalBytes    int64
	Status        string
}

// LatestScan returns the most recent completed scan.
func (s *Store) LatestScan(ctx context.Context) (*ScanRow, error) {
	return s.scanByQuery(ctx,
		`SELECT id, started_at, finished_at, downloads_root, total_files, total_bytes, status
		 FROM scans WHERE status='completed' ORDER BY id DESC LIMIT 1`)
}

// ScanByID returns a scan by its ID.
func (s *Store) ScanByID(ctx context.Context, id int64) (*ScanRow, error) {
	return s.scanByQuery(ctx,
		`SELECT id, started_at, finished_at, downloads_root, total_files, total_bytes, status
		 FROM scans WHERE id=?`, id)
}

func (s *Store) scanByQuery(ctx context.Context, q string, args ...any) (*ScanRow, error) {
	row := s.db.QueryRowContext(ctx, q, args...)
	var r ScanRow
	var finishedAt sql.NullTime
	err := row.Scan(&r.ID, &r.StartedAt, &finishedAt, &r.DownloadsRoot,
		&r.TotalFiles, &r.TotalBytes, &r.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		r.FinishedAt = &finishedAt.Time
	}
	return &r, nil
}

// FileQuery holds filters for ListFiles.
type FileQuery struct {
	ScanID     int64
	Category   string
	OlderThan  *time.Time
	MinBytes   int64
	MaxBytes   int64
	PathPrefix string
	SortBy     string // "size" | "mtime" | "path"
	Limit      int
	Offset     int
}

// FileRow is a single file record from the database.
type FileRow struct {
	ID            int64
	ScanID        int64
	Path          string
	SizeBytes     int64
	Inode         *int64
	Mtime         *time.Time
	Category      string
	LibrarySource *string
	LibraryRef    *string
	TorrentRef    *string
}

// ListFiles returns files matching the given query.
func (s *Store) ListFiles(ctx context.Context, q FileQuery) ([]FileRow, error) {
	query := `SELECT id, scan_id, path, size_bytes, inode, mtime, category,
		library_source, library_ref, torrent_ref FROM files WHERE scan_id=?`
	args := []any{q.ScanID}

	if q.Category != "" && q.Category != "all" {
		query += " AND category=?"
		args = append(args, q.Category)
	}
	if q.OlderThan != nil {
		query += " AND mtime < ?"
		args = append(args, q.OlderThan.UTC())
	}
	if q.MinBytes > 0 {
		query += " AND size_bytes >= ?"
		args = append(args, q.MinBytes)
	}
	if q.MaxBytes > 0 {
		query += " AND size_bytes <= ?"
		args = append(args, q.MaxBytes)
	}
	if q.PathPrefix != "" {
		query += " AND path LIKE ?"
		args = append(args, q.PathPrefix+"%")
	}

	switch q.SortBy {
	case "mtime":
		query += " ORDER BY mtime DESC"
	case "path":
		query += " ORDER BY path ASC"
	default:
		query += " ORDER BY size_bytes DESC"
	}

	if q.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	if q.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", q.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FileRow
	for rows.Next() {
		var r FileRow
		var inode sql.NullInt64
		var mtime sql.NullTime
		var libSrc, libRef, torRef sql.NullString
		if err := rows.Scan(&r.ID, &r.ScanID, &r.Path, &r.SizeBytes,
			&inode, &mtime, &r.Category, &libSrc, &libRef, &torRef); err != nil {
			return nil, err
		}
		if inode.Valid {
			r.Inode = &inode.Int64
		}
		if mtime.Valid {
			r.Mtime = &mtime.Time
		}
		if libSrc.Valid {
			r.LibrarySource = &libSrc.String
		}
		if libRef.Valid {
			r.LibraryRef = &libRef.String
		}
		if torRef.Valid {
			r.TorrentRef = &torRef.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LogDeletion records a deletion in the deletion_log table.
func (s *Store) LogDeletion(ctx context.Context, path string, sizeBytes, scanID int64, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deletion_log (deleted_at, path, size_bytes, scan_id, reason) VALUES (?,?,?,?,?)`,
		time.Now().UTC(), path, sizeBytes, scanID, reason)
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
