package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestGetBeadInfo_RespectsBeadsDir verifies that getBeadInfo uses BEADS_DIR
// when set in the environment, instead of stripping it and falling back to
// per-rig prefix-based directory resolution.
//
// Bug: sbx-gastown-nyru — sling's bead status check runs before
// sync-on-dispatch and calls getBeadInfo which strips BEADS_DIR, causing
// town-root beads to be unfindable from polecat worktrees.
func TestGetBeadInfo_RespectsBeadsDir(t *testing.T) {
	beads.ResetBdAllowStaleCacheForTest()
	t.Cleanup(beads.ResetBdAllowStaleCacheForTest)

	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create a stub bd that logs BEADS_DIR from its environment to a file,
	// so we can verify whether getBeadInfo preserved or stripped it.
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	envLogPath := filepath.Join(townRoot, "bd_env.log")

	bdScript := `#!/bin/sh
# Log BEADS_DIR to file so the test can verify it was passed through
echo "BEADS_DIR=${BEADS_DIR}" > "` + envLogPath + `"
cmd="$1"
shift || true
if [ "$cmd" = "--allow-stale" ]; then
  cmd="$1"
  shift || true
fi
case "$cmd" in
  show)
    echo '[{"title":"Town-root bead","status":"open","assignee":""}]'
    ;;
  version)
    echo "bd 0.1.0"
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
echo BEADS_DIR=%BEADS_DIR% > "` + envLogPath + `"
set "cmd=%1"
if "%cmd%"=="--allow-stale" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Town-root bead","status":"open","assignee":""}]
  exit /b 0
)
if "%cmd%"=="version" (
  echo bd 0.1.0
  exit /b 0
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Set BEADS_DIR — this is the scenario where a polecat worktree has
	// BEADS_DIR pointing to the town-root .beads directory.
	t.Setenv("BEADS_DIR", beadsDir)

	// getBeadInfo should succeed and preserve BEADS_DIR for bd
	info, err := getBeadInfo("sbx-test-abc")
	if err != nil {
		t.Fatalf("getBeadInfo failed: %v", err)
	}
	if info.Title != "Town-root bead" {
		t.Errorf("Title = %q, want %q", info.Title, "Town-root bead")
	}

	// Verify that BEADS_DIR was passed through to bd (not stripped)
	envLog, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatalf("reading env log: %v", err)
	}
	envLogStr := strings.TrimSpace(string(envLog))
	if !strings.Contains(envLogStr, "BEADS_DIR="+beadsDir) {
		t.Errorf("bd did not receive BEADS_DIR; env log: %q\nExpected BEADS_DIR=%s", envLogStr, beadsDir)
	}
}

// TestGetBeadInfo_DerivesBeadsDirFromTownRoot verifies that getBeadInfo
// auto-derives BEADS_DIR from the town root when not explicitly set.
// Bug: sbx-gastown-wsdu — gt should derive BEADS_DIR from town root,
// not require the env var to be set externally.
func TestGetBeadInfo_DerivesBeadsDirFromTownRoot(t *testing.T) {
	beads.ResetBdAllowStaleCacheForTest()
	t.Cleanup(beads.ResetBdAllowStaleCacheForTest)

	townRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	// Create workspace structure (mayor/town.json is the primary marker)
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	envLogPath := filepath.Join(townRoot, "bd_env.log")

	bdScript := `#!/bin/sh
echo "BEADS_DIR=${BEADS_DIR}" > "` + envLogPath + `"
cmd="$1"
shift || true
if [ "$cmd" = "--allow-stale" ]; then
  cmd="$1"
  shift || true
fi
case "$cmd" in
  show)
    echo '[{"title":"Derived bead","status":"open","assignee":""}]'
    ;;
  version)
    echo "bd 0.1.0"
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
echo BEADS_DIR=%BEADS_DIR% > "` + envLogPath + `"
set "cmd=%1"
if "%cmd%"=="--allow-stale" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Derived bead","status":"open","assignee":""}]
  exit /b 0
)
if "%cmd%"=="version" (
  echo bd 0.1.0
  exit /b 0
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Ensure BEADS_DIR is NOT set — gt should derive it
	t.Setenv("BEADS_DIR", "")
	os.Unsetenv("BEADS_DIR")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	info, err := getBeadInfo("hq-test-abc")
	if err != nil {
		t.Fatalf("getBeadInfo failed: %v", err)
	}
	if info.Title != "Derived bead" {
		t.Errorf("Title = %q, want %q", info.Title, "Derived bead")
	}

	// Verify BEADS_DIR was auto-derived from town root and passed to bd
	envLog, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatalf("reading env log: %v", err)
	}
	envLogStr := strings.TrimSpace(string(envLog))
	expectedBeadsDir := "BEADS_DIR=" + beadsDir
	if !strings.Contains(envLogStr, expectedBeadsDir) {
		t.Errorf("bd did not receive auto-derived BEADS_DIR;\n  got:  %q\n  want: %q", envLogStr, expectedBeadsDir)
	}
}

