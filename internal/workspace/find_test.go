package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func realPath(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("realpath: %v", err)
	}
	return real
}

func TestFindWithPrimaryMarker(t *testing.T) {
	// Create temp workspace structure
	root := realPath(t, t.TempDir())
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	townFile := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townFile, []byte(`{"type":"town"}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Create nested directory
	nested := filepath.Join(root, "some", "deep", "path")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	// Find from nested should return root
	found, err := Find(nested)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found != root {
		t.Errorf("Find = %q, want %q", found, root)
	}
}

func TestFindWithSecondaryMarker(t *testing.T) {
	// Create temp workspace with just mayor/ directory
	root := realPath(t, t.TempDir())
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create nested directory
	nested := filepath.Join(root, "rigs", "test")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	// Find from nested should return root
	found, err := Find(nested)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found != root {
		t.Errorf("Find = %q, want %q", found, root)
	}
}

func TestFindNotFound(t *testing.T) {
	// Create temp dir with no markers
	dir := t.TempDir()

	found, err := Find(dir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found != "" {
		t.Errorf("Find = %q, want empty string", found)
	}
}

func TestFindOrErrorNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := FindOrError(dir)
	if err != ErrNotFound {
		t.Errorf("FindOrError = %v, want ErrNotFound", err)
	}
}

func TestFindAtRoot(t *testing.T) {
	// Create workspace at temp root level
	root := realPath(t, t.TempDir())
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	townFile := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townFile, []byte(`{"type":"town"}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Find from root should return root
	found, err := Find(root)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found != root {
		t.Errorf("Find = %q, want %q", found, root)
	}
}

func TestIsWorkspace(t *testing.T) {
	root := t.TempDir()

	// Not a workspace initially
	is, err := IsWorkspace(root)
	if err != nil {
		t.Fatalf("IsWorkspace: %v", err)
	}
	if is {
		t.Error("expected not a workspace initially")
	}

	// Add primary marker (mayor/town.json)
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	townFile := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townFile, []byte(`{"type":"town"}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Now is a workspace
	is, err = IsWorkspace(root)
	if err != nil {
		t.Fatalf("IsWorkspace: %v", err)
	}
	if !is {
		t.Error("expected to be a workspace")
	}
}

func TestFindFollowsSymlinks(t *testing.T) {
	root := realPath(t, t.TempDir())
	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	townFile := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townFile, []byte(`{"type":"town"}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	linkTarget := filepath.Join(root, "actual")
	if err := os.MkdirAll(linkTarget, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	linkName := filepath.Join(root, "linked")
	if err := os.Symlink(linkTarget, linkName); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	found, err := Find(linkName)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found != root {
		t.Errorf("Find = %q, want %q", found, root)
	}
}

func TestResolvePath(t *testing.T) {
	root := realPath(t, t.TempDir())
	subdir := filepath.Join(root, "some", "path")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	resolved, err := ResolvePath(subdir)
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}

	if resolved != subdir {
		t.Errorf("ResolvePath = %q, want %q", resolved, subdir)
	}
}

func TestResolvePath_Symlink(t *testing.T) {
	root := realPath(t, t.TempDir())
	actual := filepath.Join(root, "actual")
	if err := os.MkdirAll(actual, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	link := filepath.Join(root, "link")
	if err := os.Symlink(actual, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	resolved, err := ResolvePath(link)
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}

	if resolved != actual {
		t.Errorf("ResolvePath(%q) = %q, want %q", link, resolved, actual)
	}
}

func TestRelativePathWithSymlinks(t *testing.T) {
	root := realPath(t, t.TempDir())

	mayorDir := filepath.Join(root, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	townFile := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townFile, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	worktree := filepath.Join(root, "gastownui", "polecats", "furiosa")
	if err := os.MkdirAll(worktree, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	townRoot, err := Find(worktree)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	cwd, err := ResolvePath(worktree)
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}

	relPath, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}

	if relPath != "gastownui/polecats/furiosa" {
		t.Errorf("Rel = %q, want 'gastownui/polecats/furiosa'", relPath)
	}
}
