package rig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateRefAlias(t *testing.T) {
	tests := []struct {
		alias string
		valid bool
	}{
		{"gastown", true},
		{"my-ref", true},
		{"ref_name", true},
		{"", false},
		{"has/slash", false},
		{"has\\backslash", false},
		{"has.dot", false},
		{"has space", false},
		{"has\ttab", false},
		{"has\nnewline", false},
	}

	for _, tt := range tests {
		err := ValidateRefAlias(tt.alias)
		if tt.valid && err != nil {
			t.Errorf("ValidateRefAlias(%q) = %v, want nil", tt.alias, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ValidateRefAlias(%q) = nil, want error", tt.alias)
		}
	}
}

func TestRefsDir(t *testing.T) {
	got := RefsDir("/data/gt/dma")
	want := "/data/gt/dma/refs"
	if got != want {
		t.Errorf("RefsDir = %q, want %q", got, want)
	}
}

func TestRefPath(t *testing.T) {
	got := RefPath("/data/gt/dma", "gastown")
	want := "/data/gt/dma/refs/gastown"
	if got != want {
		t.Errorf("RefPath = %q, want %q", got, want)
	}
}

func TestLinkSameTownRef(t *testing.T) {
	// Create a temp town structure
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "target-rig")
	sourceRig := filepath.Join(townRoot, "source-rig")

	// Create source rig with refinery/rig/
	sourceClone := filepath.Join(sourceRig, "refinery", "rig")
	if err := os.MkdirAll(sourceClone, 0755); err != nil {
		t.Fatal(err)
	}
	// Write a marker file so we can verify the symlink
	if err := os.WriteFile(filepath.Join(sourceClone, "marker.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create target rig dir
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Link
	if err := LinkSameTownRef(townRoot, rigPath, "source", "source-rig"); err != nil {
		t.Fatalf("LinkSameTownRef: %v", err)
	}

	// Verify symlink exists
	dest := RefPath(rigPath, "source")
	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file")
	}

	// Verify we can read through symlink
	data, err := os.ReadFile(filepath.Join(dest, "marker.txt"))
	if err != nil {
		t.Fatalf("reading through symlink: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("marker content = %q, want %q", string(data), "hello")
	}

	// Verify duplicate link fails
	if err := LinkSameTownRef(townRoot, rigPath, "source", "source-rig"); err == nil {
		t.Error("expected error on duplicate link")
	}
}

func TestLinkSameTownRef_MissingSource(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "target-rig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	err := LinkSameTownRef(townRoot, rigPath, "missing", "nonexistent-rig")
	if err == nil {
		t.Error("expected error for missing source rig")
	}
}

func TestUnlinkRef_Symlink(t *testing.T) {
	rigPath := t.TempDir()
	refsDir := RefsDir(rigPath)
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink
	target := t.TempDir()
	dest := RefPath(rigPath, "myref")
	if err := os.Symlink(target, dest); err != nil {
		t.Fatal(err)
	}

	// Unlink
	if err := UnlinkRef(rigPath, "myref"); err != nil {
		t.Fatalf("UnlinkRef: %v", err)
	}

	// Verify removed
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed")
	}
}

func TestUnlinkRef_Clone(t *testing.T) {
	rigPath := t.TempDir()
	refsDir := RefsDir(rigPath)
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a directory (simulating a clone)
	dest := RefPath(rigPath, "myclone")
	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	// Unlink
	if err := UnlinkRef(rigPath, "myclone"); err != nil {
		t.Fatalf("UnlinkRef: %v", err)
	}

	// Verify removed
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("expected clone dir to be removed")
	}
}

func TestUnlinkRef_NotFound(t *testing.T) {
	rigPath := t.TempDir()
	err := UnlinkRef(rigPath, "nonexistent")
	if err == nil {
		t.Error("expected error for missing ref")
	}
}

func TestListRefs(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "target-rig")

	// Create source rig for symlink
	sourceClone := filepath.Join(townRoot, "source-rig", "refinery", "rig")
	if err := os.MkdirAll(sourceClone, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink ref
	if err := LinkSameTownRef(townRoot, rigPath, "linked", "source-rig"); err != nil {
		t.Fatal(err)
	}

	// Create a fake clone ref (just a directory)
	clonePath := RefPath(rigPath, "cloned")
	if err := os.MkdirAll(clonePath, 0755); err != nil {
		t.Fatal(err)
	}

	refs := map[string]RefEntry{
		"linked": {FromRig: "source-rig", AddedAt: time.Now()},
		"cloned": {GitURL: "https://example.com/repo.git", AddedAt: time.Now()},
		"gone":   {GitURL: "https://example.com/gone.git", AddedAt: time.Now()},
	}

	statuses := ListRefs(rigPath, refs)
	if len(statuses) != 3 {
		t.Fatalf("ListRefs returned %d entries, want 3", len(statuses))
	}

	// Build a map for easier assertion
	byAlias := make(map[string]RefStatus)
	for _, s := range statuses {
		byAlias[s.Alias] = s
	}

	if s := byAlias["linked"]; s.Type != "symlink" || s.Status != "ok" {
		t.Errorf("linked: type=%q status=%q, want symlink/ok", s.Type, s.Status)
	}
	if s := byAlias["cloned"]; s.Type != "clone" || s.Status != "ok" {
		t.Errorf("cloned: type=%q status=%q, want clone/ok", s.Type, s.Status)
	}
	if s := byAlias["gone"]; s.Status != "missing" {
		t.Errorf("gone: status=%q, want missing", s.Status)
	}
}

func TestSyncRef_Symlink(t *testing.T) {
	rigPath := t.TempDir()
	refsDir := RefsDir(rigPath)
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink
	target := t.TempDir()
	dest := RefPath(rigPath, "myref")
	if err := os.Symlink(target, dest); err != nil {
		t.Fatal(err)
	}

	// Sync should be a no-op for symlinks
	if err := SyncRef(rigPath, "myref"); err != nil {
		t.Fatalf("SyncRef on symlink: %v", err)
	}
}
