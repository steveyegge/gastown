package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestAddIntegrationBranchField(t *testing.T) {
	tests := []struct {
		name        string
		description string
		branchName  string
		want        string
	}{
		{
			name:        "empty description",
			description: "",
			branchName:  "integration/gt-epic",
			want:        "integration_branch: integration/gt-epic",
		},
		{
			name:        "simple description",
			description: "Epic for authentication",
			branchName:  "integration/gt-auth",
			want:        "integration_branch: integration/gt-auth\nEpic for authentication",
		},
		{
			name:        "existing integration_branch field",
			description: "integration_branch: integration/old-epic\nSome description",
			branchName:  "integration/new-epic",
			want:        "integration_branch: integration/new-epic\nSome description",
		},
		{
			name:        "multiline description",
			description: "Line 1\nLine 2\nLine 3",
			branchName:  "integration/gt-xyz",
			want:        "integration_branch: integration/gt-xyz\nLine 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addIntegrationBranchField(tt.description, tt.branchName)
			if got != tt.want {
				t.Errorf("addIntegrationBranchField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseBranchName(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		wantIssue  string
		wantWorker string
	}{
		{
			name:       "polecat branch format",
			branch:     "polecat/Nux/gt-xyz",
			wantIssue:  "gt-xyz",
			wantWorker: "Nux",
		},
		{
			name:       "polecat branch with subtask",
			branch:     "polecat/Worker/gt-abc.1",
			wantIssue:  "gt-abc.1",
			wantWorker: "Worker",
		},
		{
			name:       "simple issue branch",
			branch:     "gt-xyz",
			wantIssue:  "gt-xyz",
			wantWorker: "",
		},
		{
			name:       "feature branch with issue",
			branch:     "feature/gt-abc-impl",
			wantIssue:  "gt-abc",
			wantWorker: "",
		},
		{
			name:       "no issue pattern",
			branch:     "main",
			wantIssue:  "",
			wantWorker: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := parseBranchName(tt.branch)
			if info.Issue != tt.wantIssue {
				t.Errorf("parseBranchName() Issue = %q, want %q", info.Issue, tt.wantIssue)
			}
			if info.Worker != tt.wantWorker {
				t.Errorf("parseBranchName() Worker = %q, want %q", info.Worker, tt.wantWorker)
			}
		})
	}
}

func TestFormatMRAge(t *testing.T) {
	tests := []struct {
		name      string
		createdAt string
		wantOk    bool // just check it doesn't panic/error
	}{
		{
			name:      "RFC3339 format",
			createdAt: "2025-01-01T12:00:00Z",
			wantOk:    true,
		},
		{
			name:      "alternative format",
			createdAt: "2025-01-01T12:00:00",
			wantOk:    true,
		},
		{
			name:      "invalid format",
			createdAt: "not-a-date",
			wantOk:    true, // returns "?" for invalid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMRAge(tt.createdAt)
			if tt.wantOk && result == "" {
				t.Errorf("formatMRAge() returned empty for %s", tt.createdAt)
			}
		})
	}
}

func TestGetDescriptionWithoutMRFields(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "empty description",
			description: "",
			want:        "",
		},
		{
			name:        "only MR fields",
			description: "branch: polecat/Nux/gt-xyz\ntarget: main\nworker: Nux",
			want:        "",
		},
		{
			name:        "mixed content",
			description: "branch: polecat/Nux/gt-xyz\nSome custom notes\ntarget: main",
			want:        "Some custom notes",
		},
		{
			name:        "no MR fields",
			description: "Just a regular description\nWith multiple lines",
			want:        "Just a regular description\nWith multiple lines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDescriptionWithoutMRFields(tt.description)
			if got != tt.want {
				t.Errorf("getDescriptionWithoutMRFields() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			s:      "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			s:      "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "needs truncation",
			s:      "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "very short max",
			s:      "hello",
			maxLen: 3,
			want:   "hel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string // We check for substring since styling adds ANSI codes
	}{
		{
			name:   "open status",
			status: "open",
			want:   "open",
		},
		{
			name:   "in_progress status",
			status: "in_progress",
			want:   "in_progress",
		},
		{
			name:   "closed status",
			status: "closed",
			want:   "closed",
		},
		{
			name:   "unknown status",
			status: "pending",
			want:   "pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatStatus(tt.status)
			if got == "" {
				t.Errorf("formatStatus(%q) returned empty string", tt.status)
			}
			// The result contains ANSI codes, so just check the status text is present
			if !contains(got, tt.want) {
				t.Errorf("formatStatus(%q) = %q, should contain %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestGetStatusIcon(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"open", "○"},
		{"in_progress", "▶"},
		{"closed", "✓"},
		{"unknown", "•"},
		{"", "•"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := getStatusIcon(tt.status)
			if got != tt.want {
				t.Errorf("getStatusIcon(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestFormatTimeAgo(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		wantEmpty bool
	}{
		{
			name:      "RFC3339 format",
			timestamp: "2025-01-01T12:00:00Z",
			wantEmpty: false,
		},
		{
			name:      "RFC3339 with timezone",
			timestamp: "2025-01-01T12:00:00-08:00",
			wantEmpty: false,
		},
		{
			name:      "date only format",
			timestamp: "2025-01-01",
			wantEmpty: false,
		},
		{
			name:      "datetime without Z",
			timestamp: "2025-01-01T12:00:00",
			wantEmpty: false,
		},
		{
			name:      "invalid format returns empty",
			timestamp: "not-a-date",
			wantEmpty: true,
		},
		{
			name:      "empty string returns empty",
			timestamp: "",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimeAgo(tt.timestamp)
			if tt.wantEmpty && got != "" {
				t.Errorf("formatTimeAgo(%q) = %q, want empty", tt.timestamp, got)
			}
			if !tt.wantEmpty && got == "" {
				t.Errorf("formatTimeAgo(%q) returned empty, want non-empty", tt.timestamp)
			}
		})
	}
}

// contains checks if s contains substr (helper for styled output)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFilterMRsByTarget(t *testing.T) {
	// Create test MRs with different targets
	mrs := []*beads.Issue{
		makeTestMR("mr-1", "polecat/Nux/gt-001", "integration/gt-epic", "Nux", "open"),
		makeTestMR("mr-2", "polecat/Toast/gt-002", "main", "Toast", "open"),
		makeTestMR("mr-3", "polecat/Able/gt-003", "integration/gt-epic", "Able", "open"),
		makeTestMR("mr-4", "polecat/Baker/gt-004", "integration/gt-other", "Baker", "open"),
	}

	tests := []struct {
		name         string
		targetBranch string
		wantCount    int
		wantIDs      []string
	}{
		{
			name:         "filter to integration/gt-epic",
			targetBranch: "integration/gt-epic",
			wantCount:    2,
			wantIDs:      []string{"mr-1", "mr-3"},
		},
		{
			name:         "filter to main",
			targetBranch: "main",
			wantCount:    1,
			wantIDs:      []string{"mr-2"},
		},
		{
			name:         "filter to non-existent branch",
			targetBranch: "integration/no-such-epic",
			wantCount:    0,
			wantIDs:      []string{},
		},
		{
			name:         "filter to other integration branch",
			targetBranch: "integration/gt-other",
			wantCount:    1,
			wantIDs:      []string{"mr-4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterMRsByTarget(mrs, tt.targetBranch)
			if len(got) != tt.wantCount {
				t.Errorf("filterMRsByTarget() returned %d MRs, want %d", len(got), tt.wantCount)
			}

			// Verify correct IDs
			gotIDs := make(map[string]bool)
			for _, mr := range got {
				gotIDs[mr.ID] = true
			}
			for _, wantID := range tt.wantIDs {
				if !gotIDs[wantID] {
					t.Errorf("filterMRsByTarget() missing expected MR %s", wantID)
				}
			}
		})
	}
}

func TestFilterMRsByTarget_EmptyInput(t *testing.T) {
	got := filterMRsByTarget(nil, "integration/gt-epic")
	if got != nil {
		t.Errorf("filterMRsByTarget(nil) = %v, want nil", got)
	}

	got = filterMRsByTarget([]*beads.Issue{}, "integration/gt-epic")
	if len(got) != 0 {
		t.Errorf("filterMRsByTarget([]) = %v, want empty slice", got)
	}
}

func TestFilterMRsByTarget_NoMRFields(t *testing.T) {
	// Issue without MR fields in description
	plainIssue := &beads.Issue{
		ID:          "issue-1",
		Title:       "Not an MR",
		Type:        "merge-request",
		Status:      "open",
		Description: "Just a plain description with no MR fields",
	}

	got := filterMRsByTarget([]*beads.Issue{plainIssue}, "main")
	if len(got) != 0 {
		t.Errorf("filterMRsByTarget() should filter out issues without MR fields, got %d", len(got))
	}
}

// Tests for integration branch template functionality

func TestExpandBranchTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		epicID      string
		ticket      string
		gitUserName string
		want        string
	}{
		{
			name:        "default template",
			template:    "integration/{{epic}}",
			epicID:      "gt-auth",
			ticket:      "",
			gitUserName: "",
			want:        "integration/gt-auth",
		},
		{
			name:        "with user",
			template:    "{{user}}/integration/{{epic}}",
			epicID:      "gt-auth",
			ticket:      "",
			gitUserName: "klauern",
			want:        "klauern/integration/gt-auth",
		},
		{
			name:        "with ticket",
			template:    "{{ticket}}/{{epic}}",
			epicID:      "gt-auth",
			ticket:      "PROJ-1234",
			gitUserName: "",
			want:        "PROJ-1234/gt-auth",
		},
		{
			name:        "full template",
			template:    "{{user}}/{{ticket}}/{{epic}}",
			epicID:      "gt-auth",
			ticket:      "PROJ-1234",
			gitUserName: "klauern",
			want:        "klauern/PROJ-1234/gt-auth",
		},
		{
			name:        "missing variable left as-is",
			template:    "{{user}}/{{epic}}",
			epicID:      "gt-auth",
			ticket:      "",
			gitUserName: "",
			want:        "{{user}}/gt-auth",
		},
		{
			name:        "unknown variable left as-is",
			template:    "{{unknown}}/{{epic}}",
			epicID:      "gt-auth",
			ticket:      "",
			gitUserName: "",
			want:        "{{unknown}}/gt-auth",
		},
		{
			name:        "no template variables",
			template:    "static-branch-name",
			epicID:      "gt-auth",
			ticket:      "",
			gitUserName: "",
			want:        "static-branch-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandBranchTemplate(tt.template, tt.epicID, tt.ticket, tt.gitUserName)
			if got != tt.want {
				t.Errorf("expandBranchTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid simple branch",
			input:   "integration/gt-auth",
			want:    "integration/gt-auth",
			wantErr: false,
		},
		{
			name:    "valid with user",
			input:   "klauern/PROJ-1234/gt-auth",
			want:    "klauern/PROJ-1234/gt-auth",
			wantErr: false,
		},
		{
			name:    "empty branch name",
			input:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "unexpanded template variable",
			input:   "{{user}}/gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains space",
			input:   "my branch/gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "starts with slash",
			input:   "/integration/gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "ends with slash",
			input:   "integration/gt-auth/",
			want:    "",
			wantErr: true,
		},
		{
			name:    "starts with dot",
			input:   ".hidden/gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "ends with dot",
			input:   "integration/gt-auth.",
			want:    "",
			wantErr: true,
		},
		{
			name:    "ends with .lock",
			input:   "integration/gt-auth.lock",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains double dot",
			input:   "integration/../gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains double slash",
			input:   "integration//gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains tilde",
			input:   "integration/gt~auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains caret",
			input:   "integration/gt^auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains colon",
			input:   "integration:gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains question mark",
			input:   "integration/gt-auth?",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains asterisk",
			input:   "integration/*",
			want:    "",
			wantErr: true,
		},
		{
			name:    "contains backslash",
			input:   "integration\\gt-auth",
			want:    "",
			wantErr: true,
		},
		{
			name:    "valid with dots in middle",
			input:   "integration/v1.2.3",
			want:    "integration/v1.2.3",
			wantErr: false,
		},
		{
			name:    "valid with hyphens and underscores",
			input:   "feature_branch-1/gt-auth-epic_2",
			want:    "feature_branch-1/gt-auth-epic_2",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeBranchName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeBranchName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("sanitizeBranchName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchName_Length(t *testing.T) {
	// Test branch name length limit
	longName := "integration/"
	for i := 0; i < 200; i++ {
		longName += "a"
	}

	_, err := sanitizeBranchName(longName)
	if err == nil {
		t.Error("sanitizeBranchName() should reject branch names over 200 chars")
	}
}

func TestGetIntegrationBranchFromEpic(t *testing.T) {
	tests := []struct {
		name string
		epic *beads.Issue
		want string
	}{
		{
			name: "nil epic",
			epic: nil,
			want: "",
		},
		{
			name: "empty description",
			epic: &beads.Issue{
				ID:          "gt-epic",
				Description: "",
			},
			want: "",
		},
		{
			name: "no integration_branch field",
			epic: &beads.Issue{
				ID:          "gt-epic",
				Description: "Just a description",
			},
			want: "",
		},
		{
			name: "integration_branch at start",
			epic: &beads.Issue{
				ID:          "gt-epic",
				Description: "integration_branch: klauern/PROJ-1234/gt-epic\nSome description",
			},
			want: "klauern/PROJ-1234/gt-epic",
		},
		{
			name: "integration_branch in middle",
			epic: &beads.Issue{
				ID:          "gt-epic",
				Description: "Line 1\nintegration_branch: custom/branch/name\nLine 3",
			},
			want: "custom/branch/name",
		},
		{
			name: "integration_branch case insensitive",
			epic: &beads.Issue{
				ID:          "gt-epic",
				Description: "Integration_Branch: case-test/branch\n",
			},
			want: "case-test/branch",
		},
		{
			name: "integration_branch with extra whitespace",
			epic: &beads.Issue{
				ID:          "gt-epic",
				Description: "  integration_branch:   spacy/branch  \n",
			},
			want: "spacy/branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIntegrationBranchFromEpic(tt.epic)
			if got != tt.want {
				t.Errorf("getIntegrationBranchFromEpic() = %q, want %q", got, tt.want)
			}
		})
	}
}
