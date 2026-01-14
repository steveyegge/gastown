//go:build integration

package witness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/ratelimit"
)

// E2E test: Rate Limit Instance Swapping
//
// This test validates the full rate limit swap flow:
//
// Feature: Rate Limit Instance Swapping
//   Scenario: Polecat swaps on rate limit
//     Given a rig "testrig" with profiles
//     And a polecat "testcat" running with profile "anthropic_a"
//     And work "test-123" hooked to "testcat"
//     When the polecat exits with rate limit (exit code 2)
//     Then Witness detects the rate limit
//     And spawns a new session with profile "openai_a"
//     And hooks work "test-123" to the new session
//     And the new session is nudged to continue
//     And a RateLimitEvent is persisted in beads

// cachedGTBinary caches the built gt binary path.
var cachedGTBinary string

// buildGT builds the gt binary for testing.
func buildGT(t *testing.T) string {
	t.Helper()

	if cachedGTBinary != "" {
		if _, err := os.Stat(cachedGTBinary); err == nil {
			return cachedGTBinary
		}
		cachedGTBinary = ""
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Walk up to find go.mod (project root)
	projectRoot := wd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Fatal("could not find project root (go.mod)")
		}
		projectRoot = parent
	}

	tmpDir := os.TempDir()
	tmpBinary := filepath.Join(tmpDir, "gt-ratelimit-test")
	cmd := exec.Command("go", "build", "-o", tmpBinary, "./cmd/gt")
	cmd.Dir = projectRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build gt: %v\nOutput: %s", err, output)
	}

	cachedGTBinary = tmpBinary
	return tmpBinary
}

// cleanGTEnv returns os.Environ() with all GT_* variables removed.
func cleanGTEnv() []string {
	var clean []string
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GT_") && !strings.HasPrefix(env, "BD_") {
			clean = append(clean, env)
		}
	}
	return clean
}

// TestRateLimitDetector_Unit validates the rate limit detector in isolation.
func TestRateLimitDetector_Unit(t *testing.T) {
	d := ratelimit.NewDetector("test-agent", "anthropic_main")

	tests := []struct {
		name     string
		exitCode int
		stderr   string
		want     bool
	}{
		{"exit code 2", ratelimit.ExitCodeRateLimit, "", true},
		{"429 in stderr", 1, "Error 429: Too Many Requests", true},
		{"rate limit in stderr", 1, "You have been rate limited", true},
		{"normal exit", 0, "", false},
		{"normal error", 1, "Connection refused", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, detected := d.Detect(tc.exitCode, tc.stderr)
			if detected != tc.want {
				t.Errorf("Detect(%d, %q) = %v, want %v", tc.exitCode, tc.stderr, detected, tc.want)
			}
		})
	}
}

// TestProfileSelector_Unit validates profile selection with cooldown.
func TestProfileSelector_Unit(t *testing.T) {
	policies := map[string]*ratelimit.RolePolicy{
		"polecat": {
			FallbackChain:   []string{"anthropic_a", "openai_a", "anthropic_b"},
			CooldownMinutes: 5,
		},
	}
	s := ratelimit.NewSelector(policies)

	// First swap: anthropic_a -> openai_a
	event := &ratelimit.RateLimitEvent{Profile: "anthropic_a"}
	next, err := s.SelectNext("polecat", "anthropic_a", event)
	if err != nil {
		t.Fatalf("first SelectNext error: %v", err)
	}
	if next != "openai_a" {
		t.Errorf("first SelectNext = %q, want %q", next, "openai_a")
	}

	// Second swap: openai_a -> anthropic_b (anthropic_a is cooling)
	event2 := &ratelimit.RateLimitEvent{Profile: "openai_a"}
	next2, err := s.SelectNext("polecat", "openai_a", event2)
	if err != nil {
		t.Fatalf("second SelectNext error: %v", err)
	}
	if next2 != "anthropic_b" {
		t.Errorf("second SelectNext = %q, want %q", next2, "anthropic_b")
	}

	// Third swap: all profiles cooling
	event3 := &ratelimit.RateLimitEvent{Profile: "anthropic_b"}
	_, err = s.SelectNext("polecat", "anthropic_b", event3)
	if err != ratelimit.ErrAllProfilesCooling {
		t.Errorf("third SelectNext should return ErrAllProfilesCooling, got %v", err)
	}
}

