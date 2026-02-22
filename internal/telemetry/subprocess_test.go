package telemetry

import (
	"os"
	"strings"
	"testing"
)

func TestBuildGTResourceAttrs_Empty(t *testing.T) {
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	result := buildGTResourceAttrs()
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildGTResourceAttrs_AllVars(t *testing.T) {
	t.Setenv("GT_ROLE", "mol/witness")
	t.Setenv("GT_RIG", "mol")
	t.Setenv("BD_ACTOR", "mol/witness")
	t.Setenv("GT_POLECAT", "furiosa")
	t.Setenv("GT_CREW", "")

	result := buildGTResourceAttrs()
	for _, want := range []string{"gt.role=mol/witness", "gt.rig=mol", "gt.actor=mol/witness", "gt.agent=furiosa"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in result, got %q", want, result)
		}
	}
}

func TestBuildGTResourceAttrs_PolecatTakesPriorityOverCrew(t *testing.T) {
	t.Setenv("GT_POLECAT", "furiosa")
	t.Setenv("GT_CREW", "mayor")
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("BD_ACTOR", "")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, "gt.agent=furiosa") {
		t.Errorf("expected gt.agent=furiosa (GT_POLECAT), got %q", result)
	}
	if strings.Contains(result, "gt.agent=mayor") {
		t.Errorf("GT_CREW should not override GT_POLECAT, got %q", result)
	}
}

func TestBuildGTResourceAttrs_CrewFallback(t *testing.T) {
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "mayor")
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("BD_ACTOR", "")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, "gt.agent=mayor") {
		t.Errorf("expected gt.agent=mayor from GT_CREW, got %q", result)
	}
}

func TestBuildGTResourceAttrs_Comma(t *testing.T) {
	t.Setenv("GT_ROLE", "a")
	t.Setenv("GT_RIG", "b")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	result := buildGTResourceAttrs()
	if !strings.Contains(result, ",") {
		t.Errorf("expected comma-separated result, got %q", result)
	}
}

func TestOTELEnvForSubprocess_Disabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "")
	env := OTELEnvForSubprocess()
	if env != nil {
		t.Errorf("expected nil when telemetry disabled, got %v", env)
	}
}

func TestOTELEnvForSubprocess_BothURLs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "http://localhost:9428/insert/opentelemetry/v1/logs")
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	env := OTELEnvForSubprocess()
	if len(env) == 0 {
		t.Fatal("expected non-empty env")
	}

	hasMetrics, hasLogs := false, false
	for _, e := range env {
		if strings.HasPrefix(e, "BD_OTEL_METRICS_URL=") {
			hasMetrics = true
		}
		if strings.HasPrefix(e, "BD_OTEL_LOGS_URL=") {
			hasLogs = true
		}
	}
	if !hasMetrics {
		t.Error("expected BD_OTEL_METRICS_URL in subprocess env")
	}
	if !hasLogs {
		t.Error("expected BD_OTEL_LOGS_URL in subprocess env")
	}
}

func TestOTELEnvForSubprocess_NoLogsURL(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	env := OTELEnvForSubprocess()
	for _, e := range env {
		if strings.HasPrefix(e, "BD_OTEL_LOGS_URL=") {
			t.Errorf("BD_OTEL_LOGS_URL should not appear when GT_OTEL_LOGS_URL is empty, got %q", e)
		}
	}
}

func TestOTELEnvForSubprocess_WithResourceAttrs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("GT_ROLE", "mol/witness")
	t.Setenv("GT_RIG", "mol")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	env := OTELEnvForSubprocess()
	hasAttrs := false
	for _, e := range env {
		if strings.HasPrefix(e, "OTEL_RESOURCE_ATTRIBUTES=") {
			hasAttrs = true
			if !strings.Contains(e, "gt.role=mol/witness") {
				t.Errorf("expected gt.role in OTEL_RESOURCE_ATTRIBUTES, got %q", e)
			}
		}
	}
	if !hasAttrs {
		t.Error("expected OTEL_RESOURCE_ATTRIBUTES in subprocess env when GT vars present")
	}
}

func TestSetProcessOTELAttrs_Disabled(t *testing.T) {
	t.Setenv(EnvMetricsURL, "")
	os.Unsetenv("BD_OTEL_METRICS_URL")
	os.Unsetenv("BD_OTEL_LOGS_URL")

	SetProcessOTELAttrs()

	if v := os.Getenv("BD_OTEL_METRICS_URL"); v != "" {
		t.Errorf("BD_OTEL_METRICS_URL should not be set when telemetry disabled, got %q", v)
	}
}

func TestSetProcessOTELAttrs_Enabled(t *testing.T) {
	metricsURL := "http://localhost:8428/opentelemetry/api/v1/push"
	logsURL := "http://localhost:9428/insert/opentelemetry/v1/logs"
	t.Setenv(EnvMetricsURL, metricsURL)
	t.Setenv(EnvLogsURL, logsURL)
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	SetProcessOTELAttrs()

	if got := os.Getenv("BD_OTEL_METRICS_URL"); got != metricsURL {
		t.Errorf("BD_OTEL_METRICS_URL = %q, want %q", got, metricsURL)
	}
	if got := os.Getenv("BD_OTEL_LOGS_URL"); got != logsURL {
		t.Errorf("BD_OTEL_LOGS_URL = %q, want %q", got, logsURL)
	}
}

func TestSetProcessOTELAttrs_SetsResourceAttrs(t *testing.T) {
	t.Setenv(EnvMetricsURL, "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv(EnvLogsURL, "")
	t.Setenv("GT_ROLE", "mol/witness")
	t.Setenv("GT_RIG", "mol")
	t.Setenv("BD_ACTOR", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")
	os.Unsetenv("OTEL_RESOURCE_ATTRIBUTES")

	SetProcessOTELAttrs()

	got := os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if got == "" {
		t.Error("expected OTEL_RESOURCE_ATTRIBUTES to be set")
	}
	if !strings.Contains(got, "gt.role=mol/witness") {
		t.Errorf("expected gt.role in OTEL_RESOURCE_ATTRIBUTES, got %q", got)
	}
}
