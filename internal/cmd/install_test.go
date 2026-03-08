package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/doltserver"
)

func TestBuildBdInitArgs_AlwaysIncludesServerPort(t *testing.T) {
	townDir := t.TempDir()

	args := buildBdInitArgs(townDir)

	if len(args) != 6 {
		t.Fatalf("expected 6 args, got %d: %v", len(args), args)
	}
	if args[4] != "--server-port" {
		t.Fatalf("expected args[4] = --server-port, got %q", args[4])
	}
	if args[5] != "3307" {
		t.Fatalf("expected default port 3307, got %q", args[5])
	}
}

func TestBuildBdInitArgs_RespectsGTDoltPortEnv(t *testing.T) {
	townDir := t.TempDir()

	t.Setenv("GT_DOLT_PORT", "4400")

	args := buildBdInitArgs(townDir)

	if args[5] != "4400" {
		t.Fatalf("expected port 4400 from GT_DOLT_PORT, got %q", args[5])
	}
}

func TestBuildBdInitArgs_ConfigYAMLTakesPrecedence(t *testing.T) {
	townDir := t.TempDir()
	doltDataDir := filepath.Join(townDir, ".dolt-data")
	if err := os.MkdirAll(doltDataDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configYAML := "listener:\n  host: 127.0.0.1\n  port: 5500\n"
	if err := os.WriteFile(filepath.Join(doltDataDir, "config.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	t.Setenv("GT_DOLT_PORT", "4400")

	args := buildBdInitArgs(townDir)

	if args[5] != "5500" {
		t.Fatalf("expected port 5500 from config.yaml (precedence over env), got %q", args[5])
	}
}

func TestBuildBdInitArgs_PortMatchesDefaultConfig(t *testing.T) {
	townDir := t.TempDir()

	args := buildBdInitArgs(townDir)
	cfg := doltserver.DefaultConfig(townDir)

	if args[5] != strconv.Itoa(cfg.Port) {
		t.Fatalf("port should match DefaultConfig: args=%q, config=%d", args[5], cfg.Port)
	}
}

func TestEnsureBeadsConfigYAML_CreatesWhenMissing(t *testing.T) {
	beadsDir := t.TempDir()

	if err := beads.EnsureConfigYAML(beadsDir, "hq"); err != nil {
		t.Fatalf("EnsureConfigYAML: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	got := string(data)
	want := "prefix: hq\nissue-prefix: hq\n"
	if got != want {
		t.Fatalf("config.yaml = %q, want %q", got, want)
	}
}

func TestEnsureBeadsConfigYAML_RepairsPrefixKeysAndPreservesOtherLines(t *testing.T) {
	beadsDir := t.TempDir()
	path := filepath.Join(beadsDir, "config.yaml")
	original := strings.Join([]string{
		"# existing settings",
		"prefix: wrong",
		"sync-branch: main",
		"issue-prefix: wrong",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	if err := beads.EnsureConfigYAML(beadsDir, "hq"); err != nil {
		t.Fatalf("EnsureConfigYAML: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "prefix: hq\n") {
		t.Fatalf("config.yaml missing repaired prefix: %q", text)
	}
	if !strings.Contains(text, "issue-prefix: hq\n") {
		t.Fatalf("config.yaml missing repaired issue-prefix: %q", text)
	}
	if !strings.Contains(text, "sync-branch: main\n") {
		t.Fatalf("config.yaml should preserve unrelated settings: %q", text)
	}
}

func TestEnsureBeadsConfigYAML_AddsMissingIssuePrefixKey(t *testing.T) {
	beadsDir := t.TempDir()
	path := filepath.Join(beadsDir, "config.yaml")
	if err := os.WriteFile(path, []byte("prefix: hq\n"), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	if err := beads.EnsureConfigYAML(beadsDir, "hq"); err != nil {
		t.Fatalf("EnsureConfigYAML: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "prefix: hq\n") {
		t.Fatalf("config.yaml missing prefix: %q", text)
	}
	if !strings.Contains(text, "issue-prefix: hq\n") {
		t.Fatalf("config.yaml missing issue-prefix: %q", text)
	}
}
