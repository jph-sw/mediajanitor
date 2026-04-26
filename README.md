# mediajanitor

Reconcile your downloads folder against Sonarr, Radarr, and qBittorrent. Find orphaned files safely.

## What it does

mediajanitor walks a (potentially multi-TB) downloads directory and classifies every file into one of four categories:

| Category | Meaning |
|---|---|
| **library+seeding** | Hardlinked to a Sonarr/Radarr-tracked file AND held by an active torrent — ideal state |
| **library-only** | Hardlinked to a tracked file, not currently seeding |
| **seeding-only** | Held by torrent client, not in any *arr library |
| **orphan** | Neither in a library nor seeding — safe to delete |

Matching is inode-based on Linux (survives renames and hardlinks). On other platforms it falls back to path matching with a warning.

## Install

**Go install (latest):**
```
go install github.com/jph-sw/mediajanitor/cmd/mediajanitor@latest
```

**Prebuilt binary:** Download from the [releases page](https://github.com/jph-sw/mediajanitor/releases).

**Docker:**
```
docker run --rm -v ~/.config/mediajanitor:/root/.config/mediajanitor \
  -v /downloads:/downloads \
  ghcr.io/jph-sw/mediajanitor:latest scan
```
Note: most users will want the native binary. The image exists for all-container setups and is under 15 MB (scratch base).

## Quickstart

```
mediajanitor config init     # interactive setup wizard
mediajanitor config test     # verify connectivity to all services
mediajanitor scan            # full scan, persists to SQLite
mediajanitor report          # summary of latest scan
```

## Command reference

### `config`

```
mediajanitor config init     # interactive wizard — writes config.yaml (mode 0600)
mediajanitor config show     # print config with secrets masked
mediajanitor config test     # hit each service's health endpoint, print pass/fail + timing
```

### `scan`

```
mediajanitor scan [flags]

Flags:
  --downloads PATH    override downloads root from config
  --no-progress       disable progress bar
  --json              emit JSON summary instead of human output
```

### `report`

```
mediajanitor report [flags]

Flags:
  --scan-id N                 specific past scan (default: latest)
  --category {orphan|library_only|seeding_only|library_seeding|all}
  --sort {size|mtime|path}    default: size desc
  --limit N                   max rows (default 50; 0 = unlimited)
  --older-than DURATION       e.g. 30d, 6mo, 1y
  --min-size SIZE             e.g. 100MB, 1.5GB
  --max-size SIZE
  --path-prefix PREFIX        only files under this subpath
  --json                      NDJSON output for piping
  --output FILE               write paths only (one per line)
  --group-by-dir              top-level directory size totals
```

### `clean`

Defaults to **dry-run**. Pass `--confirm` to actually delete.

```
# From a file (edit the list first):
mediajanitor report --category orphan --output orphans.txt
$EDITOR orphans.txt
mediajanitor clean --from orphans.txt --confirm

# By filter:
mediajanitor clean --category orphan --older-than 1y --min-size 100MB
mediajanitor clean --category orphan --older-than 1y --min-size 100MB --confirm

Flags:
  --from FILE              paths file (# comments, blank lines OK)
  --category               same as report
  --older-than / --min-size / --max-size / --path-prefix
  --confirm                actually delete (default: dry-run)
  --yes                    skip confirmation prompt (for scripts)
  --interactive {file|dir} prompt y/n per file or per directory
  --remove-empty-dirs      prune empty directories after deletion
  --allow-library-files    override library-file safety check
```

Every deletion is logged to `deletion_log` in SQLite before `os.Remove` is called.

### Global flags

```
  --config PATH           override config file location
  --verbose / -v          debug logging
  --quiet / -q            errors only
  --no-color              disable styled output
  --log-format {text|json}
```

### Shell completions

```
mediajanitor completion bash  >> ~/.bash_completion
mediajanitor completion zsh   > ~/.zsh/completions/_mediajanitor
mediajanitor completion fish  > ~/.config/fish/completions/mediajanitor.fish
```

## Safety model

- **Default dry-run on `clean`.** `--confirm` required for any actual deletion.
- **Protected paths.** Refuses to scan or clean `/`, `/home`, `/etc`, `/usr`, `/var`, `/root`, `$HOME`, or any ancestor of `$HOME`.
- **No symlink following** during the walk.
- **Deletion log.** Every deletion writes a `deletion_log` row before `os.Remove`. Survives crashes.
- **Library file guard.** `clean` refuses to touch `library_seeding` or `library_only` files unless `--allow-library-files` is passed.
- **Confirmation summary** before any non-dry-run delete (skipped with `--yes`).

## Config file

Stored at `$XDG_CONFIG_HOME/mediajanitor/config.yaml` (usually `~/.config/mediajanitor/config.yaml`), mode `0600`.

```yaml
downloads_root: /downloads

sonarr:
  url: http://localhost:8989
  api_key: your_key

radarr:
  url: http://localhost:7878
  api_key: your_key

qbittorrent:
  url: http://localhost:8080
  username: admin
  password: your_password
```

All values can be overridden by `MEDIAJANITOR_*` environment variables (e.g. `MEDIAJANITOR_SONARR_API_KEY`).

## Database

SQLite at `$XDG_DATA_HOME/mediajanitor/data.db` (usually `~/.local/share/mediajanitor/data.db`). Each `scan` run appends a new scan; old scans are retained for history. Schema migrations run automatically on startup via goose.

## Supported clients

| Client | Status |
|---|---|
| qBittorrent | ✓ Supported |
| Deluge | Planned |
| Transmission | Planned |

The torrent client interface (`internal/torrent.Client`) is designed for extension — adding a new client only requires implementing `ListTorrents`, `ListFiles`, `HealthCheck`, and `Name`.

## Decisions

- **`modernc.org/sqlite`** over `mattn/go-sqlite3`: pure Go, no CGO, compiles to a `scratch`-based Docker image without cross-compilation pain.
- **`koanf`** over `viper`: simpler API, explicit layering (file → env → flags), no global state.
- **inode-based matching**: survives hardlinks and renames. Path-based matching is the fallback when inode data is unavailable.
- **No `sqlc`-generated boilerplate in the skeleton**: the store uses hand-written queries. `sqlc` can be layered in once the schema stabilizes without changing any public interfaces.
- **`errgroup` for parallel fetch**: Sonarr, Radarr, and torrent clients are all queried concurrently; a single failure cancels the group and the scan is marked `failed`.
- **Scan-then-act**: `clean` always reads from a prior scan, never re-queries *arr/torrent live. This makes the delete set deterministic and auditable.
