package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestGetBeadInfo_UsesDbFlag verifies that getBeadInfo passes --db <beadsDir>
// to the bd subprocess instead of setting BEADS_DIR in the environment.
//
// Bug: sbx-gastown-yweq — setting BEADS_DIR globally breaks gt's internal
// per-rig redirect mechanism. The fix uses --db per-command instead.
func TestGetBeadInfo_UsesDbFlag(t *testing.T) {
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
	argsLogPath := filepath.Join(townRoot, "bd_args.log")
	envLogPath := filepath.Join(townRoot, "bd_env.log")

	// The bd stub logs its arguments and BEADS_DIR env to files.
	// We verify --db was passed as an arg and BEADS_DIR was NOT set.
	bdScript := `#!/bin/sh
echo "$@" > "` + argsLogPath + `"
echo "BEADS_DIR=${BEADS_DIR}" > "` + envLogPath + `"
# Parse args to find the subcommand
while [ $# -gt 0 ]; do
  case "$1" in
    --db) shift ;; # skip value
    --allow-stale) ;;
    show)
      echo '[{"title":"Db-flag bead","status":"open","assignee":""}]'
      exit 0
      ;;
    version)
      echo "bd 0.1.0"
      exit 0
      ;;
  esac
  shift
done
exit 0
`
	_ = `@echo off
echo %* > "` + argsLogPath + `"
echo BEADS_DIR=%BEADS_DIR% > "` + envLogPath + `"
echo [{"title":"Db-flag bead","status":"open","assignee":""}]
exit /b 0
`
	// Write stub directly (not via writeBDStub) to avoid --db stripping preamble,
	// since these tests specifically verify that --db is passed through.
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Ensure BEADS_DIR is NOT set — gt should use --db flag, not env
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
	if info.Title != "Db-flag bead" {
		t.Errorf("Title = %q, want %q", info.Title, "Db-flag bead")
	}

	// Verify --db was passed as a CLI argument
	argsLog, err := os.ReadFile(argsLogPath)
	if err != nil {
		t.Fatalf("reading args log: %v", err)
	}
	argsStr := strings.TrimSpace(string(argsLog))
	if !strings.Contains(argsStr, "--db "+beadsDir) && !strings.Contains(argsStr, "--db="+beadsDir) {
		t.Errorf("bd was not called with --db flag;\n  args: %q\n  want: --db %s", argsStr, beadsDir)
	}

	// Verify BEADS_DIR was NOT set in the environment (the whole point of this fix)
	envLog, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatalf("reading env log: %v", err)
	}
	envLogStr := strings.TrimSpace(string(envLog))
	if strings.Contains(envLogStr, "BEADS_DIR="+beadsDir) {
		t.Errorf("BEADS_DIR should NOT be set in env (use --db flag instead);\n  env: %q", envLogStr)
	}
}

// TestVerifyBeadExists_UsesDbFlag verifies that verifyBeadExists passes --db
// to the bd subprocess instead of setting BEADS_DIR in the environment.
func TestVerifyBeadExists_UsesDbFlag(t *testing.T) {
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
	argsLogPath := filepath.Join(townRoot, "bd_args.log")
	envLogPath := filepath.Join(townRoot, "bd_env.log")

	bdScript := `#!/bin/sh
echo "$@" > "` + argsLogPath + `"
echo "BEADS_DIR=${BEADS_DIR}" > "` + envLogPath + `"
while [ $# -gt 0 ]; do
  case "$1" in
    --db) shift ;;
    --allow-stale) ;;
    show)
      echo '[{"title":"Exists check","status":"open","assignee":""}]'
      exit 0
      ;;
    version)
      echo "bd 0.1.0"
      exit 0
      ;;
  esac
  shift
done
exit 0
`
	_ = `@echo off
echo %* > "` + argsLogPath + `"
echo BEADS_DIR=%BEADS_DIR% > "` + envLogPath + `"
echo [{"title":"Exists check","status":"open","assignee":""}]
exit /b 0
`
	// Write stub directly (not via writeBDStub) to avoid --db stripping preamble,
	// since these tests specifically verify that --db is passed through.
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
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

	// Verify --db was passed as a CLI argument
	argsLog, err := os.ReadFile(argsLogPath)
	if err != nil {
		t.Fatalf("reading args log: %v", err)
	}
	argsStr := strings.TrimSpace(string(argsLog))
	if !strings.Contains(argsStr, "--db "+beadsDir) && !strings.Contains(argsStr, "--db="+beadsDir) {
		t.Errorf("bd was not called with --db flag;\n  args: %q\n  want: --db %s", argsStr, beadsDir)
	}

	// Verify BEADS_DIR was NOT set in the environment
	envLog, err := os.ReadFile(envLogPath)
	if err != nil {
		t.Fatalf("reading env log: %v", err)
	}
	envLogStr := strings.TrimSpace(string(envLog))
	if strings.Contains(envLogStr, "BEADS_DIR="+beadsDir) {
		t.Errorf("BEADS_DIR should NOT be set in env (use --db flag instead);\n  env: %q", envLogStr)
	}
}

// TestGetBeadInfo_NoBeadsDirEnvLeak verifies that getBeadInfo does NOT set
// BEADS_DIR as a process-wide environment variable, which would break gt's
// internal per-rig redirect mechanism.
func TestGetBeadInfo_NoBeadsDirEnvLeak(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	bdScript := `#!/bin/sh
case "$1" in
  --db) shift; shift ;; # skip --db <path>
esac
case "$1" in
  --allow-stale) shift ;;
esac
case "$1" in
  show)
    echo '[{"title":"No leak","status":"open","assignee":""}]'
    ;;
  version)
    echo "bd 0.1.0"
    ;;
esac
exit 0
`
	_ = `@echo off
echo [{"title":"No leak","status":"open","assignee":""}]
exit /b 0
`
	// Write stub directly (not via writeBDStub) to avoid --db stripping preamble,
	// since these tests specifically verify that --db is passed through.
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Start with BEADS_DIR unset
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

	_, err = getBeadInfo("hq-test-abc")
	if err != nil {
		t.Fatalf("getBeadInfo failed: %v", err)
	}

	// The critical check: BEADS_DIR must NOT have been set as a process env var.
	// This was the bug — ensureBeadsDir() called os.Setenv which polluted the
	// process environment and broke gt's per-rig redirect mechanism.
	if got := os.Getenv("BEADS_DIR"); got != "" {
		t.Errorf("BEADS_DIR leaked into process environment: %q\ngetBeadInfo must use --db flag, not os.Setenv", got)
	}
}
