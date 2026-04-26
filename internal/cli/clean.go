package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jph-sw/mediajanitor/internal/format"
	"github.com/jph-sw/mediajanitor/internal/store"
)

type cleanFlags struct {
	fromFile          string
	category          string
	olderThan         string
	minSize           string
	maxSize           string
	pathPrefix        string
	confirm           bool
	yes               bool
	interactive       string // "off" | "file" | "dir"
	removeEmptyDirs   bool
	allowLibraryFiles bool
}

func newCleanCmd() *cobra.Command {
	var f cleanFlags

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Delete files (dry-run by default, use --confirm to actually delete)",
		Long: `Acts on a set of files identified either from a file (--from) or filter flags.

Defaults to dry-run. Pass --confirm to perform actual deletions.
Every deletion is logged to the database before os.Remove is called.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClean(cmd, f)
		},
	}

	cmd.Flags().StringVar(&f.fromFile, "from", "", "file with one absolute path per line (# comments allowed)")
	cmd.Flags().StringVar(&f.category, "category", "", "filter by category")
	cmd.Flags().StringVar(&f.olderThan, "older-than", "", "only files older than duration")
	cmd.Flags().StringVar(&f.minSize, "min-size", "", "minimum file size")
	cmd.Flags().StringVar(&f.maxSize, "max-size", "", "maximum file size")
	cmd.Flags().StringVar(&f.pathPrefix, "path-prefix", "", "only files under this subpath")
	cmd.Flags().BoolVar(&f.confirm, "confirm", false, "actually delete files (without this flag, dry-run)")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "skip interactive confirmation prompt (for scripting)")
	cmd.Flags().StringVar(&f.interactive, "interactive", "off", "prompt per file (file) or per directory (dir)")
	cmd.Flags().BoolVar(&f.removeEmptyDirs, "remove-empty-dirs", false, "prune empty directories after deletion")
	cmd.Flags().BoolVar(&f.allowLibraryFiles, "allow-library-files", false, "allow deleting library+seeding or library-only files (dangerous)")

	return cmd
}

func runClean(_ *cobra.Command, f cleanFlags) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	var paths []pathEntry

	if f.fromFile != "" {
		paths, err = loadPathsFromFile(f.fromFile)
		if err != nil {
			return err
		}
	} else {
		scan, err := db.LatestScan(nil)
		if err != nil {
			return fmt.Errorf("fetching scan: %w", err)
		}
		if scan == nil {
			return fmt.Errorf("no scan found — run 'mediajanitor scan' first")
		}

		q := buildFileQuery(scan.ID, reportFlags{
			category:   f.category,
			olderThan:  f.olderThan,
			minSize:    f.minSize,
			maxSize:    f.maxSize,
			pathPrefix: f.pathPrefix,
			limit:      0,
		})
		files, err := db.ListFiles(nil, q)
		if err != nil {
			return err
		}
		for _, file := range files {
			paths = append(paths, pathEntry{path: file.Path, size: file.SizeBytes, category: file.Category})
		}
	}

	if len(paths) == 0 {
		fmt.Println(Styles.Dim.Render("No files matched — nothing to do."))
		return nil
	}

	// Safety check: refuse library files unless --allow-library-files.
	if !f.allowLibraryFiles {
		for _, p := range paths {
			if p.category == "library_seeding" || p.category == "library_only" {
				return fmt.Errorf("target set contains library files (category=%s, path=%s).\nPass --allow-library-files to override this safety check", p.category, p.path)
			}
		}
	}

	var totalBytes int64
	for _, p := range paths {
		totalBytes += p.size
	}

	dryRun := !f.confirm
	verb := "Would delete"
	if !dryRun {
		verb = "Will delete"
	}
	fmt.Printf("\n%s %s files totaling %s.\n\n",
		verb,
		format.Comma(int64(len(paths))),
		format.Bytes(totalBytes))

	if dryRun {
		for _, p := range paths {
			fmt.Printf("  [dry-run] %s  %s\n", format.Bytes(p.size), p.path)
		}
		fmt.Printf("\n%s\n", Styles.Dim.Render("Re-run with --confirm to actually delete."))
		return nil
	}

	// Confirmation prompt.
	if !f.yes {
		fmt.Printf(Styles.Danger.Render("About to delete %s files totaling %s. This is irreversible. Continue? [y/N] "),
			format.Comma(int64(len(paths))), format.Bytes(totalBytes))
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	ctx := context.Background()
	scan, _ := db.LatestScan(ctx)
	var scanID int64
	if scan != nil {
		scanID = scan.ID
	}

	deleted := 0
	var freedBytes int64
	var errs []string

	for _, p := range paths {
		if f.interactive == "file" {
			fmt.Printf("  Delete %s %s? [y/N] ", format.Bytes(p.size), p.path)
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				continue
			}
		}

		// Log before delete — if the process crashes mid-way, the log still has the record.
		if logErr := db.LogDeletion(ctx, p.path, p.size, scanID, "clean"); logErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to log deletion of %s: %v\n", p.path, logErr)
		}

		if err := os.Remove(p.path); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p.path, err))
			continue
		}

		deleted++
		freedBytes += p.size
		fmt.Printf("  %s  %s  %s\n", Styles.Good.Render("✓"), format.Bytes(p.size), p.path)
	}

	fmt.Printf("\nDeleted %s files, freed %s.\n", format.Comma(int64(deleted)), format.Bytes(freedBytes))

	if len(errs) > 0 {
		fmt.Printf("\n%s\n", Styles.Danger.Render(fmt.Sprintf("%d errors:", len(errs))))
		for _, e := range errs {
			fmt.Printf("  %s\n", e)
		}
	}

	if f.removeEmptyDirs && cfg.DownloadsRoot != "" {
		pruneEmptyDirs(cfg.DownloadsRoot)
	}

	return nil
}

type pathEntry struct {
	path     string
	size     int64
	category string
}

func loadPathsFromFile(filename string) ([]pathEntry, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("opening paths file: %w", err)
	}
	defer f.Close()

	var entries []pathEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fi, err := os.Stat(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", line, err)
			continue
		}
		entries = append(entries, pathEntry{path: line, size: fi.Size()})
	}
	return entries, scanner.Err()
}

func pruneEmptyDirs(root string) {
	// Walk in reverse (deepest first via os.ReadDir recursion).
	_ = walkDirsBottomUp(root, func(path string) {
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 0 || path == root {
			return
		}
		if err := os.Remove(path); err == nil {
			fmt.Printf("  %s  (empty dir) %s\n", Styles.Dim.Render("✓"), path)
		}
	})
}

func walkDirsBottomUp(root string, fn func(string)) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			_ = walkDirsBottomUp(root+"/"+e.Name(), fn)
		}
	}
	fn(root)
	return nil
}

// cleanFlags needs access to the scan's started_at for "older than" filtering.
// Using time.Time zero as "no filter".
var _ = time.Time{}
