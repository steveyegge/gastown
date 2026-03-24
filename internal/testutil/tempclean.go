package testutil

import (
	"os"
	"path/filepath"
	"time"
)

// staleTempAge is the minimum age before a temp artifact is considered stale
// and eligible for cleanup. Set conservatively to avoid removing artifacts
// from a still-running test suite.
const staleTempAge = 4 * time.Hour

// knownTempPatterns are os.MkdirTemp patterns used by gastown production code
// and tests. When tests crash or are killed (SIGKILL), deferred cleanup never
// runs, leaving these directories orphaned in os.TempDir().
var knownTempPatterns = []string{
	"gt-clone-*",        // internal/git/git.go: cloneInternal
	"gt-agent-bin-*",    // internal/config/test_main_test.go: stub binaries
	"namepool-test-*",   // internal/polecat/namepool_test.go
	"wl-browse-*",       // internal/cmd/wl_browse.go
	"gt-test-dolt-*",    // testutil: Dolt data dirs
	"gastown-test-run-*", // test run root dirs
}

// knownTempFiles are individual files left in os.TempDir() by tests.
var knownTempFiles = []string{
	"gt-integration-test", // internal/cmd/test_helpers_test.go: cached binary
}

// CleanStaleTempDirs removes stale gastown test artifacts from the system
// temp directory. Call from TestMain before m.Run() to reclaim disk space
// and prevent "no space left on device" failures on macOS.
//
// Only removes artifacts older than staleTempAge (4h) to avoid interfering
// with concurrently running test suites.
func CleanStaleTempDirs() {
	sysTmp := os.TempDir()
	cutoff := time.Now().Add(-staleTempAge)

	// Clean known directory patterns.
	for _, pat := range knownTempPatterns {
		matches, err := filepath.Glob(filepath.Join(sysTmp, pat))
		if err != nil {
			continue
		}
		for _, m := range matches {
			removeIfStale(m, cutoff)
		}
	}

	// Clean known individual files.
	for _, name := range knownTempFiles {
		removeIfStale(filepath.Join(sysTmp, name), cutoff)
	}
}

// removeIfStale removes path if it was last modified before cutoff.
func removeIfStale(path string, cutoff time.Time) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.ModTime().Before(cutoff) {
		_ = os.RemoveAll(path)
	}
}
