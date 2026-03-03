package telemetry

import (
	"context"
	"errors"
	"sync"
	"testing"

	otellog "go.opentelemetry.io/otel/log"
)

// resetInstruments resets the sync.Once so initInstruments re-runs against
// the current (noop) global MeterProvider during tests.
func resetInstruments(t *testing.T) {
	t.Helper()
	instOnce = sync.Once{}
	t.Cleanup(func() { instOnce = sync.Once{} })
}

// --- helper functions ---

func TestStatusStr(t *testing.T) {
	if got := statusStr(nil); got != "ok" {
		t.Errorf("statusStr(nil) = %q, want \"ok\"", got)
	}
	if got := statusStr(errors.New("boom")); got != "error" {
		t.Errorf("statusStr(err) = %q, want \"error\"", got)
	}
}

func TestTruncateOutput_Short(t *testing.T) {
	if got := truncateOutput("hello", 10); got != "hello" {
		t.Errorf("short string should not be truncated, got %q", got)
	}
}

func TestTruncateOutput_Exact(t *testing.T) {
	if got := truncateOutput("abcde", 5); got != "abcde" {
		t.Errorf("string at exact limit should not be truncated, got %q", got)
	}
}

func TestTruncateOutput_Long(t *testing.T) {
	got := truncateOutput("abcdefghij", 5)
	if got != "abcde…" {
		t.Errorf("truncateOutput = %q, want %q", got, "abcde…")
	}
}

func TestTruncateOutput_Empty(t *testing.T) {
	if got := truncateOutput("", 10); got != "" {
		t.Errorf("empty string changed: %q", got)
	}
}

func TestSeverity_Nil(t *testing.T) {
	if got := severity(nil); got != otellog.SeverityInfo {
		t.Errorf("severity(nil) = %v, want SeverityInfo", got)
	}
}

func TestSeverity_Error(t *testing.T) {
	if got := severity(errors.New("err")); got != otellog.SeverityError {
		t.Errorf("severity(err) = %v, want SeverityError", got)
	}
}

func TestErrKV_Nil(t *testing.T) {
	kv := errKV(nil)
	if kv.Value.AsString() != "" {
		t.Errorf("errKV(nil) value = %q, want empty", kv.Value.AsString())
	}
}

func TestErrKV_NonNil(t *testing.T) {
	kv := errKV(errors.New("test error"))
	if kv.Value.AsString() != "test error" {
		t.Errorf("errKV(err) value = %q, want %q", kv.Value.AsString(), "test error")
	}
}

// --- Record* functions (noop providers, must not panic) ---

func TestRecordBDCall(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordBDCall(ctx, []string{"list", "--all"}, 12.5, nil, []byte("output"), "")
	RecordBDCall(ctx, []string{"status"}, 3.0, errors.New("fail"), []byte(""), "stderr msg")
	RecordBDCall(ctx, nil, 0, nil, nil, "")
}

func TestRecordBDCall_LargeOutput(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	// Content is no longer truncated; verify large payloads are accepted.
	bigStdout := make([]byte, 100_000)
	bigStderr := string(make([]byte, 50_000))
	RecordBDCall(ctx, []string{"cmd"}, 1.0, nil, bigStdout, bigStderr)
}

func TestRecordSessionStart(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordSessionStart(ctx, "sess-123", "mol/witness", nil)
	RecordSessionStart(ctx, "sess-456", "mol/refinery", errors.New("fail"))
}

func TestRecordSessionStop(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordSessionStop(ctx, "sess-123", nil)
	RecordSessionStop(ctx, "sess-456", errors.New("stop error"))
}

func TestRecordPromptSend(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordPromptSend(ctx, "sess-abc", "do the thing", 200, nil)
	RecordPromptSend(ctx, "sess-def", "", 0, errors.New("err"))
}

func TestWithRunID_RoundTrip(t *testing.T) {
	ctx := WithRunID(context.Background(), "run-abc-123")
	if got := RunIDFromCtx(ctx); got != "run-abc-123" {
		t.Errorf("RunIDFromCtx = %q, want %q", got, "run-abc-123")
	}
}

func TestRunIDFromCtx_Empty(t *testing.T) {
	// No run ID in context and GT_RUN not set → empty string.
	if got := RunIDFromCtx(context.Background()); got != "" {
		t.Errorf("RunIDFromCtx on bare context = %q, want empty (GT_RUN not set)", got)
	}
}

