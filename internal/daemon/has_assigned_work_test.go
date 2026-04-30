package daemon

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

// TestHasAssignedOpenWork_DoesNotPassRigFlag is a regression guard for a gt/bd
// compatibility break. steveyegge/beads@d7629204 removed multi-rig routing and
// retired the --rig flag from bd list/ready. gt PR #3294 (landed the same day,
// earlier) added hasAssignedOpenWork which called `bd list --rig=<name>`.
// From that point on, every invocation errored with "unknown flag: --rig",
// cmd.Output() returned err!=nil, the loop skipped all three statuses, and
// the function silently returned false — defeating the idle-reaper-protection
// purpose it was added for.
//
// This test fails if anyone reintroduces --rig to the call.
func TestHasAssignedOpenWork_DoesNotPassRigFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mock for bd")
	}
	binDir := t.TempDir()
	argsFile := filepath.Join(binDir, "bd-args.log")

	// Fake bd: log all args to a file, succeed with empty list.
	// If any invocation passes --rig, a real bd would exit 1 with
	// "unknown flag: --rig" — we mirror that here so accidental
	// regressions fail loudly instead of silently.
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  case \"$arg\" in\n" +
		"    --rig=*|--rig) echo \"Error: unknown flag: --rig\" >&2; exit 1;;\n" +
		"  esac\n" +
		"  echo \"$arg\" >> " + argsFile + "\n" +
		"done\n" +
		"echo '[]'\n"
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake bd: %v", err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&logBuf, "", 0),
		tmux:   tmux.NewTmux(),
		bdPath: bdPath,
	}

	got := d.hasAssignedOpenWork("myr", "myr/polecats/mycat")
	if got != false {
		t.Fatalf("hasAssignedOpenWork() with empty bd response = %v, want false", got)
	}

	argsBytes, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("bd was not invoked (no args file): %v", err)
	}
	argsText := string(argsBytes)

	if strings.Contains(argsText, "--rig") {
		t.Errorf("hasAssignedOpenWork passed --rig to bd — regression of d7629204 compatibility fix.\nCaptured args:\n%s", argsText)
	}

	// Sanity-check the call shape: should still pass list + assignee + status + json.
	wantArgs := []string{"list", "--assignee=", "--status=", "--json"}
	for _, want := range wantArgs {
		if !strings.Contains(argsText, want) {
			t.Errorf("expected bd args to contain %q, got:\n%s", want, argsText)
		}
	}
}
