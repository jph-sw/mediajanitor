package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/jph-sw/mediajanitor/internal/arr"
	"github.com/jph-sw/mediajanitor/internal/config"
	"github.com/jph-sw/mediajanitor/internal/torrent"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}
	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigShowCmd(),
		newConfigTestCmd(),
	)
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactively create a config file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigInit(rootF.configPath)
		},
	}
}

func runConfigInit(cfgPath string) error {
	fmt.Println(Styles.Title.Render("mediajanitor config init"))
	fmt.Println()

	r := bufio.NewReader(os.Stdin)

	prompt := func(label, defaultVal string) string {
		if defaultVal != "" {
			fmt.Printf("  %s [%s]: ", Styles.Bold.Render(label), Styles.Dim.Render(defaultVal))
		} else {
			fmt.Printf("  %s: ", Styles.Bold.Render(label))
		}
		line, _ := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defaultVal
		}
		return line
	}

	promptSecret := func(label string) string {
		fmt.Printf("  %s: ", Styles.Bold.Render(label))
		if term.IsTerminal(int(os.Stdin.Fd())) {
			b, _ := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			return string(b)
		}
		line, _ := r.ReadString('\n')
		return strings.TrimSpace(line)
	}

	fmt.Println(Styles.Section.Render("Downloads"))
	downloadsRoot := prompt("Downloads root path", "/downloads")
	fmt.Println()

	fmt.Println(Styles.Section.Render("Sonarr"))
	sonarrURL := prompt("Sonarr URL", "http://localhost:8989")
	sonarrKey := promptSecret("Sonarr API key")
	fmt.Println()

	fmt.Println(Styles.Section.Render("Radarr"))
	radarrURL := prompt("Radarr URL", "http://localhost:7878")
	radarrKey := promptSecret("Radarr API key")
	fmt.Println()

	fmt.Println(Styles.Section.Render("qBittorrent"))
	qbitURL := prompt("qBittorrent URL", "http://localhost:8080")
	qbitUser := prompt("qBittorrent username", "admin")
	qbitPass := promptSecret("qBittorrent password")
	fmt.Println()

	cfg := map[string]any{
		"downloads_root": downloadsRoot,
		"sonarr": map[string]any{
			"url":     sonarrURL,
			"api_key": sonarrKey,
		},
		"radarr": map[string]any{
			"url":     radarrURL,
			"api_key": radarrKey,
		},
		"qbittorrent": map[string]any{
			"url":      qbitURL,
			"username": qbitUser,
			"password": qbitPass,
		},
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	f, err := os.OpenFile(cfgPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf(Styles.Good.Render("✓")+" Config written to %s\n", cfgPath)
	return nil
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the loaded config (secrets masked)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(rootF.configPath)
			if err != nil {
				return err
			}
			masked := cfg.MaskSecrets()
			out, err := yaml.Marshal(masked)
			if err != nil {
				return err
			}
			fmt.Printf("# Config loaded from: %s\n\n", rootF.configPath)
			fmt.Print(string(out))
			return nil
		},
	}
}

func newConfigTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Test connectivity to each configured service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(rootF.configPath)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			type result struct {
				name    string
				elapsed time.Duration
				err     error
			}
			var results []result

			check := func(name string, fn func() error) {
				start := time.Now()
				err := fn()
				results = append(results, result{name, time.Since(start), err})
			}

			if cfg.Sonarr.URL != "" {
				check("Sonarr", func() error {
					return arr.NewSonarr(arr.SonarrConfig{
						URL:    cfg.Sonarr.URL,
						APIKey: cfg.Sonarr.APIKey,
					}).HealthCheck(ctx)
				})
			}

			if cfg.Radarr.URL != "" {
				check("Radarr", func() error {
					return arr.NewRadarr(arr.RadarrConfig{
						URL:    cfg.Radarr.URL,
						APIKey: cfg.Radarr.APIKey,
					}).HealthCheck(ctx)
				})
			}

			if cfg.QBittorrent.URL != "" {
				check("qBittorrent", func() error {
					client, err := torrent.NewQBittorrent(torrent.QBittorrentConfig{
						URL:      cfg.QBittorrent.URL,
						Username: cfg.QBittorrent.Username,
						Password: cfg.QBittorrent.Password,
					})
					if err != nil {
						return err
					}
					return client.HealthCheck(ctx)
				})
			}

			fmt.Println()
			allOK := true
			for _, r := range results {
				if r.err != nil {
					allOK = false
					fmt.Printf("  %s  %-16s  %s\n",
						Styles.Danger.Render("✗"),
						r.name,
						Styles.Danger.Render(r.err.Error()))
				} else {
					fmt.Printf("  %s  %-16s  %s\n",
						Styles.Good.Render("✓"),
						r.name,
						Styles.Dim.Render(fmt.Sprintf("%dms", r.elapsed.Milliseconds())))
				}
			}
			fmt.Println()

			if !allOK {
				return fmt.Errorf("one or more services failed health check")
			}
			return nil
		},
	}
}
