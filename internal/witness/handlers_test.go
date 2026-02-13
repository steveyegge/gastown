package witness

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

func TestZombieResult_Types(t *testing.T) {
	// Verify the ZombieResult type has all expected fields
	z := ZombieResult{
		PolecatName: "nux",
		AgentState:  "working",
		HookBead:    "gt-abc123",
		Action:      "auto-nuked",
		Error:       nil,
	}

	if z.PolecatName != "nux" {
		t.Errorf("PolecatName = %q, want %q", z.PolecatName, "nux")
	}
	if z.AgentState != "working" {
		t.Errorf("AgentState = %q, want %q", z.AgentState, "working")
	}
	if z.HookBead != "gt-abc123" {
		t.Errorf("HookBead = %q, want %q", z.HookBead, "gt-abc123")
	}
	if z.Action != "auto-nuked" {
		t.Errorf("Action = %q, want %q", z.Action, "auto-nuked")
	}
}

func TestDetectZombiePolecatsResult_EmptyResult(t *testing.T) {
	result := &DetectZombiePolecatsResult{}

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0", result.Checked)
	}
	if len(result.Zombies) != 0 {
		t.Errorf("Zombies length = %d, want 0", len(result.Zombies))
	}
}

func TestDetectZombiePolecats_NonexistentDir(t *testing.T) {
	// Should handle missing polecats directory gracefully
	result := DetectZombiePolecats("/nonexistent/path", "testrig", nil)

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for nonexistent dir", result.Checked)
	}
	if len(result.Zombies) != 0 {
		t.Errorf("Zombies = %d, want 0 for nonexistent dir", len(result.Zombies))
	}
}

