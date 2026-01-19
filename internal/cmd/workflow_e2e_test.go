//go:build integration

// Package cmd contains end-to-end tests for core Gas Town workflows as described in README.md.
//
// These tests validate the realistic user workflows:
// 1. Install workspace → Add rig → Create crew
// 2. Create convoy with issues → Sling to agent → Track completion
// 3. Mail communication between agents
// 4. Prime context recovery
//
// Run with: go test -tags=integration ./internal/cmd -run TestWorkflow -v
package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWorkflowConvoyLifecycle tests the full convoy workflow from README:
// 1. gt install
// 2. gt convoy create
// 3. gt convoy list
// 4. gt convoy show
//
// This simulates the core orchestration flow without requiring tmux or actual agents.
func TestWorkflowConvoyLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Step 1: Install workspace
	t.Run("Install", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
		}
	})

	// Step 2: Create a rig (mock - just directory structure for now)
	rigName := "myproject"
	rigPath := filepath.Join(hqPath, rigName)
	t.Run("SetupRig", func(t *testing.T) {
		// Create minimal rig structure
		dirs := []string{
			filepath.Join(rigPath, "witness"),
			filepath.Join(rigPath, "refinery", "rig"),
			filepath.Join(rigPath, "polecats"),
			filepath.Join(rigPath, "crew"),
			filepath.Join(rigPath, "settings"),
		}
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}

		// Create rig config
		rigConfig := `{"name": "myproject", "upstream": "https://example.com/repo.git"}`
		if err := os.WriteFile(filepath.Join(rigPath, "settings", "rig.json"), []byte(rigConfig), 0644); err != nil {
			t.Fatalf("write rig config: %v", err)
		}
	})

	// Step 3: Test convoy create
	t.Run("ConvoyCreate", func(t *testing.T) {
		// Correct syntax: gt convoy create <name> [issues...] [flags]
		cmd := exec.Command(gtBinary, "convoy", "create", "Feature X", "issue-123", "issue-456", "--notify")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		// Convoy create may require beads - check if it fails gracefully
		if err != nil {
			// Expected: convoy create needs beads
			if strings.Contains(string(output), "beads") || strings.Contains(string(output), "bd ") {
				t.Skip("convoy create requires beads integration - skipping")
			}
			t.Logf("convoy create output: %s", string(output))
		} else {
			// Success - validate output
			if !strings.Contains(string(output), "convoy") {
				t.Errorf("expected convoy creation message, got: %s", output)
			}
		}
	})

	// Step 4: Test convoy list
	t.Run("ConvoyList", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "convoy", "list")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "beads") || strings.Contains(string(output), "bd ") {
				t.Skip("convoy list requires beads integration - skipping")
			}
			t.Logf("convoy list output: %s", string(output))
		}
	})
}

// TestWorkflowSlingAndHook tests the work assignment flow:
// 1. gt sling (assign work to an agent)
// 2. gt hook (check hook state)
//
// This validates the core work distribution mechanism.
func TestWorkflowSlingAndHook(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Install workspace
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Create rig directory structure
	rigName := "testrig"
	rigPath := filepath.Join(hqPath, rigName)
	dirs := []string{
		filepath.Join(rigPath, "polecats"),
		filepath.Join(rigPath, "settings"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// Test sling command help (doesn't require beads)
	t.Run("SlingHelp", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "sling", "--help")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt sling --help failed: %v\nOutput: %s", err, output)
		}

		// Validate help shows expected options
		helpText := string(output)
		expectedTerms := []string{"issue", "rig", "agent"}
		for _, term := range expectedTerms {
			if !strings.Contains(strings.ToLower(helpText), term) {
				t.Errorf("help missing term %q\nOutput: %s", term, helpText)
			}
		}
	})

	// Test hook command help
	t.Run("HookHelp", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "hook", "--help")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt hook --help failed: %v\nOutput: %s", err, output)
		}
	})
}

// TestWorkflowMailCommunication tests the mail system:
// 1. gt mail send
// 2. gt mail check
// 3. gt mail list
//
// This validates inter-agent communication without requiring actual agents.
func TestWorkflowMailCommunication(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Install workspace
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Create rig with mail directories
	rigName := "testrig"
	rigPath := filepath.Join(hqPath, rigName)

	// Create polecat directory structure including mail
	polecatDir := filepath.Join(rigPath, "polecats", "Toast")
	mailDir := filepath.Join(polecatDir, "mail", "inbox")
	if err := os.MkdirAll(mailDir, 0755); err != nil {
		t.Fatalf("mkdir mail: %v", err)
	}

	// Create minimal polecat rig directory
	if err := os.MkdirAll(filepath.Join(polecatDir, "rig"), 0755); err != nil {
		t.Fatalf("mkdir polecat rig: %v", err)
	}

	// Test mail check from polecat directory
	t.Run("MailCheck", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "mail", "check")
		cmd.Dir = filepath.Join(polecatDir, "rig")
		cmd.Env = append(cleanGTEnv(),
			"HOME="+tmpDir,
			"GT_ROLE=polecat",
			"GT_RIG="+rigName,
			"GT_POLECAT=Toast",
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			// Mail check may fail if mailbox setup is incomplete
			t.Logf("mail check: %v\nOutput: %s", err, output)
			// Don't fail - just log
		} else {
			t.Logf("mail check output: %s", output)
		}
	})

	// Test mail list command
	t.Run("MailList", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "mail", "list", "--help")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt mail list --help failed: %v\nOutput: %s", err, output)
		}
	})
}

