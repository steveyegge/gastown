package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
)

func TestDiscoverHooksSkipsPolecatDotDirs(t *testing.T) {
	townRoot := setupTestTownForDotDir(t)
	rigPath := filepath.Join(townRoot, "gastown")

	settingsPath := filepath.Join(rigPath, "polecats", ".claude", ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}

	settings := `{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"Stop","command":"echo hi"}]}]}}`
	if err := os.WriteFile(settingsPath, []byte(settings), 0644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	hooks, err := discoverHooks(townRoot)
	if err != nil {
		t.Fatalf("discoverHooks: %v", err)
	}

	if len(hooks) != 0 {
		t.Fatalf("expected no hooks, got %d", len(hooks))
	}
}

func TestStartPolecatsWithWorkSkipsDotDirs(t *testing.T) {
	townRoot := setupTestTownForDotDir(t)
	rigName := "gastown"
	rigPath := filepath.Join(townRoot, rigName)

	addRigEntry(t, townRoot, rigName)

	if err := os.MkdirAll(filepath.Join(rigPath, "polecats", ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude polecat: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, "polecats", "toast"), 0755); err != nil {
		t.Fatalf("mkdir polecat: %v", err)
	}

	binDir := t.TempDir()
	bdScript := `#!/bin/sh
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
case "$cmd" in
  list)
    if [ "$(basename "$PWD")" = ".claude" ]; then
      echo '[{"id":"gt-1"}]'
    else
      echo '[]'
    fi
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	bdScriptWindows := `@echo off
setlocal enableextensions
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="list" (
  for %%I in ("%CD%") do set "dirname=%%~nxI"
  if "!dirname!"==".claude" (
    echo [{"id":"gt-1"}]
  ) else (
    echo []
  )
  exit /b 0
)
exit /b 0
`
	writeScript(t, binDir, "bd", bdScript, bdScriptWindows)

	tmuxScript := `#!/bin/sh
if [ "$1" = "has-session" ]; then
  echo "tmux error" 1>&2
  exit 1
fi
exit 0
`
	tmuxScriptWindows := `@echo off
if "%1"=="has-session" (
  echo tmux error 1>&2
  exit /b 1
)
exit /b 0
`
	writeScript(t, binDir, "tmux", tmuxScript, tmuxScriptWindows)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}

	started, errs := startPolecatsWithWork(townRoot, rigName)

	if len(started) != 0 {
		t.Fatalf("expected no polecats started, got %v", started)
	}
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestRunSessionCheckSkipsDotDirs(t *testing.T) {
	townRoot := setupTestTownForDotDir(t)
	rigName := "gastown"
	rigPath := filepath.Join(townRoot, rigName)

	addRigEntry(t, townRoot, rigName)

	if err := os.MkdirAll(filepath.Join(rigPath, "polecats", ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude polecat: %v", err)
	}

	binDir := t.TempDir()
	tmuxScript := `#!/bin/sh
if [ "$1" = "has-session" ]; then
  echo "can't find session" 1>&2
  exit 1
fi
exit 0
`
	tmuxScriptWindows := `@echo off
if "%1"=="has-session" (
  echo can't find session 1>&2
  exit /b 1
)
exit /b 0
`
	writeScript(t, binDir, "tmux", tmuxScript, tmuxScriptWindows)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runSessionCheck(&cobra.Command{}, []string{rigName}); err != nil {
			t.Fatalf("runSessionCheck: %v", err)
		}
	})

	if strings.Contains(output, ".claude") {
		t.Fatalf("expected .claude to be ignored, output:\n%s", output)
	}
}

func addRigEntry(t *testing.T, townRoot, rigName string) {
	t.Helper()

	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("load rigs.json: %v", err)
	}
	if rigsConfig.Rigs == nil {
		rigsConfig.Rigs = make(map[string]config.RigEntry)
	}
	rigsConfig.Rigs[rigName] = config.RigEntry{
		GitURL:  "file:///dev/null",
		AddedAt: time.Now(),
	}
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}
}

func setupTestTownForDotDir(t *testing.T) string {
	t.Helper()

	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs:    make(map[string]config.RigEntry),
	}
	if err := config.SaveRigsConfig(rigsPath, rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	return townRoot
}

// writeScript writes a cross-platform script to dir.
// If windowsScript is non-empty and we're on Windows, it writes name.cmd with windowsScript content.
// Otherwise, it writes name with unixScript content.
func writeScript(t *testing.T, dir, name, unixScript, windowsScript string) {
	t.Helper()

	var path string
	var content string
	var perm os.FileMode

	if runtime.GOOS == "windows" && windowsScript != "" {
		path = filepath.Join(dir, name+".cmd")
		content = windowsScript
		perm = 0644
	} else {
		path = filepath.Join(dir, name)
		content = unixScript
		perm = 0755
	}

	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
