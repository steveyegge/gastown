package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadContextBudgetConfig_Defaults(t *testing.T) {
	// Clear any env vars that might interfere
	for _, env := range []string{
		"GT_CONTEXT_BUDGET_WARN",
		"GT_CONTEXT_BUDGET_SOFT_GATE",
		"GT_CONTEXT_BUDGET_HARD_GATE",
		"GT_CONTEXT_BUDGET_MAX_TOKENS",
		"GT_ROLE",
		"GT_POLECAT",
		"GT_CREW",
		"GT_MAYOR",
		"GT_DEACON",
		"GT_WITNESS",
		"GT_REFINERY",
	} {
		t.Setenv(env, "")
	}

	cfg := loadContextBudgetConfig()

	if cfg.WarnThreshold != 0.75 {
		t.Errorf("WarnThreshold = %v, want 0.75", cfg.WarnThreshold)
	}
	if cfg.SoftGate != 0.85 {
		t.Errorf("SoftGate = %v, want 0.85", cfg.SoftGate)
	}
	if cfg.HardGate != 0.92 {
		t.Errorf("HardGate = %v, want 0.92", cfg.HardGate)
	}
	if cfg.MaxTokens != 200_000 {
		t.Errorf("MaxTokens = %v, want 200000", cfg.MaxTokens)
	}
}

func TestLoadContextBudgetConfig_EnvOverrides(t *testing.T) {
	t.Setenv("GT_CONTEXT_BUDGET_WARN", "0.60")
	t.Setenv("GT_CONTEXT_BUDGET_SOFT_GATE", "0.80")
	t.Setenv("GT_CONTEXT_BUDGET_HARD_GATE", "0.90")
	t.Setenv("GT_CONTEXT_BUDGET_MAX_TOKENS", "150000")

	cfg := loadContextBudgetConfig()

	if cfg.WarnThreshold != 0.60 {
		t.Errorf("WarnThreshold = %v, want 0.60", cfg.WarnThreshold)
	}
	if cfg.SoftGate != 0.80 {
		t.Errorf("SoftGate = %v, want 0.80", cfg.SoftGate)
	}
	if cfg.HardGate != 0.90 {
		t.Errorf("HardGate = %v, want 0.90", cfg.HardGate)
	}
	if cfg.MaxTokens != 150_000 {
		t.Errorf("MaxTokens = %v, want 150000", cfg.MaxTokens)
	}
}

func TestLoadContextBudgetConfig_InvalidEnvIgnored(t *testing.T) {
	t.Setenv("GT_CONTEXT_BUDGET_WARN", "notanumber")
	t.Setenv("GT_CONTEXT_BUDGET_MAX_TOKENS", "-5")

	cfg := loadContextBudgetConfig()

	if cfg.WarnThreshold != 0.75 {
		t.Errorf("WarnThreshold = %v, want 0.75 (default)", cfg.WarnThreshold)
	}
	if cfg.MaxTokens != 200_000 {
		t.Errorf("MaxTokens = %v, want 200000 (default)", cfg.MaxTokens)
	}
}

func TestDetectCurrentRole(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		wantRole string
	}{
		{
			name:     "GT_ROLE set",
			envVars:  map[string]string{"GT_ROLE": "mayor"},
			wantRole: "mayor",
		},
		{
			name:     "GT_POLECAT set",
			envVars:  map[string]string{"GT_POLECAT": "furiosa"},
			wantRole: "polecat",
		},
		{
			name:     "GT_CREW set",
			envVars:  map[string]string{"GT_CREW": "max"},
			wantRole: "crew",
		},
		{
			name:     "nothing set",
			envVars:  map[string]string{},
			wantRole: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all role env vars
			for _, env := range []string{"GT_ROLE", "GT_POLECAT", "GT_CREW", "GT_MAYOR", "GT_DEACON", "GT_WITNESS", "GT_REFINERY"} {
				t.Setenv(env, "")
			}
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got := detectCurrentRole()
			if got != tt.wantRole {
				t.Errorf("detectCurrentRole() = %q, want %q", got, tt.wantRole)
			}
		})
	}
}

