package format

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{int64(1.5 * float64(MiB)), "1.5 MB"},
		{15891392115, "14.8 GB"}, // int64(14.8 * GiB)
		{1319413953331, "1.2 TB"}, // int64(1.2 * TiB)
	}
	for _, tt := range tests {
		got := Bytes(tt.in)
		if got != tt.want {
			t.Errorf("Bytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"100MB", 100 * MiB, false},
		{"1.5GB", int64(1.5 * float64(GiB)), false},
		{"2TiB", 2 * TiB, false},
		{"500 KB", 500 * KiB, false},
		{"1024", 1024, false},
		{"0", 0, false},
		{"", 0, true},
		{"abc", 0, true},
		{"1ZB", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseBytes(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseBytes(%q) error = %v, wantErr = %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseBytes(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"30d", 30 * 24 * time.Hour, false},
		{"6mo", 6 * 30 * 24 * time.Hour, false},
		{"1y", 365 * 24 * time.Hour, false},
		{"2w", 14 * 24 * time.Hour, false},
		{"2h", 2 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseDuration(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseDuration(%q) error = %v, wantErr = %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestComma(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{47832, "47,832"},
		{1000000, "1,000,000"},
	}
	for _, tt := range tests {
		got := Comma(tt.in)
		if got != tt.want {
			t.Errorf("Comma(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
