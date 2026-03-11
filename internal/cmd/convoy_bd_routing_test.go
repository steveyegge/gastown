package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func captureConvoyStdoutErr(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	_ = r.Close()

	return buf.String(), runErr
}

func writeRoutingBdStub(t *testing.T, scriptBody string) {
	t.Helper()

	binDir := t.TempDir()
	bdPath := filepath.Join(binDir, "bd")
	script := "#!/bin/sh\n" + scriptBody
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func chdirConvoyTest(t *testing.T, dir string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func makeRoutingTownWorkspace(t *testing.T) (string, string) {
	t.Helper()

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	expectedWD := townRoot
	if resolved, err := filepath.EvalSymlinks(townRoot); err == nil && resolved != "" {
		expectedWD = resolved
	}
	return townRoot, expectedWD
}

func TestRunConvoyList_UsesTownRootAndStripsBeadsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, expectedWD := makeRoutingTownWorkspace(t)
	chdirConvoyTest(t, townRoot)
	t.Setenv("BEADS_DIR", "/wrong/.beads")

	scriptBody := fmt.Sprintf(`
if [ -n "$BEADS_DIR" ]; then
  echo "BEADS_DIR leaked: $BEADS_DIR" >&2
  exit 1
fi

case "$*" in
  "list --type=convoy --json --all --flat")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[{"id":"hq-cv-town","title":"Town convoy","status":"open","created_at":"2026-03-09T00:00:00Z"}]'
    ;;
  "dep list hq-cv-town --direction=down --type=tracks --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[]'
    ;;
  "show hq-cv-town --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[{"id":"hq-cv-town","title":"Town convoy","status":"open","issue_type":"convoy","dependencies":[]}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, expectedWD, expectedWD, expectedWD)
	writeRoutingBdStub(t, scriptBody)

	oldJSON, oldAll, oldStatus, oldTree := convoyListJSON, convoyListAll, convoyListStatus, convoyListTree
	convoyListJSON = true
	convoyListAll = true
	convoyListStatus = ""
	convoyListTree = false
	t.Cleanup(func() {
		convoyListJSON = oldJSON
		convoyListAll = oldAll
		convoyListStatus = oldStatus
		convoyListTree = oldTree
	})

	out, err := captureConvoyStdoutErr(t, func() error {
		return runConvoyList(nil, nil)
	})
	if err != nil {
		t.Fatalf("runConvoyList: %v", err)
	}
	if !strings.Contains(out, `"id": "hq-cv-town"`) {
		t.Fatalf("expected convoy JSON output, got:\n%s", out)
	}
}

func TestRunConvoyStatus_UsesTownRootAndStripsBeadsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, expectedWD := makeRoutingTownWorkspace(t)
	chdirConvoyTest(t, townRoot)
	t.Setenv("BEADS_DIR", "/wrong/.beads")

	scriptBody := fmt.Sprintf(`
if [ -n "$BEADS_DIR" ]; then
  echo "BEADS_DIR leaked: $BEADS_DIR" >&2
  exit 1
fi

case "$*" in
  "show hq-cv-status --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[{"id":"hq-cv-status","title":"Status convoy","status":"open","issue_type":"convoy","created_at":"2026-03-09T00:00:00Z","labels":[],"dependencies":[]}]'
    ;;
  "dep list hq-cv-status --direction=down --type=tracks --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, expectedWD, expectedWD)
	writeRoutingBdStub(t, scriptBody)

	oldJSON := convoyStatusJSON
	convoyStatusJSON = false
	t.Cleanup(func() { convoyStatusJSON = oldJSON })

	out, err := captureConvoyStdoutErr(t, func() error {
		return runConvoyStatus(nil, []string{"hq-cv-status"})
	})
	if err != nil {
		t.Fatalf("runConvoyStatus: %v", err)
	}
	if !strings.Contains(out, "hq-cv-status") || !strings.Contains(out, "Progress:  0/0 completed") {
		t.Fatalf("unexpected status output:\n%s", out)
	}
}