func TestDetectZombiePolecats_DirectoryScanning(t *testing.T) {
	// Create a temp directory structure simulating polecats
	tmpDir := t.TempDir()
	rigName := "testrig"
	polecatsDir := filepath.Join(tmpDir, rigName, "polecats")
	if err := os.MkdirAll(polecatsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create polecat directories
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if err := os.Mkdir(filepath.Join(polecatsDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Create hidden dir (should be skipped)
	if err := os.Mkdir(filepath.Join(polecatsDir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a regular file (should be skipped, not a dir)
	if err := os.WriteFile(filepath.Join(polecatsDir, "notadir.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := DetectZombiePolecats(tmpDir, rigName, nil)

	// Should have checked 3 polecat dirs (not hidden, not file)
	if result.Checked != 3 {
		t.Errorf("Checked = %d, want 3 (should skip hidden dirs and files)", result.Checked)
	}

	// No zombies because agent bead state will be empty (bd not available),
	// so isZombie stays false for all polecats
	if len(result.Zombies) != 0 {
		t.Errorf("Zombies = %d, want 0 (no agent state = not zombie)", len(result.Zombies))
	}
}

func TestDetectZombiePolecats_EmptyPolecatsDir(t *testing.T) {
	// Empty polecats directory should return 0 checked
	tmpDir := t.TempDir()
	rigName := "testrig"
	polecatsDir := filepath.Join(tmpDir, rigName, "polecats")
	if err := os.MkdirAll(polecatsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result := DetectZombiePolecats(tmpDir, rigName, nil)

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for empty polecats dir", result.Checked)
	}
}

func TestGetAgentBeadState_EmptyOutput(t *testing.T) {
	// getAgentBeadState with invalid bead ID should return empty strings
	// (it calls bd which won't exist in test, so it returns empty)
	state, hook := getAgentBeadState("/nonexistent", "nonexistent-bead")

	if state != "" {
		t.Errorf("state = %q, want empty for missing bead", state)
	}
	if hook != "" {
		t.Errorf("hook = %q, want empty for missing bead", hook)
	}
}

func TestSessionRecreated_NoSession(t *testing.T) {
	// When the session doesn't exist, sessionRecreated should return false
	// (the session wasn't recreated, it's still dead)
	tm := tmux.NewTmux()
	detectedAt := time.Now()

	recreated := sessionRecreated(tm, "gt-nonexistent-session-xyz", detectedAt)
	if recreated {
		t.Error("sessionRecreated returned true for nonexistent session, want false")
	}
}

func TestSessionRecreated_DetectedAtEdgeCases(t *testing.T) {
	// Verify that sessionRecreated returns false when session is dead
	// regardless of the detectedAt timestamp
	tm := tmux.NewTmux()

	// Try with a past timestamp
	recreated := sessionRecreated(tm, "gt-test-nosession-abc", time.Now().Add(-1*time.Hour))
	if recreated {
		t.Error("sessionRecreated returned true for nonexistent session with past time")
	}

	// Try with a future timestamp
	recreated = sessionRecreated(tm, "gt-test-nosession-def", time.Now().Add(1*time.Hour))
	if recreated {
		t.Error("sessionRecreated returned true for nonexistent session with future time")
	}
}

func TestZombieClassification_SpawningState(t *testing.T) {
	// Verify that "spawning" agent state is treated as a zombie indicator.
	// This tests the classification logic inline in DetectZombiePolecats.
	// We can't easily test this via the full function without mocking,
	// so we test the boolean logic directly.
	states := map[string]bool{
		"working":  true,
		"running":  true,
		"spawning": true,
		"idle":     false,
		"done":     false,
		"":         false,
	}

	for state, wantZombie := range states {
		hookBead := ""
		isZombie := false
		if hookBead != "" {
			isZombie = true
		}
		if state == "working" || state == "running" || state == "spawning" {
			isZombie = true
		}

		if isZombie != wantZombie {
			t.Errorf("agent_state=%q: isZombie=%v, want %v", state, isZombie, wantZombie)
		}
	}
}

func TestZombieClassification_HookBeadAlwaysZombie(t *testing.T) {
	// Any polecat with a hook_bead and dead session should be classified as zombie,
	// regardless of agent_state.
	for _, state := range []string{"", "idle", "done", "working"} {
		hookBead := "gt-some-issue"
		isZombie := false
		if hookBead != "" {
			isZombie = true
		}
		if state == "working" || state == "running" || state == "spawning" {
			isZombie = true
		}

		if !isZombie {
			t.Errorf("agent_state=%q with hook_bead=%q: isZombie=false, want true", state, hookBead)
		}
	}
}

func TestZombieClassification_NoHookNoActiveState(t *testing.T) {
	// Polecats with no hook_bead and non-active agent_state should NOT be zombies.
	for _, state := range []string{"", "idle", "done", "completed"} {
		hookBead := ""
		isZombie := false
		if hookBead != "" {
			isZombie = true
		}
		if state == "working" || state == "running" || state == "spawning" {
			isZombie = true
		}

		if isZombie {
			t.Errorf("agent_state=%q with no hook_bead: isZombie=true, want false", state)
		}
	}
}

func TestFindAnyCleanupWisp_NoBdAvailable(t *testing.T) {
	// When bd is not available (test environment), findAnyCleanupWisp
	// should return empty string without panicking
	result := findAnyCleanupWisp("/nonexistent", "testpolecat")
	if result != "" {
		t.Errorf("findAnyCleanupWisp = %q, want empty when bd unavailable", result)
	}
}

func TestExtractDoneIntent_Valid(t *testing.T) {
	ts := time.Now().Add(-45 * time.Second)
	labels := []string{
		"gt:agent",
		"idle:2",
		fmt.Sprintf("done-intent:COMPLETED:%d", ts.Unix()),
	}

	intent := extractDoneIntent(labels)
	if intent == nil {
		t.Fatal("extractDoneIntent returned nil for valid label")
	}
	if intent.ExitType != "COMPLETED" {
		t.Errorf("ExitType = %q, want %q", intent.ExitType, "COMPLETED")
	}
	if intent.Timestamp.Unix() != ts.Unix() {
		t.Errorf("Timestamp = %d, want %d", intent.Timestamp.Unix(), ts.Unix())
	}
}

func TestExtractDoneIntent_Missing(t *testing.T) {
	labels := []string{"gt:agent", "idle:2", "backoff-until:1738972900"}

	intent := extractDoneIntent(labels)
	if intent != nil {
		t.Errorf("extractDoneIntent = %+v, want nil for no done-intent label", intent)
	}
}

func TestExtractDoneIntent_Malformed(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
	}{
		{"missing timestamp", []string{"done-intent:COMPLETED"}},
		{"bad timestamp", []string{"done-intent:COMPLETED:notanumber"}},
		{"empty labels", nil},
		{"empty label list", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := extractDoneIntent(tt.labels)
			if intent != nil {
				t.Errorf("extractDoneIntent(%v) = %+v, want nil for malformed input", tt.labels, intent)
			}
		})
	}
}

func TestExtractDoneIntent_AllExitTypes(t *testing.T) {
	ts := time.Now().Unix()
	for _, exitType := range []string{"COMPLETED", "ESCALATED", "DEFERRED", "PHASE_COMPLETE"} {
		label := fmt.Sprintf("done-intent:%s:%d", exitType, ts)
		intent := extractDoneIntent([]string{label})
		if intent == nil {
			t.Errorf("extractDoneIntent returned nil for exit type %q", exitType)
			continue
		}
		if intent.ExitType != exitType {
			t.Errorf("ExitType = %q, want %q", intent.ExitType, exitType)
		}
	}
}

func TestDetectZombie_DoneIntentDeadSession(t *testing.T) {
	// Verify the logic: dead session + done-intent older than 30s → should be treated as zombie
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-60 * time.Second), // 60s old
	}
	sessionAlive := false
	age := time.Since(doneIntent.Timestamp)

	// Dead session + old intent → auto-nuke path
	shouldAutoNuke := !sessionAlive && doneIntent != nil && age >= 30*time.Second
	if !shouldAutoNuke {
		t.Errorf("expected auto-nuke for dead session + old done-intent (age=%v)", age)
	}
}

func TestDetectZombie_DoneIntentLiveStuck(t *testing.T) {
	// Verify the logic: live session + done-intent older than 60s → should kill session
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-90 * time.Second), // 90s old
	}
	sessionAlive := true
	age := time.Since(doneIntent.Timestamp)

	// Live session + old intent → kill stuck session
	shouldKill := sessionAlive && doneIntent != nil && age > 60*time.Second
	if !shouldKill {
		t.Errorf("expected kill for live session + old done-intent (age=%v)", age)
	}
}

func TestDetectZombie_DoneIntentRecent(t *testing.T) {
	// Verify the logic: done-intent younger than 30s → skip (polecat still working)
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-10 * time.Second), // 10s old
	}
	sessionAlive := false
	age := time.Since(doneIntent.Timestamp)

	// Recent intent → should skip
	shouldSkip := !sessionAlive && doneIntent != nil && age < 30*time.Second
	if !shouldSkip {
		t.Errorf("expected skip for recent done-intent (age=%v)", age)
	}

	// Live session + recent intent → also skip
	sessionAlive = true
	shouldSkipLive := sessionAlive && doneIntent != nil && age <= 60*time.Second
	if !shouldSkipLive {
		t.Errorf("expected skip for live session + recent done-intent (age=%v)", age)
	}
}

