package convoy

import (
	"fmt"
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/testutil"
)

func TestMain(m *testing.M) {
	// Start an ephemeral Dolt server for this package's tests.
	// setupTestStore sets BEADS_TEST_MODE=1, which causes the beads SDK
	// to create testdb_<hash> databases. By routing those to an isolated
	// server (via BEADS_DOLT_PORT), the databases are destroyed when the
	// server's temp data dir is removed at cleanup — preventing orphan
	// accumulation in the shared production Dolt data dir.
	if err := testutil.EnsureDoltForTestMain(); err != nil {
		fmt.Fprintf(os.Stderr, "convoy TestMain: dolt setup: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testutil.CleanupDoltServer()
	os.Exit(code)
}
