package beads

import (
	"strings"
	"testing"
)

func TestPersonaBeadID_Format(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		pname  string
		want   string
	}{
		{
			"rig-level",
			"gt", "beads", "rust-expert",
			"gt-beads-persona-rust-expert",
		},
		{
			"town-level (empty rig)",
			"gt", "", "rust-expert",
			"gt-persona-rust-expert",
		},
		{
			"different prefix",
			"bd", "myrig", "senior-dev",
			"bd-myrig-persona-senior-dev",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PersonaBeadID(tt.prefix, tt.rig, tt.pname)
			if got != tt.want {
				t.Errorf("PersonaBeadID(%q,%q,%q) = %q, want %q",
					tt.prefix, tt.rig, tt.pname, got, tt.want)
			}
		})
	}
}

func TestFormatAndParsePersonaDescription(t *testing.T) {
	content := "# Rust Expert\n\nYou are a Rust expert.\n"
	hash := "abc123def456"
	desc := formatPersonaDescription("rust-expert", hash, "rig", content)

	if !strings.Contains(desc, "managed_by: gt crew persona sync") {
		t.Error("missing managed_by line")
	}
	if !strings.Contains(desc, "hash: sha256:"+hash) {
		t.Errorf("missing hash line, got:\n%s", desc)
	}
	if !strings.Contains(desc, "source: .personas/rust-expert.md") {
		t.Error("missing source line")
	}
	if !strings.Contains(desc, "scope: rig") {
		t.Error("missing scope line")
	}

	// Check round-trip
	gotHash := parsePersonaHash(desc)
	if gotHash != hash {
		t.Errorf("parsePersonaHash = %q, want %q", gotHash, hash)
	}

	gotContent := parsePersonaContent(desc)
	if gotContent != content {
		t.Errorf("parsePersonaContent = %q, want %q", gotContent, content)
	}

	gotSource := parsePersonaSource(desc)
	if gotSource != "rig" {
		t.Errorf("parsePersonaSource = %q, want %q", gotSource, "rig")
	}
}

func TestParsePersonaHash_Missing(t *testing.T) {
	got := parsePersonaHash("no hash here")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestParsePersonaContent_NoSeparator(t *testing.T) {
	got := parsePersonaContent("no separator")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestEnsurePersonaBead_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	b := NewIsolated(tmpDir)
	if err := b.Init("test"); err != nil {
		t.Skipf("bd init failed (no bd binary): %v", err)
	}

	content := "# Rust Expert\n\nYou are a Rust expert.\n"
	hash := "deadbeef"

	id, updated, err := EnsurePersonaBead(b, "test", "myrig", "rust-expert", content, hash, false)
	if err != nil {
		t.Fatalf("EnsurePersonaBead: %v", err)
	}
	if id != "test-myrig-persona-rust-expert" {
		t.Errorf("id = %q, want %q", id, "test-myrig-persona-rust-expert")
	}
	if !updated {
		t.Error("expected updated=true for new bead")
	}

	// Verify content retrievable
	got, err := GetPersonaContent(b, id)
	if err != nil {
		t.Fatalf("GetPersonaContent: %v", err)
	}
	if got != content {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestEnsurePersonaBead_SkipsOnSameHash(t *testing.T) {
	tmpDir := t.TempDir()
	b := NewIsolated(tmpDir)
	if err := b.Init("test"); err != nil {
		t.Skipf("bd init failed (no bd binary): %v", err)
	}

	content := "# Alice\n\nYou are Alice.\n"
	hash := "samehash"

	// First create
	if _, _, err := EnsurePersonaBead(b, "test", "rig", "alice", content, hash, false); err != nil {
		t.Fatalf("first EnsurePersonaBead: %v", err)
	}

	// Second call with same hash — should skip
	_, updated, err := EnsurePersonaBead(b, "test", "rig", "alice", content, hash, false)
	if err != nil {
		t.Fatalf("second EnsurePersonaBead: %v", err)
	}
	if updated {
		t.Error("expected updated=false when hash unchanged")
	}
}

func TestEnsurePersonaBead_UpdatesOnHashChange(t *testing.T) {
	tmpDir := t.TempDir()
	b := NewIsolated(tmpDir)
	if err := b.Init("test"); err != nil {
		t.Skipf("bd init failed (no bd binary): %v", err)
	}

	// Create with initial content
	if _, _, err := EnsurePersonaBead(b, "test", "rig", "alice", "old content", "oldhash", false); err != nil {
		t.Fatalf("initial EnsurePersonaBead: %v", err)
	}

	// Update with new content
	newContent := "# Alice v2\n\nUpdated content.\n"
	id, updated, err := EnsurePersonaBead(b, "test", "rig", "alice", newContent, "newhash", false)
	if err != nil {
		t.Fatalf("update EnsurePersonaBead: %v", err)
	}
	if !updated {
		t.Error("expected updated=true when hash changed")
	}

	// Verify new content
	got, err := GetPersonaContent(b, id)
	if err != nil {
		t.Fatalf("GetPersonaContent after update: %v", err)
	}
	if got != newContent {
		t.Errorf("content after update = %q, want %q", got, newContent)
	}
}

func TestGetPersonaContent_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	b := NewIsolated(tmpDir)
	if err := b.Init("test"); err != nil {
		t.Skipf("bd init failed (no bd binary): %v", err)
	}

	got, err := GetPersonaContent(b, "test-nonexistent-persona")
	if err != nil {
		t.Fatalf("GetPersonaContent: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing bead, got %q", got)
	}
}
