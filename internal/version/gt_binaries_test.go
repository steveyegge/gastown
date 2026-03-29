package version

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverGTBinaries_MarksActiveAndPATHOrder(t *testing.T) {
	originalExec := currentExecutablePath
	originalProbe := probeGTBinary
	t.Cleanup(func() {
		currentExecutablePath = originalExec
		probeGTBinary = originalProbe
	})

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	activePath := writeFakeExecutable(t, dir1, "gt")
	shadowedPath := writeFakeExecutable(t, dir2, "gt")

	t.Setenv("PATH", dir1+string(os.PathListSeparator)+dir2)

	currentExecutablePath = func() (string, error) { return activePath, nil }
	probeGTBinary = func(path string) GTBinaryVersionInfo {
		switch path {
		case activePath:
			return GTBinaryVersionInfo{
				MainPackage: gastownGTMainPackage,
				VersionLine: "gt version 0.12.0 (dev: main@abcdef123456)",
				Version:     "0.12.0",
				Build:       "dev",
				Detail:      "main@abcdef123456",
				Commit:      "abcdef123456",
			}
		case shadowedPath:
			return GTBinaryVersionInfo{
				MainPackage: gastownGTMainPackage,
				VersionLine: "gt version 0.12.1 (Homebrew: v0.12.1@Homebrew)",
				Version:     "0.12.1",
				Build:       "Homebrew",
				Detail:      "v0.12.1@Homebrew",
			}
		default:
			t.Fatalf("unexpected probe path %q", path)
			return GTBinaryVersionInfo{}
		}
	}

	inventory := DiscoverGTBinaries()

	if inventory.ActiveIndex != 0 {
		t.Fatalf("ActiveIndex = %d, want 0", inventory.ActiveIndex)
	}
	if inventory.PathPrimaryIndex != 0 {
		t.Fatalf("PathPrimaryIndex = %d, want 0", inventory.PathPrimaryIndex)
	}
	if len(inventory.Binaries) != 2 {
		t.Fatalf("expected 2 binaries, got %d", len(inventory.Binaries))
	}

	active := inventory.Binaries[0]
	if !active.Active || !active.OnPATH || !active.PathPrimary {
		t.Fatalf("active binary flags wrong: %+v", active)
	}
	if active.VersionInfo.Version != "0.12.0" || active.VersionInfo.Build != "dev" {
		t.Fatalf("active version probe wrong: %+v", active.VersionInfo)
	}

	shadowed := inventory.Binaries[1]
	if shadowed.Active || !shadowed.OnPATH || shadowed.PathPrimary {
		t.Fatalf("shadowed binary flags wrong: %+v", shadowed)
	}
	if shadowed.PATHIndex != 1 {
		t.Fatalf("shadowed PATHIndex = %d, want 1", shadowed.PATHIndex)
	}
	if shadowed.VersionInfo.Version != "0.12.1" || shadowed.VersionInfo.Build != "Homebrew" {
		t.Fatalf("shadowed version probe wrong: %+v", shadowed.VersionInfo)
	}
}

func TestDiscoverGTBinaries_IncludesActiveOutsidePATH(t *testing.T) {
	originalExec := currentExecutablePath
	originalProbe := probeGTBinary
	t.Cleanup(func() {
		currentExecutablePath = originalExec
		probeGTBinary = originalProbe
	})

	pathDir := t.TempDir()
	activeDir := t.TempDir()
	pathBinary := writeFakeExecutable(t, pathDir, "gt")
	activeBinary := writeFakeExecutable(t, activeDir, "gt")

	t.Setenv("PATH", pathDir)

	currentExecutablePath = func() (string, error) { return activeBinary, nil }
	probeGTBinary = func(path string) GTBinaryVersionInfo {
		return GTBinaryVersionInfo{
			MainPackage: gastownGTMainPackage,
			VersionLine: fmt.Sprintf("gt version 0.12.1 (dev: main@%s)", filepath.Base(path)+"abcdef"),
			Version:     "0.12.1",
			Build:       "dev",
			Detail:      "main@" + filepath.Base(path) + "abcdef",
		}
	}

	inventory := DiscoverGTBinaries()

	if len(inventory.Binaries) != 2 {
		t.Fatalf("expected 2 binaries, got %d", len(inventory.Binaries))
	}
	if inventory.Active() == nil || inventory.Active().Path != activeBinary {
		t.Fatalf("active binary = %+v, want %q", inventory.Active(), activeBinary)
	}
	if inventory.PathPrimary() == nil || inventory.PathPrimary().Path != pathBinary {
		t.Fatalf("PATH primary = %+v, want %q", inventory.PathPrimary(), pathBinary)
	}
	if inventory.Active().OnPATH {
		t.Fatalf("active binary should not be on PATH: %+v", inventory.Active())
	}
}

func TestDiscoverGTBinaries_PreservesUnknownGTExecutables(t *testing.T) {
	originalExec := currentExecutablePath
	originalProbe := probeGTBinary
	t.Cleanup(func() {
		currentExecutablePath = originalExec
		probeGTBinary = originalProbe
	})

	dir := t.TempDir()
	unknown := writeFakeExecutable(t, dir, "gt")
	t.Setenv("PATH", dir)

	currentExecutablePath = func() (string, error) { return unknown, nil }
	probeGTBinary = func(path string) GTBinaryVersionInfo {
		return GTBinaryVersionInfo{
			Error: fmt.Errorf("unexpected main package %q", "example.com/not-gastown"),
		}
	}

	inventory := DiscoverGTBinaries()
	if len(inventory.Binaries) != 1 {
		t.Fatalf("expected 1 binary, got %d", len(inventory.Binaries))
	}
	if inventory.Binaries[0].VersionInfo.Recognized() {
		t.Fatalf("binary should not be recognized: %+v", inventory.Binaries[0].VersionInfo)
	}
}

func TestGTVersionLineRegex_ParsesKnownFormats(t *testing.T) {
	tests := []struct {
		line       string
		wantVer    string
		wantBuild  string
		wantDetail string
	}{
		{
			line:       "gt version 0.12.1 (dev: main@42f9d568fc1f)",
			wantVer:    "0.12.1",
			wantBuild:  "dev",
			wantDetail: "main@42f9d568fc1f",
		},
		{
			line:       "gt version 0.12.1 (Homebrew: v0.12.1@Homebrew)",
			wantVer:    "0.12.1",
			wantBuild:  "Homebrew",
			wantDetail: "v0.12.1@Homebrew",
		},
		{
			line:       "gt version 0.12.1",
			wantVer:    "0.12.1",
			wantBuild:  "",
			wantDetail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			matches := gtVersionLineRE.FindStringSubmatch(tt.line)
			if len(matches) == 0 {
				t.Fatalf("regex did not match %q", tt.line)
			}
			if matches[1] != tt.wantVer || matches[2] != tt.wantBuild || matches[3] != tt.wantDetail {
				t.Fatalf("parse mismatch for %q: got ver=%q build=%q detail=%q", tt.line, matches[1], matches[2], matches[3])
			}
		})
	}
}

func writeFakeExecutable(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}
