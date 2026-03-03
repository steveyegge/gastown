package cmd

import "testing"

func TestResolveFormulaLegAgent_Precedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		legAgent     string
		cliAgent     string
		formulaAgent string
		want         string
	}{
		{"all empty", "", "", "", ""},
		{"formula only", "", "", "gemini", "gemini"},
		{"cli only", "", "codex", "", "codex"},
		{"leg only", "claude-haiku", "", "", "claude-haiku"},
		{"cli overrides formula", "", "codex", "gemini", "codex"},
		{"leg overrides cli", "claude-haiku", "codex", "gemini", "claude-haiku"},
		{"leg overrides formula", "claude-haiku", "", "gemini", "claude-haiku"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resolveFormulaLegAgent(tt.legAgent, tt.cliAgent, tt.formulaAgent)
			if got != tt.want {
				t.Errorf("resolveFormulaLegAgent(%q, %q, %q) = %q, want %q",
					tt.legAgent, tt.cliAgent, tt.formulaAgent, got, tt.want)
			}
		})
	}
}
