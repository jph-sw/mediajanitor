package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/jph-sw/mediajanitor/internal/arr"
	"github.com/jph-sw/mediajanitor/internal/format"
	"github.com/jph-sw/mediajanitor/internal/scanner"
	"github.com/jph-sw/mediajanitor/internal/store"
	"github.com/jph-sw/mediajanitor/internal/torrent"
)

type scanFlags struct {
	downloadsPath string
	noProgress    bool
	jsonOutput    bool
}

func newScanCmd() *cobra.Command {
	var f scanFlags

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan downloads directory and classify files",
		Long: `Fetches file lists from Sonarr, Radarr, and qBittorrent, then walks the
downloads directory classifying each file. Results are persisted to SQLite.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runScan(cmd, f)
		},
	}

	cmd.Flags().StringVar(&f.downloadsPath, "downloads", "", "override downloads root from config")
	cmd.Flags().BoolVar(&f.noProgress, "no-progress", false, "disable progress bar")
	cmd.Flags().BoolVar(&f.jsonOutput, "json", false, "print JSON summary on completion")

	return cmd
}

// barProgress wraps schollz/progressbar to implement scanner.ProgressReporter.
type barProgress struct {
	bar *progressbar.ProgressBar
}

func (p *barProgress) SetTotal(n int64) { p.bar.ChangeMax64(n) }
func (p *barProgress) Add(n int64)      { _ = p.bar.Add64(n) }
func (p *barProgress) Finish()          { _ = p.bar.Finish(); fmt.Println() }

func runScan(cmd *cobra.Command, f scanFlags) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	downloadsRoot := cfg.DownloadsRoot
	if f.downloadsPath != "" {
		downloadsRoot = f.downloadsPath
	}

	if err := validateScanRoot(downloadsRoot); err != nil {
		return err
	}

	db, err := store.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	sonarrClient := arr.NewSonarr(arr.SonarrConfig{
		URL:    cfg.Sonarr.URL,
		APIKey: cfg.Sonarr.APIKey,
	})
	radarrClient := arr.NewRadarr(arr.RadarrConfig{
		URL:    cfg.Radarr.URL,
		APIKey: cfg.Radarr.APIKey,
	})

	qbitClient, err := torrent.NewQBittorrent(torrent.QBittorrentConfig{
		URL:      cfg.QBittorrent.URL,
		Username: cfg.QBittorrent.Username,
		Password: cfg.QBittorrent.Password,
	})
	if err != nil {
		return fmt.Errorf("creating torrent client: %w", err)
	}

	s := scanner.New(sonarrClient, radarrClient, []torrent.Client{qbitClient}, db)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var progress scanner.ProgressReporter
	if !f.noProgress && IsTTY() {
		bar := progressbar.NewOptions64(-1,
			progressbar.OptionSetDescription("Scanning "+downloadsRoot),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("files"),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionThrottle(100*time.Millisecond),
			progressbar.OptionOnCompletion(func() {}),
		)
		progress = &barProgress{bar: bar}
	}

	fmt.Fprintf(os.Stderr, "Fetching file lists from *arr and torrent clients...\n")
	result, err := s.Scan(ctx, downloadsRoot, progress)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if f.jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	printScanSummary(result)
	return nil
}

func printScanSummary(r *scanner.ScanResult) {
	took := r.FinishedAt.Sub(r.StartedAt)
	fmt.Printf("\n%s\n", Styles.Title.Render(fmt.Sprintf("Scan #%d — %s (took %s)",
		r.ScanID,
		r.StartedAt.Format("2006-01-02 15:04"),
		format.Duration(took))))
	fmt.Printf("Root: %s    Files: %s    Total: %s\n\n",
		r.DownloadsRoot,
		format.Comma(r.TotalFiles),
		format.Bytes(r.TotalBytes))

	cats := []scanner.Category{
		scanner.CategoryLibrarySeeding,
		scanner.CategoryLibraryOnly,
		scanner.CategorySeedingOnly,
		scanner.CategoryOrphan,
	}

	for _, cat := range cats {
		stats := r.ByCategory[cat]
		if stats.Count == 0 {
			continue
		}
		pct := float64(stats.Bytes) / float64(r.TotalBytes)
		indicator := CategoryIndicator(string(cat))
		label := fmt.Sprintf("%-16s", CategoryLabel(string(cat)))
		desc := CategoryDescription(string(cat))

		style := CategoryStyle(string(cat))
		fmt.Printf("  %s  %s  %8s  %6s  %s\n",
			style.Render(indicator),
			style.Render(label),
			format.Bytes(stats.Bytes),
			format.Percent(pct),
			Styles.Dim.Render(desc))
	}
	fmt.Println()
}

// validateScanRoot returns an error if root is a protected path.
func validateScanRoot(root string) error {
	protected := []string{"/", "/home", "/etc", "/usr", "/var", "/root", "/boot", "/sys", "/proc"}
	home := os.Getenv("HOME")
	if home != "" {
		protected = append(protected, home)
	}
	for _, p := range protected {
		if root == p {
			return fmt.Errorf("refusing to scan protected path %q — set a specific downloads subdirectory", root)
		}
	}
	return nil
}