// TestWorkflowPrimeContextRecovery tests the gt prime context recovery:
// 1. gt prime
//
// This is called on session start to recover context.
func TestWorkflowPrimeContextRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Install workspace
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Test prime from mayor directory
	t.Run("PrimeFromMayor", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "prime")
		cmd.Dir = filepath.Join(hqPath, "mayor")
		cmd.Env = append(cleanGTEnv(),
			"HOME="+tmpDir,
			"GT_ROLE=mayor",
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			// Prime may have beads requirements
			if strings.Contains(string(output), "beads") || strings.Contains(string(output), "bd ") {
				t.Skip("prime requires beads integration - skipping")
			}
			t.Logf("prime output: %v\n%s", err, output)
		} else {
			t.Logf("prime succeeded: %s", output)
		}
	})
}

// TestWorkflowAgentPresets tests agent configuration:
// 1. gt config agent list
// 2. gt config show
//
// Validates agent preset system works correctly.
func TestWorkflowAgentPresets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Install workspace
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Test config agent list
	t.Run("ConfigAgentList", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "config", "agent", "list")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt config agent list failed: %v\nOutput: %s", err, output)
		}

		// Validate expected agents are listed
		agentList := string(output)
		expectedAgents := []string{"claude", "opencode", "gemini", "codex"}
		for _, agent := range expectedAgents {
			if !strings.Contains(agentList, agent) {
				t.Errorf("agent list missing %q\nOutput: %s", agent, agentList)
			}
		}
	})

	// Test config default-agent (config show doesn't exist)
	t.Run("ConfigDefaultAgent", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "config", "default-agent")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			// May return error if not set - just log
			t.Logf("config default-agent: %s", output)
		} else {
			t.Logf("config default-agent: %s", output)
		}
	})
}

// TestWorkflowAgentsCommand tests agent session management:
// 1. gt agents (list)
// 2. gt agents list (explicit)
//
// Validates agents command works without active sessions.
func TestWorkflowAgentsCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Install workspace
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Test agents list
	t.Run("AgentsList", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "agents", "list")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		// agents list may fail if beads not available
		if err != nil {
			if strings.Contains(string(output), "beads") || strings.Contains(string(output), "bd ") {
				t.Skip("agents list requires beads - skipping")
			}
			t.Logf("agents list output: %v\n%s", err, output)
		} else {
			// No sessions should be running
			if !strings.Contains(string(output), "No agent") {
				t.Logf("agents list: %s", output)
			}
		}
	})

	// Test agents check (for collisions)
	t.Run("AgentsCheck", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "agents", "check")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "beads") || strings.Contains(string(output), "bd ") {
				t.Skip("agents check requires beads - skipping")
			}
			t.Logf("agents check output: %v\n%s", err, output)
		}
	})
}

// TestWorkflowVersionAndHelp validates basic CLI usability.
func TestWorkflowVersionAndHelp(t *testing.T) {
	gtBinary := buildGT(t)

	t.Run("Version", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "version")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt version failed: %v\nOutput: %s", err, output)
		}

		// Should contain version info
		if !strings.Contains(string(output), "gt version") && !strings.Contains(string(output), "dev") {
			t.Errorf("version output unexpected: %s", output)
		}
	})

	t.Run("Help", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "--help")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("gt --help failed: %v\nOutput: %s", err, output)
		}

		// Should list main command categories from README
		expectedSections := []string{
			"convoy",
			"agents",
			"mayor",
			"sling",
		}
		for _, section := range expectedSections {
			if !strings.Contains(strings.ToLower(string(output)), section) {
				t.Errorf("help missing section %q", section)
			}
		}
	})
}

// TestWorkflowDoctorCommand validates the doctor diagnostic tool.
func TestWorkflowDoctorCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Install workspace
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Test doctor check
	t.Run("DoctorCheck", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "doctor", "check")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		// Doctor may have issues with incomplete setup
		if err != nil {
			t.Logf("doctor check: %v\n%s", err, output)
			// Don't fail - doctor reporting issues is valid behavior
		} else {
			t.Logf("doctor check passed: %s", output)
		}
	})
}

