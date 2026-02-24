// Package telemetry — recorder.go
// Recording helper functions for all GT telemetry events.
// Each function emits both an OTel log event (→ VictoriaLogs) and increments
// a metric counter (→ VictoriaMetrics).
package telemetry

import (
	"context"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
)

const (
	meterRecorderName = "github.com/steveyegge/gastown"
	loggerName        = "gastown"
)

// recorderInstruments holds all lazy-initialized OTel metric instruments.
type recorderInstruments struct {
	// Counters
	bdTotal             metric.Int64Counter
	sessionTotal        metric.Int64Counter
	sessionStopTotal    metric.Int64Counter
	promptTotal         metric.Int64Counter
	paneReadTotal       metric.Int64Counter
	paneOutputTotal     metric.Int64Counter
	primeTotal         metric.Int64Counter
	agentStateTotal    metric.Int64Counter
	polecatTotal       metric.Int64Counter
	polecatRemoveTotal metric.Int64Counter
	slingTotal         metric.Int64Counter
	mailTotal          metric.Int64Counter
	nudgeTotal         metric.Int64Counter
	doneTotal          metric.Int64Counter
	daemonRestartTotal metric.Int64Counter
	formulaTotal       metric.Int64Counter
	convoyTotal        metric.Int64Counter

	// Histograms
	bdDurationHist metric.Float64Histogram
}

var (
	instOnce sync.Once
	inst     recorderInstruments
)

// initInstruments registers all recorder metric instruments against the current
// global MeterProvider. Must be called after telemetry.Init so the real
// provider is set. Also called lazily on first use as a safety net.
func initInstruments() {
	instOnce.Do(func() {
		m := otel.GetMeterProvider().Meter(meterRecorderName)

		// Counters
		inst.bdTotal, _ = m.Int64Counter("gastown.bd.calls.total",
			metric.WithDescription("Total bd CLI command invocations"),
		)
		inst.sessionTotal, _ = m.Int64Counter("gastown.session.starts.total",
			metric.WithDescription("Total agent session starts"),
		)
		inst.sessionStopTotal, _ = m.Int64Counter("gastown.session.stops.total",
			metric.WithDescription("Total agent session terminations"),
		)
		inst.promptTotal, _ = m.Int64Counter("gastown.prompt.sends.total",
			metric.WithDescription("Total tmux SendKeys prompt dispatches"),
		)
		inst.paneReadTotal, _ = m.Int64Counter("gastown.pane.reads.total",
			metric.WithDescription("Total tmux CapturePane calls"),
		)
		inst.paneOutputTotal, _ = m.Int64Counter("gastown.pane.output.total",
			metric.WithDescription("Total pane output chunks emitted to VictoriaLogs"),
		)
		inst.primeTotal, _ = m.Int64Counter("gastown.prime.total",
			metric.WithDescription("Total gt prime invocations"),
		)
		inst.agentStateTotal, _ = m.Int64Counter("gastown.agent.state_changes.total",
			metric.WithDescription("Total agent state transitions"),
		)
		inst.polecatTotal, _ = m.Int64Counter("gastown.polecat.spawns.total",
			metric.WithDescription("Total polecat spawns"),
		)
		inst.polecatRemoveTotal, _ = m.Int64Counter("gastown.polecat.removes.total",
			metric.WithDescription("Total polecat removals"),
		)
		inst.slingTotal, _ = m.Int64Counter("gastown.sling.dispatches.total",
			metric.WithDescription("Total sling work dispatches"),
		)
		inst.mailTotal, _ = m.Int64Counter("gastown.mail.operations.total",
			metric.WithDescription("Total mail/bd SDK operations"),
		)
		inst.nudgeTotal, _ = m.Int64Counter("gastown.nudge.total",
			metric.WithDescription("Total gt nudge invocations"),
		)
		inst.doneTotal, _ = m.Int64Counter("gastown.done.total",
			metric.WithDescription("Total gt done invocations (polecat work completions)"),
		)
		inst.daemonRestartTotal, _ = m.Int64Counter("gastown.daemon.agent_restarts.total",
			metric.WithDescription("Total daemon-initiated agent session restarts"),
		)
		inst.formulaTotal, _ = m.Int64Counter("gastown.formula.instantiations.total",
			metric.WithDescription("Total formula→wisp instantiations"),
		)
		inst.convoyTotal, _ = m.Int64Counter("gastown.convoy.creates.total",
			metric.WithDescription("Total auto-convoy creations"),
		)

		// Histograms
		inst.bdDurationHist, _ = m.Float64Histogram("gastown.bd.duration_ms",
			metric.WithDescription("bd CLI call round-trip latency in milliseconds"),
			metric.WithUnit("ms"),
		)
	})
}

