package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the top-level configuration for mediajanitor.
type Config struct {
	DownloadsRoot string `koanf:"downloads_root"`
	LogFormat     string `koanf:"log_format"` // "text" | "json"

	Sonarr      ArrConfig         `koanf:"sonarr"`
	Radarr      ArrConfig         `koanf:"radarr"`
	QBittorrent QBittorrentConfig `koanf:"qbittorrent"`

	DB DBConfig `koanf:"db"`

	// SafeRoots is an allowlist of directories that may be scanned/cleaned.
	// If empty, the safety check uses built-in blocklist only.
	SafeRoots []string `koanf:"safe_roots"`
}

// ArrConfig holds connection settings for Sonarr or Radarr.
type ArrConfig struct {
	URL    string `koanf:"url"`
	APIKey string `koanf:"api_key"`
}

// QBittorrentConfig holds connection settings for qBittorrent.
type QBittorrentConfig struct {
	URL      string `koanf:"url"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
}

// DBConfig holds database settings.
type DBConfig struct {
	Path string `koanf:"path"`
}

// DefaultConfigPath returns $XDG_CONFIG_HOME/mediajanitor/config.yaml.
func DefaultConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "mediajanitor", "config.yaml")
}

// DefaultDBPath returns $XDG_DATA_HOME/mediajanitor/data.db.
func DefaultDBPath() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(base, "mediajanitor", "data.db")
}

// Load reads config from the YAML file and environment variables.
// Missing file is not an error — caller can decide to run `config init`.
func Load(cfgPath string) (*Config, error) {
	k := koanf.New(".")

	// Load from file if it exists.
	if _, err := os.Stat(cfgPath); err == nil {
		if err := k.Load(file.Provider(cfgPath), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("loading config file %s: %w", cfgPath, err)
		}
	}

	// Overlay env vars: MEDIAJANITOR_SONARR_API_KEY → sonarr.api_key
	if err := k.Load(env.Provider("MEDIAJANITOR_", ".", func(s string) string {
		return strings.ReplaceAll(
			strings.ToLower(strings.TrimPrefix(s, "MEDIAJANITOR_")),
			"_", ".")
	}), nil); err != nil {
		return nil, fmt.Errorf("loading environment variables: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	// Apply defaults.
	if cfg.DB.Path == "" {
		cfg.DB.Path = DefaultDBPath()
	}

	return &cfg, nil
}

// Validate returns an error if required fields are missing.
func (c *Config) Validate() error {
	var errs []string
	if c.DownloadsRoot == "" {
		errs = append(errs, "downloads_root is required")
	}
	if c.Sonarr.URL != "" && c.Sonarr.APIKey == "" {
		errs = append(errs, "sonarr.api_key is required when sonarr.url is set")
	}
	if c.Radarr.URL != "" && c.Radarr.APIKey == "" {
		errs = append(errs, "radarr.api_key is required when radarr.url is set")
	}
	if len(errs) > 0 {
		return errors.New("config validation failed:\n  " + strings.Join(errs, "\n  "))
	}
	return nil
}

// MaskSecrets returns a copy of the config with sensitive values redacted,
// suitable for display.
func (c Config) MaskSecrets() Config {
	c.Sonarr.APIKey = maskKey(c.Sonarr.APIKey)
	c.Radarr.APIKey = maskKey(c.Radarr.APIKey)
	c.QBittorrent.Password = maskKey(c.QBittorrent.Password)
	return c
}

func maskKey(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}
