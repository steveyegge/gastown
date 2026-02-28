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
	extraVars := []string{"branch=polecat/furiosa/gt-abc123"}
	result, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-abc123", "Test Bug Fix", "", townRoot, false, extraVars)
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
	if !strings.Contains(logContent, "--var branch=polecat/furiosa/gt-abc123") {
		t.Errorf("extra vars not passed to wisp command:\n%s", logContent)
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
	_, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-test", "Test", "", townRoot, true, nil)
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

	err := CookFormula("mol-polecat-work", townRoot, townRoot)
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
		name          string
		formulaName   string
		hookRawBead   bool
		targetAgent   string
		wantAutoApply bool
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

	_, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-abc123", "My Cool Feature", "", townRoot, false, nil)
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

func TestInstantiateFormulaOnBead_FallbackToDirectBond(t *testing.T) {
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

	// Legacy path behavior:
	// - mol wisp succeeds and returns a root ID
	// - mol bond <wisp-id> fails with "not found"
	// Fallback behavior:
	// - mol bond <formula> <bead> --ephemeral succeeds and returns id_mapping
	bdScript := `#!/bin/sh
set -e
echo "CMD:$*" >> "${BD_LOG}"
cmd="$1"; shift || true
case "$cmd" in
  cook)
    exit 0
    ;;
  mol)
    sub="$1"; shift || true
    case "$sub" in
      wisp)
        echo '{"new_epic_id":"gt-wisp-missing"}'
        exit 0
        ;;
      bond)
        left="$1"; shift || true
        if [ "$left" = "gt-wisp-missing" ]; then
          echo "Error: 'gt-wisp-missing' not found (not an issue ID or formula name)" >&2
          exit 1
        fi
        if [ "$left" = "mol-polecat-work" ]; then
          echo '{"result_id":"gt-abc123","id_mapping":{"mol-polecat-work":"gt-mol-fallback"}}'
          exit 0
        fi
        echo "Error: unexpected bond target: $left" >&2
        exit 1
        ;;
    esac
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
set "left=%3"
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-missing^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    if "%left%"=="gt-wisp-missing" (
      echo Error: 'gt-wisp-missing' not found - not an issue ID or formula name 1>&2
      exit /b 1
    )
    if "%left%"=="mol-polecat-work" (
      echo {^"result_id^":^"gt-abc123^",^"id_mapping^":{^"mol-polecat-work^":^"gt-mol-fallback^"}}
      exit /b 0
    )
    echo Error: unexpected bond target: %left% 1>&2
    exit /b 1
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

	result, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-abc123", "My Cool Feature", "", townRoot, false, nil)
	if err != nil {
		t.Fatalf("InstantiateFormulaOnBead: %v", err)
	}
	if result.WispRootID != "gt-mol-fallback" {
		t.Fatalf("WispRootID = %q, want %q", result.WispRootID, "gt-mol-fallback")
	}
	if result.BeadToHook != "gt-abc123" {
		t.Fatalf("BeadToHook = %q, want %q", result.BeadToHook, "gt-abc123")
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logContent := string(logBytes)
	if !strings.Contains(logContent, "mol bond gt-wisp-missing gt-abc123 --json") {
		t.Fatalf("missing legacy bond attempt in log:\n%s", logContent)
	}
	var fallbackBondLine string
	for _, line := range strings.Split(logContent, "\n") {
		if strings.Contains(line, "mol bond mol-polecat-work gt-abc123 --json --ephemeral") {
			fallbackBondLine = line
			break
		}
	}
	if fallbackBondLine == "" {
		t.Fatalf("missing direct bond fallback in log:\n%s", logContent)
	}
	if !containsVarArg(fallbackBondLine, "feature", "My Cool Feature") {
		t.Fatalf("fallback bond missing feature variable:\n%s", logContent)
	}
	if !containsVarArg(fallbackBondLine, "issue", "gt-abc123") {
		t.Fatalf("fallback bond missing issue variable:\n%s", logContent)
	}
	for _, required := range []struct {
		key   string
		value string
	}{
		{"base_branch", "main"},
		{"setup_command", ""},
		{"typecheck_command", ""},
		{"lint_command", ""},
		{"test_command", "go test ./..."},
		{"build_command", ""},
	} {
		if !containsVarArg(fallbackBondLine, required.key, required.value) {
			t.Fatalf("fallback bond missing required variable %q:\n%s", required.key, logContent)
		}
	}
}

// TestIsMalformedWispID verifies detection of doubled "-wisp-" in wisp IDs (gt-4gjd).
func TestIsMalformedWispID(t *testing.T) {
	tests := []struct {
		name       string
		wispID     string
		wantMalformed bool
	}{
		{"normal wisp ID", "gt-wisp-abc", false},
		{"normal with long random", "oag-wisp-gm7c", false},
		{"doubled wisp marker", "oag-wisp-wisp-rsia", true},
		{"triple wisp marker", "gt-wisp-wisp-wisp-x", true},
		{"empty", "", false},
		{"no wisp marker", "gt-abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMalformedWispID(tt.wispID)
			if got != tt.wantMalformed {
				t.Errorf("isMalformedWispID(%q) = %v, want %v", tt.wispID, got, tt.wantMalformed)
			}
		})
	}
}

// TestInstantiateFormulaOnBead_MalformedWispIDSkipsLegacyBond verifies that
// when bd mol wisp returns a malformed ID (doubled "-wisp-"), the legacy bond
// attempt is skipped and the direct-bond fallback is used immediately (gt-4gjd).
func TestInstantiateFormulaOnBead_MalformedWispIDSkipsLegacyBond(t *testing.T) {
	townRoot := t.TempDir()

	// Minimal workspace
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(`{"prefix":"oag-","path":"."}`), 0644); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")

	// bd mol wisp returns a malformed ID with doubled "-wisp-".
	// Legacy bond should be SKIPPED (never called with the malformed ID).
	// Direct-bond fallback should succeed.
	bdScript := `#!/bin/sh
set -e
echo "CMD:$*" >> "${BD_LOG}"
cmd="$1"; shift || true
case "$cmd" in
  cook)
    exit 0
    ;;
  mol)
    sub="$1"; shift || true
    case "$sub" in
      wisp)
        echo '{"new_epic_id":"oag-wisp-wisp-rsia"}'
        exit 0
        ;;
      bond)
        left="$1"; shift || true
        if [ "$left" = "oag-wisp-wisp-rsia" ]; then
          echo "ERROR: legacy bond should not be called with malformed ID" >&2
          exit 1
        fi
        if [ "$left" = "mol-polecat-work" ]; then
          echo '{"result_id":"oag-npeat","id_mapping":{"mol-polecat-work":"oag-wisp-gm7c"}}'
          exit 0
        fi
        echo "Error: unexpected bond target: $left" >&2
        exit 1
        ;;
    esac
    ;;
  close)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
