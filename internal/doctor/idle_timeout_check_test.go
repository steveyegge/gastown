package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdleTimeoutCheck_Run(t *testing.T) {
	townRoot := t.TempDir()

	// Create town beads with routes.jsonl
	townBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0700); err != nil {
		t.Fatal(err)
	}
	routesContent := `{"prefix":"gt-","path":"gastown"}
{"prefix":"bd-","path":"beads"}
`
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create gastown rig with correct idle-timeout
	gastownBeads := filepath.Join(townRoot, "gastown", ".beads")
	if err := os.MkdirAll(gastownBeads, 0700); err != nil {
		t.Fatal(err)
	}
	gastownConfig := `prefix: gt
issue-prefix: gt
dolt.idle-timeout: "0"
`
	if err := os.WriteFile(filepath.Join(gastownBeads, "config.yaml"), []byte(gastownConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create beads rig WITHOUT idle-timeout
	beadsBeads := filepath.Join(townRoot, "beads", ".beads")
	if err := os.MkdirAll(beadsBeads, 0700); err != nil {
		t.Fatal(err)
	}
	beadsConfig := `prefix: bd
issue-prefix: bd
`
	if err := os.WriteFile(filepath.Join(beadsBeads, "config.yaml"), []byte(beadsConfig), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{
		TownRoot: townRoot,
	}

	check := NewIdleTimeoutCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
	if len(result.Details) != 1 {
		t.Errorf("expected 1 rig missing idle-timeout, got %d: %v", len(result.Details), result.Details)
	}
	if result.Details[0] != "beads" {
		t.Errorf("expected 'beads' in details, got %q", result.Details[0])
	}
}

func TestIdleTimeoutCheck_Run_AllCorrect(t *testing.T) {
	townRoot := t.TempDir()

	// Create town beads with routes.jsonl
	townBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0700); err != nil {
		t.Fatal(err)
	}
	routesContent := `{"prefix":"gt-","path":"gastown"}
`
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create gastown rig with correct idle-timeout
	gastownBeads := filepath.Join(townRoot, "gastown", ".beads")
	if err := os.MkdirAll(gastownBeads, 0700); err != nil {
		t.Fatal(err)
	}
	gastownConfig := `prefix: gt
issue-prefix: gt
dolt.idle-timeout: "0"
`
	if err := os.WriteFile(filepath.Join(gastownBeads, "config.yaml"), []byte(gastownConfig), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{
		TownRoot: townRoot,
	}

	check := NewIdleTimeoutCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestIdleTimeoutCheck_Fix(t *testing.T) {
	townRoot := t.TempDir()

	// Create town beads with routes.jsonl
	townBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0700); err != nil {
		t.Fatal(err)
	}
	routesContent := `{"prefix":"gt-","path":"gastown"}
`
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create gastown rig WITHOUT idle-timeout
	gastownBeads := filepath.Join(townRoot, "gastown", ".beads")
	if err := os.MkdirAll(gastownBeads, 0700); err != nil {
		t.Fatal(err)
	}
	gastownConfig := `prefix: gt
issue-prefix: gt
`
	if err := os.WriteFile(filepath.Join(gastownBeads, "config.yaml"), []byte(gastownConfig), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &CheckContext{
		TownRoot: townRoot,
	}

	check := NewIdleTimeoutCheck()
	err := check.Fix(ctx)
	if err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify config was updated
	data, err := os.ReadFile(filepath.Join(gastownBeads, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `dolt.idle-timeout: "0"`) {
		t.Errorf("config.yaml should contain dolt.idle-timeout: \"0\", got:\n%s", content)
	}
}
