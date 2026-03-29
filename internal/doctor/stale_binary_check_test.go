package doctor

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/version"
)

const gastownMainPackage = "github.com/steveyegge/gastown/cmd/gt"

func TestStaleBinaryCheck_Metadata(t *testing.T) {
	check := NewStaleBinaryCheck()

	if check.Name() != "stale-binary" {
		t.Fatalf("Name() = %q, want %q", check.Name(), "stale-binary")
	}
	if check.Description() != "List discovered gt binaries and detect shadowing or stale builds" {
		t.Fatalf("Description() = %q", check.Description())
	}
	if check.Category() != CategoryInfrastructure {
		t.Fatalf("Category() = %q, want %q", check.Category(), CategoryInfrastructure)
	}
	if check.CanFix() {
		t.Fatal("CanFix() should be false")
	}
}

func TestStaleBinaryCheck_WarnsOnShadowingAndStaleBuild(t *testing.T) {
	originalDiscover := discoverGTBinaries
	t.Cleanup(func() { discoverGTBinaries = originalDiscover })

	discoverGTBinaries = func() *version.GTBinaryInventory {
		return &version.GTBinaryInventory{
			ActiveIndex:      0,
			PathPrimaryIndex: 0,
			RepoRoot:         "/tmp/gastown",
			Binaries: []version.GTBinaryCandidate{
				{
					Path:         "/Users/test/.local/bin/gt",
					ResolvedPath: "/Users/test/.local/bin/gt",
					OnPATH:       true,
					PATHIndex:    0,
					Active:       true,
					PathPrimary:  true,
					VersionInfo: version.GTBinaryVersionInfo{
						MainPackage: gastownMainPackage,
						Version:     "0.12.0",
						Build:       "dev",
						Detail:      "main@abcdef123456",
					},
					StaleInfo: &version.StaleBinaryInfo{
						IsStale:       true,
						BinaryCommit:  "abcdef123456",
						RepoCommit:    "42f9d568fc1f",
						CommitsBehind: 4,
					},
				},
				{
					Path:         "/usr/local/bin/gt",
					ResolvedPath: "/usr/local/bin/gt",
					OnPATH:       true,
					PATHIndex:    1,
					VersionInfo: version.GTBinaryVersionInfo{
						MainPackage: gastownMainPackage,
						Version:     "0.12.1",
						Build:       "Homebrew",
						Detail:      "v0.12.1@Homebrew",
					},
				},
			},
		}
	}

	check := NewStaleBinaryCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})

	if result.Status != StatusWarning {
		t.Fatalf("Status = %v, want %v", result.Status, StatusWarning)
	}
	if !strings.Contains(result.Message, "shadows") && !strings.Contains(result.Message, "older") {
		t.Fatalf("unexpected warning message: %q", result.Message)
	}
	if result.FixHint == "" || !strings.Contains(result.FixHint, "command -v gt") {
		t.Fatalf("expected PATH-focused fix hint, got %q", result.FixHint)
	}
	if len(result.Details) < 2 {
		t.Fatalf("expected binary listing details, got %v", result.Details)
	}
	if !containsDetail(result.Details, "active PATH[0]: /Users/test/.local/bin/gt") {
		t.Fatalf("active binary not listed in details: %v", result.Details)
	}
	if !containsDetail(result.Details, "shadowed PATH[1]: /usr/local/bin/gt") {
		t.Fatalf("shadowed binary not listed in details: %v", result.Details)
	}
	if !containsDetail(result.Details, "stale vs repo 42f9d568fc1f") && !containsDetail(result.Details, "stale vs repo 42f9d568fc1") {
		t.Fatalf("stale repo detail missing: %v", result.Details)
	}
}

func TestStaleBinaryCheck_OKSingleBinary(t *testing.T) {
	originalDiscover := discoverGTBinaries
	t.Cleanup(func() { discoverGTBinaries = originalDiscover })

	discoverGTBinaries = func() *version.GTBinaryInventory {
		return &version.GTBinaryInventory{
			ActiveIndex:      0,
			PathPrimaryIndex: 0,
			Binaries: []version.GTBinaryCandidate{
				{
					Path:         "/usr/local/bin/gt",
					ResolvedPath: "/usr/local/bin/gt",
					OnPATH:       true,
					PATHIndex:    0,
					Active:       true,
					PathPrimary:  true,
					VersionInfo: version.GTBinaryVersionInfo{
						MainPackage: gastownMainPackage,
						Version:     "0.12.1",
						Build:       "Homebrew",
						Detail:      "v0.12.1@Homebrew",
					},
				},
			},
		}
	}

	check := NewStaleBinaryCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})

	if result.Status != StatusOK {
		t.Fatalf("Status = %v, want %v", result.Status, StatusOK)
	}
	if !strings.Contains(result.Message, "/usr/local/bin/gt") || !strings.Contains(result.Message, "0.12.1") {
		t.Fatalf("unexpected OK message: %q", result.Message)
	}
	if len(result.Details) != 1 {
		t.Fatalf("expected 1 detail line, got %d (%v)", len(result.Details), result.Details)
	}
}

func TestStaleBinaryCheck_ErrorsWithoutActiveBinary(t *testing.T) {
	originalDiscover := discoverGTBinaries
	t.Cleanup(func() { discoverGTBinaries = originalDiscover })

	discoverGTBinaries = func() *version.GTBinaryInventory {
		return &version.GTBinaryInventory{}
	}

	check := NewStaleBinaryCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})

	if result.Status != StatusError {
		t.Fatalf("Status = %v, want %v", result.Status, StatusError)
	}
}

func containsDetail(details []string, needle string) bool {
	for _, detail := range details {
		if strings.Contains(detail, needle) {
			return true
		}
	}
	return false
}
