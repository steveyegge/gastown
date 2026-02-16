package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func disableStillnessGate(t *testing.T) {
	t.Helper()
	if isProductionGovernanceEnv() {
		t.Fatalf("disableStillnessGate cannot be used when GT_GOVERNANCE_ENV=production")
	}
	t.Setenv("GT_STILLNESS_GATE", "off")
}

func requireSymlinkCapability(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")

	if err := os.WriteFile(target, []byte("ok"), 0644); err != nil {
		t.Fatalf("write symlink capability target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink tests skipped: Windows symlink capability unavailable (Developer Mode or elevated token required): %v", err)
		}
		t.Fatalf("symlink capability required for this test: %v", err)
	}
	_ = os.Remove(link)
}