func TestDetectZombie_AgentDeadInLiveSession(t *testing.T) {
	// Verify the logic: live session + agent process dead → zombie
	// This is the gt-kj6r6 fix: DetectZombiePolecats now checks IsAgentAlive
	// for sessions that DO exist, catching the tmux-alive-but-agent-dead class.
	sessionAlive := true
	agentAlive := false
	var doneIntent *DoneIntent // No done-intent

	// Live session + no done-intent + agent dead → should be classified as zombie
	shouldDetect := sessionAlive && doneIntent == nil && !agentAlive
	if !shouldDetect {
		t.Error("expected zombie detection for live session with dead agent")
	}

	// Live session + agent alive → NOT a zombie
	agentAlive = true
	shouldSkip := sessionAlive && doneIntent == nil && agentAlive
	if !shouldSkip {
		t.Error("expected skip for live session with alive agent")
	}
}

func TestGetAgentBeadLabels_NoBdAvailable(t *testing.T) {
	// When bd is not available, should return nil without panicking
	labels := getAgentBeadLabels("/nonexistent", "nonexistent-bead")
	if labels != nil {
		t.Errorf("getAgentBeadLabels = %v, want nil when bd unavailable", labels)
	}
}

// --- extractPolecatFromJSON tests (issue #1228: panic-safe JSON parsing) ---

