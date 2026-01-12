package ratelimit

import (
	"testing"
)

func TestDetector_ExitCode2(t *testing.T) {
	d := NewDetector()
	d.SetAgentInfo("test-agent", "anthropic_main", "")

	event, detected := d.Detect(ExitCodeRateLimit, "")
	if !detected {
		t.Fatal("expected rate limit to be detected for exit code 2")
	}
	if event == nil {
		t.Fatal("expected event to be non-nil")
	}
	if event.ExitCode != ExitCodeRateLimit {
		t.Errorf("expected exit code %d, got %d", ExitCodeRateLimit, event.ExitCode)
	}
	if event.AgentID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got %q", event.AgentID)
	}
	if event.Profile != "anthropic_main" {
		t.Errorf("expected profile 'anthropic_main', got %q", event.Profile)
	}
}

func TestDetector_StderrPatterns(t *testing.T) {
	tests := []struct {
		name     string
		stderr   string
		detected bool
	}{
		{"429 error", "Error 429: Too Many Requests", true},
		{"rate limit", "You have exceeded your rate limit", true},
		{"rate_limit", "rate_limit_exceeded", true},
		{"too many requests", "Error: too many requests, please slow down", true},
		{"overloaded", "Service is currently overloaded", true},
		{"capacity", "API capacity exceeded", true},
		{"throttled", "Request throttled, try again later", true},
		{"normal error", "Connection refused", false},
		{"empty stderr", "", false},
	}

	d := NewDetector()
	d.SetAgentInfo("test-agent", "test_profile", "")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, detected := d.Detect(0, tc.stderr)
			if detected != tc.detected {
				t.Errorf("Detect(0, %q) = %v, want %v", tc.stderr, detected, tc.detected)
			}
		})
	}
}

func TestDetector_NoRateLimit(t *testing.T) {
	d := NewDetector()
	d.SetAgentInfo("test-agent", "test_profile", "")

	tests := []struct {
		name     string
		exitCode int
		stderr   string
	}{
		{"exit 0", 0, ""},
		{"exit 1", 1, "generic error"},
		{"exit 127", 127, "command not found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event, detected := d.Detect(tc.exitCode, tc.stderr)
			if detected {
				t.Errorf("Detect(%d, %q) should not detect rate limit", tc.exitCode, tc.stderr)
			}
			if event != nil {
				t.Error("expected event to be nil when not detected")
			}
		})
	}
}

func TestDetector_ProviderDetection(t *testing.T) {
	d := NewDetector()
	d.SetAgentInfo("test-agent", "test_profile", "")

	tests := []struct {
		name     string
		stderr   string
		provider string
	}{
		{"anthropic explicit", "anthropic API rate limit exceeded", "anthropic"},
		{"openai explicit", "openai API rate limit exceeded", "openai"},
		{"claude mention", "Claude rate limit hit", "anthropic"},
		{"gpt mention", "GPT-4 rate limit", "openai"},
		{"no provider", "rate limit exceeded", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event, _ := d.Detect(ExitCodeRateLimit, tc.stderr)
			if event.Provider != tc.provider {
				t.Errorf("provider = %q, want %q", event.Provider, tc.provider)
			}
		})
	}
}

func TestExtractErrorSnippet(t *testing.T) {
	tests := []struct {
		name    string
		stderr  string
		wantLen int // Expect length <= this
	}{
		{"empty", "", 0},
		{"single line", "Error: rate limit", 17},
		{"multiline picks error", "Starting...\nError: rate limit exceeded\nDone", 50},
		{"truncates long line", string(make([]byte, 300)), 203}, // 200 + "..."
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snippet := extractErrorSnippet(tc.stderr)
			if len(snippet) > tc.wantLen && tc.wantLen > 0 {
				t.Errorf("snippet length %d > %d", len(snippet), tc.wantLen)
			}
		})
	}
}
