package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

func TestRunPSJSONIncludesAgentDetails(t *testing.T) {
	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs: map[string]config.RigEntry{
			"beads": {
				AddedAt: time.Now(),
				BeadsConfig: &config.BeadsConfig{
					Prefix: "bd-",
				},
			},
		},
	}
	if err := config.SaveRigsConfig(filepath.Join(mayorDir, "rigs.json"), rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	binDir := t.TempDir()
	tmuxScript := `#!/bin/sh
cmd="$1"
shift
case "$cmd" in
  list-sessions)
    if [ "$1" = "-F" ] && [ "$2" = "#{session_name}" ]; then
      echo "gt-beads-Toast"
      echo "gt-beads-witness"
      echo "hq-mayor"
      echo "random"
      exit 0
    fi
    if echo "$*" | grep -q "#{==:#{session_name},"; then
      session=$(echo "$*" | sed 's/.*#{==:#{session_name},\([^}]*\)}.*/\1/')
      case "$session" in
        gt-beads-Toast) echo "gt-beads-Toast|1|now|0|";;
        gt-beads-witness) echo "gt-beads-witness|1|now|1|";;
        hq-mayor) echo "hq-mayor|1|now|1|";;
      esac
      exit 0
    fi
    ;;
  list-panes)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-t" ]; then
        session="$2"
        shift 2
        continue
      fi
      if [ "$1" = "-F" ]; then
        format="$2"
        shift 2
        continue
      fi
      shift
    done
    case "$format" in
      "#{pane_current_command}")
        case "$session" in
          gt-beads-Toast) echo "node";;
          gt-beads-witness) echo "bash";;
          hq-mayor) echo "node";;
          *) echo "bash";;
        esac
        ;;
      "#{pane_pid}")
        echo "111"
        ;;
      "#{pane_current_path}")
        echo "/tmp"
        ;;
    esac
    ;;
esac
exit 0
`
	tmuxScriptWindows := `@echo off
setlocal enabledelayedexpansion
set "cmd=%1"
shift
if "%cmd%"=="list-sessions" (
  if "%1"=="-F" if "%2"=="#{session_name}" (
    echo gt-beads-Toast
    echo gt-beads-witness
    echo hq-mayor
    echo random
    exit /b 0
  )
  set "args=%1 %2 %3 %4 %5"
  echo !args! | findstr /C:"#{==:#{session_name}," >nul
  if !errorlevel!==0 (
    echo !args! | findstr /C:"gt-beads-Toast" >nul && echo gt-beads-Toast^|1^|now^|0^| && exit /b 0
    echo !args! | findstr /C:"gt-beads-witness" >nul && echo gt-beads-witness^|1^|now^|1^| && exit /b 0
    echo !args! | findstr /C:"hq-mayor" >nul && echo hq-mayor^|1^|now^|1^| && exit /b 0
  )
  exit /b 0
)
if "%cmd%"=="list-panes" (
  set "session="
  set "format="
  :parse_panes
  if "%1"=="" goto :done_panes
  if "%1"=="-t" (
    set "session=%2"
    shift
    shift
    goto :parse_panes
  )
  if "%1"=="-F" (
    set "format=%2"
    shift
    shift
    goto :parse_panes
  )
  shift
  goto :parse_panes
  :done_panes
  if "!format!"=="#{pane_current_command}" (
    if "!session!"=="gt-beads-Toast" echo node && exit /b 0
    if "!session!"=="gt-beads-witness" echo bash && exit /b 0
    if "!session!"=="hq-mayor" echo node && exit /b 0
    echo bash
    exit /b 0
  )
  if "!format!"=="#{pane_pid}" echo 111 && exit /b 0
  if "!format!"=="#{pane_current_path}" echo /tmp && exit /b 0
)
exit /b 0
`
	writeScript(t, binDir, "tmux", tmuxScript, tmuxScriptWindows)

	bdScript := `#!/bin/sh
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
case "$cmd" in
  list)
    echo '[{"id":"bd-beads-polecat-Toast","hook_bead":"bd-hook"},{"id":"hq-mayor","hook_bead":"hq-hook"}]'
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="list" (
  echo [{"id":"bd-beads-polecat-Toast","hook_bead":"bd-hook"},{"id":"hq-mayor","hook_bead":"hq-hook"}]
)
exit /b 0
`
	writeScript(t, binDir, "bd", bdScript, bdScriptWindows)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	prevJSON := psJSON
	prevVerbose := psVerbose
	t.Cleanup(func() {
		psJSON = prevJSON
		psVerbose = prevVerbose
	})
	psJSON = true
	psVerbose = false

	output := captureStdout(t, func() {
		if err := runPS(nil, nil); err != nil {
			t.Fatalf("runPS: %v", err)
		}
	})

	var sessions []SessionProcess
	if err := json.Unmarshal([]byte(output), &sessions); err != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", err, output)
	}

	byName := make(map[string]SessionProcess, len(sessions))
	for _, session := range sessions {
		byName[session.Name] = session
	}

	polecat, ok := byName["gt-beads-Toast"]
	if !ok {
		t.Fatalf("missing polecat session")
	}
	if polecat.Role != "polecat" {
		t.Fatalf("polecat role = %q, want %q", polecat.Role, "polecat")
	}
	if polecat.AgentID != "bd-beads-polecat-Toast" {
		t.Fatalf("polecat agent id = %q, want %q", polecat.AgentID, "bd-beads-polecat-Toast")
	}
	if polecat.HookBead != "bd-hook" {
		t.Fatalf("polecat hook bead = %q, want %q", polecat.HookBead, "bd-hook")
	}

	witness, ok := byName["gt-beads-witness"]
	if !ok {
		t.Fatalf("missing witness session")
	}
	if witness.Role != "witness" {
		t.Fatalf("witness role = %q, want %q", witness.Role, "witness")
	}
	if witness.AgentID != "bd-beads-witness" {
		t.Fatalf("witness agent id = %q, want %q", witness.AgentID, "bd-beads-witness")
	}

	mayor, ok := byName["hq-mayor"]
	if !ok {
		t.Fatalf("missing mayor session")
	}
	if mayor.Role != "mayor" {
		t.Fatalf("mayor role = %q, want %q", mayor.Role, "mayor")
	}
	if mayor.AgentID != "hq-mayor" {
		t.Fatalf("mayor agent id = %q, want %q", mayor.AgentID, "hq-mayor")
	}
}