func TestRecordAgentInstantiate(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordAgentInstantiate(ctx, AgentInstantiateInfo{
		RunID: "run-id-1", AgentType: "claudecode", Role: "polecat",
		AgentName: "wyvern-Toast", SessionID: "gt-wyvern-Toast", RigName: "wyvern",
		TownRoot: "/Users/pa/gt", IssueID: "GT-123", GitBranch: "feat/foo", GitCommit: "abc1234",
	})
	RecordAgentInstantiate(ctx, AgentInstantiateInfo{
		RunID: "run-id-2", AgentType: "opencode", Role: "witness",
		AgentName: "witness", SessionID: "mol-witness", RigName: "mol",
		TownRoot: "/Users/pa/gt",
	})
	RecordAgentInstantiate(ctx, AgentInstantiateInfo{
		RunID: "run-id-3", AgentType: "claudecode", Role: "mayor",
		AgentName: "mayor", SessionID: "hq-mayor", TownRoot: "/Users/pa/gt",
	})
}

func TestRecordPrime(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordPrime(ctx, "mol/witness", false, nil)
	RecordPrime(ctx, "mol/refinery", true, errors.New("prime error"))
}

func TestRecordAgentStateChange(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	bead := "bead-123"
	RecordAgentStateChange(ctx, "agent-1", "idle", nil, nil)
	RecordAgentStateChange(ctx, "agent-2", "working", &bead, nil)
	RecordAgentStateChange(ctx, "agent-3", "done", nil, errors.New("state error"))

	empty := ""
	RecordAgentStateChange(ctx, "agent-4", "idle", &empty, nil)
}

func TestRecordPolecatSpawn(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordPolecatSpawn(ctx, "furiosa", nil)
	RecordPolecatSpawn(ctx, "nux", errors.New("spawn error"))
}

func TestRecordPolecatRemove(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordPolecatRemove(ctx, "furiosa", nil)
	RecordPolecatRemove(ctx, "nux", errors.New("remove error"))
}

func TestRecordSling(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordSling(ctx, "bead-abc", "furiosa", nil)
	RecordSling(ctx, "bead-def", "nux", errors.New("sling error"))
}

func TestRecordMail(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordMail(ctx, "send", nil)
	RecordMail(ctx, "read", errors.New("mail error"))
}

func TestRecordNudge(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordNudge(ctx, "furiosa", nil)
	RecordNudge(ctx, "nux", errors.New("nudge error"))
}

func TestRecordDone(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordDone(ctx, "COMPLETED", nil)
	RecordDone(ctx, "ESCALATED", nil)
	RecordDone(ctx, "DEFERRED", errors.New("done error"))
}

func TestRecordDaemonRestart(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordDaemonRestart(ctx, "deacon")
	RecordDaemonRestart(ctx, "witness")
	RecordDaemonRestart(ctx, "polecat")
}

func TestRecordFormulaInstantiate(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordFormulaInstantiate(ctx, "my-formula", "bead-123", nil)
	RecordFormulaInstantiate(ctx, "bad-formula", "", errors.New("instantiation error"))
}

func TestRecordConvoyCreate(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordConvoyCreate(ctx, "bead-abc", nil)
	RecordConvoyCreate(ctx, "bead-def", errors.New("convoy error"))
}

func TestRecordMolCook(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordMolCook(ctx, "mol-polecat-work", nil)
	RecordMolCook(ctx, "bad-formula", errors.New("cook error"))
}

func TestRecordMolWisp(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordMolWisp(ctx, "mol-polecat-work", "gt-abc12", "bead-456", nil)
	RecordMolWisp(ctx, "mol-polecat-work", "", "", errors.New("wisp error"))
	RecordMolWisp(ctx, "formula-standalone", "gt-abc12", "", nil) // standalone (no bead)
}

func TestRecordMolSquash(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordMolSquash(ctx, "gt-abc12", 3, 5, true, nil)
	RecordMolSquash(ctx, "gt-def34", 0, 0, false, errors.New("squash error"))
}

func TestRecordMolBurn(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordMolBurn(ctx, "gt-abc12", 3, nil)
	RecordMolBurn(ctx, "gt-def34", 0, errors.New("burn error"))
}

func TestRecordBeadCreate(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordBeadCreate(ctx, "gt-abc12.s01", "gt-abc12", "mol-polecat-work")
	RecordBeadCreate(ctx, "gt-def34.s01", "gt-def34", "mol-review")
}
