package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestConvoyTracksBeadExactMatch verifies that convoyTracksBead finds a bead
// when the dep list returns the raw beadID (no external: wrapping).
func TestConvoyTracksBeadExactMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd to return a tracked dep with raw beadID
	bdScript := `#!/bin/sh
echo '[{"id":"gt-abc123"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if !convoyTracksBead(beadsDir, "hq-cv-test1", "gt-abc123") {
		t.Error("convoyTracksBead should return true for exact match")
	}
}

// TestConvoyTracksBeadExternalRef verifies that convoyTracksBead finds a bead
// when the dep list returns an external-formatted reference.
func TestConvoyTracksBeadExternalRef(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd to return a tracked dep with external:prefix:beadID format
	bdScript := `#!/bin/sh
echo '[{"id":"external:gt-abc:gt-abc123"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if !convoyTracksBead(beadsDir, "hq-cv-test2", "gt-abc123") {
		t.Error("convoyTracksBead should return true for external ref match")
	}
}

// TestConvoyTracksBeadNoMatch verifies that convoyTracksBead returns false
// when the convoy tracks a different bead.
func TestConvoyTracksBeadNoMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd to return a tracked dep with a different beadID
	bdScript := `#!/bin/sh
echo '[{"id":"gt-other456"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if convoyTracksBead(beadsDir, "hq-cv-test3", "gt-abc123") {
		t.Error("convoyTracksBead should return false when bead not tracked")
	}
}

// TestConvoyTracksBeadEmptyDeps verifies that convoyTracksBead returns false
// when the convoy has no tracked deps.
func TestConvoyTracksBeadEmptyDeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd to return empty array
	bdScript := `#!/bin/sh
echo '[]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if convoyTracksBead(beadsDir, "hq-cv-test4", "gt-abc123") {
		t.Error("convoyTracksBead should return false for empty deps")
	}
}

// TestConvoyTracksBeadMultipleDeps verifies that convoyTracksBead finds the
// target bead among multiple tracked deps.
func TestConvoyTracksBeadMultipleDeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd to return multiple tracked deps, one of which matches
	bdScript := `#!/bin/sh
echo '[{"id":"gt-other1"},{"id":"external:gt-abc:gt-abc123"},{"id":"gt-other2"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if !convoyTracksBead(beadsDir, "hq-cv-test5", "gt-abc123") {
		t.Error("convoyTracksBead should return true when bead found among multiple deps")
	}
}
