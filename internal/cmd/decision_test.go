package cmd

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestFormatOptionsSummary tests the options summary formatter.
func TestFormatOptionsSummary(t *testing.T) {
	tests := []struct {
		name    string
		options []beads.DecisionOption
		want    string
	}{
		{
			name:    "empty",
			options: nil,
			want:    "",
		},
		{
			name: "single option",
			options: []beads.DecisionOption{
				{Label: "JWT"},
			},
			want: "JWT",
		},
		{
			name: "two options",
			options: []beads.DecisionOption{
				{Label: "JWT"},
				{Label: "Session"},
			},
			want: "JWT, Session",
		},
		{
			name: "with recommended",
			options: []beads.DecisionOption{
				{Label: "JWT", Recommended: true},
				{Label: "Session"},
			},
			want: "JWT*, Session",
		},
		{
			name: "multiple recommended",
			options: []beads.DecisionOption{
				{Label: "A", Recommended: true},
				{Label: "B"},
				{Label: "C", Recommended: true},
			},
			want: "A*, B, C*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatOptionsSummary(tt.options)
			if got != tt.want {
				t.Errorf("formatOptionsSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUrgencyEmoji tests urgency emoji mapping.
func TestUrgencyEmoji(t *testing.T) {
	tests := []struct {
		urgency string
		want    string
	}{
		{beads.UrgencyHigh, "🔴"},
		{beads.UrgencyMedium, "🟡"},
		{beads.UrgencyLow, "🟢"},
		{"", "📋"},
		{"invalid", "📋"},
	}

	for _, tt := range tests {
		t.Run(tt.urgency, func(t *testing.T) {
			got := urgencyEmoji(tt.urgency)
			if got != tt.want {
				t.Errorf("urgencyEmoji(%q) = %q, want %q", tt.urgency, got, tt.want)
			}
		})
	}
}

// TestTruncateString tests string truncation.
func TestTruncateStringDecision(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is longer than max", 10, "this is..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"}, // Edge case: when maxLen is very small
		{"abcdefg", 6, "abc..."}, // 7 chars > 6, so truncate
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// TestFormatDecisionMailBody tests mail body formatting.
func TestFormatDecisionMailBody(t *testing.T) {
	fields := &beads.DecisionFields{
		Question: "Which approach?",
		Context:  "Building a new feature",
		Options: []beads.DecisionOption{
			{Label: "Fast", Description: "Quick but risky", Recommended: true},
			{Label: "Safe", Description: "Slower but safer"},
		},
		Urgency:     beads.UrgencyHigh,
		RequestedBy: "test-agent",
		Blockers:    []string{"gt-work-123"},
	}

	body := formatDecisionMailBody("hq-dec-123", fields)

	// Verify key content
	if !strings.Contains(body, "Decision ID: hq-dec-123") {
		t.Error("missing decision ID")
	}
	if !strings.Contains(body, "Urgency: high") {
		t.Error("missing urgency")
	}
	if !strings.Contains(body, "Which approach?") {
		t.Error("missing question")
	}
	if !strings.Contains(body, "Building a new feature") {
		t.Error("missing context")
	}
	if !strings.Contains(body, "1. Fast (Recommended)") {
		t.Error("missing option 1 with recommended marker")
	}
	if !strings.Contains(body, "2. Safe") {
		t.Error("missing option 2")
	}
	if !strings.Contains(body, "Blocking: gt-work-123") {
		t.Error("missing blockers")
	}
	if !strings.Contains(body, "gt decision resolve") {
		t.Error("missing resolve command hint")
	}
}

// TestFormatDecisionMailBodyNoContext tests mail without context.
func TestFormatDecisionMailBodyNoContext(t *testing.T) {
	fields := &beads.DecisionFields{
		Question: "Yes or no?",
		Options: []beads.DecisionOption{
			{Label: "Yes"},
			{Label: "No"},
		},
		Urgency:     beads.UrgencyLow,
		RequestedBy: "test",
	}

	body := formatDecisionMailBody("hq-dec-456", fields)

	if strings.Contains(body, "Context:") {
		t.Error("should not have Context section when context is empty")
	}
}

// TestFormatResolutionMailBody tests resolution notification body.
func TestFormatResolutionMailBody(t *testing.T) {
	body := formatResolutionMailBody(
		"hq-dec-123",
		"Which approach?",
		"Fast",
		"Speed is critical",
		"human",
	)

	if !strings.Contains(body, "Decision ID: hq-dec-123") {
		t.Error("missing decision ID")
	}
	if !strings.Contains(body, "Chosen: Fast") {
		t.Error("missing chosen option")
	}
	if !strings.Contains(body, "Rationale: Speed is critical") {
		t.Error("missing rationale")
	}
	if !strings.Contains(body, "Resolved by: human") {
		t.Error("missing resolved by")
	}
}

// TestFormatResolutionMailBodyNoRationale tests resolution without rationale.
func TestFormatResolutionMailBodyNoRationale(t *testing.T) {
	body := formatResolutionMailBody(
		"hq-dec-123",
		"Which?",
		"A",
		"", // no rationale
		"user",
	)

	if strings.Contains(body, "Rationale:") {
		t.Error("should not have Rationale line when rationale is empty")
	}
}

// TestFormatDecisionAge tests age formatting.
func TestFormatDecisionAge(t *testing.T) {
	// Note: These tests are time-sensitive
	// We're testing the format, not exact values

	// Invalid timestamp
	got := formatDecisionAge("invalid")
	if got != "?" {
		t.Errorf("formatDecisionAge(invalid) = %q, want '?'", got)
	}

	// Empty timestamp
	got = formatDecisionAge("")
	if got != "?" {
		t.Errorf("formatDecisionAge('') = %q, want '?'", got)
	}
}

// TestDecisionCommandFlags tests command flag definitions.
func TestDecisionCommandFlags(t *testing.T) {
	// Verify decisionCmd is properly configured
	if decisionCmd == nil {
		t.Fatal("decisionCmd is nil")
	}
	if decisionCmd.Use != "decision" {
		t.Errorf("decisionCmd.Use = %q, want 'decision'", decisionCmd.Use)
	}

	// Verify subcommands
	subCommands := decisionCmd.Commands()
	wantSubs := []string{"request", "list", "show", "resolve", "dashboard", "await"}
	for _, want := range wantSubs {
		found := false
		for _, cmd := range subCommands {
			if cmd.Use == want || strings.HasPrefix(cmd.Use, want+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing subcommand: %s", want)
		}
	}
}

// TestDecisionRequestCmdFlags tests request command flags.
func TestDecisionRequestCmdFlags(t *testing.T) {
	if decisionRequestCmd == nil {
		t.Fatal("decisionRequestCmd is nil")
	}

	// Check required flags
	flags := decisionRequestCmd.Flags()

	// Primary flags (new API)
	promptFlag := flags.Lookup("prompt")
	if promptFlag == nil {
		t.Error("missing --prompt flag")
	}

	blocksFlag := flags.Lookup("blocks")
	if blocksFlag == nil {
		t.Error("missing --blocks flag")
	}

	parentFlag := flags.Lookup("parent")
	if parentFlag == nil {
		t.Error("missing --parent flag")
	}

	// Backward compatibility aliases
	questionFlag := flags.Lookup("question")
	if questionFlag == nil {
		t.Error("missing --question alias flag")
	}

	blockerFlag := flags.Lookup("blocker")
	if blockerFlag == nil {
		t.Error("missing --blocker alias flag")
	}

	optionFlag := flags.Lookup("option")
	if optionFlag == nil {
		t.Error("missing --option flag")
	}

	urgencyFlag := flags.Lookup("urgency")
	if urgencyFlag == nil {
		t.Error("missing --urgency flag")
	} else if urgencyFlag.DefValue != "medium" {
		t.Errorf("urgency default = %q, want 'medium'", urgencyFlag.DefValue)
	}
}

// TestDecisionResolveCmdFlags tests resolve command flags.
func TestDecisionResolveCmdFlags(t *testing.T) {
	if decisionResolveCmd == nil {
		t.Fatal("decisionResolveCmd is nil")
	}

	flags := decisionResolveCmd.Flags()

	choiceFlag := flags.Lookup("choice")
	if choiceFlag == nil {
		t.Error("missing --choice flag")
	}

	rationaleFlag := flags.Lookup("rationale")
	if rationaleFlag == nil {
		t.Error("missing --rationale flag")
	}
}

// TestDecisionListCmdFlags tests list command flags.
func TestDecisionListCmdFlags(t *testing.T) {
	if decisionListCmd == nil {
		t.Fatal("decisionListCmd is nil")
	}

	flags := decisionListCmd.Flags()

	allFlag := flags.Lookup("all")
	if allFlag == nil {
		t.Error("missing --all flag")
	}

	jsonFlag := flags.Lookup("json")
	if jsonFlag == nil {
		t.Error("missing --json flag")
	}
}

// TestDecisionDashboardCmdFlags tests dashboard command flags.
func TestDecisionDashboardCmdFlags(t *testing.T) {
	if decisionDashboardCmd == nil {
		t.Fatal("decisionDashboardCmd is nil")
	}

	flags := decisionDashboardCmd.Flags()

	jsonFlag := flags.Lookup("json")
	if jsonFlag == nil {
		t.Error("missing --json flag")
	}
}

// TestDecisionAwaitCmdFlags tests await command flags.
func TestDecisionAwaitCmdFlags(t *testing.T) {
	if decisionAwaitCmd == nil {
		t.Fatal("decisionAwaitCmd is nil")
	}

	flags := decisionAwaitCmd.Flags()

	timeoutFlag := flags.Lookup("timeout")
	if timeoutFlag == nil {
		t.Error("missing --timeout flag")
	}

	jsonFlag := flags.Lookup("json")
	if jsonFlag == nil {
		t.Error("missing --json flag")
	}
}

// TestDecisionCommandHasAwait tests that await is a subcommand.
func TestDecisionCommandHasAwait(t *testing.T) {
	if decisionCmd == nil {
		t.Fatal("decisionCmd is nil")
	}

	subCommands := decisionCmd.Commands()
	found := false
	for _, cmd := range subCommands {
		if cmd.Use == "await <decision-id>" || strings.HasPrefix(cmd.Use, "await ") {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing await subcommand")
	}
}

// TestFormatDecisionsList tests the JSON list formatter.
func TestFormatDecisionsList(t *testing.T) {
	issues := []*beads.Issue{
		{
			ID:          "hq-dec-1",
			Title:       "Test 1",
			Description: "## Question\nQ1?\n## Options\n### 1. A\n### 2. B\n---\n_Requested by: test_\n_Urgency: high_",
			CreatedAt:   "2026-01-24T10:00:00Z",
		},
		{
			ID:          "hq-dec-2",
			Title:       "Test 2",
			Description: "## Question\nQ2?\n## Options\n### 1. X\n---\n_Requested by: other_\n_Urgency: low_",
			CreatedAt:   "2026-01-24T11:00:00Z",
		},
	}

	result := formatDecisionsList(issues)

	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}

	// Check first item
	if result[0]["id"] != "hq-dec-1" {
		t.Errorf("result[0][id] = %v, want 'hq-dec-1'", result[0]["id"])
	}
}

// TestFormatDecisionsListEmpty tests empty list.
func TestFormatDecisionsListEmpty(t *testing.T) {
	result := formatDecisionsList(nil)
	if result != nil {
		t.Errorf("formatDecisionsList(nil) = %v, want nil", result)
	}

	result = formatDecisionsList([]*beads.Issue{})
	if result != nil {
		t.Errorf("formatDecisionsList([]) = %v, want nil", result)
	}
}