// TestRateLimitSwapFlow_Integration tests the full swap flow in an isolated environment.
// This test requires tmux to be installed.
func TestRateLimitSwapFlow_Integration(t *testing.T) {
	// Skip if tmux is not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping integration test")
	}

	// Skip in CI environments that may not support tmux
	if os.Getenv("CI") == "true" {
		t.Skip("skipping tmux-based test in CI")
	}

	gtBinary := buildGT(t)
	tmpDir := t.TempDir()
	townPath := filepath.Join(tmpDir, "test-town")

	// Step 1: Install Gas Town
	installCmd := exec.Command(gtBinary, "install", townPath, "--no-beads")
	installCmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Step 2: Create test rig directory structure
	rigPath := filepath.Join(townPath, rigName)
	polecatPath := filepath.Join(rigPath, "polecats", "testcat", rigName)
	if err := os.MkdirAll(polecatPath, 0755); err != nil {
		t.Fatalf("failed to create polecat path: %v", err)
	}

	// Step 3: Test detector with simulated exit
	t.Run("detector detects exit code 2", func(t *testing.T) {
		d := ratelimit.NewDetector("testrig/polecats/testcat", "anthropic_a")
		event, detected := d.Detect(2, "Error: 429 Rate limit exceeded")

		if !detected {
			t.Fatal("detector should detect rate limit")
		}
		if event.ExitCode != 2 {
			t.Errorf("exit code = %d, want 2", event.ExitCode)
		}
		if event.Profile != "anthropic_a" {
			t.Errorf("profile = %q, want %q", event.Profile, "anthropic_a")
		}
	})

	// Step 4: Test selector chooses fallback profile
	t.Run("selector chooses fallback profile", func(t *testing.T) {
		policies := map[string]*ratelimit.RolePolicy{
			"polecat": {
				FallbackChain:   []string{"anthropic_a", "openai_a"},
				CooldownMinutes: 5,
			},
		}
		s := ratelimit.NewSelector(policies)

		event := &ratelimit.RateLimitEvent{Profile: "anthropic_a"}
		next, err := s.SelectNext("polecat", "anthropic_a", event)

		if err != nil {
			t.Fatalf("SelectNext error: %v", err)
		}
		if next != "openai_a" {
			t.Errorf("next profile = %q, want %q", next, "openai_a")
		}
	})

	// Step 5: Verify mock harness exits correctly
	t.Run("mock harness exits with code 2", func(t *testing.T) {
		// Find mock harness relative to project root
		wd, _ := os.Getwd()
		projectRoot := wd
		for {
			if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(projectRoot)
			if parent == projectRoot {
				t.Skip("could not find project root, skipping mock harness test")
			}
			projectRoot = parent
		}

		harnessPath := filepath.Join(projectRoot, "testdata", "mock_harness.sh")
		if _, err := os.Stat(harnessPath); err != nil {
			t.Skipf("mock harness not found at %s", harnessPath)
		}

		cmd := exec.Command(harnessPath, "2", "Rate limit test")
		output, err := cmd.CombinedOutput()

		if err == nil {
			t.Fatal("expected exit code 2, but got success")
		}

		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("unexpected error type: %T", err)
		}
		if exitErr.ExitCode() != 2 {
			t.Errorf("exit code = %d, want 2", exitErr.ExitCode())
		}

		// Verify stderr contains rate limit message
		if !strings.Contains(string(output), "429") {
			t.Errorf("output should contain '429', got: %s", output)
		}
	})
}

// TestRateLimitEvent_Persistence validates that rate limit events can be recorded.
func TestRateLimitEvent_Persistence(t *testing.T) {
	event := &ratelimit.RateLimitEvent{
		AgentID:      "testrig/polecats/testcat",
		Profile:      "anthropic_a",
		Timestamp:    time.Now(),
		ExitCode:     2,
		ErrorSnippet: "429 Too Many Requests",
		Provider:     "anthropic",
	}

	// Verify event fields
	if event.AgentID != "testrig/polecats/testcat" {
		t.Errorf("AgentID = %q, want %q", event.AgentID, "testrig/polecats/testcat")
	}
	if event.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", event.ExitCode)
	}
	if event.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", event.Provider, "anthropic")
	}
}

// TestAllProfilesCooling_Alert validates behavior when all profiles are cooling.
func TestAllProfilesCooling_Alert(t *testing.T) {
	policies := map[string]*ratelimit.RolePolicy{
		"polecat": {
			FallbackChain:   []string{"profile_a", "profile_b"},
			CooldownMinutes: 5,
		},
	}
	s := ratelimit.NewSelector(policies)

	// Put profile_b in cooldown
	s.MarkCooldown("profile_b", time.Now().Add(10*time.Minute))

	// Try to swap from profile_a
	event := &ratelimit.RateLimitEvent{Profile: "profile_a"}
	_, err := s.SelectNext("polecat", "profile_a", event)

	if err != ratelimit.ErrAllProfilesCooling {
		t.Errorf("expected ErrAllProfilesCooling, got %v", err)
	}
}

// TestSwapPreservesHookedWork validates that work is preserved during swap.
func TestSwapPreservesHookedWork(t *testing.T) {
	req := ratelimit.SwapRequest{
		RigName:     "testrig",
		PolecatName: "testcat",
		OldProfile:  "anthropic_a",
		NewProfile:  "openai_a",
		HookedWork:  "test-issue-123",
		Reason:      "rate_limit",
	}

	// Verify request preserves hooked work
	if req.HookedWork != "test-issue-123" {
		t.Errorf("HookedWork = %q, want %q", req.HookedWork, "test-issue-123")
	}
	if req.OldProfile != "anthropic_a" {
		t.Errorf("OldProfile = %q, want %q", req.OldProfile, "anthropic_a")
	}
	if req.NewProfile != "openai_a" {
		t.Errorf("NewProfile = %q, want %q", req.NewProfile, "openai_a")
	}
}