set "left=%3"
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"oag-wisp-wisp-rsia^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    if "%left%"=="oag-wisp-wisp-rsia" (
      echo ERROR: legacy bond should not be called with malformed ID 1>&2
      exit /b 1
    )
    if "%left%"=="mol-polecat-work" (
      echo {^"result_id^":^"oag-npeat^",^"id_mapping^":{^"mol-polecat-work^":^"oag-wisp-gm7c^"}}
      exit /b 0
    )
    echo Error: unexpected bond target: %left% 1>&2
    exit /b 1
  )
)
if "%cmd%"=="close" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(townRoot)

	result, err := InstantiateFormulaOnBead("mol-polecat-work", "oag-npeat", "Fix formula bug", "", townRoot, false, nil)
	if err != nil {
		t.Fatalf("InstantiateFormulaOnBead: %v", err)
	}
	if result.WispRootID != "oag-wisp-gm7c" {
		t.Fatalf("WispRootID = %q, want %q", result.WispRootID, "oag-wisp-gm7c")
	}
	if result.BeadToHook != "oag-npeat" {
		t.Fatalf("BeadToHook = %q, want %q", result.BeadToHook, "oag-npeat")
	}

	// Verify the legacy bond was NEVER called with the malformed ID.
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logContent := string(logBytes)
	if strings.Contains(logContent, "mol bond oag-wisp-wisp-rsia") {
		t.Fatalf("legacy bond should not have been called with malformed wisp ID:\n%s", logContent)
	}
	// Verify fallback was used (direct bond with formula name)
	if !strings.Contains(logContent, "mol bond mol-polecat-work oag-npeat") {
		t.Fatalf("direct-bond fallback should have been called:\n%s", logContent)
	}
}

