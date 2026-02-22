package telemetry

import (
	"os"
	"strings"
)

// buildGTResourceAttrs builds the OTEL_RESOURCE_ATTRIBUTES value from GT context
// vars present in the current process environment.
// Returns "" when no GT vars are found.
func buildGTResourceAttrs() string {
	var attrs []string
	if v := os.Getenv("GT_ROLE"); v != "" {
		attrs = append(attrs, "gt.role="+v)
	}
	if v := os.Getenv("GT_RIG"); v != "" {
		attrs = append(attrs, "gt.rig="+v)
	}
	if v := os.Getenv("BD_ACTOR"); v != "" {
		attrs = append(attrs, "gt.actor="+v)
	}
	// Polecat and crew carry their agent name in different vars.
	if v := os.Getenv("GT_POLECAT"); v != "" {
		attrs = append(attrs, "gt.agent="+v)
	} else if v := os.Getenv("GT_CREW"); v != "" {
		attrs = append(attrs, "gt.agent="+v)
	}
	return strings.Join(attrs, ",")
}

// SetProcessOTELAttrs sets OTEL-related variables in the current process
// environment so that all bd subprocesses spawned via exec.Command inherit
// them automatically — no per-call injection needed.
//
// Sets:
//   - OTEL_RESOURCE_ATTRIBUTES — GT context labels (gt.role, gt.rig, …)
//   - BD_OTEL_METRICS_URL      — bd's own metrics var (mirrors GT_OTEL_METRICS_URL)
//   - BD_OTEL_LOGS_URL         — bd's own logs var   (mirrors GT_OTEL_LOGS_URL)
//
// Called once at gt startup (Execute) when telemetry is active.
// No-op when GT_OTEL_METRICS_URL is not set.
func SetProcessOTELAttrs() {
	metricsURL := os.Getenv(EnvMetricsURL)
	if metricsURL == "" {
		return
	}
	if attrs := buildGTResourceAttrs(); attrs != "" {
		_ = os.Setenv("OTEL_RESOURCE_ATTRIBUTES", attrs)
	}
	// Mirror GT vars into bd's own var names so bd subprocesses
	// emit their metrics to the same VictoriaMetrics instance.
	_ = os.Setenv("BD_OTEL_METRICS_URL", metricsURL)
	if logsURL := os.Getenv(EnvLogsURL); logsURL != "" {
		_ = os.Setenv("BD_OTEL_LOGS_URL", logsURL)
	}
}

// OTELEnvForSubprocess returns OTEL environment variables to inject into bd
// subprocesses when cmd.Env is built explicitly (overriding os.Environ).
//
// Complements SetProcessOTELAttrs for callers that construct cmd.Env manually
// (beads.go run, mail/bd.go runBdCommand) so the vars aren't lost when the
// explicit env slice is built from scratch instead of os.Environ().
//
// Returns nil when GT telemetry is not active (GT_OTEL_METRICS_URL not set).
func OTELEnvForSubprocess() []string {
	metricsURL := os.Getenv(EnvMetricsURL)
	if metricsURL == "" {
		return nil
	}
	var env []string
	if attrs := buildGTResourceAttrs(); attrs != "" {
		env = append(env, "OTEL_RESOURCE_ATTRIBUTES="+attrs)
	}
	env = append(env, "BD_OTEL_METRICS_URL="+metricsURL)
	if logsURL := os.Getenv(EnvLogsURL); logsURL != "" {
		env = append(env, "BD_OTEL_LOGS_URL="+logsURL)
	}
	return env
}
