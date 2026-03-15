package cmd

import (
	"os"
	"strings"
	"testing"
)

// TestSlingConvoyCallsEnsureCustomTypes verifies that both createAutoConvoy and
// createBatchConvoy call beads.EnsureCustomTypes before bd create --type=convoy.
// Without this, bd rejects "convoy" as an invalid issue type because it's a
// custom type that must be registered first.
func TestSlingConvoyCallsEnsureCustomTypes(t *testing.T) {
	// Read the source file and verify EnsureCustomTypes is called before bd create
	srcPath := findRepoFile(t, "internal/cmd/sling_convoy.go")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("reading sling_convoy.go: %v", err)
	}

	content := string(data)

	// Count EnsureCustomTypes calls — should be at least 2 (one per function)
	count := strings.Count(content, "beads.EnsureCustomTypes")
	if count < 2 {
		t.Errorf("sling_convoy.go should call beads.EnsureCustomTypes at least 2 times (createAutoConvoy + createBatchConvoy), found %d", count)
	}

	// Verify EnsureCustomTypes appears BEFORE the bd create call in each function
	for _, funcName := range []string{"createAutoConvoy", "createBatchConvoy"} {
		funcIdx := strings.Index(content, "func "+funcName)
		if funcIdx == -1 {
			t.Errorf("function %s not found in sling_convoy.go", funcName)
			continue
		}

		funcContent := content[funcIdx:]
		ensureIdx := strings.Index(funcContent, "beads.EnsureCustomTypes")
		createIdx := strings.Index(funcContent, `"--type=convoy"`)

		if ensureIdx == -1 {
			t.Errorf("%s: beads.EnsureCustomTypes not found", funcName)
			continue
		}
		if createIdx == -1 {
			t.Errorf("%s: --type=convoy not found", funcName)
			continue
		}
		if ensureIdx > createIdx {
			t.Errorf("%s: beads.EnsureCustomTypes must be called BEFORE bd create --type=convoy", funcName)
		}
	}
}
