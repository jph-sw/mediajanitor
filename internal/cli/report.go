package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jph-sw/mediajanitor/internal/format"
	"github.com/jph-sw/mediajanitor/internal/store"
)

type reportFlags struct {
	scanID     int64
	category   string
	sortBy     string
	limit      int
	olderThan  string
	minSize    string
	maxSize    string
	pathPrefix string
	jsonOutput bool
	outputFile string
	groupByDir bool
}

func newReportCmd() *cobra.Command {
	var f reportFlags

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show report from the latest scan",
		Long: `Reads from the latest scan in SQLite (or --scan-id for a specific past scan)
and prints a summary or drill-down of file classifications.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReport(cmd, f)
		},
	}

	cmd.Flags().Int64Var(&f.scanID, "scan-id", 0, "specific scan ID (default: latest)")
	cmd.Flags().StringVar(&f.category, "category", "", "filter: orphan, library_only, seeding_only, library_seeding, all")
	cmd.Flags().StringVar(&f.sortBy, "sort", "size", "sort by: size, mtime, path")
	cmd.Flags().IntVar(&f.limit, "limit", 50, "max rows (0 = unlimited)")
	cmd.Flags().StringVar(&f.olderThan, "older-than", "", "only files older than duration (e.g. 30d, 6mo, 1y)")
	cmd.Flags().StringVar(&f.minSize, "min-size", "", "minimum file size (e.g. 100MB)")
	cmd.Flags().StringVar(&f.maxSize, "max-size", "", "maximum file size (e.g. 1.5GB)")
	cmd.Flags().StringVar(&f.pathPrefix, "path-prefix", "", "only files under this subpath")
	cmd.Flags().BoolVar(&f.jsonOutput, "json", false, "NDJSON output (one object per line)")
	cmd.Flags().StringVar(&f.outputFile, "output", "", "write paths only to file (one per line)")
	cmd.Flags().BoolVar(&f.groupByDir, "group-by-dir", false, "collapse to top-level directories with size totals")

	return cmd
}

func runReport(_ *cobra.Command, f reportFlags) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	var scan *store.ScanRow
	if f.scanID > 0 {
		scan, err = db.ScanByID(nil, f.scanID)
	} else {
		scan, err = db.LatestScan(nil)
	}
	if err != nil {
		return fmt.Errorf("fetching scan: %w", err)
	}
	if scan == nil {
		return fmt.Errorf("no scan found — run 'mediajanitor scan' first")
	}

	q := buildFileQuery(scan.ID, f)

	// Summary-only mode (no category filter, no size filter, etc.)
	if f.category == "" && f.olderThan == "" && f.minSize == "" && f.maxSize == "" && f.pathPrefix == "" && !f.groupByDir {
		return printScanReport(db, scan, f)
	}

	files, err := db.ListFiles(nil, q)
	if err != nil {
		return fmt.Errorf("querying files: %w", err)
	}

	if f.outputFile != "" {
		return writePathsFile(f.outputFile, files)
	}

	if f.jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		for _, file := range files {
			if err := enc.Encode(file); err != nil {
				return err
			}
		}
		return nil
	}

	if f.groupByDir {
		printGroupedByDir(files, scan.DownloadsRoot)
		return nil
	}

	printFileTable(files, scan)
	return nil
}

func buildFileQuery(scanID int64, f reportFlags) store.FileQuery {
	q := store.FileQuery{
		ScanID:     scanID,
		Category:   f.category,
		SortBy:     f.sortBy,
		Limit:      f.limit,
		PathPrefix: f.pathPrefix,
	}

	if f.olderThan != "" {
		if d, err := format.ParseDuration(f.olderThan); err == nil {
			t := time.Now().Add(-d)
			q.OlderThan = &t
		}
	}
	if f.minSize != "" {
		if n, err := format.ParseBytes(f.minSize); err == nil {
			q.MinBytes = n
		}
	}
	if f.maxSize != "" {
		if n, err := format.ParseBytes(f.maxSize); err == nil {
			q.MaxBytes = n
		}
	}
	return q
}

func printScanReport(db *store.Store, scan *store.ScanRow, f reportFlags) error {
	var took string
	if scan.FinishedAt != nil {
		took = format.Duration(scan.FinishedAt.Sub(scan.StartedAt))
	}

	fmt.Printf("\n%s\n",
		Styles.Title.Render(fmt.Sprintf("Scan #%d — %s (took %s)",
			scan.ID,
			scan.StartedAt.Format("2006-01-02 15:04"),
			took)))
	fmt.Printf("Root: %s    Files: %s    Total: %s\n\n",
		scan.DownloadsRoot,
		format.Comma(scan.TotalFiles),
		format.Bytes(scan.TotalBytes))

	cats := []string{"library_seeding", "library_only", "seeding_only", "orphan"}
	for _, cat := range cats {
		catFiles, err := db.ListFiles(nil, store.FileQuery{ScanID: scan.ID, Category: cat, Limit: 0})
		if err != nil {
			continue
		}
		if len(catFiles) == 0 {
			continue
		}
		var totalBytes int64
		for _, file := range catFiles {
			totalBytes += file.SizeBytes
		}
		pct := float64(totalBytes) / float64(scan.TotalBytes)
		indicator := CategoryIndicator(cat)
		label := CategoryLabel(cat)
		desc := CategoryDescription(cat)
		style := CategoryStyle(cat)

		fmt.Printf("  %s  %-20s  %8s  %6s  %s\n",
			style.Render(indicator),
			style.Render(label),
			format.Bytes(totalBytes),
			format.Percent(pct),
			Styles.Dim.Render(desc))
	}
	fmt.Println()
	return nil
}

func printFileTable(files []store.FileRow, scan *store.ScanRow) {
	if len(files) == 0 {
		fmt.Println(Styles.Dim.Render("  (no files match)"))
		return
	}

	fmt.Printf("\n%s\n\n",
		Styles.Header.Render(fmt.Sprintf("%-8s  %-12s  %-16s  %s", "SIZE", "MTIME", "CATEGORY", "PATH")))

	for _, f := range files {
		mtime := ""
		if f.Mtime != nil {
			mtime = f.Mtime.Format("2006-01-02")
		}
		style := CategoryStyle(f.Category)
		fmt.Printf("%-8s  %-12s  %s  %s\n",
			format.Bytes(f.SizeBytes),
			mtime,
			style.Render(fmt.Sprintf("%-16s", CategoryLabel(f.Category))),
			f.Path)
	}
	_ = scan
}

func printGroupedByDir(files []store.FileRow, downloadsRoot string) {
	dirBytes := make(map[string]int64)
	dirCount := make(map[string]int)

	for _, f := range files {
		rel := f.Path
		if len(rel) > len(downloadsRoot)+1 {
			rel = rel[len(downloadsRoot)+1:]
		}
		// Get top-level directory component.
		top := rel
		for i, c := range rel {
			if c == '/' {
				top = rel[:i]
				break
			}
		}
		if top == "" {
			top = rel
		}
		dirBytes[top] += f.SizeBytes
		dirCount[top]++
	}

	fmt.Printf("\n%s\n\n",
		Styles.Header.Render(fmt.Sprintf("%-12s  %-8s  %s", "SIZE", "FILES", "DIRECTORY")))

	for dir, size := range dirBytes {
		fmt.Printf("%-12s  %-8s  %s/%s\n",
			format.Bytes(size),
			format.Comma(int64(dirCount[dir])),
			downloadsRoot, dir)
	}
}

func writePathsFile(path string, files []store.FileRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, file := range files {
		fmt.Fprintln(f, file.Path)
	}
	fmt.Printf("Wrote %d paths to %s\n", len(files), path)
	return nil
}
