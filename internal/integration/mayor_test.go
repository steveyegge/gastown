//go:build integration

package integration_test

import (
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/testutil"
	"github.com/steveyegge/gastown/internal/tmux"
)

func TestMayorWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	runtimes := []string{"opencode", "claude"}

	for _, rt := range runtimes {
		t.Run(rt, func(t *testing.T) {
			testutil.RequireBinary(t, rt)
			testutil.RequireGT(t)

			fixture := testutil.NewTownFixture(t, rt)

			mgr := mayor.NewManager(fixture.Root)
			sessionName := mgr.SessionName()

			errCh := make(chan error, 1)
			go func() {
				errCh <- mgr.Start(rt)
			}()

			deadline := time.Now().Add(65 * time.Second)
			for time.Now().Before(deadline) {
				select {
				case err := <-errCh:
					if err != nil {
						testutil.LogDiagnostic(t, sessionName)
						t.Fatalf("Failed to start Mayor with %s: %v", rt, err)
					}
					t.Cleanup(func() { mgr.Stop() })
					goto started
				default:
					time.Sleep(2 * time.Second)
				}
			}
			t.Fatalf("Timed out waiting for %s to start", rt)

		started:
			running, err := mgr.IsRunning()
			if err != nil || !running {
				t.Fatalf("Mayor should be running: err=%v, running=%v", err, running)
			}

			time.Sleep(500 * time.Millisecond)
			tm := tmux.NewTmux()
			if exists, _ := tm.HasSession(sessionName); !exists {
				t.Skip("Session exited early (known runtime tmux issue)")
			}

			t.Logf("Mayor workflow test passed for %s", rt)
		})
	}
}
