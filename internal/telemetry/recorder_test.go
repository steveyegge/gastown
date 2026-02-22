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

func TestRecordBDCall_TruncatesLongOutput(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	bigStdout := make([]byte, maxStdoutLog+100)
	bigStderr := string(make([]byte, maxStderrLog+100))
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

func TestRecordPaneRead(t *testing.T) {
	resetInstruments(t)
	ctx := context.Background()

	RecordPaneRead(ctx, "sess-abc", 50, 4096, nil)
	RecordPaneRead(ctx, "sess-def", 0, 0, errors.New("read error"))
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
