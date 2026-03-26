package refinery

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestParseOwnerRepo(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "HTTPS with .git",
			url:       "https://github.com/steveyegge/gastown.git",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "HTTPS without .git",
			url:       "https://github.com/steveyegge/gastown",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "SSH",
			url:       "git@github.com:steveyegge/gastown.git",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "SSH without .git",
			url:       "git@github.com:steveyegge/gastown",
			wantOwner: "steveyegge",
			wantRepo:  "gastown",
		},
		{
			name:      "HTTP",
			url:       "http://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "whitespace trimmed",
			url:       "  https://github.com/owner/repo.git  ",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:    "invalid URL",
			url:     "not-a-url",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseOwnerRepo(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got owner=%q repo=%q", owner, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}

func TestBuildBatchPRBody(t *testing.T) {
	t.Run("with tracked issues", func(t *testing.T) {
		issues := []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}{
			{ID: "gt-abc", Title: "Add feature X", Status: "closed"},
			{ID: "gt-def", Title: "Fix bug Y", Status: "in_progress"},
			{ID: "gt-ghi", Title: "Refactor Z", Status: "open"},
		}

		body := buildBatchPRBody(
			"My Feature Convoy",
			"owner: mayor/\nmerge: batch-pr\n\nThis convoy implements the feature.",
			issues,
		)

		// Should contain the convoy title as heading.
		if !strings.Contains(body, "## My Feature Convoy") {
			t.Error("expected convoy title heading")
		}

		// Should contain description text (not metadata).
		if !strings.Contains(body, "This convoy implements the feature.") {
			t.Error("expected convoy description text")
		}

		// Should NOT contain metadata fields.
		if strings.Contains(body, "owner: mayor/") {
			t.Error("should strip owner metadata field")
		}
		if strings.Contains(body, "merge: batch-pr") {
			t.Error("should strip merge metadata field")
		}

		// Should contain tracked issues section.
		if !strings.Contains(body, "## Tracked Issues") {
			t.Error("expected tracked issues section")
		}

		// Closed issues should be checked.
		if !strings.Contains(body, "[x] **gt-abc**: Add feature X (closed)") {
			t.Error("expected closed issue to be checked")
		}

		// Open issues should be unchecked.
		if !strings.Contains(body, "[ ] **gt-def**: Fix bug Y (in_progress)") {
			t.Error("expected in_progress issue to be unchecked")
		}
		if !strings.Contains(body, "[ ] **gt-ghi**: Refactor Z (open)") {
			t.Error("expected open issue to be unchecked")
		}

		// Issues should be sorted by ID.
		abcIdx := strings.Index(body, "gt-abc")
		defIdx := strings.Index(body, "gt-def")
		ghiIdx := strings.Index(body, "gt-ghi")
		if abcIdx > defIdx || defIdx > ghiIdx {
			t.Error("issues should be sorted by ID")
		}
	})

	t.Run("no tracked issues", func(t *testing.T) {
		body := buildBatchPRBody("Empty Convoy", "Some description", nil)

		if !strings.Contains(body, "## Empty Convoy") {
			t.Error("expected convoy title heading")
		}
		if strings.Contains(body, "## Tracked Issues") {
			t.Error("should not have tracked issues section when empty")
		}
	})

	t.Run("strips all metadata fields", func(t *testing.T) {
		desc := "owner: mayor/\nnotify: witness/\nmolecule: gt-mol-abc\n" +
			"merge: batch-pr\nbase_branch: main\nintegration_branch: integration/feat\n" +
			"pr_url: https://github.com/o/r/pull/1\npr_number: 42\n" +
			"watchers: a,b\nnudge_watchers: c\n\nActual description."

		body := buildBatchPRBody("Test", desc, nil)

		for _, field := range []string{
			"owner:", "notify:", "molecule:", "merge:", "base_branch:",
			"integration_branch:", "pr_url:", "pr_number:", "watchers:", "nudge_watchers:",
		} {
			if strings.Contains(body, field) {
				t.Errorf("should strip metadata field %q", field)
			}
		}

		if !strings.Contains(body, "Actual description.") {
			t.Error("should keep non-metadata content")
		}
	})

	t.Run("empty status defaults to pending", func(t *testing.T) {
		issues := []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		}{
			{ID: "gt-xyz", Title: "Task", Status: ""},
		}

		body := buildBatchPRBody("Convoy", "", issues)

		if !strings.Contains(body, "(pending)") {
			t.Error("expected empty status to show as 'pending'")
		}
	})
}

func TestUpdateBatchPRDescription_SkipsNonConvoyMRs(t *testing.T) {
	// MR with no ConvoyID should be a no-op.
	e := &Engineer{
		rig:    &rig.Rig{Name: "test-rig", Path: t.TempDir()},
		config: DefaultMergeQueueConfig(),
		output: &strings.Builder{},
	}

	mr := &MRInfo{
		ID:     "gt-mr1",
		Target: "integration/feat",
		// No ConvoyID — should return immediately.
	}

	// Should not panic or error.
	e.updateBatchPRDescription(mr)

	output := e.output.(*strings.Builder).String()
	if strings.Contains(output, "Batch-PR") {
		t.Error("should not attempt PR update for non-convoy MR")
	}
}

func TestUpdateBatchPRDescription_SkipsDefaultBranchTarget(t *testing.T) {
	e := &Engineer{
		rig:    &rig.Rig{Name: "test-rig", Path: t.TempDir()},
		config: DefaultMergeQueueConfig(),
		output: &strings.Builder{},
	}

	mr := &MRInfo{
		ID:       "gt-mr1",
		Target:   "main", // Default branch — not an integration branch.
		ConvoyID: "hq-cv-abc",
	}

	e.updateBatchPRDescription(mr)

	output := e.output.(*strings.Builder).String()
	if strings.Contains(output, "Batch-PR") {
		t.Error("should not attempt PR update for default branch target")
	}
}
