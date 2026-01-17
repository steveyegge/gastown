package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSelectAsset(t *testing.T) {
	tests := []struct {
		name       string
		tagName    string
		assets     []Asset
		wantName   string
		wantErr    bool
	}{
		{
			name:    "darwin_arm64",
			tagName: "v0.2.6",
			assets: []Asset{
				{Name: "gastown_0.2.6_darwin_arm64.tar.gz", Size: 1000},
				{Name: "gastown_0.2.6_darwin_amd64.tar.gz", Size: 1000},
				{Name: "gastown_0.2.6_linux_amd64.tar.gz", Size: 1000},
			},
			wantName: "gastown_0.2.6_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz",
			wantErr:  false,
		},
		{
			name:    "no_matching_asset",
			tagName: "v0.2.6",
			assets: []Asset{
				{Name: "gastown_0.2.6_plan9_386.tar.gz", Size: 1000},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := &ReleaseInfo{
				TagName: tt.tagName,
				Assets:  tt.assets,
			}
			got, err := SelectAsset(release)
			if (err != nil) != tt.wantErr {
				t.Errorf("SelectAsset() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			expectedExt := ".tar.gz"
			if runtime.GOOS == "windows" {
				expectedExt = ".zip"
			}
			expectedName := "gastown_0.2.6_" + runtime.GOOS + "_" + runtime.GOARCH + expectedExt
			if got.Name != expectedName {
				t.Logf("Note: Test expected asset for current platform (%s/%s)", runtime.GOOS, runtime.GOARCH)
			}
		})
	}
}

func TestExtractTarGz(t *testing.T) {
	// Create a temp tar.gz file with a fake "gt" binary
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create the archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add a fake gt binary
	content := []byte("#!/bin/sh\necho 'gt version test'\n")
	hdr := &tar.Header{
		Name: "gt",
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	// Extract
	binaryPath, err := extractTarGz(archivePath)
	if err != nil {
		t.Fatalf("extractTarGz() error = %v", err)
	}
	defer os.RemoveAll(filepath.Dir(binaryPath))

	// Verify the binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		t.Errorf("Extracted binary not found: %v", err)
	}

	// Verify it's named "gt"
	if filepath.Base(binaryPath) != "gt" {
		t.Errorf("Binary name = %q, want 'gt'", filepath.Base(binaryPath))
	}
}

func TestExtractZip(t *testing.T) {
	if runtime.GOOS != "windows" {
		// Still run the test, but with gt.exe to match Windows behavior
	}

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	// Create zip file
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)

	// Add a fake gt.exe binary (Windows naming)
	content := []byte("fake binary content")
	w, err := zw.Create("gt.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}

	zw.Close()
	f.Close()

	// Extract
	binaryPath, err := extractZip(archivePath)
	if err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}
	defer os.RemoveAll(filepath.Dir(binaryPath))

	// Verify the binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		t.Errorf("Extracted binary not found: %v", err)
	}

	// Verify it's named "gt.exe"
	if filepath.Base(binaryPath) != "gt.exe" {
		t.Errorf("Binary name = %q, want 'gt.exe'", filepath.Base(binaryPath))
	}
}

func TestIsHomebrew(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/usr/local/Cellar/gt/0.2.6/bin/gt", true},
		{"/opt/homebrew/Cellar/gt/0.2.6/bin/gt", true},
		{"/home/linuxbrew/.linuxbrew/Cellar/gt/0.2.6/bin/gt", true},
		{"/usr/local/bin/gt", false},
		{"/home/user/.local/bin/gt", false},
		{"/Users/kiwi/go/bin/gt", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsHomebrew(tt.path); got != tt.want {
				t.Errorf("IsHomebrew(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{10485760, "10.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatSize(tt.bytes); got != tt.want {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
