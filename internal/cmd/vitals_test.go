package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVitalsFlag(t *testing.T) {
	tests := []struct {
		cmdLine string
		flag    string
		want    string
	}{
		{"dolt sql-server --port 3307", "--port", "3307"},
		{"dolt sql-server --port=52879 --data-dir=/tmp", "--port", "52879"},
		{"dolt sql-server --data-dir /tmp", "--port", ""},
		{"dolt sql-server", "--port", ""},
	}
	for _, tt := range tests {
		got := vitalsFlag(tt.cmdLine, tt.flag)
		if got != tt.want {
			t.Errorf("vitalsFlag(%q, %q) = %q, want %q", tt.cmdLine, tt.flag, got, tt.want)
		}
	}
}

func TestVitalsFormatCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{8263, "8,263"},
	}
	for _, tt := range tests {
		got := vitalsFormatCount(tt.n)
		if got != tt.want {
			t.Errorf("vitalsFormatCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestVitalsShortHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got := vitalsShortHome(filepath.Join(home, "gt", ".dolt-backup"))
	if got != "~/gt/.dolt-backup" {
		t.Errorf("vitalsShortHome: got %q, want %q", got, "~/gt/.dolt-backup")
	}

	got = vitalsShortHome("/tmp/other")
	if got != "/tmp/other" {
		t.Errorf("vitalsShortHome(/tmp/other) = %q, want /tmp/other", got)
	}
}
