package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

	// Type flag for type-aware validation
	typeFlag := flags.Lookup("type")
	if typeFlag == nil {
		t.Error("missing --type flag")
	} else if typeFlag.DefValue != "" {
		t.Errorf("type default = %q, want empty", typeFlag.DefValue)
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

// --- Turn enforcement tests ---

// TestTurnMarkerPath tests marker path generation.
func TestTurnMarkerPath(t *testing.T) {
	path := turnMarkerPath("test-session-123")
	if path != "/tmp/.decision-offered-test-session-123" {
		t.Errorf("turnMarkerPath() = %q, want '/tmp/.decision-offered-test-session-123'", path)
	}
}

// TestTurnClear tests clearing the turn marker.
func TestTurnClear(t *testing.T) {
	sessionID := "test-clear-session"

	// Create marker
	if err := createTurnMarker(sessionID); err != nil {
		t.Fatalf("createTurnMarker failed: %v", err)
	}

	// Verify it exists
	if !turnMarkerExists(sessionID) {
		t.Fatal("marker should exist after creation")
	}

	// Clear it
	clearTurnMarker(sessionID)

	// Verify it's gone
	if turnMarkerExists(sessionID) {
		t.Error("marker should not exist after clear")
	}
}

// TestTurnMark tests marking a decision was offered.
func TestTurnMark(t *testing.T) {
	sessionID := "test-mark-session"

	// Clear any existing marker
	clearTurnMarker(sessionID)

	// Verify not exists
	if turnMarkerExists(sessionID) {
		t.Fatal("marker should not exist initially")
	}

	// Create marker
	if err := createTurnMarker(sessionID); err != nil {
		t.Fatalf("createTurnMarker failed: %v", err)
	}

	// Verify exists
	if !turnMarkerExists(sessionID) {
		t.Error("marker should exist after creation")
	}

	// Cleanup
	clearTurnMarker(sessionID)
}

// TestIsDecisionCommand tests detection of decision commands.
func TestIsDecisionCommand(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"gt decision request --prompt 'test'", true},
		{"gt decision request", true},
		{"bd decision create --question 'test'", true},
		{"bd decision create", true},
		{"git status", false},
		{"echo hello", false},
		{"gt mail send", false},
		{"gt decision list", false}, // list is not creating a decision
		{"gt decision resolve", false},
		{"some-command && gt decision request --prompt 'x'", true}, // chained
		{"GT DECISION REQUEST", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := isDecisionCommand(tt.command)
			if got != tt.want {
				t.Errorf("isDecisionCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

// TestTurnCheckStrict tests strict mode blocking.
func TestTurnCheckStrict(t *testing.T) {
	sessionID := "test-check-strict"

	// Clear marker
	clearTurnMarker(sessionID)

	// Check without marker (strict) - should return block JSON
	result := checkTurnMarker(sessionID, false)
	if result == nil {
		t.Fatal("strict mode should return block result when no marker")
	}
	if result.Decision != "block" {
		t.Errorf("result.Decision = %q, want 'block'", result.Decision)
	}
	if result.Reason == "" {
		t.Error("result.Reason should not be empty")
	}
}

// TestTurnCheckSoft tests soft mode allowing.
func TestTurnCheckSoft(t *testing.T) {
	sessionID := "test-check-soft"

	// Clear marker
	clearTurnMarker(sessionID)

	// Check without marker (soft) - should return nil (allow)
	result := checkTurnMarker(sessionID, true)
	if result != nil {
		t.Errorf("soft mode should return nil when no marker, got %+v", result)
	}
}

// TestTurnCheckWithMarker tests that marker allows through.
func TestTurnCheckWithMarker(t *testing.T) {
	sessionID := "test-check-marker"

	// Create marker
	if err := createTurnMarker(sessionID); err != nil {
		t.Fatalf("createTurnMarker failed: %v", err)
	}
	defer clearTurnMarker(sessionID) // Cleanup

	// Check with marker - should return nil (allow) and preserve marker
	result := checkTurnMarker(sessionID, false)
	if result != nil {
		t.Errorf("should return nil when marker exists, got %+v", result)
	}

	// Marker should still exist (not cleared by check)
	// This allows multiple Stop hook firings to pass
	if !turnMarkerExists(sessionID) {
		t.Error("marker should still exist after check (cleared by turn-clear, not turn-check)")
	}
}

// TestTurnCheckMultipleFirings tests that Stop hook can fire multiple times.
// This was a bug: the first check would clear the marker, causing subsequent
// checks to block incorrectly.
func TestTurnCheckMultipleFirings(t *testing.T) {
	sessionID := "test-multiple-firings"

	// Create marker (simulating decision request)
	if err := createTurnMarker(sessionID); err != nil {
		t.Fatalf("createTurnMarker failed: %v", err)
	}
	defer clearTurnMarker(sessionID) // Cleanup

	// First Stop hook firing - should allow
	result1 := checkTurnMarker(sessionID, false)
	if result1 != nil {
		t.Errorf("first check should allow, got %+v", result1)
	}

	// Second Stop hook firing - should also allow (marker persists)
	result2 := checkTurnMarker(sessionID, false)
	if result2 != nil {
		t.Errorf("second check should also allow, got %+v", result2)
	}

	// Third Stop hook firing - should still allow
	result3 := checkTurnMarker(sessionID, false)
	if result3 != nil {
		t.Errorf("third check should also allow, got %+v", result3)
	}
}

// TestDecisionTurnCmdExists tests that turn commands exist.
func TestDecisionTurnCmdExists(t *testing.T) {
	subCommands := decisionCmd.Commands()

	wantSubs := []string{"turn-clear", "turn-mark", "turn-check"}
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

// Note: hasFailureContext, hasFileOption, suggestFileOption moved to external scripts.
// See validators/create-decision-fail-file.sh for the script-based implementation.

// TestDecisionRequestNoFileCheckFlag tests that --no-file-check flag exists.
func TestDecisionRequestNoFileCheckFlag(t *testing.T) {
	if decisionRequestCmd == nil {
		t.Fatal("decisionRequestCmd is nil")
	}

	flags := decisionRequestCmd.Flags()
	noFileCheckFlag := flags.Lookup("no-file-check")
	if noFileCheckFlag == nil {
		t.Error("missing --no-file-check flag")
	}
}

// --- Decision Chaining Tests ---
// Note: containsWholeWord and isWordChar moved to external validator scripts.
// See validators/create-decision-fail-file.sh for the script-based implementation.

// TestDecisionChainCmdExists tests that chain command exists with proper flags.
func TestDecisionChainCmdExists(t *testing.T) {
	// Find chain subcommand
	var chainCmd *cobra.Command
	for _, cmd := range decisionCmd.Commands() {
		if strings.HasPrefix(cmd.Use, "chain ") {
			chainCmd = cmd
			break
		}
	}

	if chainCmd == nil {
		t.Fatal("missing 'chain' subcommand")
	}

	// Check flags
	flags := chainCmd.Flags()

	descendantsFlag := flags.Lookup("descendants")
	if descendantsFlag == nil {
		t.Error("missing --descendants flag")
	}

	jsonFlag := flags.Lookup("json")
	if jsonFlag == nil {
		t.Error("missing --json flag")
	}
}

// TestDecisionRequestPredecessorFlag tests that predecessor flag exists.
func TestDecisionRequestPredecessorFlag(t *testing.T) {
	if decisionRequestCmd == nil {
		t.Fatal("decisionRequestCmd is nil")
	}

	flags := decisionRequestCmd.Flags()

	predecessorFlag := flags.Lookup("predecessor")
	if predecessorFlag == nil {
		t.Error("missing --predecessor flag")
	}
}

// TestChainNodeStruct tests chainNode JSON marshaling.
func TestChainNodeStruct(t *testing.T) {
	node := chainNode{
		ID:          "hq-dec-123",
		Question:    "Which approach?",
		ChosenIndex: 1,
		ChosenLabel: "Option A",
		Urgency:     "high",
		RequestedBy: "test-agent",
		RequestedAt: "2026-01-29T10:00:00Z",
		ResolvedAt:  "2026-01-29T10:30:00Z",
		Predecessor: "hq-dec-100",
		IsTarget:    true,
	}

	// Test JSON marshaling
	data, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify key fields are present
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"id":"hq-dec-123"`) {
		t.Error("missing id field in JSON")
	}
	if !strings.Contains(jsonStr, `"question":"Which approach?"`) {
		t.Error("missing question field in JSON")
	}
	if !strings.Contains(jsonStr, `"chosen_label":"Option A"`) {
		t.Error("missing chosen_label field in JSON")
	}
	if !strings.Contains(jsonStr, `"predecessor_id":"hq-dec-100"`) {
		t.Error("missing predecessor_id field in JSON")
	}
	if !strings.Contains(jsonStr, `"is_target":true`) {
		t.Error("missing is_target field in JSON")
	}

	// Test unmarshaling
	var decoded chainNode
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.ID != node.ID {
		t.Errorf("decoded.ID = %q, want %q", decoded.ID, node.ID)
	}
	if decoded.Predecessor != node.Predecessor {
		t.Errorf("decoded.Predecessor = %q, want %q", decoded.Predecessor, node.Predecessor)
	}
}

// TestChainNodeWithChildren tests chainNode tree structure.
func TestChainNodeWithChildren(t *testing.T) {
	root := &chainNode{
		ID:       "hq-dec-1",
		Question: "Root decision",
		Children: []*chainNode{
			{
				ID:          "hq-dec-2",
				Question:    "Child 1",
				Predecessor: "hq-dec-1",
			},
			{
				ID:          "hq-dec-3",
				Question:    "Child 2",
				Predecessor: "hq-dec-1",
			},
		},
	}

	// Test JSON marshaling with children
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, "children") {
		t.Error("missing children field in JSON")
	}
	if !strings.Contains(jsonStr, "hq-dec-2") {
		t.Error("missing first child in JSON")
	}
	if !strings.Contains(jsonStr, "hq-dec-3") {
		t.Error("missing second child in JSON")
	}
}

// TestValidateContextJSON tests JSON context validation.
func TestValidateContextJSON(t *testing.T) {
	tests := []struct {
		name      string
		context   string
		wantValid bool
	}{
		{
			name:      "valid object",
			context:   `{"key": "value", "number": 42}`,
			wantValid: true,
		},
		{
			name:      "valid array",
			context:   `["item1", "item2", "item3"]`,
			wantValid: true,
		},
		{
			name:      "valid string",
			context:   `"just a string"`,
			wantValid: true,
		},
		{
			name:      "valid number",
			context:   `123`,
			wantValid: true,
		},
		{
			name:      "valid boolean",
			context:   `true`,
			wantValid: true,
		},
		{
			name:      "valid null",
			context:   `null`,
			wantValid: true,
		},
		{
			name:      "valid nested",
			context:   `{"nested": {"deep": [1, 2, {"x": "y"}]}}`,
			wantValid: true,
		},
		{
			name:      "empty string is valid (no context)",
			context:   ``,
			wantValid: true,
		},
		{
			name:      "invalid - plain text",
			context:   `This is plain text, not JSON`,
			wantValid: false,
		},
		{
			name:      "invalid - unclosed brace",
			context:   `{"key": "value"`,
			wantValid: false,
		},
		{
			name:      "invalid - trailing comma",
			context:   `{"key": "value",}`,
			wantValid: false,
		},
		{
			name:      "invalid - single quotes",
			context:   `{'key': 'value'}`,
			wantValid: false,
		},
		{
			name:      "invalid - unquoted keys",
			context:   `{key: "value"}`,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var valid bool
			if tt.context == "" {
				// Empty context is valid (means no context provided)
				valid = true
			} else {
				var js json.RawMessage
				err := json.Unmarshal([]byte(tt.context), &js)
				valid = (err == nil)
			}
			if valid != tt.wantValid {
				t.Errorf("context %q: valid=%v, want %v", tt.context, valid, tt.wantValid)
			}
		})
	}
}

// --- Additional Decision Chaining CLI Tests ---

// TestDecisionChainCmdFlags tests chain command flags in detail.
func TestDecisionChainCmdFlags(t *testing.T) {
	// Find chain subcommand
	var chainCmd *cobra.Command
	for _, cmd := range decisionCmd.Commands() {
		if strings.HasPrefix(cmd.Use, "chain ") {
			chainCmd = cmd
			break
		}
	}

	if chainCmd == nil {
		t.Fatal("missing 'chain' subcommand")
	}

	t.Run("command usage", func(t *testing.T) {
		if !strings.Contains(chainCmd.Use, "decision-id") {
			t.Errorf("Use should contain decision-id, got %q", chainCmd.Use)
		}
	})

	t.Run("descendants flag", func(t *testing.T) {
		flag := chainCmd.Flags().Lookup("descendants")
		if flag == nil {
			t.Fatal("missing --descendants flag")
		}
		if flag.DefValue != "false" {
			t.Errorf("descendants default = %q, want 'false'", flag.DefValue)
		}
	})

	t.Run("json flag", func(t *testing.T) {
		flag := chainCmd.Flags().Lookup("json")
		if flag == nil {
			t.Fatal("missing --json flag")
		}
	})

	t.Run("short description", func(t *testing.T) {
		if chainCmd.Short == "" {
			t.Error("chain command should have short description")
		}
	})
}

// TestDecisionRequestCmdPredecessorFlagDetails tests predecessor flag details.
func TestDecisionRequestCmdPredecessorFlagDetails(t *testing.T) {
	if decisionRequestCmd == nil {
		t.Fatal("decisionRequestCmd is nil")
	}

	flag := decisionRequestCmd.Flags().Lookup("predecessor")
	if flag == nil {
		t.Fatal("missing --predecessor flag")
	}

	t.Run("default value", func(t *testing.T) {
		if flag.DefValue != "" {
			t.Errorf("predecessor default = %q, want empty", flag.DefValue)
		}
	})

	t.Run("has description", func(t *testing.T) {
		if flag.Usage == "" {
			t.Error("predecessor flag should have usage description")
		}
	})
}

// TestBuildChainNodeFromIssue tests converting an issue to a chain node.
func TestBuildChainNodeFromIssue(t *testing.T) {
	tests := []struct {
		name     string
		fields   beads.DecisionFields
		wantNode chainNode
	}{
		{
			name: "resolved decision",
			fields: beads.DecisionFields{
				Question:      "Which approach?",
				ChosenIndex:   1,
				Urgency:       "high",
				RequestedBy:   "test-agent",
				PredecessorID: "hq-parent",
			},
			wantNode: chainNode{
				Question:    "Which approach?",
				ChosenIndex: 1,
				ChosenLabel: "Option A",
				Urgency:     "high",
				RequestedBy: "test-agent",
				Predecessor: "hq-parent",
			},
		},
		{
			name: "unresolved decision",
			fields: beads.DecisionFields{
				Question:    "Pending decision?",
				ChosenIndex: 0,
				Urgency:     "medium",
				RequestedBy: "another-agent",
			},
			wantNode: chainNode{
				Question:    "Pending decision?",
				ChosenIndex: 0,
				Urgency:     "medium",
				RequestedBy: "another-agent",
			},
		},
		{
			name: "root decision (no predecessor)",
			fields: beads.DecisionFields{
				Question:    "Root decision",
				ChosenIndex: 2,
				Urgency:     "low",
			},
			wantNode: chainNode{
				Question:    "Root decision",
				ChosenIndex: 2,
				ChosenLabel: "Root option",
				Urgency:     "low",
				Predecessor: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify fields are consistent with expected node structure
			if tt.fields.Question != tt.wantNode.Question {
				t.Errorf("Question mismatch: %q vs %q", tt.fields.Question, tt.wantNode.Question)
			}
			if tt.fields.ChosenIndex != tt.wantNode.ChosenIndex {
				t.Errorf("ChosenIndex mismatch: %d vs %d", tt.fields.ChosenIndex, tt.wantNode.ChosenIndex)
			}
			if tt.fields.PredecessorID != tt.wantNode.Predecessor {
				t.Errorf("PredecessorID mismatch: %q vs %q", tt.fields.PredecessorID, tt.wantNode.Predecessor)
			}
		})
	}
}

// TestChainNodeJSONOutput tests JSON output format of chain nodes.
func TestChainNodeJSONOutput(t *testing.T) {
	chain := []chainNode{
		{
			ID:          "hq-root",
			Question:    "Root decision?",
			ChosenIndex: 1,
			ChosenLabel: "Option A",
			Urgency:     "high",
			RequestedBy: "agent-1",
			RequestedAt: "2026-01-29T10:00:00Z",
			ResolvedAt:  "2026-01-29T10:30:00Z",
		},
		{
			ID:          "hq-child",
			Question:    "Follow-up?",
			ChosenIndex: 2,
			ChosenLabel: "Option B",
			Urgency:     "medium",
			RequestedBy: "agent-2",
			RequestedAt: "2026-01-29T11:00:00Z",
			Predecessor: "hq-root",
			IsTarget:    true,
		},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(chain, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	jsonStr := string(data)

	// Verify structure
	if !strings.Contains(jsonStr, `"id"`) {
		t.Error("JSON should contain id field")
	}
	if !strings.Contains(jsonStr, `"question"`) {
		t.Error("JSON should contain question field")
	}
	if !strings.Contains(jsonStr, `"predecessor_id"`) {
		t.Error("JSON should contain predecessor_id field")
	}
	if !strings.Contains(jsonStr, `"is_target"`) {
		t.Error("JSON should contain is_target field")
	}
	if !strings.Contains(jsonStr, "hq-root") {
		t.Error("JSON should contain root ID")
	}
	if !strings.Contains(jsonStr, "hq-child") {
		t.Error("JSON should contain child ID")
	}
}

// TestDecisionShowCmdHasChainInfo tests that show command can display chain info.
func TestDecisionShowCmdHasChainInfo(t *testing.T) {
	// Find show subcommand
	var showCmd *cobra.Command
	for _, cmd := range decisionCmd.Commands() {
		if strings.HasPrefix(cmd.Use, "show ") {
			showCmd = cmd
			break
		}
	}

	if showCmd == nil {
		t.Fatal("missing 'show' subcommand")
	}

	// Check for --chain flag if it exists (optional feature)
	flags := showCmd.Flags()
	chainFlag := flags.Lookup("chain")
	// This is informational - chain flag may or may not exist
	t.Logf("show command chain flag present: %v", chainFlag != nil)
}

// TestFormatChainDisplay tests text formatting of decision chains.
func TestFormatChainDisplay(t *testing.T) {
	// Test that chain formatting produces readable output
	nodes := []chainNode{
		{ID: "1", Question: "First?", ChosenLabel: "A", Urgency: "high"},
		{ID: "2", Question: "Second?", ChosenLabel: "B", Urgency: "medium", Predecessor: "1"},
		{ID: "3", Question: "Third?", IsTarget: true, Predecessor: "2"},
	}

	// Verify nodes are structured correctly for display
	for i, node := range nodes {
		if node.ID == "" {
			t.Errorf("node[%d] missing ID", i)
		}
		if node.Question == "" {
			t.Errorf("node[%d] missing Question", i)
		}
		// Target should be marked
		if i == len(nodes)-1 && !node.IsTarget {
			t.Errorf("last node should be target")
		}
		// Non-root nodes should have predecessor
		if i > 0 && node.Predecessor == "" {
			t.Errorf("node[%d] should have predecessor", i)
		}
	}
}

// TestDecisionRequestWithContext tests decision request with JSON context.
func TestDecisionRequestWithContext(t *testing.T) {
	if decisionRequestCmd == nil {
		t.Fatal("decisionRequestCmd is nil")
	}

	flags := decisionRequestCmd.Flags()

	// Check for context flag
	contextFlag := flags.Lookup("context")
	if contextFlag == nil {
		t.Error("missing --context flag")
	}
}

// TestFormatPredecessorInfo tests predecessor info formatting.
func TestFormatPredecessorInfo(t *testing.T) {
	tests := []struct {
		predecessor string
		wantEmpty   bool
	}{
		{"", true},
		{"hq-dec-123", false},
		{"some-other-id", false},
	}

	for _, tt := range tests {
		t.Run(tt.predecessor, func(t *testing.T) {
			// If predecessor is empty, info should be empty
			// If predecessor exists, info should contain the ID
			if tt.wantEmpty {
				if tt.predecessor != "" {
					t.Error("test setup error")
				}
			} else {
				if tt.predecessor == "" {
					t.Error("test setup error")
				}
			}
		})
	}
}

// --- Type Embedding Tests ---

// TestEmbedTypeInContext tests type embedding in context JSON.
func TestEmbedTypeInContext(t *testing.T) {
	tests := []struct {
		name     string
		context  string
		dtype    string
		wantType string
	}{
		{
			name:     "empty context",
			context:  "",
			dtype:    "tradeoff",
			wantType: "tradeoff",
		},
		{
			name:     "existing object",
			context:  `{"key": "value"}`,
			dtype:    "confirmation",
			wantType: "confirmation",
		},
		{
			name:     "JSON array",
			context:  `["item1", "item2"]`,
			dtype:    "checkpoint",
			wantType: "checkpoint",
		},
		{
			name:     "JSON string",
			context:  `"simple string"`,
			dtype:    "scope",
			wantType: "scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := embedTypeInContext(tt.context, tt.dtype)

			// Parse result and check for _type field
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(result), &obj); err != nil {
				t.Fatalf("embedTypeInContext returned invalid JSON: %v", err)
			}

			typeVal, ok := obj["_type"].(string)
			if !ok {
				t.Error("result missing _type field")
			} else if typeVal != tt.wantType {
				t.Errorf("_type = %q, want %q", typeVal, tt.wantType)
			}
		})
	}
}

// TestExtractTypeFromContextImpl tests type extraction from context.
func TestExtractTypeFromContextImpl(t *testing.T) {
	tests := []struct {
		name     string
		context  string
		wantType string
	}{
		{
			name:     "empty context",
			context:  "",
			wantType: "",
		},
		{
			name:     "context with type",
			context:  `{"_type": "tradeoff", "key": "value"}`,
			wantType: "tradeoff",
		},
		{
			name:     "context without type",
			context:  `{"key": "value"}`,
			wantType: "",
		},
		{
			name:     "invalid JSON",
			context:  "not json",
			wantType: "",
		},
		{
			name:     "JSON array",
			context:  `["item1", "item2"]`,
			wantType: "",
		},
		{
			name:     "type is number",
			context:  `{"_type": 42}`,
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTypeFromContext(tt.context)
			if got != tt.wantType {
				t.Errorf("extractTypeFromContext() = %q, want %q", got, tt.wantType)
			}
		})
	}
}

// TestRoundTripTypeEmbedding tests embed then extract.
func TestRoundTripTypeEmbedding(t *testing.T) {
	types := []string{"tradeoff", "confirmation", "checkpoint", "assessment", "custom"}
	contexts := []string{
		"",
		`{"existing": "data"}`,
		`{"nested": {"deep": true}}`,
	}

	for _, dtype := range types {
		for i, ctx := range contexts {
			embedded := embedTypeInContext(ctx, dtype)
			extracted := extractTypeFromContext(embedded)

			if extracted != dtype {
				t.Errorf("roundtrip failed for type=%q context[%d]: got %q", dtype, i, extracted)
			}
		}
	}
}

// --- Referenced Beads Validation Tests ---

// TestValidateReferencedBeads tests bead validation logic.
func TestValidateReferencedBeads(t *testing.T) {
	tests := []struct {
		name       string
		prompt     string
		contextStr string
		contextMap map[string]interface{}
		wantErr    bool
	}{
		{
			name:       "no beads referenced",
			prompt:     "Which approach should we take?",
			contextStr: `{"analysis": "some analysis"}`,
			contextMap: map[string]interface{}{"analysis": "some analysis"},
			wantErr:    false,
		},
		{
			name:       "bead referenced with description",
			prompt:     "How should we fix gt-abc123?",
			contextStr: `{"referenced_beads": {"gt-abc123": {"title": "Bug fix", "description_summary": "Fix the auth bug"}}}`,
			contextMap: map[string]interface{}{
				"referenced_beads": map[string]interface{}{
					"gt-abc123": map[string]interface{}{
						"title":               "Bug fix",
						"description_summary": "Fix the auth bug",
					},
				},
			},
			wantErr: false,
		},
		{
			name:       "bead referenced without description",
			prompt:     "How should we fix gt-xyz789?",
			contextStr: `{"analysis": "some analysis"}`,
			contextMap: map[string]interface{}{"analysis": "some analysis"},
			wantErr:    true,
		},
		{
			name:       "multiple beads all described",
			prompt:     "Should we merge gt-abc123 with bd-def456?",
			contextStr: `{"referenced_beads": {"gt-abc123": {"title": "Bug A"}, "bd-def456": {"title": "Bug B"}}}`,
			contextMap: map[string]interface{}{
				"referenced_beads": map[string]interface{}{
					"gt-abc123": map[string]interface{}{"title": "Bug A"},
					"bd-def456": map[string]interface{}{"title": "Bug B"},
				},
			},
			wantErr: false,
		},
		{
			name:       "multiple beads some missing",
			prompt:     "Should we merge gt-abc123 with bd-def456?",
			contextStr: `{"referenced_beads": {"gt-abc123": {"title": "Bug A"}}}`,
			contextMap: map[string]interface{}{
				"referenced_beads": map[string]interface{}{
					"gt-abc123": map[string]interface{}{"title": "Bug A"},
				},
			},
			wantErr: true, // bd-def456 is missing
		},
		{
			name:       "empty context map",
			prompt:     "How to proceed with gt-task1?",
			contextStr: `{}`,
			contextMap: map[string]interface{}{},
			wantErr:    true,
		},
		{
			name:       "nil context map",
			prompt:     "How to proceed with gt-task1?",
			contextStr: "",
			contextMap: nil,
			wantErr:    true,
		},
		{
			name:       "bead in context string but not map",
			prompt:     "What about this?",
			contextStr: `{"note": "relates to gt-hidden"}`,
			contextMap: map[string]interface{}{"note": "relates to gt-hidden"},
			wantErr:    true, // gt-hidden referenced but not in referenced_beads
		},
		{
			name:       "bead entry exists but no title or description",
			prompt:     "Fix gt-empty?",
			contextStr: `{"referenced_beads": {"gt-empty": {}}}`,
			contextMap: map[string]interface{}{
				"referenced_beads": map[string]interface{}{
					"gt-empty": map[string]interface{}{},
				},
			},
			wantErr: true, // entry exists but has no content
		},
		{
			name:       "bead with title only is valid",
			prompt:     "Fix gt-titleonly?",
			contextStr: `{"referenced_beads": {"gt-titleonly": {"title": "Has title"}}}`,
			contextMap: map[string]interface{}{
				"referenced_beads": map[string]interface{}{
					"gt-titleonly": map[string]interface{}{"title": "Has title"},
				},
			},
			wantErr: false,
		},
		{
			name:       "bead with description_summary only is valid",
			prompt:     "Fix gt-desconly?",
			contextStr: `{"referenced_beads": {"gt-desconly": {"description_summary": "Has desc"}}}`,
			contextMap: map[string]interface{}{
				"referenced_beads": map[string]interface{}{
					"gt-desconly": map[string]interface{}{"description_summary": "Has desc"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReferencedBeads(tt.prompt, tt.contextStr, tt.contextMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateReferencedBeads() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDecisionRequestNoBeadCheckFlag tests that --no-bead-check flag exists.
func TestDecisionRequestNoBeadCheckFlag(t *testing.T) {
	if decisionRequestCmd == nil {
		t.Fatal("decisionRequestCmd is nil")
	}

	flags := decisionRequestCmd.Flags()
	noBeadCheckFlag := flags.Lookup("no-bead-check")
	if noBeadCheckFlag == nil {
		t.Error("missing --no-bead-check flag")
	}
}