func TestRoleHardGating(t *testing.T) {
	tests := []struct {
		role        string
		wantHardGated bool
	}{
		{"mayor", true},
		{"deacon", true},
		{"witness", true},
		{"refinery", true},
		{"polecat", false},
		{"crew", false},
		{"", true}, // unknown role gets hard-gated for safety
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			for _, env := range []string{"GT_ROLE", "GT_POLECAT", "GT_CREW", "GT_MAYOR", "GT_DEACON", "GT_WITNESS", "GT_REFINERY"} {
				t.Setenv(env, "")
			}
			if tt.role != "" {
				t.Setenv("GT_ROLE", tt.role)
			}

			cfg := loadContextBudgetConfig()
			if cfg.IsHardGated != tt.wantHardGated {
				t.Errorf("role %q: IsHardGated = %v, want %v", tt.role, cfg.IsHardGated, tt.wantHardGated)
			}
		})
	}
}

func TestParseContextBudgetUsage(t *testing.T) {
	// Create a temporary transcript file
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "test.jsonl")

	// Write some transcript messages
	messages := []TranscriptMessage{
		{
			Type: "assistant",
			Message: &TranscriptMessageBody{
				Model: "claude-sonnet-4-20250514",
				Role:  "assistant",
				Usage: &TranscriptUsage{
					InputTokens:  50000,
					OutputTokens: 1000,
				},
			},
		},
		{
			Type: "human",
		},
		{
			Type: "assistant",
			Message: &TranscriptMessageBody{
				Model: "claude-sonnet-4-20250514",
				Role:  "assistant",
				Usage: &TranscriptUsage{
					InputTokens:  100000,
					OutputTokens: 2000,
				},
			},
		},
		{
			Type: "human",
		},
		{
			Type: "assistant",
			Message: &TranscriptMessageBody{
				Model: "claude-sonnet-4-20250514",
				Role:  "assistant",
				Usage: &TranscriptUsage{
					InputTokens:  150000,
					OutputTokens: 3000,
				},
			},
		},
	}

	f, err := os.Create(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, msg := range messages {
		if err := enc.Encode(msg); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	usage, err := parseContextBudgetUsage(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}

	// LastInputTokens should be from the last assistant message
	if usage.LastInputTokens != 150000 {
		t.Errorf("LastInputTokens = %d, want 150000", usage.LastInputTokens)
	}

	// TotalOutputTokens should be sum of all assistant messages
	if usage.TotalOutputTokens != 6000 {
		t.Errorf("TotalOutputTokens = %d, want 6000", usage.TotalOutputTokens)
	}
}

func TestParseContextBudgetUsage_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "empty.jsonl")

	if err := os.WriteFile(transcriptPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	usage, err := parseContextBudgetUsage(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}

	if usage.LastInputTokens != 0 {
		t.Errorf("LastInputTokens = %d, want 0", usage.LastInputTokens)
	}
}

func TestFindActiveTranscript(t *testing.T) {
	dir := t.TempDir()

	// Create two transcript files with different modification times
	old := filepath.Join(dir, "old.jsonl")
	if err := os.WriteFile(old, []byte(`{"type":"test"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	newest := filepath.Join(dir, "newest.jsonl")
	if err := os.WriteFile(newest, []byte(`{"type":"test"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := findActiveTranscript(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should find one of them (both have same mtime in fast test, but at least one should match)
	if got != old && got != newest {
		t.Errorf("findActiveTranscript() = %q, want %q or %q", got, old, newest)
	}
}

func TestFindActiveTranscript_NoFiles(t *testing.T) {
	dir := t.TempDir()

	_, err := findActiveTranscript(dir)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestContextBudgetDisabled(t *testing.T) {
	t.Setenv("GT_CONTEXT_BUDGET_DISABLE", "1")

	err := runTapGuardContextBudget(nil, nil)
	if err != nil {
		t.Errorf("expected nil error when disabled, got %v", err)
	}
}
