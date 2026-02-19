package observe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
)

// --- Source integration tests ---

func TestSource_TailsNewLines(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		SourceKind:      "log",
		RedactionPolicy: "none",
	}
	src, err := NewSource("test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	// Write some lines after source creation.
	f, err := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("INFO first line\n")
	_, _ = f.WriteString("INFO second line\n")
	f.Close()

	// Wait for events (with timeout).
	var got []string
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-src.Events():
			got = append(got, ev.Message)
		case <-deadline:
			t.Fatalf("timed out waiting for events, got %d", len(got))
		}
	}

	if got[0] != "INFO first line" || got[1] != "INFO second line" {
		t.Errorf("unexpected messages: %v", got)
	}
}

func TestSource_AppliesRedaction(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		SourceKind:      "log",
		RedactionPolicy: "standard",
	}
	src, err := NewSource("redact-test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	f, _ := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("INFO user@example.com from 10.0.0.1\n")
	f.Close()

	select {
	case ev := <-src.Events():
		if ev.Message != "INFO [REDACTED_EMAIL] from [REDACTED_IP]" {
			t.Errorf("unexpected redacted message: %s", ev.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSource_RawFieldIsRedacted(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		SourceKind:      "log",
		RedactionPolicy: "standard",
	}
	src, err := NewSource("raw-test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	f, _ := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("INFO secret@example.com logged in\n")
	f.Close()

	select {
	case ev := <-src.Events():
		// Raw should also be redacted to prevent PII leaks.
		if strings.Contains(ev.Raw, "secret@example.com") {
			t.Errorf("Raw field contains un-redacted PII: %s", ev.Raw)
		}
		if ev.Raw != ev.Message {
			t.Errorf("Raw should equal Message for observe sources\nRaw:     %s\nMessage: %s", ev.Raw, ev.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestSource_FiltersBySeverity(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		SourceKind:      "log",
		RedactionPolicy: "none",
		RoutingRules: &config.ObservabilityRoutingRules{
			SeverityThreshold: "warn",
		},
	}
	src, err := NewSource("sev-test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	f, _ := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("INFO this should be filtered\n")
	_, _ = f.WriteString("WARN this should pass\n")
	_, _ = f.WriteString("ERROR this should also pass\n")
	f.Close()

	var got []string
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-src.Events():
			got = append(got, ev.Message)
		case <-deadline:
			t.Fatalf("timed out; got %v", got)
		}
	}

	if got[0] != "WARN this should pass" {
		t.Errorf("first event should be WARN line, got: %s", got[0])
	}
	if got[1] != "ERROR this should also pass" {
		t.Errorf("second event should be ERROR line, got: %s", got[1])
	}
}

func TestSource_UnknownSeverityPassesThroughFilter(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		SourceKind:      "log",
		RedactionPolicy: "none",
		RoutingRules: &config.ObservabilityRoutingRules{
			SeverityThreshold: "error",
		},
	}
	src, err := NewSource("unknown-sev", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	f, _ := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	// A line with no severity keyword should pass through even with error threshold.
	_, _ = f.WriteString("just a plain log line\n")
	f.Close()

	select {
	case ev := <-src.Events():
		if ev.Message != "just a plain log line" {
			t.Errorf("unexpected message: %s", ev.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out — plain line was incorrectly filtered")
	}
}

func TestSource_CloseStopsGoroutine(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		RedactionPolicy: "none",
	}
	src, err := NewSource("close-test", cfg)
	if err != nil {
		t.Fatal(err)
	}

	if err := src.Close(); err != nil {
		t.Fatalf("close returned error: %v", err)
	}

	// After Close + wg.Wait, the events channel must be closed.
	select {
	case _, ok := <-src.Events():
		if ok {
			t.Error("expected events channel to be closed after Close()")
		}
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for events channel to close")
	}
}

func TestSource_DoubleCloseIsSafe(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		RedactionPolicy: "none",
	}
	src, err := NewSource("double-close", cfg)
	if err != nil {
		t.Fatal(err)
	}

	// First close should succeed.
	if err := src.Close(); err != nil {
		t.Fatalf("first close returned error: %v", err)
	}

	// Second close should not panic or return an error.
	if err := src.Close(); err != nil {
		t.Fatalf("second close returned error: %v", err)
	}
}

func TestSource_MissingFile(t *testing.T) {
	cfg := &config.ObservabilitySourceConfig{
		Path:            "/nonexistent/path/to/file.log",
		RedactionPolicy: "none",
	}
	_, err := NewSource("missing", cfg)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSource_NilConfig(t *testing.T) {
	_, err := NewSource("nil-cfg", nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestSource_EmptyPath(t *testing.T) {
	cfg := &config.ObservabilitySourceConfig{
		RedactionPolicy: "none",
	}
	_, err := NewSource("empty-path", cfg)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestSource_EventType(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		SourceKind:      "log",
		ServiceID:       "svc-a",
		RedactionPolicy: "none",
	}
	src, err := NewSource("type-test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	f, _ := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("INFO hello world\n")
	f.Close()

	select {
	case ev := <-src.Events():
		if ev.Type != events.TypeObserveLog {
			t.Errorf("expected type %s, got %s", events.TypeObserveLog, ev.Type)
		}
		if ev.Actor != "svc-a/type-test" {
			t.Errorf("expected actor svc-a/type-test, got %s", ev.Actor)
		}
		if ev.Role != "observe" {
			t.Errorf("expected role observe, got %s", ev.Role)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestSource_NoServiceID_ActorFallback(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path:            tmp,
		RedactionPolicy: "none",
		// No ServiceID set
	}
	src, err := NewSource("fallback-test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	f, _ := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("some log line\n")
	f.Close()

	select {
	case ev := <-src.Events():
		if ev.Actor != "observe/fallback-test" {
			t.Errorf("expected actor observe/fallback-test, got %s", ev.Actor)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

// --- inferSeverity unit tests ---

func TestInferSeverity(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"error_upper", "2024-01-01 ERROR something broke", "error"},
		{"error_mixed", "Error: something broke", "error"},
		{"err_space", "ERR something broke", "error"},
		{"warn_upper", "2024-01-01 WARN something odd", "warn"},
		{"warning", "WARNING: disk space low", "warn"},
		{"debug_upper", "DEBUG checking cache", "debug"},
		{"info_upper", "INFO server started", "info"},
		{"info_mixed", "Info: all good", "info"},
		{"no_severity", "just a plain line", ""},
		{"empty_string", "", ""},
		{"numbers_only", "12345", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferSeverity(tt.line)
			if got != tt.want {
				t.Errorf("inferSeverity(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

// --- passesSeverityFilter unit tests ---

func TestPassesSeverityFilter(t *testing.T) {
	tests := []struct {
		name      string
		threshold string // "" means nil RoutingRules
		severity  string
		want      bool
	}{
		{"nil_rules_any_severity", "", "error", true},
		{"empty_threshold", "empty", "warn", true}, // special: we create rules with empty threshold
		{"unknown_severity_passes", "error", "", true},
		{"unrecognized_severity_passes", "warn", "fatal", true},
		{"invalid_threshold_passes", "critical", "info", true},
		{"info_meets_info", "info", "info", true},
		{"warn_meets_info", "info", "warn", true},
		{"debug_below_info", "info", "debug", false},
		{"info_below_warn", "warn", "info", false},
		{"error_meets_error", "error", "error", true},
		{"warn_below_error", "error", "warn", false},
		{"case_insensitive", "WARN", "warn", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Source{cfg: &config.ObservabilitySourceConfig{}}
			if tt.threshold != "" {
				thresh := tt.threshold
				if thresh == "empty" {
					thresh = ""
				}
				s.cfg.RoutingRules = &config.ObservabilityRoutingRules{
					SeverityThreshold: thresh,
				}
			}
			got := s.passesSeverityFilter(tt.severity)
			if got != tt.want {
				t.Errorf("passesSeverityFilter(%q) with threshold %q = %v, want %v",
					tt.severity, tt.threshold, got, tt.want)
			}
		})
	}
}

// --- DefaultRedactionPolicy test ---

func TestSource_DefaultRedactionPolicyIsStandard(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ObservabilitySourceConfig{
		Path: tmp,
		// No RedactionPolicy set — should default to "standard"
	}
	src, err := NewSource("default-redact", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	f, _ := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("INFO email test@example.com found\n")
	f.Close()

	select {
	case ev := <-src.Events():
		if strings.Contains(ev.Message, "test@example.com") {
			t.Errorf("default redaction should have redacted email, got: %s", ev.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}