func TestCleanupOrphansDryRunReports(t *testing.T) {
	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	binDir := t.TempDir()
	tmuxScript := `#!/bin/sh
cmd="$1"
shift
case "$cmd" in
  list-panes)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-t" ]; then
        session="$2"
        shift 2
        continue
      fi
      if [ "$1" = "-F" ]; then
        format="$2"
        shift 2
        continue
      fi
      shift
    done
    case "$format" in
      "#{pane_current_command}")
        case "$session" in
          gt-beads-Toast) echo "bash";;
          *) echo "node";;
        esac
        ;;
      "#{pane_pid}")
        echo "999"
        ;;
    esac
    ;;
esac
exit 0
`
	tmuxScriptWindows := `@echo off
setlocal enabledelayedexpansion
set "cmd=%1"
if "%cmd%"=="list-panes" (
  set "session="
  set "format="
  :parse_args
  shift
  if "%1"=="" goto :done_args
  if "%1"=="-t" (
    set "session=%2"
    shift
    goto :parse_args
  )
  if "%1"=="-F" (
    set "format=%2"
    shift
    goto :parse_args
  )
  goto :parse_args
  :done_args
  if "!format!"=="#{pane_current_command}" (
    if "!session!"=="gt-beads-Toast" echo bash && exit /b 0
    echo node
    exit /b 0
  )
  if "!format!"=="#{pane_pid}" echo 999 && exit /b 0
)
exit /b 0
`
	writeScript(t, binDir, "tmux", tmuxScript, tmuxScriptWindows)

	pgrepScript := `#!/bin/sh
exit 1
`
	pgrepScriptWindows := `@echo off
exit /b 1
`
	writeScript(t, binDir, "pgrep", pgrepScript, pgrepScriptWindows)

	bdScript := `#!/bin/sh
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
case "$cmd" in
  list)
    echo '[{"id":"bd-beads-polecat-Toast","hook_bead":"bd-123"},{"id":"bd-beads-crew-amy","hook_bead":"bd-234"}]'
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="list" (
  echo [{"id":"bd-beads-polecat-Toast","hook_bead":"bd-123"},{"id":"bd-beads-crew-amy","hook_bead":"bd-234"}]
)
exit /b 0
`
	writeScript(t, binDir, "bd", bdScript, bdScriptWindows)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	prevDryRun := cleanupDryRun
	t.Cleanup(func() { cleanupDryRun = prevDryRun })
	cleanupDryRun = true

	output := captureStdout(t, func() {
		if err := runCleanupOrphans(nil, nil); err != nil {
			t.Fatalf("runCleanupOrphans: %v", err)
		}
	})

	if !strings.Contains(output, "Found 1 orphaned work items") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "gt-beads-Toast") {
		t.Fatalf("missing session name in output: %s", output)
	}
	if strings.Contains(output, "bd-beads-crew-amy") {
		t.Fatalf("unexpected crew agent in output: %s", output)
	}
}

