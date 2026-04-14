package daemon

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// writeFakeTmuxForRestart creates a fake tmux binary suited for testing the
// stuck-deacon restart path. It differs from writeFakeTmuxCrashLoop (which
// only needs has-session) because:
//
//   - display-message returns "claude" so that IsAgentAlive returns true and
//     deacon.Manager.Start returns ErrAlreadyRunning immediately, avoiding the
//     180-second WaitForCommand timeout.
//   - capture-pane returns ">" (a prompt indicator) so AcceptStartupDialogs
//     exits early instead of polling for 8 seconds.
//
// All commands are logged to TMUX_LOG (if set) for assertion.
func writeFakeTmuxForRestart(t *testing.T, dir string) {
	t.Helper()
	script := `#!/usr/bin/env bash
set -euo pipefail

cmd=""
skip_next=0
for arg in "$@"; do
  if [[ "$skip_next" -eq 1 ]]; then
    skip_next=0
    continue
  fi
  if [[ "$arg" == "-u" ]]; then
    continue
  fi
  if [[ "$arg" == "-L" ]]; then
    skip_next=1
    continue
  fi
  cmd="$arg"
  break
done

if [[ -n "${TMUX_LOG:-}" ]]; then
  printf "%s %s\n" "$cmd" "$*" >> "$TMUX_LOG"
fi

if [[ "${1:-}" == "-V" ]]; then
  echo "tmux 3.3a"
  exit 0
fi

if [[ "$cmd" == "has-session" ]]; then
  exit 0
fi

# Return "claude" for display-message so IsAgentAlive returns true.
# This causes deacon.Manager.Start to return ErrAlreadyRunning quickly,
# avoiding the 3-minute WaitForCommand polling timeout.
if [[ "$cmd" == "display-message" ]]; then
  echo "claude"
  exit 0
fi

# Return a prompt indicator so AcceptWorkspaceTrustDialog exits early.
if [[ "$cmd" == "capture-pane" ]]; then
  echo ">"
  exit 0
fi

exit 0
`
	path := filepath.Join(dir, "tmux")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
}

// TestCheckDeaconHeartbeat_VeryStaleTriggersRestart is a regression test for
// gh-3638: "Daemon stuck-deacon handler logs but doesn't restart".
//
// Before commit e4fac780, checkDeaconHeartbeat contained three "Detection only"
// stubs that logged a stuck-deacon message but never called restartStuckDeacon.
// A user reported a 155-hour deacon outage caused by this — the daemon knew the
// deacon was stuck but took no action.
//
// This test verifies that a very stale heartbeat (>= 20 min) causes the daemon
// to call kill-session on the deacon tmux session (i.e., actually restarts it).
func TestCheckDeaconHeartbeat_VeryStaleTriggersRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows — fake tmux requires bash")
	}

	townRoot := t.TempDir()
	fakeBinDir := t.TempDir()
	tmuxLog := filepath.Join(t.TempDir(), "tmux.log")
	if err := os.WriteFile(tmuxLog, []byte{}, 0o644); err != nil {
		t.Fatalf("create tmux log: %v", err)
	}

	writeFakeTmuxForRestart(t, fakeBinDir)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX_LOG", tmuxLog)

	// 21 minutes old — crosses the IsVeryStale threshold (20 min).
	writeDeaconHeartbeat(t, townRoot, 21*time.Minute)

	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(io.Discard, "", 0),
		tmux:   tmux.NewTmux(),
		ctx:    context.Background(),
	}

	d.checkDeaconHeartbeat()

	data, err := os.ReadFile(tmuxLog)
	if err != nil {
		t.Fatalf("read tmux log: %v", err)
	}

	kills := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(line, "kill-session ") {
			kills++
		}
	}
	if kills == 0 {
		t.Fatalf("kill-session not called — stuck-deacon restart was not triggered\n"+
			"tmux log:\n%s\n\n"+
			"Regression: gh-3638 — daemon detected stuck deacon but did not restart it.",
			string(data))
	}
}
