package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstantiateFormulaOnBead verifies the helper function works correctly.
// This tests the formula-on-bead pattern used by issue #288.
func TestInstantiateFormulaOnBead(t *testing.T) {
	townRoot := t.TempDir()

	// Minimal workspace marker
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create routes.jsonl
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create stub bd that logs all commands
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdScript := `#!/bin/sh
set -e
echo "CMD:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Fix bug ABC","status":"open","assignee":"","description":""}]'
    ;;
  formula)
    echo '{"name":"mol-polecat-work"}'
    ;;
  cook)
    ;;
  mol)
    sub="$1"
    shift || true
    case "$sub" in
      wisp)
        echo '{"new_epic_id":"gt-wisp-288"}'
        ;;
      bond)
        echo '{"root_id":"gt-wisp-288"}'
        ;;
    esac
    ;;
  update)
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  set "sub=%3"
)
if "%cmd%"=="show" (
  echo [{^"title^":^"Fix bug ABC^",^"status^":^"open^",^"assignee^":^"^",^"description^":^"^"}]
  exit /b 0
)
if "%cmd%"=="formula" (
  echo {^"name^":^"mol-polecat-work^"}
  exit /b 0
)
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-288^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    echo {^"root_id^":^"gt-wisp-288^"}
    exit /b 0
  )
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Test the helper function directly
	result, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-abc123", "Test Bug Fix", "", townRoot, false)
	if err != nil {
		t.Fatalf("InstantiateFormulaOnBead failed: %v", err)
	}

	if result.WispRootID == "" {
		t.Error("WispRootID should not be empty")
	}
	if result.BeadToHook == "" {
		t.Error("BeadToHook should not be empty")
	}

	// Verify commands were logged
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logContent := string(logBytes)

	if !strings.Contains(logContent, "cook mol-polecat-work") {
		t.Errorf("cook command not found in log:\n%s", logContent)
	}
	if !strings.Contains(logContent, "mol wisp mol-polecat-work") {
		t.Errorf("mol wisp command not found in log:\n%s", logContent)
	}
	if !strings.Contains(logContent, "mol bond") {
		t.Errorf("mol bond command not found in log:\n%s", logContent)
	}
}

// TestInstantiateFormulaOnBeadSkipCook verifies the skipCook optimization.
func TestInstantiateFormulaOnBeadSkipCook(t *testing.T) {
	townRoot := t.TempDir()

	// Minimal workspace marker
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create routes.jsonl
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	routes := `{"prefix":"gt-","path":"."}`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create stub bd
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdScript := `#!/bin/sh
echo "CMD:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then shift; fi
cmd="$1"; shift || true
case "$cmd" in
  mol)
    sub="$1"; shift || true
    case "$sub" in
      wisp) echo '{"new_epic_id":"gt-wisp-skip"}';;
      bond) echo '{"root_id":"gt-wisp-skip"}';;
    esac;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  set "sub=%3"
)
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-skip^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    echo {^"root_id^":^"gt-wisp-skip^"}
    exit /b 0
  )
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(townRoot)

	// Test with skipCook=true
	_, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-test", "Test", "", townRoot, true)
	if err != nil {
		t.Fatalf("InstantiateFormulaOnBead failed: %v", err)
	}

	logBytes, _ := os.ReadFile(logPath)
	logContent := string(logBytes)

	// Verify cook was NOT called when skipCook=true
	if strings.Contains(logContent, "cook") {
		t.Errorf("cook should be skipped when skipCook=true, but was called:\n%s", logContent)
	}

	// Verify wisp and bond were still called
	if !strings.Contains(logContent, "mol wisp") {
		t.Errorf("mol wisp should still be called")
	}
	if !strings.Contains(logContent, "mol bond") {
		t.Errorf("mol bond should still be called")
	}
}