// TestVerifyBeadExists_DerivesBeadsDirFromTownRoot verifies that
// verifyBeadExists auto-derives BEADS_DIR from the town root when not set.
func TestVerifyBeadExists_DerivesBeadsDirFromTownRoot(t *testing.T) {
	beads.ResetBdAllowStaleCacheForTest()
	t.Cleanup(beads.ResetBdAllowStaleCacheForTest)

	townRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	envLogPath := filepath.Join(townRoot, "bd_env.log")

	bdScript := `#!/bin/sh
echo "BEADS_DIR=${BEADS_DIR}" > "` + envLogPath + `"
cmd="$1"
shift || true
if [ "$cmd" = "--allow-stale" ]; then
  cmd="$1"
  shift || true
fi
case "$cmd" in
  show)
    echo '[{"title":"Derived exists","status":"open","assignee":""}]'
    ;;
  version)
    echo "bd 0.1.0"
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
echo BEADS_DIR=%BEADS_DIR% > "` + envLogPath + `"
set "cmd=%1"
if "%cmd%"=="--allow-stale" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Derived exists","status":"open","assignee":""}]
  exit /b 0
)
if "%cmd%"=="version" (
  echo bd 0.1.0
  exit /b 0
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	t.Setenv("BEADS_DIR", "")
	os.Unsetenv("BEADS_DIR")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	err = verifyBeadExists("hq-test-abc")
	if err != nil {
		t.Fatalf("verifyBeadExists failed: %v", err)
	}

	// Verify BEADS_DIR was auto-derived from town root
	envLog, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatalf("reading env log: %v", err)
	}
	envLogStr := strings.TrimSpace(string(envLog))
	expectedBeadsDir := "BEADS_DIR=" + beadsDir
	if !strings.Contains(envLogStr, expectedBeadsDir) {
		t.Errorf("bd did not receive auto-derived BEADS_DIR;\n  got:  %q\n  want: %q", envLogStr, expectedBeadsDir)
	}
}

// TestVerifyBeadExists_RespectsBeadsDir verifies the same BEADS_DIR fix
// for verifyBeadExists, which has the same StripBeadsDir pattern.
func TestVerifyBeadExists_RespectsBeadsDir(t *testing.T) {
	beads.ResetBdAllowStaleCacheForTest()
	t.Cleanup(beads.ResetBdAllowStaleCacheForTest)

	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	envLogPath := filepath.Join(townRoot, "bd_env.log")

	bdScript := `#!/bin/sh
echo "BEADS_DIR=${BEADS_DIR}" > "` + envLogPath + `"
cmd="$1"
shift || true
if [ "$cmd" = "--allow-stale" ]; then
  cmd="$1"
  shift || true
fi
case "$cmd" in
  show)
    echo '[{"title":"Exists check bead","status":"open","assignee":""}]'
    ;;
  version)
    echo "bd 0.1.0"
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
echo BEADS_DIR=%BEADS_DIR% > "` + envLogPath + `"
set "cmd=%1"
if "%cmd%"=="--allow-stale" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Exists check bead","status":"open","assignee":""}]
  exit /b 0
)
if "%cmd%"=="version" (
  echo bd 0.1.0
  exit /b 0
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BEADS_DIR", beadsDir)

	err := verifyBeadExists("sbx-test-abc")
	if err != nil {
		t.Fatalf("verifyBeadExists failed: %v", err)
	}

	envLog, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatalf("reading env log: %v", err)
	}
	envLogStr := strings.TrimSpace(string(envLog))
	if !strings.Contains(envLogStr, "BEADS_DIR="+beadsDir) {
		t.Errorf("bd did not receive BEADS_DIR; env log: %q\nExpected BEADS_DIR=%s", envLogStr, beadsDir)
	}
}