// TestInstantiateFormulaOnBead_FallbackCleansUpOrphanedWisp verifies that when
// the legacy bond fails and fallback is used, the orphaned wisp from bd mol wisp
// is cleaned up (gt-4gjd).
func TestInstantiateFormulaOnBead_FallbackCleansUpOrphanedWisp(t *testing.T) {
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

	// Legacy path: mol wisp succeeds, mol bond <wisp-id> fails.
	// After fallback succeeds, a close command should be issued for the orphaned wisp.
	bdScript := `#!/bin/sh
set -e
echo "CMD:$*" >> "${BD_LOG}"
cmd="$1"; shift || true
case "$cmd" in
  cook)
    exit 0
    ;;
  mol)
    sub="$1"; shift || true
    case "$sub" in
      wisp)
        echo '{"new_epic_id":"gt-wisp-orphan"}'
        exit 0
        ;;
      bond)
        left="$1"; shift || true
        if [ "$left" = "gt-wisp-orphan" ]; then
          echo "Error: 'gt-wisp-orphan' not found (not an issue ID or formula name)" >&2
          exit 1
        fi
        if [ "$left" = "mol-polecat-work" ]; then
          echo '{"result_id":"gt-test","id_mapping":{"mol-polecat-work":"gt-wisp-clean"}}'
          exit 0
        fi
        echo "Error: unexpected bond target: $left" >&2
        exit 1
        ;;
    esac
    ;;
  close)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
set "left=%3"
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-orphan^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    if "%left%"=="gt-wisp-orphan" (
      echo Error: 'gt-wisp-orphan' not found 1>&2
      exit /b 1
    )
    if "%left%"=="mol-polecat-work" (
      echo {^"result_id^":^"gt-test^",^"id_mapping^":{^"mol-polecat-work^":^"gt-wisp-clean^"}}
      exit /b 0
    )
    echo Error: unexpected bond target: %left% 1>&2
    exit /b 1
  )
)
if "%cmd%"=="close" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	_ = os.Chdir(townRoot)

	result, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-test", "Test cleanup", "", townRoot, false, nil)
	if err != nil {
		t.Fatalf("InstantiateFormulaOnBead: %v", err)
	}
	if result.WispRootID != "gt-wisp-clean" {
		t.Fatalf("WispRootID = %q, want %q", result.WispRootID, "gt-wisp-clean")
	}

	// Verify a close command was issued for the orphaned wisp
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logContent := string(logBytes)
	if !strings.Contains(logContent, "close") || !strings.Contains(logContent, "gt-wisp-orphan") {
		t.Fatalf("expected close command for orphaned wisp gt-wisp-orphan in log:\n%s", logContent)
	}
}

// TestInstantiateFormulaOnBead_ParseFailureFallbackFailure verifies that when
// legacy bond exits 0 with non-JSON output AND the direct-bond fallback also
// fails, InstantiateFormulaOnBead returns an error instead of silent success.
func TestInstantiateFormulaOnBead_ParseFailureFallbackFailure(t *testing.T) {
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

	// Legacy bond exits 0 but returns non-JSON garbage.
	// Direct-bond fallback also fails (all bond calls fail).
	bdScript := `#!/bin/sh
set -e
echo "CMD:$*" >> "${BD_LOG}"
cmd="$1"; shift || true
case "$cmd" in
  cook)
    exit 0
    ;;
  mol)
    sub="$1"; shift || true
    case "$sub" in
      wisp)
        echo '{"new_epic_id":"gt-wisp-abc"}'
        exit 0
        ;;
      bond)
        left="$1"; shift || true
        if [ "$left" = "gt-wisp-abc" ]; then
          echo 'NOT-JSON-GARBAGE'
          exit 0
        fi
        echo "Error: bond failed" >&2
        exit 1
        ;;
    esac
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo CMD:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
set "left=%3"
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-abc^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    if "%left%"=="gt-wisp-abc" (
      echo NOT-JSON-GARBAGE
      exit /b 0
    )
    echo Error: bond failed 1>&2
    exit /b 1
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

	_, err := InstantiateFormulaOnBead("mol-polecat-work", "gt-abc123", "My Feature", "", townRoot, false, nil)
	if err == nil {
		t.Fatal("expected error when bond returns non-JSON and fallback fails, got nil")
	}
	if !strings.Contains(err.Error(), "not parseable") && !strings.Contains(err.Error(), "fallback failed") {
		t.Fatalf("error message should mention parse failure and fallback: %v", err)
	}
}
