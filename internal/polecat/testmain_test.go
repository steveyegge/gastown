package polecat

import (
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/testutil"
)

func TestMain(m *testing.M) {
	// Clean up stale temp artifacts from previous test runs to prevent
	// "no space left on device" failures on macOS.
	testutil.CleanStaleTempDirs()

	code := m.Run()
	testutil.TerminateDoltContainer()
	os.Exit(code)
}
