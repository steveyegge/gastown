package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeGT(t *testing.T, dir string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "gt.bat")
		if err := os.WriteFile(path, []byte("@echo off\r\nexit /b 0\r\n"), 0755); err != nil {
			t.Fatal(err)
		}
		return path
	}

	path := filepath.Join(dir, "gt")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGTBinaryShadowCheck_Metadata(t *testing.T) {
	check := NewGTBinaryShadowCheck()

	if check.Name() != "gt-binary-shadow" {
		t.Fatalf("Name() = %q, want %q", check.Name(), "gt-binary-shadow")
	}
	if check.Description() != "Check whether a canonical ~/.local/bin/gt install is shadowed on PATH" {
		t.Fatalf("Description() = %q", check.Description())
	}
	if check.Category() != CategoryInfrastructure {
		t.Fatalf("Category() = %q, want %q", check.Category(), CategoryInfrastructure)
	}
	if check.CanFix() {
		t.Fatal("CanFix() should be false")
	}
}

func TestGTBinaryShadowCheck_NoCanonicalInstall(t *testing.T) {
	home := t.TempDir()
	pathDir := t.TempDir()
	writeFakeGT(t, pathDir)

	t.Setenv("HOME", home)
	t.Setenv("PATH", pathDir)

	check := NewGTBinaryShadowCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})
	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "No canonical source install detected") {
		t.Fatalf("unexpected message: %q", result.Message)
	}
}

func TestGTBinaryShadowCheck_CanonicalInstallWins(t *testing.T) {
	home := t.TempDir()
	canonicalDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(canonicalDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFakeGT(t, canonicalDir)

	t.Setenv("HOME", home)
	t.Setenv("PATH", canonicalDir)

	check := NewGTBinaryShadowCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})
	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "PATH resolves canonical gt install") {
		t.Fatalf("unexpected message: %q", result.Message)
	}
}

func TestGTBinaryShadowCheck_WarnsWhenShadowed(t *testing.T) {
	home := t.TempDir()
	canonicalDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(canonicalDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFakeGT(t, canonicalDir)

	shadowDir := t.TempDir()
	shadowPath := writeFakeGT(t, shadowDir)

	t.Setenv("HOME", home)
	t.Setenv("PATH", shadowDir+string(os.PathListSeparator)+canonicalDir)

	check := NewGTBinaryShadowCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "PATH resolves gt to") {
		t.Fatalf("unexpected message: %q", result.Message)
	}
	if !strings.Contains(result.Message, filepath.Base(shadowPath)) {
		t.Fatalf("message should mention shadow path, got: %q", result.Message)
	}
	if result.FixHint == "" {
		t.Fatal("expected fix hint")
	}
}

func TestIsLikelyHomebrewGT(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/usr/local/bin/gt", true},
		{"/opt/homebrew/bin/gt", true},
		{"/usr/local/Cellar/gastown/0.12.1/bin/gt", true},
		{"/tmp/custom/gt", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isLikelyHomebrewGT(tt.path); got != tt.want {
			t.Fatalf("isLikelyHomebrewGT(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
