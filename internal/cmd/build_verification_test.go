package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// findProjectRoot walks up from the current directory to locate go.mod.
func findProjectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

// buildGTWithLdflags builds the gt binary with BuiltProperly=1 set via ldflags
// so that the binary's persistent-pre-run check passes. The binary is placed in
// the given directory and its path is returned.
func buildGTWithLdflags(t *testing.T, outputDir string) string {
	t.Helper()
	projectRoot := findProjectRoot(t)

	binaryName := "gt-bvt"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(outputDir, binaryName)

	ldflags := "-X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1"
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", binaryPath, "./cmd/gt")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build gt binary: %v\nOutput: %s", err, output)
	}
	return binaryPath
}

// TestGoBuildSucceeds verifies that `go build ./cmd/gt/` compiles without errors.
// This is the most basic build verification: does the code compile?
func TestGoBuildSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	projectRoot := findProjectRoot(t)

	cmd := exec.Command("go", "build", "./cmd/gt/")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./cmd/gt/ failed: %v\nOutput: %s", err, output)
	}
}

// TestBinaryIsProduced builds the gt binary to a temporary directory and verifies
// the resulting file exists, has non-zero size, and has executable permissions.
func TestBinaryIsProduced(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	tmpDir := t.TempDir()
	projectRoot := findProjectRoot(t)

	binaryName := "gt"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/gt")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build -o %s failed: %v\nOutput: %s", binaryPath, err, output)
	}

	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("binary not found at %s: %v", binaryPath, err)
	}

	if info.Size() == 0 {
		t.Errorf("binary has zero size")
	}

	// On Unix-like systems, verify the binary is executable.
	if runtime.GOOS != "windows" {
		mode := info.Mode()
		if mode&0111 == 0 {
			t.Errorf("binary is not executable: mode=%v", mode)
		}
	}
}

// TestBinaryVersionFlag builds the gt binary with proper ldflags and verifies
// that running it with --version exits cleanly and produces version output.
func TestBinaryVersionFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	tmpDir := t.TempDir()
	binaryPath := buildGTWithLdflags(t, tmpDir)

	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt --version failed: %v\nOutput: %s", err, output)
	}

	outStr := string(output)
	if !strings.Contains(outStr, "gt version") {
		t.Errorf("expected output to contain 'gt version', got: %s", outStr)
	}
}

// TestBinaryHelpFlag builds the gt binary and verifies that --help exits
// cleanly and prints usage information including the binary name and
// available commands.
func TestBinaryHelpFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	tmpDir := t.TempDir()
	binaryPath := buildGTWithLdflags(t, tmpDir)

	cmd := exec.Command(binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt --help failed: %v\nOutput: %s", err, output)
	}

	outStr := string(output)

	// Verify key usage markers are present.
	// The help output uses grouped command categories (e.g. "Work Management:",
	// "Agent Management:") rather than a flat "Available Commands" section.
	checks := []struct {
		substr string
		label  string
	}{
		{"gt", "binary name"},
		{"Usage", "usage section header"},
	}
	for _, c := range checks {
		if !strings.Contains(outStr, c.substr) {
			t.Errorf("expected --help output to contain %q (%s), got:\n%s", c.substr, c.label, outStr)
		}
	}

	// Verify at least one command group header is present (the help output
	// organises commands into groups rather than listing "Available Commands").
	hasCommandGroup := strings.Contains(outStr, "Available Commands") ||
		strings.Contains(outStr, "Work Management:") ||
		strings.Contains(outStr, "Agent Management:")
	if !hasCommandGroup {
		t.Errorf("expected --help output to contain a command group header, got:\n%s", outStr)
	}
}

// TestGoVetPasses runs `go vet ./...` across the entire project and verifies
// the static analysis reports no issues.
func TestGoVetPasses(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping vet test in short mode")
	}

	projectRoot := findProjectRoot(t)

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet ./... failed: %v\nOutput: %s", err, output)
	}
}

// TestGoTestRace verifies that the project compiles cleanly with the race
// detector enabled. This does not run the full test suite; it only checks
// that the build succeeds with -race, which confirms CGO compatibility and
// the absence of build-time issues with the race instrumentation.
func TestGoTestRace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race build test in short mode")
	}

	projectRoot := findProjectRoot(t)

	tmpDir := t.TempDir()
	binaryName := "gt-race"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)

	cmd := exec.Command("go", "build", "-race", "-o", binaryPath, "./cmd/gt")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build -race failed: %v\nOutput: %s", err, output)
	}

	// Verify the race-enabled binary was actually produced.
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("race binary not found at %s: %v", binaryPath, err)
	}
	if info.Size() == 0 {
		t.Errorf("race binary has zero size")
	}
}
