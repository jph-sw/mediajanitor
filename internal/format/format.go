package format

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	_   = iota
	KiB = 1 << (10 * iota)
	MiB
	GiB
	TiB
)

// Bytes formats a byte count as a human-readable string (e.g. "14.8 TB").
func Bytes(n int64) string {
	switch {
	case n >= TiB:
		return fmt.Sprintf("%.1f TB", float64(n)/float64(TiB))
	case n >= GiB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GiB))
	case n >= MiB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MiB))
	case n >= KiB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KiB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ParseBytes parses a human-readable size string into bytes.
// Accepts: "100MB", "1.5GB", "2TiB", "500 KB", etc.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	// Find where the number ends.
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("no numeric prefix in %q", s)
	}

	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, fmt.Errorf("parsing number in %q: %w", s, err)
	}

	unit := strings.ToUpper(strings.TrimSpace(s[i:]))
	var mult float64
	switch unit {
	case "", "B":
		mult = 1
	case "K", "KB", "KIB":
		mult = float64(KiB)
	case "M", "MB", "MIB":
		mult = float64(MiB)
	case "G", "GB", "GIB":
		mult = float64(GiB)
	case "T", "TB", "TIB":
		mult = float64(TiB)
	default:
		return 0, fmt.Errorf("unknown unit %q in %q", unit, s)
	}

	return int64(num * mult), nil
}

// Duration formats a duration as a short human string ("6m12s", "2h5m").
func Duration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// ParseDuration parses durations like "30d", "6mo", "1y", "2h", "30m".
// Falls back to time.ParseDuration for standard Go formats.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// Handle custom suffixes first.
	for _, suffix := range []struct {
		sfx  string
		days int
	}{
		{"mo", 30},
		{"y", 365},
		{"d", 1},
		{"w", 7},
	} {
		if strings.HasSuffix(s, suffix.sfx) {
			numStr := strings.TrimSuffix(s, suffix.sfx)
			n, err := strconv.Atoi(numStr)
			if err != nil {
				return 0, fmt.Errorf("parsing %q: %w", s, err)
			}
			return time.Duration(n*suffix.days) * 24 * time.Hour, nil
		}
	}

	return time.ParseDuration(s)
}

// Percent formats a ratio (0–1) as a percentage string ("55.4%").
func Percent(ratio float64) string {
	return fmt.Sprintf("%.1f%%", ratio*100)
}

// Comma inserts thousands separators into an integer string.
func Comma(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
