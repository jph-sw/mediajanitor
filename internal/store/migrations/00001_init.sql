-- +goose Up
CREATE TABLE scans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at DATETIME NOT NULL,
    finished_at DATETIME,
    downloads_root TEXT NOT NULL,
    total_files INTEGER NOT NULL DEFAULT 0,
    total_bytes INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL  -- 'running' | 'completed' | 'failed'
);

CREATE TABLE files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id INTEGER NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    inode INTEGER,
    mtime DATETIME,
    category TEXT NOT NULL,  -- 'library_seeding' | 'library_only' | 'seeding_only' | 'orphan'
    library_source TEXT,     -- 'sonarr' | 'radarr' | NULL
    library_ref TEXT,        -- e.g. series/movie title
    torrent_ref TEXT
);

CREATE INDEX idx_files_scan_category ON files(scan_id, category);
CREATE INDEX idx_files_size ON files(scan_id, size_bytes DESC);
CREATE INDEX idx_files_mtime ON files(scan_id, mtime);

CREATE TABLE deletion_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    deleted_at DATETIME NOT NULL,
    path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    scan_id INTEGER REFERENCES scans(id),
    reason TEXT
);

-- +goose Down
DROP TABLE deletion_log;
DROP INDEX idx_files_mtime;
DROP INDEX idx_files_size;
DROP INDEX idx_files_scan_category;
DROP TABLE files;
DROP TABLE scans;
