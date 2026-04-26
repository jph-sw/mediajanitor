package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/jph-sw/mediajanitor/internal/config"
)

// rootFlags holds the global flag values parsed by the root command.
type rootFlags struct {
	configPath string
	verbose    bool
	quiet      bool
	noColor    bool
	logFormat  string
}

var rootF rootFlags

// New returns the root cobra command with all subcommands registered.
func New(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "mediajanitor",
		Short: "Reconcile your downloads folder against Sonarr, Radarr, and qBittorrent",
		Long: `mediajanitor walks a downloads directory and classifies every file as:

  library+seeding  hardlinked to a *arr-tracked file AND held by an active torrent
  library-only     hardlinked to a *arr-tracked file, not seeding
  seeding-only     held by torrent client, not in any *arr library
  orphan           neither in a library nor seeding — safe to delete

Run 'mediajanitor config init' to get started.`,
		Version:           version,
		SilenceUsage:      true,
		PersistentPreRunE: persistentPreRun,
	}

	root.PersistentFlags().StringVar(&rootF.configPath, "config", config.DefaultConfigPath(), "config file path")
	root.PersistentFlags().BoolVarP(&rootF.verbose, "verbose", "v", false, "enable debug logging")
	root.PersistentFlags().BoolVarP(&rootF.quiet, "quiet", "q", false, "only log errors")
	root.PersistentFlags().BoolVar(&rootF.noColor, "no-color", false, "disable styled output")
	root.PersistentFlags().StringVar(&rootF.logFormat, "log-format", "", "log format: text or json (default: text on TTY, json when piped)")

	root.AddCommand(
		newConfigCmd(),
		newScanCmd(),
		newReportCmd(),
		newCleanCmd(),
		newCompletionCmd(root),
	)

	return root
}

func persistentPreRun(cmd *cobra.Command, _ []string) error {
	// Skip setup for completion commands.
	if cmd.Name() == "completion" || cmd.Parent() != nil && cmd.Parent().Name() == "completion" {
		return nil
	}

	logFormat := rootF.logFormat
	if logFormat == "" {
		if IsTTY() {
			logFormat = "text"
		} else {
			logFormat = "json"
		}
	}

	level := slog.LevelInfo
	if rootF.verbose {
		level = slog.LevelDebug
	}
	if rootF.quiet {
		level = slog.LevelError
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if logFormat == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))

	if rootF.noColor {
		// Disable lipgloss color output globally.
		os.Setenv("NO_COLOR", "1")
	}

	return nil
}

// loadConfig loads and validates the config, printing a helpful error if missing.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load(rootF.configPath)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%w\n\nRun 'mediajanitor config init' to create a config file", err)
	}
	return cfg, nil
}

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:       "completion {bash|zsh|fish|powershell}",
		Short:     "Generate shell completion scripts",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(os.Stdout)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unknown shell %q", args[0])
			}
		},
	}
}
