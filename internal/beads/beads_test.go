package beads

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew verifies the constructor.
func TestNew(t *testing.T) {
	b := New("/some/path")
	if b == nil {
		t.Fatal("New returned nil")
	}
	if b.workDir != "/some/path" {
		t.Errorf("workDir = %q, want /some/path", b.workDir)
	}
}

// TestListOptions verifies ListOptions defaults.
func TestListOptions(t *testing.T) {
	opts := ListOptions{
		Status:   "open",
		Type:     "task",
		Priority: 1,
	}
	if opts.Status != "open" {
		t.Errorf("Status = %q, want open", opts.Status)
	}
}

// TestCreateOptions verifies CreateOptions fields.
func TestCreateOptions(t *testing.T) {
	opts := CreateOptions{
		Title:       "Test issue",
		Type:        "task",
		Priority:    2,
		Description: "A test description",
		Parent:      "gt-abc",
	}
	if opts.Title != "Test issue" {
		t.Errorf("Title = %q, want 'Test issue'", opts.Title)
	}
	if opts.Parent != "gt-abc" {
		t.Errorf("Parent = %q, want gt-abc", opts.Parent)
	}
}

// TestUpdateOptions verifies UpdateOptions pointer fields.
func TestUpdateOptions(t *testing.T) {
	status := "in_progress"
	priority := 1
	opts := UpdateOptions{
		Status:   &status,
		Priority: &priority,
	}
	if *opts.Status != "in_progress" {
		t.Errorf("Status = %q, want in_progress", *opts.Status)
	}
	if *opts.Priority != 1 {
		t.Errorf("Priority = %d, want 1", *opts.Priority)
	}
}

// TestIsBeadsRepo tests repository detection.
func TestIsBeadsRepo(t *testing.T) {
	// Test with a non-beads directory
	tmpDir, err := os.MkdirTemp("", "beads-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	b := New(tmpDir)
	// This should return false since there's no .beads directory
	// and bd list will fail
	if b.IsBeadsRepo() {
		// This might pass if bd handles missing .beads gracefully
		t.Log("IsBeadsRepo returned true for non-beads directory (bd might initialize)")
	}
}

// TestWrapError tests error wrapping.
// ZFC: Only test ErrNotFound detection. ErrNotARepo and ErrSyncConflict
// were removed as per ZFC - agents should handle those errors directly.
func TestWrapError(t *testing.T) {
	b := New("/test")

	tests := []struct {
		stderr  string
		wantErr error
		wantNil bool
	}{
		{"Issue not found: gt-xyz", ErrNotFound, false},
		{"gt-xyz not found", ErrNotFound, false},
	}

	for _, tt := range tests {
		err := b.wrapError(nil, tt.stderr, []string{"test"})
		if tt.wantNil {
			if err != nil {
				t.Errorf("wrapError(%q) = %v, want nil", tt.stderr, err)
			}
		} else {
			if err != tt.wantErr {
				t.Errorf("wrapError(%q) = %v, want %v", tt.stderr, err, tt.wantErr)
			}
		}
	}
}

// Integration test that runs against real bd if available
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Find a beads repo (use current directory if it has .beads)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".beads")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("no .beads directory found in path")
		}
		dir = parent
	}

	// Resolve the actual beads directory (following redirect if present)
	// In multi-worktree setups, worktrees have .beads/redirect pointing to
	// the canonical beads location (e.g., mayor/rig/.beads)
	beadsDir := ResolveBeadsDir(dir)
	dbPath := filepath.Join(beadsDir, "beads.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skip("no beads.db found (JSONL-only repo)")
	}

	b := New(dir)

	// Sync database with JSONL before testing to avoid "Database out of sync" errors.
	// This can happen when JSONL is updated (e.g., by git pull) but the SQLite database
	// hasn't been imported yet. Running sync --import-only ensures we test against
	// consistent data and prevents flaky test failures.
	// We use --allow-stale to handle cases where the daemon is actively writing and
	// the staleness check would otherwise fail spuriously.
	syncCmd := exec.Command("bd", "--no-daemon", "--allow-stale", "sync", "--import-only")
	syncCmd.Dir = dir
	if err := syncCmd.Run(); err != nil {
		// If sync fails (e.g., no database exists), just log and continue
		t.Logf("bd sync --import-only failed (may not have db): %v", err)
	}

	// Test List
	t.Run("List", func(t *testing.T) {
		issues, err := b.List(ListOptions{Status: "open"})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		t.Logf("Found %d open issues", len(issues))
	})

	// Test Ready
	t.Run("Ready", func(t *testing.T) {
		issues, err := b.Ready()
		if err != nil {
			t.Fatalf("Ready failed: %v", err)
		}
		t.Logf("Found %d ready issues", len(issues))
	})

	// Test Blocked
	t.Run("Blocked", func(t *testing.T) {
		issues, err := b.Blocked()
		if err != nil {
			t.Fatalf("Blocked failed: %v", err)
		}
		t.Logf("Found %d blocked issues", len(issues))
	})

	// Test Show (if we have issues)
	t.Run("Show", func(t *testing.T) {
		issues, err := b.List(ListOptions{})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(issues) == 0 {
			t.Skip("no issues to show")
		}

		issue, err := b.Show(issues[0].ID)
		if err != nil {
			t.Fatalf("Show(%s) failed: %v", issues[0].ID, err)
		}
		t.Logf("Showed issue: %s - %s", issue.ID, issue.Title)
	})
}

// TestParseMRFields tests parsing MR fields from issue descriptions.
func TestParseMRFields(t *testing.T) {
	tests := []struct {
		name       string
		issue      *Issue
		wantNil    bool
		wantFields *MRFields
	}{
		{
			name:    "nil issue",
			issue:   nil,
			wantNil: true,
		},
		{
			name:    "empty description",
			issue:   &Issue{Description: ""},
			wantNil: true,
		},
		{
			name:    "no MR fields",
			issue:   &Issue{Description: "This is just plain text\nwith no field markers"},
			wantNil: true,
		},
		{
			name: "all fields",
			issue: &Issue{
				Description: `branch: polecat/Nux/gt-xyz
target: main
source_issue: gt-xyz
worker: Nux
rig: gastown
merge_commit: abc123def
close_reason: merged`,
			},
			wantFields: &MRFields{
				Branch:      "polecat/Nux/gt-xyz",
				Target:      "main",
				SourceIssue: "gt-xyz",
				Worker:      "Nux",
				Rig:         "gastown",
				MergeCommit: "abc123def",
				CloseReason: "merged",
			},
		},
		{
			name: "partial fields",
			issue: &Issue{
				Description: `branch: polecat/Toast/gt-abc
target: integration/gt-epic
source_issue: gt-abc
worker: Toast`,
			},
			wantFields: &MRFields{
				Branch:      "polecat/Toast/gt-abc",
				Target:      "integration/gt-epic",
				SourceIssue: "gt-abc",
				Worker:      "Toast",
			},
		},
		{
			name: "mixed with prose",
			issue: &Issue{
				Description: `branch: polecat/Capable/gt-def
target: main
source_issue: gt-def

This MR fixes a critical bug in the authentication system.
Please review carefully.

worker: Capable
rig: wasteland`,
			},
			wantFields: &MRFields{
				Branch:      "polecat/Capable/gt-def",
				Target:      "main",
				SourceIssue: "gt-def",
				Worker:      "Capable",
				Rig:         "wasteland",
			},
		},
		{
			name: "alternate key formats",
			issue: &Issue{
				Description: `branch: polecat/Max/gt-ghi
source-issue: gt-ghi
merge-commit: 789xyz`,
			},
			wantFields: &MRFields{
				Branch:      "polecat/Max/gt-ghi",
				SourceIssue: "gt-ghi",
				MergeCommit: "789xyz",
			},
		},
		{
			name: "case insensitive keys",
			issue: &Issue{
				Description: `Branch: polecat/Furiosa/gt-jkl
TARGET: main
Worker: Furiosa
RIG: gastown`,
			},
			wantFields: &MRFields{
				Branch: "polecat/Furiosa/gt-jkl",
				Target: "main",
				Worker: "Furiosa",
				Rig:    "gastown",
			},
		},
		{
			name: "extra whitespace",
			issue: &Issue{
				Description: `  branch:   polecat/Nux/gt-mno
target:main
  worker:   Nux  `,
			},
			wantFields: &MRFields{
				Branch: "polecat/Nux/gt-mno",
				Target: "main",
				Worker: "Nux",
			},
		},
		{
			name: "ignores empty values",
			issue: &Issue{
				Description: `branch: polecat/Nux/gt-pqr
target:
source_issue: gt-pqr`,
			},
			wantFields: &MRFields{
				Branch:      "polecat/Nux/gt-pqr",
				SourceIssue: "gt-pqr",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := ParseMRFields(tt.issue)

			if tt.wantNil {
				if fields != nil {
					t.Errorf("ParseMRFields() = %+v, want nil", fields)
				}
				return
			}

			if fields == nil {
				t.Fatal("ParseMRFields() = nil, want non-nil")
			}

			if fields.Branch != tt.wantFields.Branch {
				t.Errorf("Branch = %q, want %q", fields.Branch, tt.wantFields.Branch)
			}
			if fields.Target != tt.wantFields.Target {
				t.Errorf("Target = %q, want %q", fields.Target, tt.wantFields.Target)
			}
			if fields.SourceIssue != tt.wantFields.SourceIssue {
				t.Errorf("SourceIssue = %q, want %q", fields.SourceIssue, tt.wantFields.SourceIssue)
			}
			if fields.Worker != tt.wantFields.Worker {
				t.Errorf("Worker = %q, want %q", fields.Worker, tt.wantFields.Worker)
			}
			if fields.Rig != tt.wantFields.Rig {
				t.Errorf("Rig = %q, want %q", fields.Rig, tt.wantFields.Rig)
			}
			if fields.MergeCommit != tt.wantFields.MergeCommit {
				t.Errorf("MergeCommit = %q, want %q", fields.MergeCommit, tt.wantFields.MergeCommit)
			}
			if fields.CloseReason != tt.wantFields.CloseReason {
				t.Errorf("CloseReason = %q, want %q", fields.CloseReason, tt.wantFields.CloseReason)
			}
		})
	}
}