// statusStr returns "ok" or "error" depending on whether err is nil.
func statusStr(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

// emit sends an OTel log event with the given body and key-value attributes.
func emit(ctx context.Context, body string, sev otellog.Severity, attrs ...otellog.KeyValue) {
	logger := global.GetLoggerProvider().Logger(loggerName)
	var r otellog.Record
	r.SetBody(otellog.StringValue(body))
	r.SetSeverity(sev)
	r.AddAttributes(attrs...)
	logger.Emit(ctx, r)
}

// errKV returns a log KeyValue with the error message, or empty string if nil.
func errKV(err error) otellog.KeyValue {
	if err != nil {
		return otellog.String("error", err.Error())
	}
	return otellog.String("error", "")
}

// severity returns SeverityInfo on success, SeverityError on failure.
func severity(err error) otellog.Severity {
	if err != nil {
		return otellog.SeverityError
	}
	return otellog.SeverityInfo
}

const (
	// maxStdoutLog is the maximum number of bytes of stdout captured in logs.
	maxStdoutLog = 2048
	// maxStderrLog is the maximum number of bytes of stderr captured in logs.
	maxStderrLog = 1024
)

// truncateOutput trims s to max bytes and appends "…" when truncated.
// Avoids splitting multi-byte UTF-8 characters at the boundary.
func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Walk back from the cut point to avoid splitting a multi-byte rune.
	truncated := s[:max]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "…"
}

// RecordBDCall records a bd CLI invocation with duration (metrics + log event).
// args is the full argument list; args[0] is used as the subcommand label.
// durationMs is the wall-clock time of the subprocess in milliseconds.
// stdout and stderr are the raw process outputs; both are truncated before logging.
//
// stdout and stderr are only included in the log event when GT_LOG_BD_OUTPUT=true.
// They are opt-in because bd output may contain sensitive data (API tokens, PII).
// See docs/OTEL-ENV-VARS.md for details.
func RecordBDCall(ctx context.Context, args []string, durationMs float64, err error, stdout []byte, stderr string) {
	initInstruments()
	subcommand := ""
	if len(args) > 0 {
		subcommand = args[0]
	}
	status := statusStr(err)
	attrs := metric.WithAttributes(
		attribute.String("status", status),
		attribute.String("subcommand", subcommand),
	)
	inst.bdTotal.Add(ctx, 1, attrs)
	inst.bdDurationHist.Record(ctx, durationMs, attrs)
	kvs := []otellog.KeyValue{
		otellog.String("subcommand", subcommand),
		otellog.String("args", strings.Join(args, " ")),
		otellog.Float64("duration_ms", durationMs),
		otellog.String("status", status),
		errKV(err),
	}
	// stdout/stderr are opt-in: they may contain tokens or PII returned by bd.
	if os.Getenv("GT_LOG_BD_OUTPUT") == "true" {
		kvs = append(kvs,
			otellog.String("stdout", truncateOutput(string(stdout), maxStdoutLog)),
			otellog.String("stderr", truncateOutput(stderr, maxStderrLog)),
		)
	}
	emit(ctx, "bd.call", severity(err), kvs...)
}