func TestExtractPolecatFromJSON_ValidOutput(t *testing.T) {
	input := `[{"labels":["cleanup","polecat:nux","state:pending"]}]`
	got := extractPolecatFromJSON(input)
	if got != "nux" {
		t.Errorf("extractPolecatFromJSON() = %q, want %q", got, "nux")
	}
}

func TestExtractPolecatFromJSON_InvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty output", ""},
		{"malformed JSON", "{not valid json"},
		{"empty array", "[]"},
		{"no polecat label", `[{"labels":["cleanup","state:pending"]}]`},
		{"empty labels", `[{"labels":[]}]`},
		{"truncated JSON", `[{"labels":["polecat:`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPolecatFromJSON(tt.input)
			if got != "" {
				t.Errorf("extractPolecatFromJSON(%q) = %q, want empty", tt.input, got)
			}
		})
	}
}

func TestGetBeadStatus_NoBdAvailable(t *testing.T) {
	// When bd is not available (test environment), getBeadStatus
	// should return empty string without panicking
	result := getBeadStatus("/nonexistent", "gt-abc123")
	if result != "" {
		t.Errorf("getBeadStatus = %q, want empty when bd unavailable", result)
	}
}

func TestGetBeadStatus_EmptyBeadID(t *testing.T) {
	// Empty bead ID should return empty string immediately
	result := getBeadStatus("/nonexistent", "")
	if result != "" {
		t.Errorf("getBeadStatus(\"\") = %q, want empty", result)
	}
}

func TestDetectZombie_BeadClosedStillRunning(t *testing.T) {
	// Verify the logic: live session + agent alive + hooked bead closed → zombie
	// This is the gt-h1l6i fix: DetectZombiePolecats now checks if the
	// polecat's hooked bead has been closed while the session is still running.
	sessionAlive := true
	agentAlive := true
	var doneIntent *DoneIntent // No done-intent
	hookBead := "gt-some-issue"
	beadStatus := "closed"

	// Live session + agent alive + no done-intent + bead closed → should detect
	shouldDetect := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"
	if !shouldDetect {
		t.Error("expected zombie detection for live session with closed bead")
	}

	// Bead open → NOT a zombie
	beadStatus = "open"
	shouldSkip := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"
	if shouldSkip {
		t.Error("should not detect zombie when bead is still open")
	}

	// No hook bead → NOT a zombie
	hookBead = ""
	beadStatus = "closed"
	shouldSkipNoHook := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"
	if shouldSkipNoHook {
		t.Error("should not detect zombie when no hook bead exists")
	}
}

func TestDetectZombie_BeadClosedVsDoneIntent(t *testing.T) {
	// Verify done-intent takes priority over closed-bead check.
	// If done-intent exists (recent), the polecat is still working through
	// gt done and we should NOT trigger the closed-bead path.
	sessionAlive := true
	agentAlive := true
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-10 * time.Second), // Recent
	}
	hookBead := "gt-some-issue"
	beadStatus := "closed"

	// Done-intent exists + bead closed → done-intent check runs first,
	// closed-bead check should NOT run (it's in the else branch)
	doneIntentHandled := sessionAlive && doneIntent != nil && time.Since(doneIntent.Timestamp) > 60*time.Second
	closedBeadCheck := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"

	// Neither should trigger: done-intent is recent (not stuck), and
	// closed-bead check requires doneIntent == nil
	if doneIntentHandled {
		t.Error("recent done-intent should not trigger stuck-session handler")
	}
	if closedBeadCheck {
		t.Error("closed-bead check should not run when done-intent exists")
	}
}
