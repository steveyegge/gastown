package beads

import (
	"fmt"
	"testing"
)

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
			name:        "field present",
			description: "integration_branch: integration/gt-epic",
			want:        "integration/gt-epic",
		},
		{
			name:        "field with surrounding text",
			description: "Some description\nintegration_branch: feature/my-branch\nMore text",
			want:        "feature/my-branch",
		},
		{
			name:        "case insensitive",
			description: "INTEGRATION_BRANCH: integration/GT-123",
			want:        "integration/GT-123",
		},
		{
			name:        "field not present",
			description: "Some description\nbase_branch: develop\n",
			want:        "",
		},
		{
			name:        "field with extra whitespace",
			description: "  integration_branch:   integration/spaced  ",
			want:        "integration/spaced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetIntegrationBranchField(tt.description)
			if got != tt.want {
				t.Errorf("GetIntegrationBranchField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetBaseBranchField(t *testing.T) {
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
			name:        "field present",
			description: "base_branch: develop",
			want:        "develop",
		},
		{
			name:        "alongside integration_branch",
			description: "integration_branch: integration/gt-epic\nbase_branch: release/v2",
			want:        "release/v2",
		},
		{
			name:        "field not present",
			description: "integration_branch: integration/gt-epic",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBaseBranchField(tt.description)
			if got != tt.want {
				t.Errorf("GetBaseBranchField() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
			name:        "add to existing",
			description: "Some description",
			branchName:  "integration/gt-epic",
			want:        "integration_branch: integration/gt-epic\nSome description",
		},
		{
			name:        "replace existing",
			description: "integration_branch: old-branch\nSome description",
			branchName:  "integration/new-branch",
			want:        "integration_branch: integration/new-branch\nSome description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddIntegrationBranchField(tt.description, tt.branchName)
			if got != tt.want {
				t.Errorf("AddIntegrationBranchField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddBaseBranchField(t *testing.T) {
	tests := []struct {
		name        string
		description string
		baseBranch  string
		want        string
	}{
		{
			name:        "empty description",
			description: "",
			baseBranch:  "develop",
			want:        "base_branch: develop",
		},
		{
			name:        "add to existing",
			description: "integration_branch: integration/gt-epic",
			baseBranch:  "develop",
			want:        "base_branch: develop\nintegration_branch: integration/gt-epic",
		},
		{
			name:        "replace existing",
			description: "base_branch: old\nSome text",
			baseBranch:  "release/v2",
			want:        "base_branch: release/v2\nSome text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddBaseBranchField(tt.description, tt.baseBranch)
			if got != tt.want {
				t.Errorf("AddBaseBranchField() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchSegment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple lowercase", input: "add-user-auth", want: "add-user-auth"},
		{name: "mixed case", input: "Add User Auth", want: "add-user-auth"},
		{name: "underscores", input: "add_user_auth", want: "add-user-auth"},
		{name: "special chars", input: "fix: handle @mentions & #tags!", want: "fix-handle-mentions-tags"},
		{name: "consecutive hyphens", input: "foo---bar", want: "foo-bar"},
		{name: "leading/trailing special", input: "  --hello world--  ", want: "hello-world"},
		{name: "empty string", input: "", want: ""},
		{name: "already clean", input: "clean-branch-name", want: "clean-branch-name"},
		{name: "unicode chars", input: "café résumé", want: "caf-r-sum"},
		{
			name:  "long string truncated to 60",
			input: "this-is-a-very-long-branch-name-that-exceeds-the-sixty-character-limit-for-readability",
			want:  "this-is-a-very-long-branch-name-that-exceeds-the-sixty-chara",
		},
		{
			// 59 a's + space + "bbbbb" → sanitized to 59 a's + "-bbbbb" (65 chars)
			// Truncated to 60 → "aaa...a-" → trailing hyphen trimmed → 59 a's
			name:  "truncation trims trailing hyphen",
			input: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa bbbbb",
			want:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{name: "numbers preserved", input: "v2.0 release", want: "v2-0-release"},
		{name: "only special chars", input: "!@#$%", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeBranchSegment(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeBranchSegment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildIntegrationBranchName(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		epicID    string
		epicTitle string
		want      string
	}{
		{
			name:      "default template uses title",
			template:  "",
			epicID:    "gt-epic",
			epicTitle: "Add User Authentication",
			want:      "integration/add-user-authentication",
		},
		{
			name:      "custom template with epic",
			template:  "feature/{epic}",
			epicID:    "RA-123",
			epicTitle: "Some Feature",
			want:      "feature/RA-123",
		},
		{
			name:      "custom template with prefix",
			template:  "{prefix}/integration/{epic}",
			epicID:    "PROJ-456",
			epicTitle: "Big Epic",
			want:      "PROJ/integration/PROJ-456",
		},
		{
			name:      "template with title variable",
			template:  "integration/{title}",
			epicID:    "gt-123",
			epicTitle: "Fix Login Bug",
			want:      "integration/fix-login-bug",
		},
		{
			name:      "template with both epic and title",
			template:  "{epic}/{title}",
			epicID:    "gt-42",
			epicTitle: "Refactor DB Layer",
			want:      "gt-42/refactor-db-layer",
		},
		{
			name:      "empty title with title template falls back to epic ID",
			template:  "integration/{title}",
			epicID:    "gt-99",
			epicTitle: "",
			want:      "integration/gt-99",
		},
		{
			name:      "special-chars-only title falls back to epic ID",
			template:  "integration/{title}",
			epicID:    "gt-42",
			epicTitle: "!@#$%",
			want:      "integration/gt-42",
		},
		{
			name:      "old default template still works",
			template:  "integration/{epic}",
			epicID:    "gt-epic",
			epicTitle: "Ignored Title",
			want:      "integration/gt-epic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildIntegrationBranchName(tt.template, tt.epicID, tt.epicTitle)
			if got != tt.want {
				t.Errorf("BuildIntegrationBranchName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractEpicPrefix(t *testing.T) {
	tests := []struct {
		epicID string
		want   string
	}{
		{"RA-123", "RA"},
		{"PROJ-456", "PROJ"},
		{"abc", "abc"},
		{"a-b-c", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.epicID, func(t *testing.T) {
			got := ExtractEpicPrefix(tt.epicID)
			if got != tt.want {
				t.Errorf("ExtractEpicPrefix(%q) = %q, want %q", tt.epicID, got, tt.want)
			}
		})
	}
}

// mockBranchChecker implements BranchChecker for testing.
type mockBranchChecker struct {
	localBranches  map[string]bool
	remoteBranches map[string]bool // key: "remote/branch"
	localErr       error           // if set, BranchExists returns this error
	remoteErr      error           // if set, RemoteBranchExists returns this error
}

func (m *mockBranchChecker) BranchExists(name string) (bool, error) {
	if m.localErr != nil {
		return false, m.localErr
	}
	return m.localBranches[name], nil
}

func (m *mockBranchChecker) RemoteBranchExists(remote, name string) (bool, error) {
	if m.remoteErr != nil {
		return false, m.remoteErr
	}
	key := remote + "/" + name
	return m.remoteBranches[key], nil
}

// mockIssueShower implements IssueShower for testing DetectIntegrationBranch.
type mockIssueShower struct {
	issues map[string]*Issue
}

func (m *mockIssueShower) Show(id string) (*Issue, error) {
	issue, ok := m.issues[id]
	if !ok {
		return nil, fmt.Errorf("issue %s not found", id)
	}
	return issue, nil
}

func TestDetectIntegrationBranch(t *testing.T) {
	t.Run("child of epic with metadata and remote branch", func(t *testing.T) {
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-epic"},
			"gt-epic": {ID: "gt-epic", Type: "epic", Description: "integration_branch: custom/branch"},
		}}
		checker := &mockBranchChecker{
			localBranches:  map[string]bool{"custom/branch": true},
			remoteBranches: map[string]bool{"origin/custom/branch": true},
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "custom/branch" {
			t.Errorf("got %q, want %q", got, "custom/branch")
		}
	})

	t.Run("child of epic without metadata falls back to naming on remote", func(t *testing.T) {
		// Title "gt-epic" sanitizes to "gt-epic", matching the remote branch name
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-epic"},
			"gt-epic": {ID: "gt-epic", Type: "epic", Title: "gt-epic", Description: "Some epic description"},
		}}
		checker := &mockBranchChecker{
			remoteBranches: map[string]bool{"origin/integration/gt-epic": true},
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "integration/gt-epic" {
			t.Errorf("got %q, want %q", got, "integration/gt-epic")
		}
	})

	t.Run("nested parent chain task-to-subtask-to-epic", func(t *testing.T) {
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-sub":  {ID: "gt-sub", Type: "task", Parent: "gt-task"},
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-epic"},
			"gt-epic": {ID: "gt-epic", Type: "epic", Description: "integration_branch: nested/branch"},
		}}
		checker := &mockBranchChecker{
			localBranches:  map[string]bool{"nested/branch": true},
			remoteBranches: map[string]bool{"origin/nested/branch": true},
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-sub")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "nested/branch" {
			t.Errorf("got %q, want %q", got, "nested/branch")
		}
	})

	t.Run("no epic in parent chain returns empty", func(t *testing.T) {
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-other"},
			"gt-other": {ID: "gt-other", Type: "task", Parent: ""},
		}}
		checker := &mockBranchChecker{}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("epic exists but no branch found anywhere", func(t *testing.T) {
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-epic"},
			"gt-epic": {ID: "gt-epic", Type: "epic", Description: "No metadata here", Parent: ""},
		}}
		// No branches exist at all
		checker := &mockBranchChecker{}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("max depth exceeded returns empty", func(t *testing.T) {
		issues := make(map[string]*Issue)
		// Build a chain of 12 tasks (exceeds maxDepth of 10)
		for i := 0; i < 12; i++ {
			id := fmt.Sprintf("gt-%d", i)
			parent := ""
			if i < 11 {
				parent = fmt.Sprintf("gt-%d", i+1)
			}
			issues[id] = &Issue{ID: id, Type: "task", Parent: parent}
		}
		// Put an epic at the very end (unreachable within depth 10)
		issues["gt-11"].Type = "epic"
		issues["gt-11"].Description = "integration_branch: deep/branch"

		shower := &mockIssueShower{issues: issues}
		checker := &mockBranchChecker{
			localBranches:  map[string]bool{"deep/branch": true},
			remoteBranches: map[string]bool{"origin/deep/branch": true},
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string (max depth exceeded)", got)
		}
	})

	t.Run("issue not found propagates error", func(t *testing.T) {
		shower := &mockIssueShower{issues: map[string]*Issue{}}
		checker := &mockBranchChecker{}

		_, err := DetectIntegrationBranch(shower, checker, "gt-missing")
		if err == nil {
			t.Fatal("expected error for missing issue, got nil")
		}
	})

	t.Run("BranchExists error is swallowed when remote also fails (best-effort)", func(t *testing.T) {
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-epic"},
			"gt-epic": {ID: "gt-epic", Type: "epic", Description: "integration_branch: custom/branch"},
		}}
		checker := &mockBranchChecker{
			remoteErr: fmt.Errorf("network timeout"),
			localErr:  fmt.Errorf("git repo corrupted"),
		}

		// Detection is best-effort: both paths swallow errors and return ""
		result, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("expected no error (best-effort detection), got: %v", err)
		}
		if result != "" {
			t.Errorf("expected empty result when both checks fail, got %q", result)
		}
	})

	t.Run("RemoteBranchExists error is non-fatal and continues to parent", func(t *testing.T) {
		// Epic1 has no local branch and remote check errors out.
		// Epic1's parent is Epic2 which has a local branch.
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task":  {ID: "gt-task", Type: "task", Parent: "gt-epic1"},
			"gt-epic1": {ID: "gt-epic1", Type: "epic", Description: "No metadata", Parent: "gt-epic2"},
			"gt-epic2": {ID: "gt-epic2", Type: "epic", Description: "integration_branch: parent/branch"},
		}}
		checker := &mockBranchChecker{
			remoteErr:     fmt.Errorf("network timeout"),
			localBranches: map[string]bool{"parent/branch": true},
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "parent/branch" {
			t.Errorf("got %q, want %q (should skip epic1 and find epic2's branch)", got, "parent/branch")
		}
	})

	t.Run("stale local branch ignored when remote is deleted", func(t *testing.T) {
		// Local branch exists but remote was deleted — should NOT use stale local ref
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-epic"},
			"gt-epic": {ID: "gt-epic", Type: "epic", Description: "integration_branch: integration/gt-epic", Parent: ""},
		}}
		checker := &mockBranchChecker{
			localBranches:  map[string]bool{"integration/gt-epic": true},
			remoteBranches: map[string]bool{}, // remote deleted
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string (stale local ref should be ignored)", got)
		}
	})

	t.Run("local branch used as fallback when remote check fails", func(t *testing.T) {
		// Remote check errors out (network issue) — fall back to local
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-epic"},
			"gt-epic": {ID: "gt-epic", Type: "epic", Description: "integration_branch: integration/gt-epic", Parent: ""},
		}}
		checker := &mockBranchChecker{
			localBranches: map[string]bool{"integration/gt-epic": true},
			remoteErr:     fmt.Errorf("network timeout"),
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "integration/gt-epic" {
			t.Errorf("got %q, want %q (should fall back to local when remote errors)", got, "integration/gt-epic")
		}
	})

	t.Run("legacy epic-ID branch found via fallback template", func(t *testing.T) {
		// Epic has no metadata, title doesn't match the branch name.
		// The branch was created with the old "integration/{epic}" convention.
		// Detection should try {title} first, fail, then find the {epic} branch.
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task": {ID: "gt-task", Type: "task", Parent: "gt-abc"},
			"gt-abc":  {ID: "gt-abc", Type: "epic", Title: "Add User Auth", Description: "Some description"},
		}}
		checker := &mockBranchChecker{
			// Only the legacy-style branch exists
			remoteBranches: map[string]bool{"origin/integration/gt-abc": true},
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "integration/gt-abc" {
			t.Errorf("got %q, want %q (legacy fallback)", got, "integration/gt-abc")
		}
	})

	t.Run("epic without branch continues to grandparent epic", func(t *testing.T) {
		// task -> epic1 (no branch anywhere) -> epic2 (has remote branch)
		shower := &mockIssueShower{issues: map[string]*Issue{
			"gt-task":  {ID: "gt-task", Type: "task", Parent: "gt-epic1"},
			"gt-epic1": {ID: "gt-epic1", Type: "epic", Description: "No branch metadata", Parent: "gt-epic2"},
			"gt-epic2": {ID: "gt-epic2", Type: "epic", Description: "integration_branch: grandparent/branch"},
		}}
		checker := &mockBranchChecker{
			localBranches:  map[string]bool{"grandparent/branch": true},
			remoteBranches: map[string]bool{"origin/grandparent/branch": true},
		}

		got, err := DetectIntegrationBranch(shower, checker, "gt-task")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "grandparent/branch" {
			t.Errorf("got %q, want %q", got, "grandparent/branch")
		}
	})
}
