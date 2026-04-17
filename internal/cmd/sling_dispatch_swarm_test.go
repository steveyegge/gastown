package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTrySwarmDelegate_NostownNotInstalled(t *testing.T) {
	// Force a PATH that has no nostown binary.
	t.Setenv("PATH", t.TempDir())

	params := SlingParams{
		BeadID:      "bd-1",
		TownRoot:    t.TempDir(),
		RigName:     "rig1",
		SwarmConfig: &SwarmConfig{N: 3, Strategy: "majority"},
	}

	result, err := trySwarmDelegate(params)
	if err != nil {
		t.Fatalf("err = %v, want nil (graceful skip when nostown missing)", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil (caller falls through to single-agent)", result)
	}
}

func TestTrySwarmDelegate_NostownReturnsResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary not portable to Windows")
	}

	binDir := t.TempDir()
	fakePath := filepath.Join(binDir, "nostown")
	script := `#!/bin/sh
cat <<'EOF'
{"BeadID":"bd-1","PolecatName":"pc-1","Success":true,"AttachedMolecule":"mol-x"}
EOF
`
	if err := os.WriteFile(fakePath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake nostown: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	params := SlingParams{
		BeadID:      "bd-1",
		TownRoot:    t.TempDir(),
		RigName:     "rig1",
		SwarmConfig: &SwarmConfig{N: 3, Strategy: "majority"},
	}

	result, err := trySwarmDelegate(params)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("result = nil, want parsed SlingResult")
	}
	if result.BeadID != "bd-1" {
		t.Errorf("BeadID = %q, want bd-1", result.BeadID)
	}
	if !result.Success {
		t.Errorf("Success = false, want true")
	}
	if result.PolecatName != "pc-1" {
		t.Errorf("PolecatName = %q, want pc-1", result.PolecatName)
	}
	if result.AttachedMolecule != "mol-x" {
		t.Errorf("AttachedMolecule = %q, want mol-x", result.AttachedMolecule)
	}
}

func TestTrySwarmDelegate_NostownReturnsInvalidJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary not portable to Windows")
	}

	binDir := t.TempDir()
	fakePath := filepath.Join(binDir, "nostown")
	script := `#!/bin/sh
echo not json
`
	if err := os.WriteFile(fakePath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake nostown: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	params := SlingParams{
		BeadID:      "bd-1",
		SwarmConfig: &SwarmConfig{N: 3, Strategy: "majority"},
	}

	if _, err := trySwarmDelegate(params); err == nil {
		t.Fatal("err = nil, want JSON parse error")
	}
}

func TestTrySwarmDelegate_NostownExitsNonZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake binary not portable to Windows")
	}

	binDir := t.TempDir()
	fakePath := filepath.Join(binDir, "nostown")
	script := `#!/bin/sh
echo "boom" >&2
exit 7
`
	if err := os.WriteFile(fakePath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake nostown: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	params := SlingParams{
		BeadID:      "bd-1",
		SwarmConfig: &SwarmConfig{N: 3, Strategy: "majority"},
	}

	if _, err := trySwarmDelegate(params); err == nil {
		t.Fatal("err = nil, want non-zero exit error")
	}
}
