package ratelimit

import (
	"testing"
)

func TestDetectRateLimit(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		wantDetected   bool
		wantProvider   string
		wantStatusCode int
		wantGlobal     bool
	}{
		{
			name:         "no rate limit",
			output:       "Everything is working fine",
			wantDetected: false,
		},
		{
			name:           "429 status code",
			output:         "Error: 429 Too Many Requests",
			wantDetected:   true,
			wantStatusCode: 429,
		},
		{
			name:         "rate limited message",
			output:       "API error: rate limited, please try again later",
			wantDetected: true,
		},
		{
			name:         "too many requests",
			output:       "Error: Too many requests. Please slow down.",
			wantDetected: true,
		},
		{
			name:         "quota exceeded",
			output:       "Error: Quota exceeded for the day",
			wantDetected: true,
		},
		{
			name:         "anthropic rate limit",
			output:       "Error from Anthropic: rate_limit_error - Too many requests",
			wantDetected: true,
			wantProvider: "anthropic",
		},
		{
			name:         "anthropic overloaded",
			output:       "The Anthropic API is currently overloaded. All servers are at capacity.",
			wantDetected: true,
			wantProvider: "anthropic",
			wantGlobal:   true,
		},
		{
			name:         "claude overloaded",
			output:       "Claude is currently experiencing high demand. Service overloaded.",
			wantDetected: true,
			wantProvider: "anthropic",
			wantGlobal:   true,
		},
		{
			name:         "openai rate limit",
			output:       "OpenAI error: Rate limit reached for tokens per minute",
			wantDetected: true,
			wantProvider: "openai",
		},
		{
			name:         "throttled",
			output:       "Request throttled. Please retry.",
			wantDetected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectRateLimit(tt.output)

			if result.Detected != tt.wantDetected {
				t.Errorf("Detected = %v, want %v", result.Detected, tt.wantDetected)
			}

			if tt.wantProvider != "" && result.Provider != tt.wantProvider {
				t.Errorf("Provider = %v, want %v", result.Provider, tt.wantProvider)
			}

			if tt.wantStatusCode != 0 && result.StatusCode != tt.wantStatusCode {
				t.Errorf("StatusCode = %v, want %v", result.StatusCode, tt.wantStatusCode)
			}

			if result.IsGlobal != tt.wantGlobal {
				t.Errorf("IsGlobal = %v, want %v", result.IsGlobal, tt.wantGlobal)
			}
		})
	}
}

func TestDetectStuck(t *testing.T) {
	tests := []struct {
		name                string
		output              string
		lastActivityMinutes int
		wantDetected        bool
	}{
		{
			name:                "active agent",
			output:              "Processing request...",
			lastActivityMinutes: 5,
			wantDetected:        false,
		},
		{
			name:                "explicit stuck",
			output:              "Agent appears stuck, no response received",
			lastActivityMinutes: 5,
			wantDetected:        true,
		},
		{
			name:                "timeout detected",
			output:              "Request timeout occurred",
			lastActivityMinutes: 5,
			wantDetected:        true,
		},
		{
			name:                "long inactivity with rate limit",
			output:              "Previous error: 429 Too Many Requests",
			lastActivityMinutes: 20,
			wantDetected:        true,
		},
		{
			name:                "long inactivity no rate limit",
			output:              "Everything looks normal",
			lastActivityMinutes: 20,
			wantDetected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectStuck(tt.output, tt.lastActivityMinutes)

			if result.Detected != tt.wantDetected {
				t.Errorf("Detected = %v, want %v", result.Detected, tt.wantDetected)
			}
		})
	}
}

func TestAnalyzeExitCode(t *testing.T) {
	tests := []struct {
		name         string
		exitCode     int
		wantDetected bool
	}{
		{
			name:         "success",
			exitCode:     0,
			wantDetected: false,
		},
		{
			name:         "generic error",
			exitCode:     1,
			wantDetected: false,
		},
		{
			name:         "temporary failure",
			exitCode:     75,
			wantDetected: true,
		},
		{
			name:         "timeout",
			exitCode:     124,
			wantDetected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnalyzeExitCode(tt.exitCode)

			if result.Detected != tt.wantDetected {
				t.Errorf("Detected = %v, want %v", result.Detected, tt.wantDetected)
			}
		})
	}
}

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		maxLen int
		want   string
	}{
		{
			name:   "short message",
			msg:    "Hello",
			maxLen: 10,
			want:   "Hello",
		},
		{
			name:   "exact length",
			msg:    "Hello",
			maxLen: 5,
			want:   "Hello",
		},
		{
			name:   "truncated",
			msg:    "Hello World!",
			maxLen: 10,
			want:   "Hello W...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateMessage(tt.msg, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}
