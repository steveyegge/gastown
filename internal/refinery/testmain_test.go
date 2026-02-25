package refinery

import (
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.CleanupDoltServer()
	os.Exit(code)
}