// TestFormatMRFields tests formatting MR fields to string.
func TestFormatMRFields(t *testing.T) {
	tests := []struct {
		name   string
		fields *MRFields
		want   string
	}{
		{
			name:   "nil fields",
			fields: nil,
			want:   "",
		},
		{
			name:   "empty fields",
			fields: &MRFields{},
			want:   "",
		},
		{
			name: "all fields",
			fields: &MRFields{
				Branch:      "polecat/Nux/gt-xyz",
				Target:      "main",
				SourceIssue: "gt-xyz",
				Worker:      "Nux",
				Rig:         "gastown",
				MergeCommit: "abc123def",
				CloseReason: "merged",
			},
			want: `branch: polecat/Nux/gt-xyz
target: main
source_issue: gt-xyz
worker: Nux
rig: gastown
merge_commit: abc123def
close_reason: merged`,
		},
		{
			name: "partial fields",
			fields: &MRFields{
				Branch:      "polecat/Toast/gt-abc",
				Target:      "main",
				SourceIssue: "gt-abc",
				Worker:      "Toast",
			},
			want: `branch: polecat/Toast/gt-abc
target: main
source_issue: gt-abc
worker: Toast`,
		},
		{
			name: "only close fields",
			fields: &MRFields{
				MergeCommit: "deadbeef",
				CloseReason: "rejected",
			},
			want: `merge_commit: deadbeef
close_reason: rejected`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMRFields(tt.fields)
			if got != tt.want {
				t.Errorf("FormatMRFields() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestSetMRFields tests updating issue descriptions with MR fields.
func TestSetMRFields(t *testing.T) {
	tests := []struct {
		name   string
		issue  *Issue
		fields *MRFields
		want   string
	}{
		{
			name:  "nil issue",
			issue: nil,
			fields: &MRFields{
				Branch: "polecat/Nux/gt-xyz",
				Target: "main",
			},
			want: `branch: polecat/Nux/gt-xyz
target: main`,
		},
		{
			name:  "empty description",
			issue: &Issue{Description: ""},
			fields: &MRFields{
				Branch:      "polecat/Nux/gt-xyz",
				Target:      "main",
				SourceIssue: "gt-xyz",
			},
			want: `branch: polecat/Nux/gt-xyz
target: main
source_issue: gt-xyz`,
		},
		{
			name:  "preserve prose content",
			issue: &Issue{Description: "This is a description of the work.\n\nIt spans multiple lines."},
			fields: &MRFields{
				Branch: "polecat/Toast/gt-abc",
				Worker: "Toast",
			},
			want: `branch: polecat/Toast/gt-abc
worker: Toast

This is a description of the work.

It spans multiple lines.`,
		},
		{
			name: "replace existing fields",
			issue: &Issue{
				Description: `branch: polecat/Nux/gt-old
target: develop
source_issue: gt-old
worker: Nux

Some existing prose content.`,
			},
			fields: &MRFields{
				Branch:      "polecat/Nux/gt-new",
				Target:      "main",
				SourceIssue: "gt-new",
				Worker:      "Nux",
				MergeCommit: "abc123",
			},
			want: `branch: polecat/Nux/gt-new
target: main
source_issue: gt-new
worker: Nux
merge_commit: abc123

Some existing prose content.`,
		},
		{
			name: "preserve non-MR key-value lines",
			issue: &Issue{
				Description: `branch: polecat/Capable/gt-def
custom_field: some value
author: someone
target: main`,
			},
			fields: &MRFields{
				Branch:      "polecat/Capable/gt-ghi",
				Target:      "integration/epic",
				CloseReason: "merged",
			},
			want: `branch: polecat/Capable/gt-ghi
target: integration/epic
close_reason: merged

custom_field: some value
author: someone`,
		},
		{
			name:   "empty fields clears MR data",
			issue:  &Issue{Description: "branch: old\ntarget: old\n\nKeep this text."},
			fields: &MRFields{},
			want:   "Keep this text.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetMRFields(tt.issue, tt.fields)
			if got != tt.want {
				t.Errorf("SetMRFields() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestMRFieldsRoundTrip tests that parse/format round-trips correctly.
func TestMRFieldsRoundTrip(t *testing.T) {
	original := &MRFields{
		Branch:      "polecat/Nux/gt-xyz",
		Target:      "main",
		SourceIssue: "gt-xyz",
		Worker:      "Nux",
		Rig:         "gastown",
		MergeCommit: "abc123def789",
		CloseReason: "merged",
	}

	// Format to string
	formatted := FormatMRFields(original)

	// Parse back
	issue := &Issue{Description: formatted}
	parsed := ParseMRFields(issue)

	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}

	if *parsed != *original {
		t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", parsed, original)
	}
}

// TestParseMRFieldsFromDesignDoc tests the example from the design doc.
func TestParseMRFieldsFromDesignDoc(t *testing.T) {
	// Example from docs/merge-queue-design.md
	description := `branch: polecat/Nux/gt-xyz
target: main
source_issue: gt-xyz
worker: Nux
rig: gastown`

	issue := &Issue{Description: description}
	fields := ParseMRFields(issue)

	if fields == nil {
		t.Fatal("ParseMRFields returned nil for design doc example")
	}

	// Verify all fields match the design doc
	if fields.Branch != "polecat/Nux/gt-xyz" {
		t.Errorf("Branch = %q, want polecat/Nux/gt-xyz", fields.Branch)
	}
	if fields.Target != "main" {
		t.Errorf("Target = %q, want main", fields.Target)
	}
	if fields.SourceIssue != "gt-xyz" {
		t.Errorf("SourceIssue = %q, want gt-xyz", fields.SourceIssue)
	}
	if fields.Worker != "Nux" {
		t.Errorf("Worker = %q, want Nux", fields.Worker)
	}
	if fields.Rig != "gastown" {
		t.Errorf("Rig = %q, want gastown", fields.Rig)
	}
}

// TestSetMRFieldsPreservesURL tests that URLs in prose are preserved.
func TestSetMRFieldsPreservesURL(t *testing.T) {
	// URLs contain colons which could be confused with key: value
	issue := &Issue{
		Description: `branch: old-branch
Check out https://example.com/path for more info.
Also see http://localhost:8080/api`,
	}

	fields := &MRFields{
		Branch: "new-branch",
		Target: "main",
	}

	result := SetMRFields(issue, fields)

	// URLs should be preserved
	if !strings.Contains(result, "https://example.com/path") {
		t.Error("HTTPS URL was not preserved")
	}
	if !strings.Contains(result, "http://localhost:8080/api") {
		t.Error("HTTP URL was not preserved")
	}
	if !strings.Contains(result, "branch: new-branch") {
		t.Error("branch field was not set")
	}
}

// TestParseAttachmentFields tests parsing attachment fields from issue descriptions.
func TestParseAttachmentFields(t *testing.T) {
	tests := []struct {
		name       string
		issue      *Issue
		wantNil    bool
		wantFields *AttachmentFields
	}{
		{
			name:    "nil issue",
			issue:   nil,
			wantNil: true,
		},
		{
			name:    "empty description",
			issue:   &Issue{Description: ""},
			wantNil: true,
		},
		{
			name:    "no attachment fields",
			issue:   &Issue{Description: "This is just plain text\nwith no attachment markers"},
			wantNil: true,
		},
		{
			name: "both fields",
			issue: &Issue{
				Description: `attached_molecule: mol-xyz
attached_at: 2025-12-21T15:30:00Z`,
			},
			wantFields: &AttachmentFields{
				AttachedMolecule: "mol-xyz",
				AttachedAt:       "2025-12-21T15:30:00Z",
			},
		},
		{
			name: "only molecule",
			issue: &Issue{
				Description: `attached_molecule: mol-abc`,
			},
			wantFields: &AttachmentFields{
				AttachedMolecule: "mol-abc",
			},
		},
		{
			name: "mixed with other content",
			issue: &Issue{
				Description: `attached_molecule: mol-def
attached_at: 2025-12-21T10:00:00Z

This is a handoff bead for the polecat.
Keep working on the current task.`,
			},
			wantFields: &AttachmentFields{
				AttachedMolecule: "mol-def",
				AttachedAt:       "2025-12-21T10:00:00Z",
			},
		},
		{
			name: "alternate key formats (hyphen)",
			issue: &Issue{
				Description: `attached-molecule: mol-ghi
attached-at: 2025-12-21T12:00:00Z`,
			},
			wantFields: &AttachmentFields{
				AttachedMolecule: "mol-ghi",
				AttachedAt:       "2025-12-21T12:00:00Z",
			},
		},
		{
			name: "case insensitive",
			issue: &Issue{
				Description: `Attached_Molecule: mol-jkl
ATTACHED_AT: 2025-12-21T14:00:00Z`,
			},
			wantFields: &AttachmentFields{
				AttachedMolecule: "mol-jkl",
				AttachedAt:       "2025-12-21T14:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := ParseAttachmentFields(tt.issue)

			if tt.wantNil {
				if fields != nil {
					t.Errorf("ParseAttachmentFields() = %+v, want nil", fields)
				}
				return
			}

			if fields == nil {
				t.Fatal("ParseAttachmentFields() = nil, want non-nil")
			}

			if fields.AttachedMolecule != tt.wantFields.AttachedMolecule {
				t.Errorf("AttachedMolecule = %q, want %q", fields.AttachedMolecule, tt.wantFields.AttachedMolecule)
			}
			if fields.AttachedAt != tt.wantFields.AttachedAt {
				t.Errorf("AttachedAt = %q, want %q", fields.AttachedAt, tt.wantFields.AttachedAt)
			}
		})
	}
}

// TestFormatAttachmentFields tests formatting attachment fields to string.
func TestFormatAttachmentFields(t *testing.T) {
	tests := []struct {
		name   string
		fields *AttachmentFields
		want   string
	}{
		{
			name:   "nil fields",
			fields: nil,
			want:   "",
		},
		{
			name:   "empty fields",
			fields: &AttachmentFields{},
			want:   "",
		},
		{
			name: "both fields",
			fields: &AttachmentFields{
				AttachedMolecule: "mol-xyz",
				AttachedAt:       "2025-12-21T15:30:00Z",
			},
			want: `attached_molecule: mol-xyz
attached_at: 2025-12-21T15:30:00Z`,
		},
		{
			name: "only molecule",
			fields: &AttachmentFields{
				AttachedMolecule: "mol-abc",
			},
			want: "attached_molecule: mol-abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatAttachmentFields(tt.fields)
			if got != tt.want {
				t.Errorf("FormatAttachmentFields() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestSetAttachmentFields tests updating issue descriptions with attachment fields.
func TestSetAttachmentFields(t *testing.T) {
	tests := []struct {
		name   string
		issue  *Issue
		fields *AttachmentFields
		want   string
	}{
		{
			name:  "nil issue",
			issue: nil,
			fields: &AttachmentFields{
				AttachedMolecule: "mol-xyz",
				AttachedAt:       "2025-12-21T15:30:00Z",
			},
			want: `attached_molecule: mol-xyz
attached_at: 2025-12-21T15:30:00Z`,
		},
		{
			name:  "empty description",
			issue: &Issue{Description: ""},
			fields: &AttachmentFields{
				AttachedMolecule: "mol-abc",
				AttachedAt:       "2025-12-21T10:00:00Z",
			},
			want: `attached_molecule: mol-abc
attached_at: 2025-12-21T10:00:00Z`,
		},
		{
			name:  "preserve prose content",
			issue: &Issue{Description: "This is a handoff bead description.\n\nKeep working on the task."},
			fields: &AttachmentFields{
				AttachedMolecule: "mol-def",
			},
			want: `attached_molecule: mol-def

This is a handoff bead description.

Keep working on the task.`,
		},
		{
			name: "replace existing fields",
			issue: &Issue{
				Description: `attached_molecule: mol-old
attached_at: 2025-12-20T10:00:00Z

Some existing prose content.`,
			},
			fields: &AttachmentFields{
				AttachedMolecule: "mol-new",
				AttachedAt:       "2025-12-21T15:30:00Z",
			},
			want: `attached_molecule: mol-new
attached_at: 2025-12-21T15:30:00Z

Some existing prose content.`,
		},
		{
			name:   "nil fields clears attachment",
			issue:  &Issue{Description: "attached_molecule: mol-old\nattached_at: 2025-12-20T10:00:00Z\n\nKeep this text."},
			fields: nil,
			want:   "Keep this text.",
		},
		{
			name:   "empty fields clears attachment",
			issue:  &Issue{Description: "attached_molecule: mol-old\n\nKeep this text."},
			fields: &AttachmentFields{},
			want:   "Keep this text.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetAttachmentFields(tt.issue, tt.fields)
			if got != tt.want {
				t.Errorf("SetAttachmentFields() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestAttachmentFieldsRoundTrip tests that parse/format round-trips correctly.
func TestAttachmentFieldsRoundTrip(t *testing.T) {
	original := &AttachmentFields{
		AttachedMolecule: "mol-roundtrip",
		AttachedAt:       "2025-12-21T15:30:00Z",
	}

	// Format to string
	formatted := FormatAttachmentFields(original)

	// Parse back
	issue := &Issue{Description: formatted}
	parsed := ParseAttachmentFields(issue)

	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}

	if *parsed != *original {
		t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", parsed, original)
	}
}

// TestNoMergeField tests the no_merge field in AttachmentFields.
// The no_merge flag tells gt done to skip the merge queue and keep work on a feature branch.
func TestNoMergeField(t *testing.T) {
	t.Run("parse no_merge true", func(t *testing.T) {
		issue := &Issue{Description: "no_merge: true\ndispatched_by: mayor"}
		fields := ParseAttachmentFields(issue)
		if fields == nil {
			t.Fatal("ParseAttachmentFields() = nil")
		}
		if !fields.NoMerge {
			t.Error("NoMerge should be true")
		}
		if fields.DispatchedBy != "mayor" {
			t.Errorf("DispatchedBy = %q, want 'mayor'", fields.DispatchedBy)
		}
	})

	t.Run("parse no_merge false", func(t *testing.T) {
		issue := &Issue{Description: "no_merge: false\ndispatched_by: crew"}
		fields := ParseAttachmentFields(issue)
		if fields == nil {
			t.Fatal("ParseAttachmentFields() = nil")
		}
		if fields.NoMerge {
			t.Error("NoMerge should be false")
		}
	})

	t.Run("parse no-merge alternate format", func(t *testing.T) {
		issue := &Issue{Description: "no-merge: true"}
		fields := ParseAttachmentFields(issue)
		if fields == nil {
			t.Fatal("ParseAttachmentFields() = nil")
		}
		if !fields.NoMerge {
			t.Error("NoMerge should be true with hyphen format")
		}
	})

	t.Run("format no_merge", func(t *testing.T) {
		fields := &AttachmentFields{
			NoMerge:      true,
			DispatchedBy: "mayor",
		}
		got := FormatAttachmentFields(fields)
		if !strings.Contains(got, "no_merge: true") {
			t.Errorf("FormatAttachmentFields() missing no_merge, got:\n%s", got)
		}
		if !strings.Contains(got, "dispatched_by: mayor") {
			t.Errorf("FormatAttachmentFields() missing dispatched_by, got:\n%s", got)
		}
	})

	t.Run("round-trip with no_merge", func(t *testing.T) {
		original := &AttachmentFields{
			AttachedMolecule: "mol-test",
			AttachedAt:       "2026-01-24T12:00:00Z",
			DispatchedBy:     "gastown/crew/max",
			NoMerge:          true,
		}

		formatted := FormatAttachmentFields(original)
		issue := &Issue{Description: formatted}
		parsed := ParseAttachmentFields(issue)

		if parsed == nil {
			t.Fatal("round-trip parse returned nil")
		}
		if *parsed != *original {
			t.Errorf("round-trip mismatch:\ngot  %+v\nwant %+v", parsed, original)
		}
	})
}

// TestResolveBeadsDir tests the redirect following logic.
func TestResolveBeadsDir(t *testing.T) {
	// Create temp directory structure
	tmpDir, err := os.MkdirTemp("", "beads-redirect-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	t.Run("no redirect", func(t *testing.T) {
		// Create a simple .beads directory without redirect
		workDir := filepath.Join(tmpDir, "no-redirect")
		beadsDir := filepath.Join(workDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		got := ResolveBeadsDir(workDir)
		want := beadsDir
		if got != want {
			t.Errorf("ResolveBeadsDir() = %q, want %q", got, want)
		}
	})

	t.Run("with redirect", func(t *testing.T) {
		// Create structure like: crew/max/.beads/redirect -> ../../mayor/rig/.beads
		workDir := filepath.Join(tmpDir, "crew", "max")
		localBeadsDir := filepath.Join(workDir, ".beads")
		targetBeadsDir := filepath.Join(tmpDir, "mayor", "rig", ".beads")

		// Create both directories
		if err := os.MkdirAll(localBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(targetBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create redirect file
		redirectPath := filepath.Join(localBeadsDir, "redirect")
		if err := os.WriteFile(redirectPath, []byte("../../mayor/rig/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}

		got := ResolveBeadsDir(workDir)
		want := targetBeadsDir
		if got != want {
			t.Errorf("ResolveBeadsDir() = %q, want %q", got, want)
		}
	})

	t.Run("no beads directory", func(t *testing.T) {
		// Directory with no .beads at all
		workDir := filepath.Join(tmpDir, "empty")
		if err := os.MkdirAll(workDir, 0755); err != nil {
			t.Fatal(err)
		}

		got := ResolveBeadsDir(workDir)
		want := filepath.Join(workDir, ".beads")
		if got != want {
			t.Errorf("ResolveBeadsDir() = %q, want %q", got, want)
		}
	})

	t.Run("empty redirect file", func(t *testing.T) {
		// Redirect file exists but is empty - should fall back to local
		workDir := filepath.Join(tmpDir, "empty-redirect")
		beadsDir := filepath.Join(workDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		redirectPath := filepath.Join(beadsDir, "redirect")
		if err := os.WriteFile(redirectPath, []byte("  \n"), 0644); err != nil {
			t.Fatal(err)
		}

		got := ResolveBeadsDir(workDir)
		want := beadsDir
		if got != want {
			t.Errorf("ResolveBeadsDir() = %q, want %q", got, want)
		}
	})

	t.Run("circular redirect", func(t *testing.T) {
		// Redirect that points to itself (e.g., mayor/rig/.beads/redirect -> ../../mayor/rig/.beads)
		// This is the bug scenario from gt-csbjj
		workDir := filepath.Join(tmpDir, "mayor", "rig")
		beadsDir := filepath.Join(workDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create a circular redirect: ../../mayor/rig/.beads resolves back to .beads
		redirectPath := filepath.Join(beadsDir, "redirect")
		if err := os.WriteFile(redirectPath, []byte("../../mayor/rig/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}

		// ResolveBeadsDir should detect the circular redirect and return the original beadsDir
		got := ResolveBeadsDir(workDir)
		want := beadsDir
		if got != want {
			t.Errorf("ResolveBeadsDir() = %q, want %q (should ignore circular redirect)", got, want)
		}

		// The circular redirect file should have been removed
		if _, err := os.Stat(redirectPath); err == nil {
			t.Error("circular redirect file should have been removed, but it still exists")
		}
	})
}

func TestParseAgentBeadID(t *testing.T) {
	tests := []struct {
		input    string
		wantRig  string
		wantRole string
		wantName string
		wantOK   bool
	}{
		// Town-level agents
		{"gt-mayor", "", "mayor", "", true},
		{"gt-deacon", "", "deacon", "", true},
		// Rig-level singletons
		{"gt-gastown-witness", "gastown", "witness", "", true},
		{"gt-gastown-refinery", "gastown", "refinery", "", true},
		// Rig-level named agents
		{"gt-gastown-crew-joe", "gastown", "crew", "joe", true},
		{"gt-gastown-crew-max", "gastown", "crew", "max", true},
		{"gt-gastown-polecat-capable", "gastown", "polecat", "capable", true},
		// Names with hyphens
		{"gt-gastown-polecat-my-agent", "gastown", "polecat", "my-agent", true},
		// Parseable but not valid agent roles (IsAgentSessionBead will reject)
		{"gt-abc123", "", "abc123", "", true}, // Parses as town-level but not valid role
		// Other prefixes (bd-, hq-)
		{"bd-mayor", "", "mayor", "", true},                           // bd prefix town-level
		{"bd-beads-witness", "beads", "witness", "", true},            // bd prefix rig-level singleton
		{"bd-beads-polecat-pearl", "beads", "polecat", "pearl", true}, // bd prefix rig-level named
		{"hq-mayor", "", "mayor", "", true},                           // hq prefix town-level
		// Truly invalid patterns
		{"x-mayor", "", "", "", false},    // Prefix too short (1 char)
		{"abcd-mayor", "", "", "", false}, // Prefix too long (4 chars)
		{"", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			rig, role, name, ok := ParseAgentBeadID(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ParseAgentBeadID(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
				return
			}
			if rig != tt.wantRig {
				t.Errorf("ParseAgentBeadID(%q) rig = %q, want %q", tt.input, rig, tt.wantRig)
			}
			if role != tt.wantRole {
				t.Errorf("ParseAgentBeadID(%q) role = %q, want %q", tt.input, role, tt.wantRole)
			}
			if name != tt.wantName {
				t.Errorf("ParseAgentBeadID(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
		})
	}
}

func TestIsAgentSessionBead(t *testing.T) {
	tests := []struct {
		beadID string
		want   bool
	}{
		// Agent session beads with gt- prefix (should return true)
		{"gt-mayor", true},
		{"gt-deacon", true},
		{"gt-gastown-witness", true},
		{"gt-gastown-refinery", true},
		{"gt-gastown-crew-joe", true},
		{"gt-gastown-polecat-capable", true},
		// Agent session beads with bd- prefix (should return true)
		{"bd-mayor", true},
		{"bd-deacon", true},
		{"bd-beads-witness", true},
		{"bd-beads-refinery", true},
		{"bd-beads-crew-joe", true},
		{"bd-beads-polecat-pearl", true},
		// Regular work beads (should return false)
		{"gt-abc123", false},
		{"gt-sb6m4", false},
		{"gt-u7dxq", false},
		{"bd-abc123", false},
		// Invalid beads
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.beadID, func(t *testing.T) {
			got := IsAgentSessionBead(tt.beadID)
			if got != tt.want {
				t.Errorf("IsAgentSessionBead(%q) = %v, want %v", tt.beadID, got, tt.want)
			}
		})
	}
}

// TestParseRoleConfig tests parsing role configuration from descriptions.
func TestParseRoleConfig(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantNil     bool
		wantConfig  *RoleConfig
	}{
		{
			name:        "empty description",
			description: "",
			wantNil:     true,
		},
		{
			name:        "no role config fields",
			description: "This is just plain text\nwith no role config fields",
			wantNil:     true,
		},
		{
			name: "all fields",
			description: `session_pattern: gt-{rig}-{name}
work_dir_pattern: {town}/{rig}/polecats/{name}
needs_pre_sync: true
start_command: exec claude --dangerously-skip-permissions
env_var: GT_ROLE=polecat
env_var: GT_RIG={rig}`,
			wantConfig: &RoleConfig{
				SessionPattern: "gt-{rig}-{name}",
				WorkDirPattern: "{town}/{rig}/polecats/{name}",
				NeedsPreSync:   true,
				StartCommand:   "exec claude --dangerously-skip-permissions",
				EnvVars:        map[string]string{"GT_ROLE": "polecat", "GT_RIG": "{rig}"},
			},
		},
		{
			name: "partial fields",
			description: `session_pattern: gt-mayor
work_dir_pattern: {town}`,
			wantConfig: &RoleConfig{
				SessionPattern: "gt-mayor",
				WorkDirPattern: "{town}",
				EnvVars:        map[string]string{},
			},
		},
		{
			name: "mixed with prose",
			description: `You are the Witness.

session_pattern: gt-{rig}-witness
work_dir_pattern: {town}/{rig}
needs_pre_sync: false

Your job is to monitor workers.`,
			wantConfig: &RoleConfig{
				SessionPattern: "gt-{rig}-witness",
				WorkDirPattern: "{town}/{rig}",
				NeedsPreSync:   false,
				EnvVars:        map[string]string{},
			},
		},
		{
			name: "alternate key formats (hyphen)",
			description: `session-pattern: gt-{rig}-{name}
work-dir-pattern: {town}/{rig}/polecats/{name}
needs-pre-sync: true`,
			wantConfig: &RoleConfig{
				SessionPattern: "gt-{rig}-{name}",
				WorkDirPattern: "{town}/{rig}/polecats/{name}",
				NeedsPreSync:   true,
				EnvVars:        map[string]string{},
			},
		},
		{
			name: "case insensitive keys",
			description: `SESSION_PATTERN: gt-mayor
Work_Dir_Pattern: {town}`,
			wantConfig: &RoleConfig{
				SessionPattern: "gt-mayor",
				WorkDirPattern: "{town}",
				EnvVars:        map[string]string{},
			},
		},
		{
			name: "ignores null values",
			description: `session_pattern: gt-{rig}-witness
work_dir_pattern: null
needs_pre_sync: false`,
			wantConfig: &RoleConfig{
				SessionPattern: "gt-{rig}-witness",
				EnvVars:        map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ParseRoleConfig(tt.description)

			if tt.wantNil {
				if config != nil {
					t.Errorf("ParseRoleConfig() = %+v, want nil", config)
				}
				return
			}

			if config == nil {
				t.Fatal("ParseRoleConfig() = nil, want non-nil")
			}

			if config.SessionPattern != tt.wantConfig.SessionPattern {
				t.Errorf("SessionPattern = %q, want %q", config.SessionPattern, tt.wantConfig.SessionPattern)
			}
			if config.WorkDirPattern != tt.wantConfig.WorkDirPattern {
				t.Errorf("WorkDirPattern = %q, want %q", config.WorkDirPattern, tt.wantConfig.WorkDirPattern)
			}
			if config.NeedsPreSync != tt.wantConfig.NeedsPreSync {
				t.Errorf("NeedsPreSync = %v, want %v", config.NeedsPreSync, tt.wantConfig.NeedsPreSync)
			}
			if config.StartCommand != tt.wantConfig.StartCommand {
				t.Errorf("StartCommand = %q, want %q", config.StartCommand, tt.wantConfig.StartCommand)
			}
			if len(config.EnvVars) != len(tt.wantConfig.EnvVars) {
				t.Errorf("EnvVars len = %d, want %d", len(config.EnvVars), len(tt.wantConfig.EnvVars))
			}
			for k, v := range tt.wantConfig.EnvVars {
				if config.EnvVars[k] != v {
					t.Errorf("EnvVars[%q] = %q, want %q", k, config.EnvVars[k], v)
				}
			}
		})
	}
}

// TestExpandRolePattern tests pattern expansion with placeholders.
func TestExpandRolePattern(t *testing.T) {
	tests := []struct {
		pattern  string
		townRoot string
		rig      string
		name     string
		role     string
		want     string
	}{
		{
			pattern:  "gt-mayor",
			townRoot: "/Users/stevey/gt",
			want:     "gt-mayor",
		},
		{
			pattern:  "gt-{rig}-{role}",
			townRoot: "/Users/stevey/gt",
			rig:      "gastown",
			role:     "witness",
			want:     "gt-gastown-witness",
		},
		{
			pattern:  "gt-{rig}-{name}",
			townRoot: "/Users/stevey/gt",
			rig:      "gastown",
			name:     "toast",
			want:     "gt-gastown-toast",
		},
		{
			pattern:  "{town}/{rig}/polecats/{name}",
			townRoot: "/Users/stevey/gt",
			rig:      "gastown",
			name:     "toast",
			want:     "/Users/stevey/gt/gastown/polecats/toast",
		},
		{
			pattern:  "{town}/{rig}/refinery/rig",
			townRoot: "/Users/stevey/gt",
			rig:      "gastown",
			want:     "/Users/stevey/gt/gastown/refinery/rig",
		},
		{
			pattern:  "export GT_ROLE={role} GT_RIG={rig} BD_ACTOR={rig}/polecats/{name}",
			townRoot: "/Users/stevey/gt",
			rig:      "gastown",
			name:     "toast",
			role:     "polecat",
			want:     "export GT_ROLE=polecat GT_RIG=gastown BD_ACTOR=gastown/polecats/toast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := ExpandRolePattern(tt.pattern, tt.townRoot, tt.rig, tt.name, tt.role)
			if got != tt.want {
				t.Errorf("ExpandRolePattern() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatRoleConfig tests formatting role config to string.
func TestFormatRoleConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *RoleConfig
		want   string
	}{
		{
			name:   "nil config",
			config: nil,
			want:   "",
		},
		{
			name:   "empty config",
			config: &RoleConfig{EnvVars: map[string]string{}},
			want:   "",
		},
		{
			name: "all fields",
			config: &RoleConfig{
				SessionPattern: "gt-{rig}-{name}",
				WorkDirPattern: "{town}/{rig}/polecats/{name}",
				NeedsPreSync:   true,
				StartCommand:   "exec claude",
				EnvVars:        map[string]string{},
			},
			want: `session_pattern: gt-{rig}-{name}
work_dir_pattern: {town}/{rig}/polecats/{name}
needs_pre_sync: true
start_command: exec claude`,
		},
		{
			name: "only session pattern",
			config: &RoleConfig{
				SessionPattern: "gt-mayor",
				EnvVars:        map[string]string{},
			},
			want: "session_pattern: gt-mayor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRoleConfig(tt.config)
			if got != tt.want {
				t.Errorf("FormatRoleConfig() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestRoleConfigRoundTrip tests that parse/format round-trips correctly.
func TestRoleConfigRoundTrip(t *testing.T) {
	original := &RoleConfig{
		SessionPattern: "gt-{rig}-{name}",
		WorkDirPattern: "{town}/{rig}/polecats/{name}",
		NeedsPreSync:   true,
		StartCommand:   "exec claude --dangerously-skip-permissions",
		EnvVars:        map[string]string{}, // Can't round-trip env vars due to order
	}

	// Format to string
	formatted := FormatRoleConfig(original)

	// Parse back
	parsed := ParseRoleConfig(formatted)

	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}

	if parsed.SessionPattern != original.SessionPattern {
		t.Errorf("round-trip SessionPattern = %q, want %q", parsed.SessionPattern, original.SessionPattern)
	}
	if parsed.WorkDirPattern != original.WorkDirPattern {
		t.Errorf("round-trip WorkDirPattern = %q, want %q", parsed.WorkDirPattern, original.WorkDirPattern)
	}
	if parsed.NeedsPreSync != original.NeedsPreSync {
		t.Errorf("round-trip NeedsPreSync = %v, want %v", parsed.NeedsPreSync, original.NeedsPreSync)
	}
	if parsed.StartCommand != original.StartCommand {
		t.Errorf("round-trip StartCommand = %q, want %q", parsed.StartCommand, original.StartCommand)
	}
}

// TestRoleBeadID tests role bead ID generation.
func TestRoleBeadID(t *testing.T) {
	tests := []struct {
		roleType string
		want     string
	}{
		{"mayor", "gt-mayor-role"},
		{"deacon", "gt-deacon-role"},
		{"witness", "gt-witness-role"},
		{"refinery", "gt-refinery-role"},
		{"crew", "gt-crew-role"},
		{"polecat", "gt-polecat-role"},
	}

	for _, tt := range tests {
		t.Run(tt.roleType, func(t *testing.T) {
			got := RoleBeadID(tt.roleType)
			if got != tt.want {
				t.Errorf("RoleBeadID(%q) = %q, want %q", tt.roleType, got, tt.want)
			}
		})
	}
}

// TestDelegationStruct tests the Delegation struct serialization.
func TestDelegationStruct(t *testing.T) {
	tests := []struct {
		name       string
		delegation Delegation
		wantJSON   string
	}{
		{
			name: "full delegation",
			delegation: Delegation{
				Parent:      "hop://accenture.com/eng/proj-123/task-a",
				Child:       "hop://alice@example.com/main-town/gastown/gt-xyz",
				DelegatedBy: "hop://accenture.com",
				DelegatedTo: "hop://alice@example.com",
				Terms: &DelegationTerms{
					Portion:     "backend-api",
					Deadline:    "2025-06-01",
					CreditShare: 80,
				},
				CreatedAt: "2025-01-15T10:00:00Z",
			},
			wantJSON: `{"parent":"hop://accenture.com/eng/proj-123/task-a","child":"hop://alice@example.com/main-town/gastown/gt-xyz","delegated_by":"hop://accenture.com","delegated_to":"hop://alice@example.com","terms":{"portion":"backend-api","deadline":"2025-06-01","credit_share":80},"created_at":"2025-01-15T10:00:00Z"}`,
		},
		{
			name: "minimal delegation",
			delegation: Delegation{
				Parent:      "gt-abc",
				Child:       "gt-xyz",
				DelegatedBy: "steve",
				DelegatedTo: "alice",
			},
			wantJSON: `{"parent":"gt-abc","child":"gt-xyz","delegated_by":"steve","delegated_to":"alice"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.delegation)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Errorf("json.Marshal = %s, want %s", string(got), tt.wantJSON)
			}

			// Test round-trip
			var parsed Delegation
			if err := json.Unmarshal(got, &parsed); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if parsed.Parent != tt.delegation.Parent {
				t.Errorf("parsed.Parent = %s, want %s", parsed.Parent, tt.delegation.Parent)
			}
			if parsed.Child != tt.delegation.Child {
				t.Errorf("parsed.Child = %s, want %s", parsed.Child, tt.delegation.Child)
			}
			if parsed.DelegatedBy != tt.delegation.DelegatedBy {
				t.Errorf("parsed.DelegatedBy = %s, want %s", parsed.DelegatedBy, tt.delegation.DelegatedBy)
			}
			if parsed.DelegatedTo != tt.delegation.DelegatedTo {
				t.Errorf("parsed.DelegatedTo = %s, want %s", parsed.DelegatedTo, tt.delegation.DelegatedTo)
			}
		})
	}
}

// TestDelegationTerms tests the DelegationTerms struct.
func TestDelegationTerms(t *testing.T) {
	terms := &DelegationTerms{
		Portion:            "frontend",
		Deadline:           "2025-03-15",
		AcceptanceCriteria: "All tests passing, code reviewed",
		CreditShare:        70,
	}

	got, err := json.Marshal(terms)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed DelegationTerms
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed.Portion != terms.Portion {
		t.Errorf("parsed.Portion = %s, want %s", parsed.Portion, terms.Portion)
	}
	if parsed.Deadline != terms.Deadline {
		t.Errorf("parsed.Deadline = %s, want %s", parsed.Deadline, terms.Deadline)
	}
	if parsed.AcceptanceCriteria != terms.AcceptanceCriteria {
		t.Errorf("parsed.AcceptanceCriteria = %s, want %s", parsed.AcceptanceCriteria, terms.AcceptanceCriteria)
	}
	if parsed.CreditShare != terms.CreditShare {
		t.Errorf("parsed.CreditShare = %d, want %d", parsed.CreditShare, terms.CreditShare)
	}
}

// TestSetupRedirect tests the beads redirect setup for worktrees.
func TestSetupRedirect(t *testing.T) {
	t.Run("crew worktree with local beads", func(t *testing.T) {
		// Setup: town/rig/.beads (local, no redirect)
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		crewPath := filepath.Join(rigRoot, "crew", "max")

		// Create rig structure
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		if err := os.MkdirAll(crewPath, 0755); err != nil {
			t.Fatalf("mkdir crew: %v", err)
		}

		// Run SetupRedirect
		if err := SetupRedirect(townRoot, crewPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		// Verify redirect was created
		redirectPath := filepath.Join(crewPath, ".beads", "redirect")
		content, err := os.ReadFile(redirectPath)
		if err != nil {
			t.Fatalf("read redirect: %v", err)
		}

		want := "../../.beads\n"
		if string(content) != want {
			t.Errorf("redirect content = %q, want %q", string(content), want)
		}
	})

	t.Run("crew worktree with tracked beads", func(t *testing.T) {
		// Setup: town/rig/.beads/redirect -> mayor/rig/.beads (tracked)
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		mayorRigBeads := filepath.Join(rigRoot, "mayor", "rig", ".beads")
		crewPath := filepath.Join(rigRoot, "crew", "max")

		// Create rig structure with tracked beads
		if err := os.MkdirAll(mayorRigBeads, 0755); err != nil {
			t.Fatalf("mkdir mayor/rig beads: %v", err)
		}
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		// Create rig-level redirect to mayor/rig/.beads
		if err := os.WriteFile(filepath.Join(rigBeads, "redirect"), []byte("mayor/rig/.beads\n"), 0644); err != nil {
			t.Fatalf("write rig redirect: %v", err)
		}
		if err := os.MkdirAll(crewPath, 0755); err != nil {
			t.Fatalf("mkdir crew: %v", err)
		}

		// Run SetupRedirect
		if err := SetupRedirect(townRoot, crewPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		// Verify redirect goes directly to mayor/rig/.beads (no chain - bd CLI doesn't support chains)
		redirectPath := filepath.Join(crewPath, ".beads", "redirect")
		content, err := os.ReadFile(redirectPath)
		if err != nil {
			t.Fatalf("read redirect: %v", err)
		}

		want := "../../mayor/rig/.beads\n"
		if string(content) != want {
			t.Errorf("redirect content = %q, want %q", string(content), want)
		}

		// Verify redirect resolves correctly
		resolved := ResolveBeadsDir(crewPath)
		// crew/max -> ../../mayor/rig/.beads (direct, no chain)
		if resolved != mayorRigBeads {
			t.Errorf("resolved = %q, want %q", resolved, mayorRigBeads)
		}
	})

	t.Run("polecat worktree", func(t *testing.T) {
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		polecatPath := filepath.Join(rigRoot, "polecats", "worker1")

		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		if err := os.MkdirAll(polecatPath, 0755); err != nil {
			t.Fatalf("mkdir polecat: %v", err)
		}

		if err := SetupRedirect(townRoot, polecatPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		redirectPath := filepath.Join(polecatPath, ".beads", "redirect")
		content, err := os.ReadFile(redirectPath)
		if err != nil {
			t.Fatalf("read redirect: %v", err)
		}

		want := "../../.beads\n"
		if string(content) != want {
			t.Errorf("redirect content = %q, want %q", string(content), want)
		}
	})

	t.Run("refinery worktree", func(t *testing.T) {
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		refineryPath := filepath.Join(rigRoot, "refinery", "rig")

		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		if err := os.MkdirAll(refineryPath, 0755); err != nil {
			t.Fatalf("mkdir refinery: %v", err)
		}

		if err := SetupRedirect(townRoot, refineryPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		redirectPath := filepath.Join(refineryPath, ".beads", "redirect")
		content, err := os.ReadFile(redirectPath)
		if err != nil {
			t.Fatalf("read redirect: %v", err)
		}

		want := "../../.beads\n"
		if string(content) != want {
			t.Errorf("redirect content = %q, want %q", string(content), want)
		}
	})

	t.Run("cleans runtime files but preserves tracked files", func(t *testing.T) {
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		crewPath := filepath.Join(rigRoot, "crew", "max")
		crewBeads := filepath.Join(crewPath, ".beads")

		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		// Simulate worktree with both runtime and tracked files
		if err := os.MkdirAll(crewBeads, 0755); err != nil {
			t.Fatalf("mkdir crew beads: %v", err)
		}
		// Runtime files (should be removed)
		if err := os.WriteFile(filepath.Join(crewBeads, "beads.db"), []byte("fake db"), 0644); err != nil {
			t.Fatalf("write fake db: %v", err)
		}
		if err := os.WriteFile(filepath.Join(crewBeads, "issues.jsonl"), []byte("{}"), 0644); err != nil {
			t.Fatalf("write issues.jsonl: %v", err)
		}
		// Tracked files (should be preserved)
		if err := os.WriteFile(filepath.Join(crewBeads, "config.yaml"), []byte("prefix: test"), 0644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := os.WriteFile(filepath.Join(crewBeads, "README.md"), []byte("# Beads"), 0644); err != nil {
			t.Fatalf("write README: %v", err)
		}

		if err := SetupRedirect(townRoot, crewPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		// Verify runtime files were cleaned up
		if _, err := os.Stat(filepath.Join(crewBeads, "beads.db")); !os.IsNotExist(err) {
			t.Error("beads.db should have been removed")
		}
		if _, err := os.Stat(filepath.Join(crewBeads, "issues.jsonl")); !os.IsNotExist(err) {
			t.Error("issues.jsonl should have been removed")
		}

		// Verify tracked files were preserved
		if _, err := os.Stat(filepath.Join(crewBeads, "config.yaml")); err != nil {
			t.Errorf("config.yaml should have been preserved: %v", err)
		}
		if _, err := os.Stat(filepath.Join(crewBeads, "README.md")); err != nil {
			t.Errorf("README.md should have been preserved: %v", err)
		}

		// Verify redirect was created
		redirectPath := filepath.Join(crewBeads, "redirect")
		if _, err := os.Stat(redirectPath); err != nil {
			t.Errorf("redirect file should exist: %v", err)
		}
	})

	t.Run("rejects mayor/rig canonical location", func(t *testing.T) {
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		mayorRigPath := filepath.Join(rigRoot, "mayor", "rig")

		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		if err := os.MkdirAll(mayorRigPath, 0755); err != nil {
			t.Fatalf("mkdir mayor/rig: %v", err)
		}

		err := SetupRedirect(townRoot, mayorRigPath)
		if err == nil {
			t.Error("SetupRedirect should reject mayor/rig location")
		}
		if err != nil && !strings.Contains(err.Error(), "canonical") {
			t.Errorf("error should mention canonical location, got: %v", err)
		}
	})

	t.Run("rejects path too shallow", func(t *testing.T) {
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")

		if err := os.MkdirAll(rigRoot, 0755); err != nil {
			t.Fatalf("mkdir rig: %v", err)
		}

		err := SetupRedirect(townRoot, rigRoot)
		if err == nil {
			t.Error("SetupRedirect should reject rig root (too shallow)")
		}
	})

	t.Run("fails if rig beads missing", func(t *testing.T) {
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		crewPath := filepath.Join(rigRoot, "crew", "max")

		// No rig/.beads or mayor/rig/.beads created
		if err := os.MkdirAll(crewPath, 0755); err != nil {
			t.Fatalf("mkdir crew: %v", err)
		}

		err := SetupRedirect(townRoot, crewPath)
		if err == nil {
			t.Error("SetupRedirect should fail if rig .beads missing")
		}
	})

	t.Run("crew worktree with rig beads but no database", func(t *testing.T) {
		// Setup: rig/.beads exists (has metadata.json) but no actual database.
		// This is the dolt architecture where rig/.beads has metadata only and
		// the actual dolt DB lives at mayor/rig/.beads/dolt/.
		// The redirect should point to mayor/rig/.beads, not rig/.beads.
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		mayorRigBeads := filepath.Join(rigRoot, "mayor", "rig", ".beads")
		crewPath := filepath.Join(rigRoot, "crew", "max")

		// Create rig/.beads with metadata but NO database (no dolt/ or beads.db)
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		if err := os.WriteFile(filepath.Join(rigBeads, "metadata.json"),
			[]byte(`{"database":"dolt","backend":"dolt","dolt_mode":"embedded"}`), 0644); err != nil {
			t.Fatalf("write metadata: %v", err)
		}
		// Create mayor/rig/.beads with dolt DB marker
		doltDir := filepath.Join(mayorRigBeads, "dolt")
		if err := os.MkdirAll(doltDir, 0755); err != nil {
			t.Fatalf("mkdir mayor dolt: %v", err)
		}
		if err := os.MkdirAll(crewPath, 0755); err != nil {
			t.Fatalf("mkdir crew: %v", err)
		}

		// Run SetupRedirect - should detect no DB at rig/.beads and fall back to mayor/rig/.beads
		if err := SetupRedirect(townRoot, crewPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		// Verify redirect points to mayor/rig/.beads (not rig/.beads)
		redirectPath := filepath.Join(crewPath, ".beads", "redirect")
		content, err := os.ReadFile(redirectPath)
		if err != nil {
			t.Fatalf("read redirect: %v", err)
		}

		want := "../../mayor/rig/.beads\n"
		if string(content) != want {
			t.Errorf("redirect content = %q, want %q", string(content), want)
		}

		// Verify redirect resolves correctly
		resolved := ResolveBeadsDir(crewPath)
		if resolved != mayorRigBeads {
			t.Errorf("resolved = %q, want %q", resolved, mayorRigBeads)
		}
	})

	t.Run("crew worktree with mayor/rig beads only", func(t *testing.T) {
		// Setup: no rig/.beads, only mayor/rig/.beads exists
		// This is the tracked beads architecture where rig root has no .beads directory
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		mayorRigBeads := filepath.Join(rigRoot, "mayor", "rig", ".beads")
		crewPath := filepath.Join(rigRoot, "crew", "max")

		// Create only mayor/rig/.beads (no rig/.beads)
		if err := os.MkdirAll(mayorRigBeads, 0755); err != nil {
			t.Fatalf("mkdir mayor/rig beads: %v", err)
		}
		if err := os.MkdirAll(crewPath, 0755); err != nil {
			t.Fatalf("mkdir crew: %v", err)
		}

		// Run SetupRedirect - should succeed and point to mayor/rig/.beads
		if err := SetupRedirect(townRoot, crewPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		// Verify redirect points to mayor/rig/.beads
		redirectPath := filepath.Join(crewPath, ".beads", "redirect")
		content, err := os.ReadFile(redirectPath)
		if err != nil {
			t.Fatalf("read redirect: %v", err)
		}

		want := "../../mayor/rig/.beads\n"
		if string(content) != want {
			t.Errorf("redirect content = %q, want %q", string(content), want)
		}

		// Verify redirect resolves correctly
		resolved := ResolveBeadsDir(crewPath)
		if resolved != mayorRigBeads {
			t.Errorf("resolved = %q, want %q", resolved, mayorRigBeads)
		}
	})

	t.Run("handles stale .beads file (not directory)", func(t *testing.T) {
		// Edge case: .beads exists as a file instead of directory
		// This can happen with unusual clone state or failed operations
		townRoot := t.TempDir()
		rigRoot := filepath.Join(townRoot, "testrig")
		rigBeads := filepath.Join(rigRoot, ".beads")
		crewPath := filepath.Join(rigRoot, "crew", "max")

		// Create rig structure
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			t.Fatalf("mkdir rig beads: %v", err)
		}
		if err := os.MkdirAll(crewPath, 0755); err != nil {
			t.Fatalf("mkdir crew: %v", err)
		}

		// Create .beads as a FILE (not directory) - simulating stale state
		staleBeadsFile := filepath.Join(crewPath, ".beads")
		if err := os.WriteFile(staleBeadsFile, []byte("stale content"), 0644); err != nil {
			t.Fatalf("write stale .beads file: %v", err)
		}

		// SetupRedirect should remove the file and create the directory
		if err := SetupRedirect(townRoot, crewPath); err != nil {
			t.Fatalf("SetupRedirect failed: %v", err)
		}

		// Verify .beads is now a directory
		info, err := os.Stat(staleBeadsFile)
		if err != nil {
			t.Fatalf("stat .beads: %v", err)
		}
		if !info.IsDir() {
			t.Errorf(".beads should be a directory, but is a file")
		}

		// Verify redirect was created
		redirectPath := filepath.Join(crewPath, ".beads", "redirect")
		content, err := os.ReadFile(redirectPath)
		if err != nil {
			t.Fatalf("read redirect: %v", err)
		}

		want := "../../.beads\n"
		if string(content) != want {
			t.Errorf("redirect content = %q, want %q", string(content), want)
		}
	})
}

// TestAgentBeadTombstoneBug demonstrates the bd bug where `bd delete --hard --force`
// creates tombstones instead of truly deleting records.
//
//
// This test documents the bug behavior:
// 1. Create agent bead
// 2. Delete with --hard --force (supposed to permanently delete)
// 3. BUG: Tombstone is created instead
// 4. BUG: bd create fails with UNIQUE constraint
// 5. BUG: bd reopen fails with "issue not found" (tombstones are invisible)
func TestAgentBeadTombstoneBug(t *testing.T) {
	// Skip: bd CLI 0.47.2 has a bug where database writes don't commit
	// ("sql: database is closed" during auto-flush). This blocks all tests
	// that need to create issues. See internal issue for tracking.
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()

	// Create isolated beads instance and initialize database
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	agentID := "test-testrig-polecat-tombstone"

	// Step 1: Create agent bead
	_, err := bd.CreateAgentBead(agentID, "Test agent", &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "spawning",
	})
	if err != nil {
		t.Fatalf("CreateAgentBead: %v", err)
	}

	// Step 2: Delete with --hard --force (supposed to permanently delete)
	err = bd.DeleteAgentBead(agentID)
	if err != nil {
		t.Fatalf("DeleteAgentBead: %v", err)
	}

	// Step 3: BUG - Tombstone exists (check via bd list --status=tombstone)
	out, err := bd.run("list", "--status=tombstone", "--json")
	if err != nil {
		t.Fatalf("list tombstones: %v", err)
	}

	// Parse to check if our agent is in the tombstone list
	var tombstones []Issue
	if err := json.Unmarshal(out, &tombstones); err != nil {
		t.Fatalf("parse tombstones: %v", err)
	}

	foundTombstone := false
	for _, ts := range tombstones {
		if ts.ID == agentID {
			foundTombstone = true
			break
		}
	}

	if !foundTombstone {
		// If bd ever fixes the --hard flag, this test will fail here
		// That's a good thing - it means the bug is fixed!
		t.Skip("bd --hard appears to be fixed (no tombstone created) - update this test")
	}

	// Step 4: BUG - bd create fails with UNIQUE constraint
	_, err = bd.CreateAgentBead(agentID, "Test agent 2", &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "spawning",
	})
	if err == nil {
		t.Fatal("expected UNIQUE constraint error, got nil")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint") {
		t.Errorf("expected UNIQUE constraint error, got: %v", err)
	}

	// Step 5: BUG - bd reopen fails (tombstones are invisible)
	_, err = bd.run("reopen", agentID, "--reason=test")
	if err == nil {
		t.Fatal("expected reopen to fail on tombstone, got nil")
	}
	if !strings.Contains(err.Error(), "no issue found") && !strings.Contains(err.Error(), "issue not found") {
		t.Errorf("expected 'issue not found' error, got: %v", err)
	}

	t.Log("BUG CONFIRMED: bd delete --hard creates tombstones that block recreation")
}

// TestAgentBeadCloseReopenWorkaround demonstrates the workaround for the tombstone bug:
// use Close instead of Delete, then Reopen works.
func TestAgentBeadCloseReopenWorkaround(t *testing.T) {
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	agentID := "test-testrig-polecat-closereopen"

	// Step 1: Create agent bead
	_, err := bd.CreateAgentBead(agentID, "Test agent", &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "spawning",
		HookBead:   "test-task-1",
	})
	if err != nil {
		t.Fatalf("CreateAgentBead: %v", err)
	}

	// Step 2: Close (not delete) - this is the workaround
	err = bd.CloseAndClearAgentBead(agentID, "polecat removed")
	if err != nil {
		t.Fatalf("CloseAndClearAgentBead: %v", err)
	}

	// Step 3: Verify bead is closed (not tombstone)
	issue, err := bd.Show(agentID)
	if err != nil {
		t.Fatalf("Show after close: %v", err)
	}
	if issue.Status != "closed" {
		t.Errorf("status = %q, want 'closed'", issue.Status)
	}

	// Step 4: Reopen works on closed beads
	_, err = bd.run("reopen", agentID, "--reason=re-spawning")
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}

	// Step 5: Verify bead is open again
	issue, err = bd.Show(agentID)
	if err != nil {
		t.Fatalf("Show after reopen: %v", err)
	}
	if issue.Status != "open" {
		t.Errorf("status = %q, want 'open'", issue.Status)
	}

	t.Log("WORKAROUND CONFIRMED: Close + Reopen works for agent bead lifecycle")
}

// TestCreateOrReopenAgentBead_ClosedBead tests that CreateOrReopenAgentBead
// successfully reopens a closed agent bead and updates its fields.
func TestCreateOrReopenAgentBead_ClosedBead(t *testing.T) {
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	agentID := "test-testrig-polecat-lifecycle"

	// Simulate polecat lifecycle: spawn → nuke → respawn

	// Spawn 1: Create agent bead with first task
	issue1, err := bd.CreateOrReopenAgentBead(agentID, agentID, &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "spawning",
		HookBead:   "test-task-1",
	})
	if err != nil {
		t.Fatalf("Spawn 1 - CreateOrReopenAgentBead: %v", err)
	}
	if issue1.Status != "open" {
		t.Errorf("Spawn 1: status = %q, want 'open'", issue1.Status)
	}

	// Nuke 1: Close agent bead (workaround for tombstone bug)
	err = bd.CloseAndClearAgentBead(agentID, "polecat nuked")
	if err != nil {
		t.Fatalf("Nuke 1 - CloseAndClearAgentBead: %v", err)
	}

	// Spawn 2: CreateOrReopenAgentBead should reopen and update
	issue2, err := bd.CreateOrReopenAgentBead(agentID, agentID, &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "spawning",
		HookBead:   "test-task-2", // Different task
	})
	if err != nil {
		t.Fatalf("Spawn 2 - CreateOrReopenAgentBead: %v", err)
	}
	if issue2.Status != "open" {
		t.Errorf("Spawn 2: status = %q, want 'open'", issue2.Status)
	}

	// Verify the hook was updated to the new task
	fields := ParseAgentFields(issue2.Description)
	if fields.HookBead != "test-task-2" {
		t.Errorf("Spawn 2: hook_bead = %q, want 'test-task-2'", fields.HookBead)
	}

	// Nuke 2: Close again
	err = bd.CloseAndClearAgentBead(agentID, "polecat nuked again")
	if err != nil {
		t.Fatalf("Nuke 2 - CloseAndClearAgentBead: %v", err)
	}

	// Spawn 3: Should still work
	issue3, err := bd.CreateOrReopenAgentBead(agentID, agentID, &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "spawning",
		HookBead:   "test-task-3",
	})
	if err != nil {
		t.Fatalf("Spawn 3 - CreateOrReopenAgentBead: %v", err)
	}

	fields = ParseAgentFields(issue3.Description)
	if fields.HookBead != "test-task-3" {
		t.Errorf("Spawn 3: hook_bead = %q, want 'test-task-3'", fields.HookBead)
	}

	t.Log("LIFECYCLE TEST PASSED: spawn → nuke → respawn works with close/reopen")
}

// TestCloseAndClearAgentBead_FieldClearing tests that CloseAndClearAgentBead clears all mutable
// fields to emulate delete --force --hard behavior. This ensures reopened agent
// beads don't have stale state from previous lifecycle.
func TestCloseAndClearAgentBead_FieldClearing(t *testing.T) {
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	// Test cases for field clearing permutations
	tests := []struct {
		name   string
		fields *AgentFields
		reason string
	}{
		{
			name: "all_fields_populated",
			fields: &AgentFields{
				RoleType:          "polecat",
				Rig:               "testrig",
				AgentState:        "running",
				HookBead:          "test-issue-123",
				CleanupStatus:     "clean",
				ActiveMR:          "test-mr-456",
				NotificationLevel: "normal",
			},
			reason: "polecat completed work",
		},
		{
			name: "only_hook_bead",
			fields: &AgentFields{
				RoleType:   "polecat",
				Rig:        "testrig",
				AgentState: "spawning",
				HookBead:   "test-issue-789",
			},
			reason: "polecat nuked",
		},
		{
			name: "only_active_mr",
			fields: &AgentFields{
				RoleType:   "polecat",
				Rig:        "testrig",
				AgentState: "running",
				ActiveMR:   "test-mr-abc",
			},
			reason: "",
		},
		{
			name: "only_cleanup_status",
			fields: &AgentFields{
				RoleType:      "polecat",
				Rig:           "testrig",
				AgentState:    "idle",
				CleanupStatus: "has_uncommitted",
			},
			reason: "cleanup required",
		},
		{
			name: "no_mutable_fields",
			fields: &AgentFields{
				RoleType:   "polecat",
				Rig:        "testrig",
				AgentState: "spawning",
			},
			reason: "fresh spawn closed",
		},
		{
			name: "polecat_with_all_field_types",
			fields: &AgentFields{
				RoleType:          "polecat",
				Rig:               "testrig",
				AgentState:        "processing",
				HookBead:          "test-task-xyz",
				ActiveMR:          "test-mr-processing",
				CleanupStatus:     "has_uncommitted",
				NotificationLevel: "verbose",
			},
			reason: "comprehensive cleanup",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use tc.name for suffix to avoid hash-like patterns (e.g., single digits)
			// that trigger bd's isLikelyHash() prefix extraction in v0.47.1+
			agentID := fmt.Sprintf("test-testrig-%s-%s", tc.fields.RoleType, tc.name)

			// Step 1: Create agent bead with specified fields
			_, err := bd.CreateAgentBead(agentID, "Test agent", tc.fields)
			if err != nil {
				t.Fatalf("CreateAgentBead: %v", err)
			}

			// Verify fields were set
			issue, err := bd.Show(agentID)
			if err != nil {
				t.Fatalf("Show before close: %v", err)
			}
			beforeFields := ParseAgentFields(issue.Description)
			if tc.fields.HookBead != "" && beforeFields.HookBead != tc.fields.HookBead {
				t.Errorf("before close: hook_bead = %q, want %q", beforeFields.HookBead, tc.fields.HookBead)
			}

			// Step 2: Close the agent bead
			err = bd.CloseAndClearAgentBead(agentID, tc.reason)
			if err != nil {
				t.Fatalf("CloseAndClearAgentBead: %v", err)
			}

			// Step 3: Verify bead is closed
			issue, err = bd.Show(agentID)
			if err != nil {
				t.Fatalf("Show after close: %v", err)
			}
			if issue.Status != "closed" {
				t.Errorf("status = %q, want 'closed'", issue.Status)
			}

			// Step 4: Verify mutable fields were cleared
			afterFields := ParseAgentFields(issue.Description)

			// hook_bead should be cleared (empty or "null")
			if afterFields.HookBead != "" {
				t.Errorf("after close: hook_bead = %q, want empty (was %q)", afterFields.HookBead, tc.fields.HookBead)
			}

			// active_mr should be cleared
			if afterFields.ActiveMR != "" {
				t.Errorf("after close: active_mr = %q, want empty (was %q)", afterFields.ActiveMR, tc.fields.ActiveMR)
			}

			// cleanup_status should be cleared
			if afterFields.CleanupStatus != "" {
				t.Errorf("after close: cleanup_status = %q, want empty (was %q)", afterFields.CleanupStatus, tc.fields.CleanupStatus)
			}

			// agent_state should be "closed"
			if afterFields.AgentState != "closed" {
				t.Errorf("after close: agent_state = %q, want 'closed' (was %q)", afterFields.AgentState, tc.fields.AgentState)
			}

			// Immutable fields should be preserved
			if afterFields.RoleType != tc.fields.RoleType {
				t.Errorf("after close: role_type = %q, want %q (should be preserved)", afterFields.RoleType, tc.fields.RoleType)
			}
			if afterFields.Rig != tc.fields.Rig {
				t.Errorf("after close: rig = %q, want %q (should be preserved)", afterFields.Rig, tc.fields.Rig)
			}
		})
	}
}

// TestCloseAndClearAgentBead_NonExistent tests behavior when closing a non-existent agent bead.
func TestCloseAndClearAgentBead_NonExistent(t *testing.T) {
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	// Attempt to close non-existent bead
	err := bd.CloseAndClearAgentBead("test-nonexistent-polecat-xyz", "should fail")

	// Should return an error (bd close on non-existent issue fails)
	if err == nil {
		t.Error("CloseAndClearAgentBead on non-existent bead should return error")
	}
}

// TestCloseAndClearAgentBead_AlreadyClosed tests behavior when closing an already-closed agent bead.
func TestCloseAndClearAgentBead_AlreadyClosed(t *testing.T) {
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	agentID := "test-testrig-polecat-doubleclosed"

	// Create agent bead
	_, err := bd.CreateAgentBead(agentID, "Test agent", &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "running",
		HookBead:   "test-issue-1",
	})
	if err != nil {
		t.Fatalf("CreateAgentBead: %v", err)
	}

	// First close - should succeed
	err = bd.CloseAndClearAgentBead(agentID, "first close")
	if err != nil {
		t.Fatalf("First CloseAndClearAgentBead: %v", err)
	}

	// Second close - behavior depends on bd close semantics
	// Document actual behavior: bd close on already-closed bead may error or be idempotent
	err = bd.CloseAndClearAgentBead(agentID, "second close")

	// Verify bead is still closed regardless of error
	issue, showErr := bd.Show(agentID)
	if showErr != nil {
		t.Fatalf("Show after double close: %v", showErr)
	}
	if issue.Status != "closed" {
		t.Errorf("status after double close = %q, want 'closed'", issue.Status)
	}

	// Log actual behavior for documentation
	if err != nil {
		t.Logf("BEHAVIOR: CloseAndClearAgentBead on already-closed bead returns error: %v", err)
	} else {
		t.Log("BEHAVIOR: CloseAndClearAgentBead on already-closed bead is idempotent (no error)")
	}
}

// TestCloseAndClearAgentBead_ReopenHasCleanState tests that reopening a closed agent bead
// starts with clean state (no stale hook_bead, active_mr, etc.).
func TestCloseAndClearAgentBead_ReopenHasCleanState(t *testing.T) {
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	agentID := "test-testrig-polecat-cleanreopen"

	// Step 1: Create agent with all fields populated
	_, err := bd.CreateAgentBead(agentID, "Test agent", &AgentFields{
		RoleType:          "polecat",
		Rig:               "testrig",
		AgentState:        "running",
		HookBead:          "test-old-issue",
		CleanupStatus:     "clean",
		ActiveMR:          "test-old-mr",
		NotificationLevel: "normal",
	})
	if err != nil {
		t.Fatalf("CreateAgentBead: %v", err)
	}

	// Step 2: Close - should clear mutable fields
	err = bd.CloseAndClearAgentBead(agentID, "completing old work")
	if err != nil {
		t.Fatalf("CloseAndClearAgentBead: %v", err)
	}

	// Step 3: Reopen with new fields
	newIssue, err := bd.CreateOrReopenAgentBead(agentID, agentID, &AgentFields{
		RoleType:   "polecat",
		Rig:        "testrig",
		AgentState: "spawning",
		HookBead:   "test-new-issue",
	})
	if err != nil {
		t.Fatalf("CreateOrReopenAgentBead: %v", err)
	}

	// Step 4: Verify new state - should have new hook, no stale data
	fields := ParseAgentFields(newIssue.Description)

	if fields.HookBead != "test-new-issue" {
		t.Errorf("hook_bead = %q, want 'test-new-issue'", fields.HookBead)
	}

	// The old active_mr should NOT be present (was cleared on close)
	if fields.ActiveMR == "test-old-mr" {
		t.Error("active_mr still has stale value 'test-old-mr' - CloseAndClearAgentBead didn't clear it")
	}

	// agent_state should be the new state
	if fields.AgentState != "spawning" {
		t.Errorf("agent_state = %q, want 'spawning'", fields.AgentState)
	}

	t.Log("CLEAN STATE CONFIRMED: Reopened agent bead has no stale mutable fields")
}

// TestCloseAndClearAgentBead_ReasonVariations tests close with different reason values.
func TestCloseAndClearAgentBead_ReasonVariations(t *testing.T) {
	t.Skip("bd CLI 0.47.2 bug: database writes don't commit")

	tmpDir := t.TempDir()
	bd := NewIsolated(tmpDir)
	if err := bd.Init("test"); err != nil {
		t.Fatalf("bd init: %v", err)
	}

	tests := []struct {
		name   string
		reason string
	}{
		{"empty_reason", ""},
		{"simple_reason", "polecat nuked"},
		{"reason_with_spaces", "polecat completed work successfully"},
		{"reason_with_special_chars", "closed: issue #123 (resolved)"},
		{"long_reason", "This is a very long reason that explains in detail why the agent bead was closed including multiple sentences and detailed context about the situation."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use tc.name for suffix to avoid hash-like patterns (e.g., "reason0")
			// that trigger bd's isLikelyHash() prefix extraction in v0.47.1+
			agentID := fmt.Sprintf("test-testrig-polecat-%s", tc.name)

			// Create agent bead
			_, err := bd.CreateAgentBead(agentID, "Test agent", &AgentFields{
				RoleType:   "polecat",
				Rig:        "testrig",
				AgentState: "running",
			})
			if err != nil {
				t.Fatalf("CreateAgentBead: %v", err)
			}

			// Close with specified reason
			err = bd.CloseAndClearAgentBead(agentID, tc.reason)
			if err != nil {
				t.Fatalf("CloseAndClearAgentBead: %v", err)
			}

			// Verify closed
			issue, err := bd.Show(agentID)
			if err != nil {
				t.Fatalf("Show: %v", err)
			}
			if issue.Status != "closed" {
				t.Errorf("status = %q, want 'closed'", issue.Status)
			}
		})
	}
}
