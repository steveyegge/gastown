package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseBeadsVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    beadsVersion
		wantErr bool
	}{
		{"0.44.0", beadsVersion{0, 44, 0}, false},
		{"1.2.3", beadsVersion{1, 2, 3}, false},
		{"0.44.0-dev", beadsVersion{0, 44, 0}, false},
		{"v0.44.0", beadsVersion{0, 44, 0}, false},
		{"0.44", beadsVersion{0, 44, 0}, false},
		{"10.20.30", beadsVersion{10, 20, 30}, false},
		{"invalid", beadsVersion{}, true},
		{"", beadsVersion{}, true},
		{"a.b.c", beadsVersion{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseBeadsVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBeadsVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseBeadsVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestBeadsVersionCompare(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		{"0.44.0", "0.44.0", 0},
		{"0.44.0", "0.43.0", 1},
		{"0.43.0", "0.44.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"0.44.1", "0.44.0", 1},
		{"0.44.0", "0.44.1", -1},
		{"1.2.3", "1.2.3", 0},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			v1, err := parseBeadsVersion(tt.v1)
			if err != nil {
				t.Fatalf("failed to parse v1 %q: %v", tt.v1, err)
			}
			v2, err := parseBeadsVersion(tt.v2)
			if err != nil {
				t.Fatalf("failed to parse v2 %q: %v", tt.v2, err)
			}

			got := v1.compare(v2)
			if got != tt.want {
				t.Errorf("(%s).compare(%s) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestValidateBeadsVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"meets minimum", "0.44.0", false},
		{"above minimum", "0.45.0", false},
		{"below minimum", "0.43.0", true},
		{"way above minimum", "1.0.0", false},
		{"invalid version", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBeadsVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBeadsVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestVersionCacheSaveLoad(t *testing.T) {
	// Create a temp directory for the cache
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Ensure the .gt directory exists
	gtDir := filepath.Join(tmpDir, ".gt")
	if err := os.MkdirAll(gtDir, 0755); err != nil {
		t.Fatalf("failed to create .gt dir: %v", err)
	}

	// Test saving and loading cache
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	cache := &beadsVersionCache{
		BdPath:  "/usr/local/bin/bd",
		BdMtime: testTime,
		Version: "0.45.0",
	}

	saveVersionCache(cache)

	loaded := loadVersionCache()
	if loaded == nil {
		t.Fatal("loadVersionCache returned nil")
	}

	if loaded.BdPath != cache.BdPath {
		t.Errorf("BdPath = %q, want %q", loaded.BdPath, cache.BdPath)
	}
	if !loaded.BdMtime.Equal(cache.BdMtime) {
		t.Errorf("BdMtime = %v, want %v", loaded.BdMtime, cache.BdMtime)
	}
	if loaded.Version != cache.Version {
		t.Errorf("Version = %q, want %q", loaded.Version, cache.Version)
	}
}

func TestVersionCacheInvalidation(t *testing.T) {
	// Create a temp directory for the cache
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Ensure the .gt directory exists
	gtDir := filepath.Join(tmpDir, ".gt")
	if err := os.MkdirAll(gtDir, 0755); err != nil {
		t.Fatalf("failed to create .gt dir: %v", err)
	}

	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	cache := &beadsVersionCache{
		BdPath:  "/usr/local/bin/bd",
		BdMtime: testTime,
		Version: "0.45.0",
	}
	saveVersionCache(cache)

	loaded := loadVersionCache()
	if loaded == nil {
		t.Fatal("loadVersionCache returned nil")
	}

	// Same path and mtime should be a cache hit
	if loaded.BdPath != "/usr/local/bin/bd" || !loaded.BdMtime.Equal(testTime) {
		t.Error("cache should match for same path and mtime")
	}

	// Different path should not match
	if loaded.BdPath == "/different/path/bd" {
		t.Error("cache should not match for different path")
	}

	// Different mtime should not match
	differentTime := testTime.Add(time.Hour)
	if loaded.BdMtime.Equal(differentTime) {
		t.Error("cache should not match for different mtime")
	}
}

func TestLoadVersionCacheNoFile(t *testing.T) {
	// Create a temp directory with no cache file
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Should return nil when no cache file exists
	loaded := loadVersionCache()
	if loaded != nil {
		t.Errorf("loadVersionCache should return nil when no cache file exists, got %+v", loaded)
	}
}