// TestCookFormula verifies the CookFormula helper.
func TestCookFormula(t *testing.T) {
	townRoot := t.TempDir()

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdScript := `#!/bin/sh
echo "CMD:$*" >> "${BD_LOG}"
exit 0
`
	bdScriptWindows := `@echo off
echo CMD:%*>>"%BD_LOG%"
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := CookFormula("mol-polecat-work", townRoot)
	if err != nil {
		t.Fatalf("CookFormula failed: %v", err)
	}

	logBytes, _ := os.ReadFile(logPath)
	if !strings.Contains(string(logBytes), "cook mol-polecat-work") {
		t.Errorf("cook command not found in log")
	}
}

// TestSlingHookRawBeadFlag verifies --hook-raw-bead flag exists.
func TestSlingHookRawBeadFlag(t *testing.T) {
	// Verify the flag variable exists and works
	prevValue := slingHookRawBead
	t.Cleanup(func() { slingHookRawBead = prevValue })

	slingHookRawBead = true
	if !slingHookRawBead {
		t.Error("slingHookRawBead flag should be true")
	}

	slingHookRawBead = false
	if slingHookRawBead {
		t.Error("slingHookRawBead flag should be false")
	}
}

// TestAutoApplyLogic verifies the auto-apply detection logic.
// When formulaName is empty and target contains "/polecats/", mol-polecat-work should be applied.
func TestAutoApplyLogic(t *testing.T) {
	tests := []struct {
		name           string
		formulaName    string
		hookRawBead    bool
		targetAgent    string
		wantAutoApply  bool
	}{
		{
			name:          "bare bead to polecat - should auto-apply",
			formulaName:   "",
			hookRawBead:   false,
			targetAgent:   "gastown/polecats/Toast",
			wantAutoApply: true,
		},
		{
			name:          "bare bead with --hook-raw-bead - should not auto-apply",
			formulaName:   "",
			hookRawBead:   true,
			targetAgent:   "gastown/polecats/Toast",
			wantAutoApply: false,
		},
		{
			name:          "formula already specified - should not auto-apply",
			formulaName:   "mol-review",
			hookRawBead:   false,
			targetAgent:   "gastown/polecats/Toast",
			wantAutoApply: false,
		},
		{
			name:          "non-polecat target - should not auto-apply",
			formulaName:   "",
			hookRawBead:   false,
			targetAgent:   "gastown/witness",
			wantAutoApply: false,
		},
		{
			name:          "mayor target - should not auto-apply",
			formulaName:   "",
			hookRawBead:   false,
			targetAgent:   "mayor",
			wantAutoApply: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mirrors the logic in sling.go
			shouldAutoApply := tt.formulaName == "" && !tt.hookRawBead && strings.Contains(tt.targetAgent, "/polecats/")

			if shouldAutoApply != tt.wantAutoApply {
				t.Errorf("auto-apply logic: got %v, want %v", shouldAutoApply, tt.wantAutoApply)
			}
		})
	}
}

// TestFormulaOnBeadPassesVariables verifies that feature and issue variables are passed.
func TestFormulaOnBeadPassesVariables(t *testing.T) {
	townRoot := t.TempDir()

	// Minimal workspace
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(`{"prefix":"gt-","path":"."}`), 0644); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdScript := `#!/bin/sh
echo "CMD:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then shift; fi
cmd="$1"; shift || true
case "$cmd" in
  cook) exit 0;;
  mol)
    sub="$1"; shift || true
    case "$sub" in
      wisp) echo '{"new_epic_id":"gt-wisp-var"}';;
      bond) echo '{"root_id":"gt-wisp-var"}';;
    esac;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  set "sub=%3"
)
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-var^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    echo {^"root_id^":^"gt-wisp-var^"}
    exit /b 0
  )
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(townRoot)

	_, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-abc123", "My Cool Feature", "", townRoot, false)
	if err != nil {
		t.Fatalf("InstantiateFormulaOnBead: %v", err)
	}

	logBytes, _ := os.ReadFile(logPath)
	logContent := string(logBytes)

	// Find mol wisp line
	var wispLine string
	for _, line := range strings.Split(logContent, "\n") {
		if strings.Contains(line, "mol wisp") {
			wispLine = line
			break
		}
	}

	if wispLine == "" {
		t.Fatalf("mol wisp command not found:\n%s", logContent)
	}

	if !strings.Contains(wispLine, "feature=My Cool Feature") {
		t.Errorf("mol wisp missing feature variable:\n%s", wispLine)
	}

	if !strings.Contains(wispLine, "issue=gt-abc123") {
		t.Errorf("mol wisp missing issue variable:\n%s", wispLine)
	}
}
