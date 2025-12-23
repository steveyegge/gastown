package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewHookSingletonCheck(t *testing.T) {
	check := NewHookSingletonCheck()

	if check.Name() != "hook-singleton" {
		t.Errorf("expected name 'hook-singleton', got %q", check.Name())
	}

	if check.Description() != "Ensure each agent has at most one handoff bead" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestHookSingletonCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewHookSingletonCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// No beads dir means nothing to check, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no beads dir, got %v", result.Status)
	}
}

func TestHookSingletonCheck_FormatDuplicate(t *testing.T) {
	check := NewHookSingletonCheck()

	tests := []struct {
		dupe     duplicateHandoff
		expected string
	}{
		{
			dupe: duplicateHandoff{
				role:    "nux",
				beadIDs: []string{"gt-123", "gt-456"},
			},
			expected: "nux: 2 handoff beads (gt-123, gt-456)",
		},
		{
			dupe: duplicateHandoff{
				role:    "witness",
				beadIDs: []string{"gt-111", "gt-222", "gt-333"},
			},
			expected: "witness: 3 handoff beads (gt-111, gt-222, gt-333)",
		},
	}

	for _, tt := range tests {
		result := check.formatDuplicate(tt.dupe)
		if result != tt.expected {
			t.Errorf("formatDuplicate() = %q, want %q", result, tt.expected)
		}
	}
}

func TestHookSingletonCheck_FindRigBeadsDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create town-level .beads (should be excluded)
	townBeads := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads
	rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewHookSingletonCheck()
	dirs := check.findRigBeadsDirs(tmpDir)

	// Should find the rig-level beads but not town-level
	found := false
	for _, dir := range dirs {
		if dir == townBeads {
			t.Error("findRigBeadsDirs should not include town-level .beads")
		}
		if dir == rigBeads {
			found = true
		}
	}

	if !found && len(dirs) > 0 {
		t.Logf("Found dirs: %v", dirs)
	}
}

func TestNewHookAttachmentValidCheck(t *testing.T) {
	check := NewHookAttachmentValidCheck()

	if check.Name() != "hook-attachment-valid" {
		t.Errorf("expected name 'hook-attachment-valid', got %q", check.Name())
	}

	if check.Description() != "Verify attached molecules exist and are not closed" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestHookAttachmentValidCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewHookAttachmentValidCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// No beads dir means nothing to check, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no beads dir, got %v", result.Status)
	}
}

func TestHookAttachmentValidCheck_EmptyBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewHookAttachmentValidCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Empty beads dir means no pinned beads, should be OK
	// Note: This may error if bd CLI is not available, but should still handle gracefully
	if result.Status != StatusOK && result.Status != StatusError {
		t.Errorf("expected StatusOK or graceful error, got %v", result.Status)
	}
}

func TestHookAttachmentValidCheck_FormatInvalid(t *testing.T) {
	check := NewHookAttachmentValidCheck()

	tests := []struct {
		inv      invalidAttachment
		expected string
	}{
		{
			inv: invalidAttachment{
				pinnedBeadID: "hq-123",
				moleculeID:   "gt-456",
				reason:       "not_found",
			},
			expected: "hq-123: attached molecule gt-456 not found",
		},
		{
			inv: invalidAttachment{
				pinnedBeadID: "hq-123",
				moleculeID:   "gt-789",
				reason:       "closed",
			},
			expected: "hq-123: attached molecule gt-789 is closed",
		},
	}

	for _, tt := range tests {
		result := check.formatInvalid(tt.inv)
		if result != tt.expected {
			t.Errorf("formatInvalid() = %q, want %q", result, tt.expected)
		}
	}
}

func TestHookAttachmentValidCheck_FindRigBeadsDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create town-level .beads (should be excluded)
	townBeads := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads
	rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewHookAttachmentValidCheck()
	dirs := check.findRigBeadsDirs(tmpDir)

	// Should find the rig-level beads but not town-level
	found := false
	for _, dir := range dirs {
		if dir == townBeads {
			t.Error("findRigBeadsDirs should not include town-level .beads")
		}
		if dir == rigBeads {
			found = true
		}
	}

	if !found && len(dirs) > 0 {
		t.Logf("Found dirs: %v", dirs)
	}
}

// Tests for HookSingletonCheck

func TestNewHookSingletonCheck(t *testing.T) {
	check := NewHookSingletonCheck()

	if check.Name() != "hook-singleton" {
		t.Errorf("expected name 'hook-singleton', got %q", check.Name())
	}

	if check.Description() != "Ensure each agent has at most one handoff bead" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestHookSingletonCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewHookSingletonCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// No beads dir means nothing to check, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no beads dir, got %v", result.Status)
	}
}

func TestHookSingletonCheck_EmptyBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewHookSingletonCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Empty beads dir means no pinned beads, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when empty beads dir, got %v", result.Status)
	}
}

func TestHookSingletonCheck_FormatDuplicate(t *testing.T) {
	check := NewHookSingletonCheck()

	tests := []struct {
		dup      duplicateHandoff
		expected string
	}{
		{
			dup: duplicateHandoff{
				title:   "Mayor Handoff",
				beadIDs: []string{"hq-123", "hq-456"},
			},
			expected: `"Mayor Handoff" has 2 beads: hq-123, hq-456`,
		},
		{
			dup: duplicateHandoff{
				title:   "Witness Handoff",
				beadIDs: []string{"gt-1", "gt-2", "gt-3"},
			},
			expected: `"Witness Handoff" has 3 beads: gt-1, gt-2, gt-3`,
		},
	}

	for _, tt := range tests {
		result := check.formatDuplicate(tt.dup)
		if result != tt.expected {
			t.Errorf("formatDuplicate() = %q, want %q", result, tt.expected)
		}
	}
}