func TestCleanupSessionsDryRunReportsZombies(t *testing.T) {
	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "tmux.log")
	tmuxScript := `#!/bin/sh
cmd="$1"
shift
case "$cmd" in
  list-sessions)
    if [ "$1" = "-F" ] && [ "$2" = "#{session_name}" ]; then
      echo "gt-gastown-Toast"
      echo "gt-gastown-crew-amy"
      echo "random"
      exit 0
    fi
    ;;
  has-session)
    exit 0
    ;;
  list-panes)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-t" ]; then
        session="$2"
        shift 2
        continue
      fi
      if [ "$1" = "-F" ]; then
        format="$2"
        shift 2
        continue
      fi
      shift
    done
    case "$format" in
      "#{pane_current_command}")
        case "$session" in
          gt-gastown-Toast) echo "bash";;
          gt-gastown-crew-amy) echo "node";;
          *) echo "bash";;
        esac
        ;;
    esac
    ;;
  kill-session)
    echo "$*" >> "$TMUX_LOG"
    exit 0
    ;;
esac
exit 0
`
	tmuxScriptWindows := `@echo off
setlocal enabledelayedexpansion
set "cmd=%1"
shift
if "%cmd%"=="list-sessions" (
  if "%1"=="-F" if "%2"=="#{session_name}" (
    echo gt-gastown-Toast
    echo gt-gastown-crew-amy
    echo random
    exit /b 0
  )
  exit /b 0
)
if "%cmd%"=="has-session" exit /b 0
if "%cmd%"=="list-panes" (
  set "session="
  set "format="
  :parse_panes
  if "%1"=="" goto :done_panes
  if "%1"=="-t" (
    set "session=%2"
    shift
    shift
    goto :parse_panes
  )
  if "%1"=="-F" (
    set "format=%2"
    shift
    shift
    goto :parse_panes
  )
  shift
  goto :parse_panes
  :done_panes
  if "!format!"=="#{pane_current_command}" (
    if "!session!"=="gt-gastown-Toast" echo bash && exit /b 0
    if "!session!"=="gt-gastown-crew-amy" echo node && exit /b 0
    echo bash
    exit /b 0
  )
)
if "%cmd%"=="kill-session" (
  echo %1 %2 %3 %4 >> "%TMUX_LOG%"
  exit /b 0
)
exit /b 0
`
	writeScript(t, binDir, "tmux", tmuxScript, tmuxScriptWindows)
	t.Setenv("TMUX_LOG", logPath)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	prevDryRun := cleanupDryRun
	t.Cleanup(func() { cleanupDryRun = prevDryRun })
	cleanupDryRun = true

	output := captureStdout(t, func() {
		if err := runCleanupSessions(nil, nil); err != nil {
			t.Fatalf("runCleanupSessions: %v", err)
		}
	})

	if !strings.Contains(output, "Found 1 zombie sessions") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "gt-gastown-Toast") {
		t.Fatalf("missing zombie session in output: %s", output)
	}

	if _, err := os.Stat(logPath); err == nil {
		logBytes, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read tmux log: %v", err)
		}
		if strings.TrimSpace(string(logBytes)) != "" {
			t.Fatalf("unexpected kill-session calls: %s", string(logBytes))
		}
	}
}

func TestCleanupStaleRunsPolecatStale(t *testing.T) {
	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	rigsConfig := &config.RigsConfig{
		Version: 1,
		Rigs: map[string]config.RigEntry{
			"alpha": {AddedAt: time.Now()},
			"beta":  {AddedAt: time.Now()},
		},
	}
	if err := config.SaveRigsConfig(filepath.Join(mayorDir, "rigs.json"), rigsConfig); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "gt.log")
	gtScript := `#!/bin/sh
echo "$*" >> "$GT_LOG"
exit 0
`
	gtScriptWindows := `@echo off
echo %* >> "%GT_LOG%"
exit /b 0
`
	writeScript(t, binDir, "gt", gtScript, gtScriptWindows)
	t.Setenv("GT_LOG", logPath)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	prevDryRun := cleanupDryRun
	t.Cleanup(func() { cleanupDryRun = prevDryRun })
	cleanupDryRun = false

	if err := runCleanupStale(nil, nil); err != nil {
		t.Fatalf("runCleanupStale: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read gt log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 gt calls, got %d: %s", len(lines), string(logBytes))
	}

	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "polecat stale alpha --cleanup") {
		t.Fatalf("missing alpha cleanup call: %s", got)
	}
	if !strings.Contains(got, "polecat stale beta --cleanup") {
		t.Fatalf("missing beta cleanup call: %s", got)
	}
}