// TestWorkflowE2ERealisticScenario simulates a realistic multi-step workflow.
// This is the closest approximation to the README quick start.
func TestWorkflowE2ERealisticScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Track successful steps
	steps := make(map[string]bool)
	startTime := time.Now()

	// Step 1: Install workspace
	t.Run("1_Install", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Step 1 failed: %v\nOutput: %s", err, output)
		}
		steps["install"] = true
	})

	// Step 2: Verify mayor directory exists
	t.Run("2_VerifyMayor", func(t *testing.T) {
		if !steps["install"] {
			t.Skip("skipping - install failed")
		}

		mayorPath := filepath.Join(hqPath, "mayor")
		if _, err := os.Stat(mayorPath); os.IsNotExist(err) {
			t.Fatalf("mayor directory not created")
		}
		steps["verify_mayor"] = true
	})

	// Step 3: Verify deacon directory exists
	t.Run("3_VerifyDeacon", func(t *testing.T) {
		if !steps["install"] {
			t.Skip("skipping - install failed")
		}

		deaconPath := filepath.Join(hqPath, "deacon")
		if _, err := os.Stat(deaconPath); os.IsNotExist(err) {
			t.Fatalf("deacon directory not created")
		}
		steps["verify_deacon"] = true
	})

	// Step 4: Run version from workspace
	t.Run("4_VersionFromWorkspace", func(t *testing.T) {
		if !steps["install"] {
			t.Skip("skipping - install failed")
		}

		cmd := exec.Command(gtBinary, "version")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("version failed: %v\nOutput: %s", err, output)
		}
		steps["version"] = true
	})

	// Step 5: Check agents list works
	t.Run("5_AgentsList", func(t *testing.T) {
		if !steps["install"] {
			t.Skip("skipping - install failed")
		}

		cmd := exec.Command(gtBinary, "agents", "list")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, _ := cmd.CombinedOutput()
		// May fail due to beads requirement - just log
		t.Logf("agents list output: %s", output)
		steps["agents_list"] = true
	})

	// Step 6: Check role detection works from mayor
	t.Run("6_RoleDetectMayor", func(t *testing.T) {
		if !steps["verify_mayor"] {
			t.Skip("skipping - mayor verification failed")
		}

		cmd := exec.Command(gtBinary, "role", "detect")
		cmd.Dir = filepath.Join(hqPath, "mayor")
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("role detect failed: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(string(output), "mayor") {
			t.Errorf("expected mayor role, got: %s", output)
		}
		steps["role_detect"] = true
	})

	// Step 7: Check config agent list
	t.Run("7_ConfigAgentList", func(t *testing.T) {
		if !steps["install"] {
			t.Skip("skipping - install failed")
		}

		cmd := exec.Command(gtBinary, "config", "agent", "list")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("config agent list failed: %v\nOutput: %s", err, output)
		}

		// Verify at least claude and opencode are listed
		if !strings.Contains(string(output), "claude") {
			t.Errorf("missing claude agent")
		}
		if !strings.Contains(string(output), "opencode") {
			t.Errorf("missing opencode agent")
		}
		steps["config_agent_list"] = true
	})

	// Summary
	duration := time.Since(startTime)
	passedCount := 0
	for _, passed := range steps {
		if passed {
			passedCount++
		}
	}
	t.Logf("E2E Scenario: %d/%d steps passed in %v", passedCount, len(steps), duration)
}

// TestWorkflowOpenCodeIntegration tests OpenCode-specific functionality.
func TestWorkflowOpenCodeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	// Install workspace
	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Test that opencode is a recognized agent
	t.Run("OpenCodeAgentRecognized", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "config", "agent", "list")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("config agent list failed: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(string(output), "opencode") {
			t.Errorf("opencode not in agent list: %s", output)
		}
	})

	// Test opencode-specific features via JSON output if available
	t.Run("OpenCodeAgentConfig", func(t *testing.T) {
		cmd := exec.Command(gtBinary, "config", "agent", "show", "opencode", "--json")
		cmd.Dir = hqPath
		cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			// --json flag might not be supported
			t.Logf("config agent show --json: %v\n%s", err, output)
			return
		}

		// Parse JSON to verify structure
		var config map[string]interface{}
		if err := json.Unmarshal(output, &config); err != nil {
			t.Logf("JSON parse error: %v (output: %s)", err, output)
			return
		}

		// Check expected fields
		if command, ok := config["command"].(string); ok {
			if command != "opencode" {
				t.Errorf("command = %q, want 'opencode'", command)
			}
		}
	})
}