// RecordSessionStart records an agent session start (metrics + log event).
func RecordSessionStart(ctx context.Context, sessionID, role string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.sessionTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("role", role),
		),
	)
	emit(ctx, "session.start", severity(err),
		otellog.String("session_id", sessionID),
		otellog.String("role", role),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordSessionStop records an agent session termination (metrics + log event).
func RecordSessionStop(ctx context.Context, sessionID string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.sessionStopTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "session.stop", severity(err),
		otellog.String("session_id", sessionID),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordPromptSend records a tmux SendKeys prompt dispatch (metrics + log event).
func RecordPromptSend(ctx context.Context, session, keys string, debounceMs int, err error) {
	initInstruments()
	status := statusStr(err)
	inst.promptTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "prompt.send", severity(err),
		otellog.String("session", session),
		otellog.Int64("keys_len", int64(len(keys))),
		otellog.Int64("debounce_ms", int64(debounceMs)),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordPaneRead records a tmux CapturePane call (metrics + log event).
func RecordPaneRead(ctx context.Context, session string, lines, contentLen int, err error) {
	initInstruments()
	status := statusStr(err)
	inst.paneReadTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "pane.read", severity(err),
		otellog.String("session", session),
		otellog.Int64("lines_requested", int64(lines)),
		otellog.Int64("content_len", int64(contentLen)),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordPrime records a gt prime invocation (metrics + log event).
func RecordPrime(ctx context.Context, role string, hookMode bool, err error) {
	initInstruments()
	status := statusStr(err)
	inst.primeTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("role", role),
			attribute.Bool("hook_mode", hookMode),
		),
	)
	emit(ctx, "prime", severity(err),
		otellog.String("role", role),
		otellog.Bool("hook_mode", hookMode),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordPrimeContext logs the formula/context rendered by gt prime.
// This lets operators see exactly what context each agent started with,
// correlated to the Claude API calls that follow. Only emits when telemetry
// is active (no-op otherwise). The formula may be empty for compact/resume primes
// or when the fallback (non-template) path is used.
func RecordPrimeContext(ctx context.Context, formula, role string, hookMode bool) {
	if formula == "" {
		return
	}
	initInstruments()
	emit(ctx, "prime.context", otellog.SeverityInfo,
		otellog.String("role", role),
		otellog.Bool("hook_mode", hookMode),
		otellog.String("formula", formula),
	)
}

// RecordAgentStateChange records an agent state transition (metrics + log event).
func RecordAgentStateChange(ctx context.Context, agentID, newState string, hookBead *string, err error) {
	initInstruments()
	status := statusStr(err)
	hasHookBead := hookBead != nil && *hookBead != ""
	inst.agentStateTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("new_state", newState),
		),
	)
	emit(ctx, "agent.state_change", severity(err),
		otellog.String("agent_id", agentID),
		otellog.String("new_state", newState),
		otellog.Bool("has_hook_bead", hasHookBead),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordPolecatSpawn records a polecat spawn attempt (metrics + log event).
func RecordPolecatSpawn(ctx context.Context, name string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.polecatTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "polecat.spawn", severity(err),
		otellog.String("name", name),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordPolecatRemove records a polecat removal (metrics + log event).
func RecordPolecatRemove(ctx context.Context, name string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.polecatRemoveTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "polecat.remove", severity(err),
		otellog.String("name", name),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordSling records a sling work dispatch (metrics + log event).
func RecordSling(ctx context.Context, bead, target string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.slingTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "sling", severity(err),
		otellog.String("bead", bead),
		otellog.String("target", target),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordMail records a mail/bd SDK operation (metrics + log event).
func RecordMail(ctx context.Context, operation string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.mailTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("operation", operation),
		),
	)
	emit(ctx, "mail", severity(err),
		otellog.String("operation", operation),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordNudge records a gt nudge invocation (metrics + log event).
func RecordNudge(ctx context.Context, target string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.nudgeTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "nudge", severity(err),
		otellog.String("target", target),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordDone records a gt done invocation — polecat work completion (metrics + log event).
// exitType is one of COMPLETED, ESCALATED, DEFERRED.
func RecordDone(ctx context.Context, exitType string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.doneTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("exit_type", exitType),
		),
	)
	emit(ctx, "done", severity(err),
		otellog.String("exit_type", exitType),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordDaemonRestart records a daemon-initiated agent session restart (metrics + log event).
// agentType is e.g. "deacon", "witness-myrig", "refinery-myrig".
func RecordDaemonRestart(ctx context.Context, agentType string) {
	initInstruments()
	inst.daemonRestartTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("agent_type", agentType)),
	)
	emit(ctx, "daemon.restart", otellog.SeverityInfo,
		otellog.String("agent_type", agentType),
	)
}

// RecordFormulaInstantiate records a formula→wisp instantiation (metrics + log event).
func RecordFormulaInstantiate(ctx context.Context, formulaName, beadID string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.formulaTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", status),
			attribute.String("formula", formulaName),
		),
	)
	emit(ctx, "formula.instantiate", severity(err),
		otellog.String("formula_name", formulaName),
		otellog.String("bead_id", beadID),
		otellog.String("status", status),
		errKV(err),
	)
}

// RecordConvoyCreate records an auto-convoy creation (metrics + log event).
func RecordConvoyCreate(ctx context.Context, beadID string, err error) {
	initInstruments()
	status := statusStr(err)
	inst.convoyTotal.Add(ctx, 1,
		metric.WithAttributes(attribute.String("status", status)),
	)
	emit(ctx, "convoy.create", severity(err),
		otellog.String("bead_id", beadID),
		otellog.String("status", status),
		errKV(err),
	)
}

const maxPaneOutputLog = 8192

// RecordPaneOutput emits a chunk of raw pane output (ANSI already stripped) to VictoriaLogs.
// Opt-in: only called when GT_LOG_PANE_OUTPUT=true.
func RecordPaneOutput(ctx context.Context, sessionID, content string) {
	initInstruments()
	inst.paneOutputTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("session", sessionID),
	))
	emit(ctx, "pane.output", otellog.SeverityInfo,
		otellog.String("session", sessionID),
		otellog.String("content", truncateOutput(content, maxPaneOutputLog)),
	)
}
