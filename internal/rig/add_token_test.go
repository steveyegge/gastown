package rig

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRemoveRigPathIfOwned_TokenMatches verifies that when the on-disk token
// matches the expected token, the directory is removed.
func TestRemoveRigPathIfOwned_TokenMatches(t *testing.T) {
	rigPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rigPath, "some-file"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	token, err := newAddOwnershipToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := writeAddOwnershipToken(rigPath, token); err != nil {
		t.Fatalf("write token: %v", err)
	}

	removeRigPathIfOwned(rigPath, token)

	if _, err := os.Stat(rigPath); !os.IsNotExist(err) {
		t.Fatalf("expected rig path to be removed, stat err=%v", err)
	}
}

// TestRemoveRigPathIfOwned_TokenMismatch verifies that when the on-disk token
// belongs to a different invocation, the directory is preserved (gh#3683).
func TestRemoveRigPathIfOwned_TokenMismatch(t *testing.T) {
	rigPath := t.TempDir()
	preserved := filepath.Join(rigPath, "preserve-me")
	if err := os.WriteFile(preserved, []byte("important"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Disk has a token from a *different* (later, successful) add.
	if err := writeAddOwnershipToken(rigPath, "newer-token"); err != nil {
		t.Fatalf("write token: %v", err)
	}

	// Stale rollback runs with its own (older) token.
	removeRigPathIfOwned(rigPath, "older-token")

	if _, err := os.Stat(preserved); err != nil {
		t.Fatalf("preserved file was deleted: %v", err)
	}
}

// TestRemoveRigPathIfOwned_TokenMissingNonEmpty verifies that a missing token
// on a non-empty directory is treated as not-owned and skipped — covers the
// case where a successful re-add has already cleared its token.
func TestRemoveRigPathIfOwned_TokenMissingNonEmpty(t *testing.T) {
	rigPath := t.TempDir()
	preserved := filepath.Join(rigPath, "rig-content")
	if err := os.WriteFile(preserved, []byte("important"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	removeRigPathIfOwned(rigPath, "stale-token")

	if _, err := os.Stat(preserved); err != nil {
		t.Fatalf("preserved file was deleted: %v", err)
	}
}

// TestRemoveRigPathIfOwned_TokenMissingEmpty verifies that an empty directory
// (no token, no content) is removed — there's nothing to protect.
func TestRemoveRigPathIfOwned_TokenMissingEmpty(t *testing.T) {
	rigPath := t.TempDir()

	removeRigPathIfOwned(rigPath, "stale-token")

	if _, err := os.Stat(rigPath); !os.IsNotExist(err) {
		t.Fatalf("expected empty rig path to be removed, stat err=%v", err)
	}
}

// TestRemoveRigPathIfOwned_NoExpectedToken verifies that when no token was
// ever issued (early failure path), cleanup proceeds unconditionally.
func TestRemoveRigPathIfOwned_NoExpectedToken(t *testing.T) {
	rigPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rigPath, "x"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	removeRigPathIfOwned(rigPath, "")

	if _, err := os.Stat(rigPath); !os.IsNotExist(err) {
		t.Fatalf("expected rig path to be removed, stat err=%v", err)
	}
}

// TestRemoveRigPathIfOwned_PathMissing covers the os.ReadDir error path
// inside removeRigPathIfOwned (rigPath does not exist).
func TestRemoveRigPathIfOwned_PathMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	// Should not panic; should not create the path.
	removeRigPathIfOwned(missing, "stale-token")

	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("expected missing path to remain missing, stat err=%v", err)
	}
}

// TestStampAddOwnershipToken_RoundTrip covers the success path: a fresh
// token is written and round-trips via readAddOwnershipToken.
func TestStampAddOwnershipToken_RoundTrip(t *testing.T) {
	rigPath := t.TempDir()

	token, err := stampAddOwnershipToken(rigPath)
	if err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	got := readAddOwnershipToken(rigPath)
	if got != token {
		t.Errorf("readAddOwnershipToken = %q, want %q", got, token)
	}
}

// TestStampAddOwnershipToken_WriteFailureCleansUp covers the error path
// where the underlying os.WriteFile call fails (the rigPath's parent does
// not exist). The helper must surface the error rather than silently
// returning an empty token.
func TestStampAddOwnershipToken_WriteFailureCleansUp(t *testing.T) {
	parent := t.TempDir()
	bogus := filepath.Join(parent, "missing-parent", "rig")

	token, err := stampAddOwnershipToken(bogus)
	if err == nil {
		t.Fatalf("expected error for unwritable rigPath, got token=%q", token)
	}
	if token != "" {
		t.Errorf("token should be empty on error, got %q", token)
	}
}

// TestClearAddOwnershipToken_OnAbsentFile covers the error-return path of
// clearAddOwnershipToken (called best-effort on success in AddRig).
func TestClearAddOwnershipToken_OnAbsentFile(t *testing.T) {
	rigPath := t.TempDir()

	if err := clearAddOwnershipToken(rigPath); err == nil {
		t.Fatal("expected error removing nonexistent token file")
	}
}
