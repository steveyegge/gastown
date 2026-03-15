package circuit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempTown(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestNewBreaker(t *testing.T) {
	b := NewBreaker("beacon")
	if b.Rig != "beacon" {
		t.Errorf("expected rig=beacon, got %s", b.Rig)
	}
	if b.State != StateClosed {
		t.Errorf("expected state=closed, got %s", b.State)
	}
	if b.Threshold != DefaultFailureThreshold {
		t.Errorf("expected threshold=%d, got %d", DefaultFailureThreshold, b.Threshold)
	}
	if b.Stages[StageWitness] == nil || b.Stages[StageRefinery] == nil {
		t.Error("stages not initialized")
	}
}

func TestLoadMissing(t *testing.T) {
	town := tempTown(t)
	b, err := Load(town, "beacon")
	if err != nil {
		t.Fatal(err)
	}
	if b.State != StateClosed {
		t.Errorf("expected closed state for missing file, got %s", b.State)
	}
}

func TestSaveAndLoad(t *testing.T) {
	town := tempTown(t)
	b := NewBreaker("beacon")
	b.RecordFailure(StageWitness, "zombie detected")
	b.RecordFailure(StageWitness, "orphan found")

	if err := Save(town, b); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(town, "beacon")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Stages[StageWitness].ConsecutiveFailures != 2 {
		t.Errorf("expected 2 failures, got %d", loaded.Stages[StageWitness].ConsecutiveFailures)
	}
	if loaded.State != StateClosed {
		t.Errorf("expected closed (below threshold), got %s", loaded.State)
	}
}

func TestRecordFailureOpensCircuit(t *testing.T) {
	b := NewBreaker("beacon")
	b.Threshold = 3

	b.RecordFailure(StageWitness, "fail 1")
	if b.State != StateClosed {
		t.Fatal("should still be closed after 1 failure")
	}

	b.RecordFailure(StageWitness, "fail 2")
	if b.State != StateClosed {
		t.Fatal("should still be closed after 2 failures")
	}

	opened := b.RecordFailure(StageWitness, "fail 3")
	if !opened {
		t.Error("RecordFailure should return true when circuit opens")
	}
	if b.State != StateOpen {
		t.Errorf("expected open, got %s", b.State)
	}
	if b.OpenedBy != StageWitness {
		t.Errorf("expected opened_by=witness, got %s", b.OpenedBy)
	}
}

func TestRecordFailureIndependentStages(t *testing.T) {
	b := NewBreaker("beacon")
	b.Threshold = 3

	// 2 witness failures + 2 refinery failures should NOT open
	b.RecordFailure(StageWitness, "w1")
	b.RecordFailure(StageWitness, "w2")
	b.RecordFailure(StageRefinery, "r1")
	b.RecordFailure(StageRefinery, "r2")

	if b.State != StateClosed {
		t.Errorf("should still be closed, cross-stage failures are independent")
	}

	// 3rd refinery failure opens it
	opened := b.RecordFailure(StageRefinery, "r3")
	if !opened {
		t.Error("should have opened on 3rd refinery failure")
	}
	if b.OpenedBy != StageRefinery {
		t.Errorf("expected opened_by=refinery, got %s", b.OpenedBy)
	}
}

func TestRecordSuccessResetsCounter(t *testing.T) {
	b := NewBreaker("beacon")
	b.RecordFailure(StageWitness, "f1")
	b.RecordFailure(StageWitness, "f2")
	b.RecordSuccess(StageWitness)

	if b.Stages[StageWitness].ConsecutiveFailures != 0 {
		t.Errorf("expected 0 after success, got %d", b.Stages[StageWitness].ConsecutiveFailures)
	}
}

func TestCheckDispatchClosed(t *testing.T) {
	b := NewBreaker("beacon")
	if err := b.CheckDispatch(); err != nil {
		t.Errorf("closed circuit should allow dispatch: %v", err)
	}
}

func TestCheckDispatchOpen(t *testing.T) {
	b := NewBreaker("beacon")
	b.Threshold = 1
	b.RecordFailure(StageRefinery, "merge failed")

	err := b.CheckDispatch()
	if err == nil {
		t.Fatal("open circuit should block dispatch")
	}
	openErr, ok := err.(*OpenError)
	if !ok {
		t.Fatalf("expected *OpenError, got %T", err)
	}
	if openErr.Rig != "beacon" {
		t.Errorf("expected rig=beacon, got %s", openErr.Rig)
	}
	if openErr.Stage != StageRefinery {
		t.Errorf("expected stage=refinery, got %s", openErr.Stage)
	}
}

func TestCheckDispatchOpenToHalfOpen(t *testing.T) {
	b := NewBreaker("beacon")
	b.Threshold = 1
	b.RecordFailure(StageWitness, "fail")

	// Simulate timeout expired by backdating OpenedAt
	past := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	b.OpenedAt = past

	if err := b.CheckDispatch(); err != nil {
		t.Errorf("should allow dispatch after timeout (half-open): %v", err)
	}
	if b.State != StateHalfOpen {
		t.Errorf("expected half_open, got %s", b.State)
	}
}

func TestHalfOpenSuccessCloses(t *testing.T) {
	b := NewBreaker("beacon")
	b.State = StateHalfOpen
	b.OpenedBy = StageWitness

	b.RecordSuccess(StageWitness)

	if b.State != StateClosed {
		t.Errorf("expected closed after half-open success, got %s", b.State)
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	b := NewBreaker("beacon")
	b.State = StateHalfOpen
	b.OpenedBy = StageWitness
	b.Threshold = 3

	opened := b.RecordFailure(StageWitness, "half-open fail")
	if !opened {
		t.Error("should return true when reopening from half-open")
	}
	if b.State != StateOpen {
		t.Errorf("expected open after half-open failure, got %s", b.State)
	}
}

func TestReset(t *testing.T) {
	b := NewBreaker("beacon")
	b.Threshold = 1
	b.RecordFailure(StageWitness, "fail")
	b.RecordFailure(StageRefinery, "fail")

	if b.State != StateOpen {
		t.Fatal("should be open before reset")
	}

	b.Reset("mayor")

	if b.State != StateClosed {
		t.Errorf("expected closed after reset, got %s", b.State)
	}
	if b.ResetBy != "mayor" {
		t.Errorf("expected reset_by=mayor, got %s", b.ResetBy)
	}
	if b.Stages[StageWitness].ConsecutiveFailures != 0 {
		t.Error("witness failures should be reset")
	}
	if b.Stages[StageRefinery].ConsecutiveFailures != 0 {
		t.Error("refinery failures should be reset")
	}
}

func TestRecordFailureAndSave(t *testing.T) {
	town := tempTown(t)
	opened, err := RecordFailureAndSave(town, "beacon", StageWitness, "test fail")
	if err != nil {
		t.Fatal(err)
	}
	if opened {
		t.Error("should not open after 1 failure")
	}

	// Verify persistence
	b, err := Load(town, "beacon")
	if err != nil {
		t.Fatal(err)
	}
	if b.Stages[StageWitness].ConsecutiveFailures != 1 {
		t.Errorf("expected 1 failure on disk, got %d", b.Stages[StageWitness].ConsecutiveFailures)
	}
}

func TestRecordSuccessAndSave(t *testing.T) {
	town := tempTown(t)
	RecordFailureAndSave(town, "beacon", StageRefinery, "f1")
	RecordFailureAndSave(town, "beacon", StageRefinery, "f2")

	if err := RecordSuccessAndSave(town, "beacon", StageRefinery); err != nil {
		t.Fatal(err)
	}

	b, err := Load(town, "beacon")
	if err != nil {
		t.Fatal(err)
	}
	if b.Stages[StageRefinery].ConsecutiveFailures != 0 {
		t.Errorf("expected 0 after success, got %d", b.Stages[StageRefinery].ConsecutiveFailures)
	}
}

func TestCheckDispatchForRigTransition(t *testing.T) {
	town := tempTown(t)

	// Build up to threshold
	b := NewBreaker("beacon")
	b.Threshold = 2
	b.RecordFailure(StageWitness, "f1")
	b.RecordFailure(StageWitness, "f2")
	// Backdate so timeout has expired
	b.OpenedAt = time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	Save(town, b)

	// CheckDispatchForRig should transition to HALF_OPEN and save
	if err := CheckDispatchForRig(town, "beacon"); err != nil {
		t.Errorf("should allow dispatch after timeout: %v", err)
	}

	// Verify the transition was persisted
	loaded, err := Load(town, "beacon")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateHalfOpen {
		t.Errorf("expected half_open persisted, got %s", loaded.State)
	}
}

func TestLoadLegacyMissingStages(t *testing.T) {
	town := tempTown(t)
	// Write a minimal JSON without stages
	data := []byte(`{"rig":"beacon","state":"closed"}`)
	path := stateFile(town, "beacon")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	b, err := Load(town, "beacon")
	if err != nil {
		t.Fatal(err)
	}
	if b.Stages[StageWitness] == nil || b.Stages[StageRefinery] == nil {
		t.Error("missing stages should be initialized on load")
	}
	if b.Threshold != DefaultFailureThreshold {
		t.Errorf("zero threshold should default to %d, got %d", DefaultFailureThreshold, b.Threshold)
	}
}

func TestOpenErrorMessage(t *testing.T) {
	e := &OpenError{
		Rig:       "beacon",
		Stage:     StageWitness,
		OpenedAt:  "2026-03-15T12:00:00Z",
		Reason:    "zombie polecat detected",
		Threshold: 3,
	}
	msg := e.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
	// Verify it contains actionable information
	for _, want := range []string{"beacon", "witness", "gt circuit reset"} {
		if !contains(msg, want) {
			t.Errorf("error message should contain %q, got: %s", want, msg)
		}
	}
}

func TestStateFileJSON(t *testing.T) {
	town := tempTown(t)
	b := NewBreaker("beacon")
	b.Threshold = 2
	b.RecordFailure(StageWitness, "test")
	Save(town, b)

	// Read raw JSON and verify structure
	data, err := os.ReadFile(stateFile(town, "beacon"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if raw["rig"] != "beacon" {
		t.Errorf("expected rig=beacon in JSON, got %v", raw["rig"])
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
