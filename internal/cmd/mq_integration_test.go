package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

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

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		wantErr    bool
	}{
		{
			name:       "valid simple branch",
			branchName: "integration/gt-epic",
			wantErr:    false,
		},
		{
			name:       "valid nested branch",
			branchName: "user/project/feature",
			wantErr:    false,
		},
		{
			name:       "valid with hyphens and underscores",
			branchName: "user-name/feature_branch",
			wantErr:    false,
		},
		{
			name:       "empty branch name",
			branchName: "",
			wantErr:    true,
		},
		{
			name:       "contains tilde",
			branchName: "branch~1",
			wantErr:    true,
		},
		{
			name:       "contains caret",
			branchName: "branch^2",
			wantErr:    true,
		},
		{
			name:       "contains colon",
			branchName: "branch:ref",
			wantErr:    true,
		},
		{
			name:       "contains space",
			branchName: "branch name",
			wantErr:    true,
		},
		{
			name:       "contains backslash",
			branchName: "branch\\name",
			wantErr:    true,
		},
		{
			name:       "contains double dot",
			branchName: "branch..name",
			wantErr:    true,
		},
		{
			name:       "contains at-brace",
			branchName: "branch@{name}",
			wantErr:    true,
		},
		{
			name:       "ends with .lock",
			branchName: "branch.lock",
			wantErr:    true,
		},
		{
			name:       "starts with slash",
			branchName: "/branch",
			wantErr:    true,
		},
		{
			name:       "ends with slash",
			branchName: "branch/",
			wantErr:    true,
		},
		{
			name:       "starts with dot",
			branchName: ".branch",
			wantErr:    true,
		},
		{
			name:       "ends with dot",
			branchName: "branch.",
			wantErr:    true,
		},
		{
			name:       "consecutive slashes",
			branchName: "branch//name",
			wantErr:    true,
		},
		{
			name:       "contains question mark",
			branchName: "branch?name",
			wantErr:    true,
		},
		{
			name:       "contains asterisk",
			branchName: "branch*name",
			wantErr:    true,
		},
		{
			name:       "contains open bracket",
			branchName: "branch[name",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.branchName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBranchName(%q) error = %v, wantErr %v", tt.branchName, err, tt.wantErr)
			}
		})
	}
}

