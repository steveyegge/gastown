package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeTranscriptLines writes a JSONL transcript file from a slice of JSON objects.
func writeTranscriptLines(t *testing.T, dir string, lines []map[string]any) string {
	t.Helper()
	f, err := os.CreateTemp(dir, "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("creating temp transcript: %v", err)
	}
	defer f.Close()
	for _, line := range lines {
		b, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("marshaling line: %v", err)
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatalf("writing line: %v", err)
		}
	}
	return f.Name()
}

func TestReadLastTurnContext_SingleAssistantEntry(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscriptLines(t, dir, []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"model": "claude-haiku-4-5-20251001",
				"usage": map[string]any{
					"input_tokens":               100_000,
					"cache_read_input_tokens":    10_000,
					"cache_creation_input_tokens": 5_000,
				},
			},
		},
	})

	model, total, err := readLastTurnContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "claude-haiku-4-5-20251001" {
		t.Errorf("model = %q, want %q", model, "claude-haiku-4-5-20251001")
	}
	// 100k + 10k + 5k = 115k
	if total != 115_000 {
		t.Errorf("totalTokens = %d, want 115000", total)
	}
}

func TestReadLastTurnContext_MultipleEntries_UsesLast(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscriptLines(t, dir, []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"model": "claude-haiku-4-5-20251001",
				"usage": map[string]any{
					"input_tokens":               50_000,
					"cache_read_input_tokens":    0,
					"cache_creation_input_tokens": 0,
				},
			},
		},
		{
			"type": "user",
			"message": map[string]any{
				"content": "hello",
			},
		},
		{
			"type": "assistant",
			"message": map[string]any{
				"model": "claude-sonnet-4-6",
				"usage": map[string]any{
					"input_tokens":               130_000,
					"cache_read_input_tokens":    0,
					"cache_creation_input_tokens": 0,
				},
			},
		},
	})

	model, total, err := readLastTurnContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want %q", model, "claude-sonnet-4-6")
	}
	if total != 130_000 {
		t.Errorf("totalTokens = %d, want 130000", total)
	}
}

func TestReadLastTurnContext_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("creating empty file: %v", err)
	}

	model, total, err := readLastTurnContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "" {
		t.Errorf("model = %q, want empty", model)
	}
	if total != 0 {
		t.Errorf("totalTokens = %d, want 0", total)
	}
}

func TestReadLastTurnContext_OnlyUserMessages(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscriptLines(t, dir, []map[string]any{
		{"type": "user", "message": map[string]any{"content": "hi"}},
		{"type": "system", "content": "You are helpful."},
	})

	model, total, err := readLastTurnContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "" || total != 0 {
		t.Errorf("expected empty result for non-assistant transcript, got model=%q total=%d", model, total)
	}
}

func TestReadLastTurnContext_SkipsZeroTokenEntries(t *testing.T) {
	dir := t.TempDir()
	path := writeTranscriptLines(t, dir, []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"model": "claude-haiku-4-5-20251001",
				"usage": map[string]any{
					"input_tokens":               0,
					"cache_read_input_tokens":    0,
					"cache_creation_input_tokens": 0,
				},
			},
		},
		{
			"type": "assistant",
			"message": map[string]any{
				"model": "claude-sonnet-4-6",
				"usage": map[string]any{
					"input_tokens":               80_000,
					"cache_read_input_tokens":    0,
					"cache_creation_input_tokens": 0,
				},
			},
		},
	})

	model, total, err := readLastTurnContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want %q", model, "claude-sonnet-4-6")
	}
	if total != 80_000 {
		t.Errorf("totalTokens = %d, want 80000", total)
	}
}

func TestReadLastTurnContext_PreservesModelFromPriorEntry(t *testing.T) {
	// If a later entry has no model field, the last-seen non-empty model is kept.
	dir := t.TempDir()
	path := writeTranscriptLines(t, dir, []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"model": "claude-haiku-4-5-20251001",
				"usage": map[string]any{
					"input_tokens": 50_000,
				},
			},
		},
		{
			"type": "assistant",
			"message": map[string]any{
				// no "model" key
				"usage": map[string]any{
					"input_tokens": 90_000,
				},
			},
		},
	})

	model, total, err := readLastTurnContext(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Model from first entry carried forward since second has no model.
	if model != "claude-haiku-4-5-20251001" {
		t.Errorf("model = %q, want %q", model, "claude-haiku-4-5-20251001")
	}
	if total != 90_000 {
		t.Errorf("totalTokens = %d, want 90000", total)
	}
}

func TestReadLastTurnContext_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	defer f.Close()

	// Write one malformed line and one valid assistant line.
	lines := []string{
		"not valid json{{{{",
		`{"type":"assistant","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":60000}}}`,
	}
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatalf("writing line: %v", err)
		}
	}

	model, total, err := readLastTurnContext(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want %q", model, "claude-sonnet-4-6")
	}
	if total != 60_000 {
		t.Errorf("totalTokens = %d, want 60000", total)
	}
}

func TestContextWindowForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"claude-haiku-4-5-20251001", 200_000},
		{"claude-sonnet-4-6", 200_000},
		{"claude-opus-4-6", 200_000},
		{"claude-3-5-sonnet-20241022", 200_000},
		{"", 200_000},           // unknown model falls back to default
		{"gpt-4o", 200_000},     // non-Claude model also uses default
	}

	for _, tt := range tests {
		got := contextWindowForModel(tt.model)
		if got != tt.want {
			t.Errorf("contextWindowForModel(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestContextLevelThresholds(t *testing.T) {
	tests := []struct {
		name       string
		percentage float64
		wantLevel  string
	}{
		{"below warn", 64.9, "LOW"},
		{"at warn boundary", 65.0, "WARN"},
		{"between warn and critical", 72.0, "WARN"},
		{"just below critical", 79.9, "WARN"},
		{"at critical boundary", 80.0, "CRITICAL"},
		{"above critical", 95.0, "CRITICAL"},
		{"zero", 0.0, "LOW"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := "LOW"
			switch {
			case tt.percentage >= contextCriticalThresholdPct:
				level = "CRITICAL"
			case tt.percentage >= contextWarnThresholdPct:
				level = "WARN"
			}
			if level != tt.wantLevel {
				t.Errorf("percentage=%.1f → level=%q, want %q", tt.percentage, level, tt.wantLevel)
			}
		})
	}
}
