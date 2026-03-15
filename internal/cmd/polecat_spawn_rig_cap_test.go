package cmd

import (
	"os"
	"strings"
	"testing"
)

// TestPolecatSpawnChecksRigMaxPolecats verifies that SpawnPolecatForSling
// checks the per-rig max_polecats config before spawning.
func TestPolecatSpawnChecksRigMaxPolecats(t *testing.T) {
	srcPath := findRepoFile(t, "internal/cmd/polecat_spawn.go")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("reading polecat_spawn.go: %v", err)
	}

	content := string(data)

	// Should call GetIntConfig("max_polecats") for per-rig limit
	if !strings.Contains(content, `GetIntConfig("max_polecats")`) {
		t.Error("polecat_spawn.go should call GetIntConfig(\"max_polecats\") for per-rig capacity check")
	}

	// Should call countActivePolecatsForRig for per-rig counting
	if !strings.Contains(content, "countActivePolecatsForRig") {
		t.Error("polecat_spawn.go should call countActivePolecatsForRig for per-rig polecat counting")
	}

	// Per-rig check should appear BEFORE global check
	rigCheckIdx := strings.Index(content, "countActivePolecatsForRig")
	globalCheckIdx := strings.Index(content, "countActivePolecats()")
	if rigCheckIdx == -1 || globalCheckIdx == -1 {
		t.Fatal("could not find both rig and global polecat count checks")
	}
	if rigCheckIdx > globalCheckIdx {
		t.Error("per-rig max_polecats check should appear before global cap check")
	}
}
