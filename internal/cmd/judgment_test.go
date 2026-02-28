package cmd

import (
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Valid inputs
		{"", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"2h30m", 2*time.Hour + 30*time.Minute, false},
		{"90m", 90 * time.Minute, false},

		// Invalid inputs
		{"0d", 0, true},
		{"-1d", 0, true},
		{"-24h", 0, true},
		{"0h", 0, true},
		{"1.5d", 0, true},   // Sscanf %d won't parse float
		{"abcd", 0, true},   // not a valid duration
		{"d", 0, true},      // no number before d
		{"-0h", 0, true},    // zero duration
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseWindow(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseWindow(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseWindow(%q) error = %v, want nil", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseWindow(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateJudgmentStr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"short string unchanged", "hello", 10, "hello"},
		{"exact length unchanged", "hello", 5, "hello"},
		{"truncated with ellipsis", "hello world", 8, "hello w…"},
		{"single char max", "hello", 1, "…"},
		// Multi-byte rune safety
		{"utf8 no truncation", "héllo", 5, "héllo"},
		{"utf8 truncated cleanly", "héllo wörld", 8, "héllo w…"},
		{"cjk characters", "你好世界测试", 4, "你好世…"},
		{"emoji", "👋🌍🚀🎉", 3, "👋🌍…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateJudgmentStr(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncateJudgmentStr(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}