func TestGetIntegrationBranchField(t *testing.T) {
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
			name:        "field at beginning",
			description: "integration_branch: klauern/PROJ-123/RA-epic\nSome description",
			want:        "klauern/PROJ-123/RA-epic",
		},
		{
			name:        "field in middle",
			description: "Some text\nintegration_branch: custom/branch\nMore text",
			want:        "custom/branch",
		},
		{
			name:        "field with extra whitespace",
			description: "  integration_branch:   spaced/branch  \nOther content",
			want:        "spaced/branch",
		},
		{
			name:        "no integration_branch field",
			description: "Just a plain description\nWith multiple lines",
			want:        "",
		},
		{
			name:        "mixed case field name",
			description: "Integration_branch: CamelCase/branch",
			want:        "CamelCase/branch",
		},
		{
			name:        "default format",
			description: "integration_branch: integration/gt-epic\nEpic for auth work",
			want:        "integration/gt-epic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIntegrationBranchField(tt.description)
			if got != tt.want {
				t.Errorf("getIntegrationBranchField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetRigGit(t *testing.T) {
	t.Run("bare repo exists", func(t *testing.T) {
		tmp := t.TempDir()
		bareRepo := filepath.Join(tmp, ".repo.git")
		if err := os.Mkdir(bareRepo, 0o755); err != nil {
			t.Fatal(err)
		}

		g, err := getRigGit(tmp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g == nil {
			t.Fatal("expected non-nil Git")
		}
	})

	t.Run("mayor/rig exists without bare repo", func(t *testing.T) {
		tmp := t.TempDir()
		mayorRig := filepath.Join(tmp, "mayor", "rig")
		if err := os.MkdirAll(mayorRig, 0o755); err != nil {
			t.Fatal(err)
		}

		g, err := getRigGit(tmp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g == nil {
			t.Fatal("expected non-nil Git")
		}
	})

	t.Run("neither exists returns error", func(t *testing.T) {
		tmp := t.TempDir()

		_, err := getRigGit(tmp)
		if err == nil {
			t.Fatal("expected error for empty directory")
		}
		if !strings.Contains(err.Error(), "no repo base found") {
			t.Errorf("expected 'no repo base found' error, got: %v", err)
		}
	})

	t.Run("bare repo takes precedence over mayor/rig", func(t *testing.T) {
		tmp := t.TempDir()
		bareRepo := filepath.Join(tmp, ".repo.git")
		if err := os.Mkdir(bareRepo, 0o755); err != nil {
			t.Fatal(err)
		}
		mayorRig := filepath.Join(tmp, "mayor", "rig")
		if err := os.MkdirAll(mayorRig, 0o755); err != nil {
			t.Fatal(err)
		}

		g, err := getRigGit(tmp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g == nil {
			t.Fatal("expected non-nil Git")
		}
		// When bare repo exists, WorkDir() returns "" (bare repo mode)
		if g.WorkDir() != "" {
			t.Errorf("expected empty WorkDir for bare repo, got %q", g.WorkDir())
		}
	})
}

func TestGetIntegrationBranchTemplate(t *testing.T) {
	t.Run("CLI override provided", func(t *testing.T) {
		tmp := t.TempDir()
		got := getIntegrationBranchTemplate(tmp, "custom/{epic}")
		if got != "custom/{epic}" {
			t.Errorf("got %q, want %q", got, "custom/{epic}")
		}
	})

	t.Run("config has template", func(t *testing.T) {
		tmp := t.TempDir()
		settingsDir := filepath.Join(tmp, "settings")
		if err := os.Mkdir(settingsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := map[string]interface{}{
			"type":    "rig-settings",
			"version": 1,
			"merge_queue": map[string]interface{}{
				"integration_branch_template": "{prefix}/{epic}",
			},
		}
		data, _ := json.Marshal(cfg)
		if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		got := getIntegrationBranchTemplate(tmp, "")
		if got != "{prefix}/{epic}" {
			t.Errorf("got %q, want %q", got, "{prefix}/{epic}")
		}
	})

	t.Run("config exists but no template returns default", func(t *testing.T) {
		tmp := t.TempDir()
		settingsDir := filepath.Join(tmp, "settings")
		if err := os.Mkdir(settingsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := map[string]interface{}{
			"type":        "rig-settings",
			"version":     1,
			"merge_queue": map[string]interface{}{},
		}
		data, _ := json.Marshal(cfg)
		if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		got := getIntegrationBranchTemplate(tmp, "")
		if got != defaultIntegrationBranchTemplate {
			t.Errorf("got %q, want %q", got, defaultIntegrationBranchTemplate)
		}
	})

	t.Run("no config file returns default", func(t *testing.T) {
		tmp := t.TempDir()
		got := getIntegrationBranchTemplate(tmp, "")
		if got != defaultIntegrationBranchTemplate {
			t.Errorf("got %q, want %q", got, defaultIntegrationBranchTemplate)
		}
	})
}

func TestIsReadyToLand(t *testing.T) {
	tests := []struct {
		name           string
		aheadCount     int
		childrenTotal  int
		childrenClosed int
		pendingMRCount int
		want           bool
	}{
		{
			name:           "all conditions met",
			aheadCount:     3,
			childrenTotal:  5,
			childrenClosed: 5,
			pendingMRCount: 0,
			want:           true,
		},
		{
			name:           "no commits ahead of main",
			aheadCount:     0,
			childrenTotal:  5,
			childrenClosed: 5,
			pendingMRCount: 0,
			want:           false,
		},
		{
			name:           "no children (empty epic)",
			aheadCount:     3,
			childrenTotal:  0,
			childrenClosed: 0,
			pendingMRCount: 0,
			want:           false,
		},
		{
			name:           "not all children closed",
			aheadCount:     3,
			childrenTotal:  5,
			childrenClosed: 3,
			pendingMRCount: 0,
			want:           false,
		},
		{
			name:           "pending MRs still open",
			aheadCount:     3,
			childrenTotal:  5,
			childrenClosed: 5,
			pendingMRCount: 2,
			want:           false,
		},
		{
			name:           "single child closed with commits",
			aheadCount:     1,
			childrenTotal:  1,
			childrenClosed: 1,
			pendingMRCount: 0,
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isReadyToLand(tt.aheadCount, tt.childrenTotal, tt.childrenClosed, tt.pendingMRCount)
			if got != tt.want {
				t.Errorf("isReadyToLand(%d, %d, %d, %d) = %v, want %v",
					tt.aheadCount, tt.childrenTotal, tt.childrenClosed, tt.pendingMRCount, got, tt.want)
			}
		})
	}
}

// TestResolveEpicTarget verifies that the --epic flag resolution uses the configured
// integration branch template rather than hardcoding "integration/" prefix.
// This is the regression test for the bug where mq_submit.go used:
//
//	target = "integration/" + mqSubmitEpic  // WRONG: ignores custom template
//
// The fix uses getIntegrationBranchTemplate + buildIntegrationBranchName instead.
func TestResolveEpicTarget(t *testing.T) {
	tests := []struct {
		name     string
		epicID   string
		template string // empty means default template (from getIntegrationBranchTemplate)
		want     string
	}{
		{
			name:     "default template produces integration/ prefix",
			epicID:   "gt-epic",
			template: "", // will use defaultIntegrationBranchTemplate
			want:     "integration/gt-epic",
		},
		{
			name:     "custom prefix/epic template",
			epicID:   "gt-epic",
			template: "{prefix}/{epic}",
			want:     "gt/gt-epic",
		},
		{
			name:     "custom feature prefix template",
			epicID:   "proj-123",
			template: "feature/{epic}",
			want:     "feature/proj-123",
		},
		{
			name:     "template with no placeholder prefix",
			epicID:   "gt-abc",
			template: "release/{epic}",
			want:     "release/gt-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()

			if tt.template != "" {
				// Write config with custom template
				settingsDir := filepath.Join(tmp, "settings")
				if err := os.Mkdir(settingsDir, 0o755); err != nil {
					t.Fatal(err)
				}
				cfg := map[string]interface{}{
					"type":    "rig-settings",
					"version": 1,
					"merge_queue": map[string]interface{}{
						"integration_branch_template": tt.template,
					},
				}
				data, _ := json.Marshal(cfg)
				if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			// This mirrors the fixed production code in mq_submit.go:
			//   template := getIntegrationBranchTemplate(rigPath, "")
			//   target = buildIntegrationBranchName(template, mqSubmitEpic)
			template := getIntegrationBranchTemplate(tmp, "")
			got := buildIntegrationBranchName(template, tt.epicID)

			if got != tt.want {
				t.Errorf("resolveEpicTarget(%q) with template %q = %q, want %q",
					tt.epicID, template, got, tt.want)
			}
		})
	}
}
