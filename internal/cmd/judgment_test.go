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
		// Valid cases.
		{"", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"2h30m", 2*time.Hour + 30*time.Minute, false},
		{"90m", 90 * time.Minute, false},
		// Invalid cases.
		{"0d", 0, true},
		{"-1d", 0, true},
		{"-24h", 0, true},
		{"0h", 0, true},
		{"1.5d", 0, true},
		{"abcd", 0, true},
		{"d", 0, true},
		{"-0h", 0, true},
	}

	for _, tt := range tests {
		t.Run("input="+tt.input, func(t *testing.T) {
			got, err := parseWindow(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseWindow(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
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
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncate", "hello world", 5, "hell…"},
		{"max_1", "hello", 1, "…"},
		{"max_0", "hello", 0, ""},
		// UTF-8 safety.
		{"accented", "café résumé", 6, "café …"},
		{"cjk", "日本語テスト", 4, "日本語…"},
		{"emoji", "👋🌍🎉✨", 3, "👋🌍…"},
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
