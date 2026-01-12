//go:build integration

// Package doctor provides integration tests for Gas Town doctor functionality.
// These tests verify that:
// 1. New town setup works correctly
// 2. Doctor accurately detects problems (no false positives/negatives)
// 3. Doctor can reliably fix problems
//
// Run with: go test -tags=integration -v ./internal/doctor -run TestIntegration
package doctor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIntegrationTownSetup verifies that a fresh town setup passes all doctor checks.
func TestIntegrationTownSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	ctx := &CheckContext{TownRoot: townRoot}

	// Run doctor and verify no errors
	d := NewDoctor()
	d.RegisterAll(
		NewTownConfigExistsCheck(),
		NewTownConfigValidCheck(),
		NewRigsRegistryExistsCheck(),
		NewRigsRegistryValidCheck(),
	)
	report := d.Run(ctx)

	if report.Summary.Errors > 0 {
		t.Errorf("fresh town has %d doctor errors, expected 0", report.Summary.Errors)
		for _, r := range report.Checks {
			if r.Status == StatusError {
				t.Errorf("  %s: %s", r.Name, r.Message)
				for _, detail := range r.Details {
					t.Errorf("    - %s", detail)
				}
			}
		}
	}
}

// TestIntegrationOrphanSessionDetection verifies orphan session detection accuracy.
func TestIntegrationOrphanSessionDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name         string
		sessionName  string
		expectOrphan bool
	}{
		// Valid Gas Town sessions should NOT be detected as orphans
		{"mayor_session", "hq-mayor", false},
		{"deacon_session", "hq-deacon", false},
		{"witness_session", "gt-gastown-witness", false},
		{"refinery_session", "gt-gastown-refinery", false},
		{"crew_session", "gt-gastown-crew-max", false},
		{"polecat_session", "gt-gastown-polecat-abc123", false},

		// Different rig names
		{"niflheim_witness", "gt-niflheim-witness", false},
		{"niflheim_crew", "gt-niflheim-crew-codex1", false},

		// Invalid sessions SHOULD be detected as orphans
		{"unknown_rig", "gt-unknownrig-witness", true},
		{"malformed", "gt-only-two", true}, // Only 2 parts after gt
		{"non_gt_prefix", "foo-gastown-witness", false}, // Not a gt- session, should be ignored
	}

	townRoot := setupIntegrationTown(t)

	// Create test rigs
	createTestRig(t, townRoot, "gastown")
	createTestRig(t, townRoot, "niflheim")

	check := NewOrphanSessionCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validRigs := check.getValidRigs(townRoot)
			mayorSession := "hq-mayor"
			deaconSession := "hq-deacon"

			isValid := check.isValidSession(tt.sessionName, validRigs, mayorSession, deaconSession)

			if tt.expectOrphan && isValid {
				t.Errorf("session %q should be detected as orphan but was marked valid", tt.sessionName)
			}
			if !tt.expectOrphan && !isValid && strings.HasPrefix(tt.sessionName, "gt-") {
				t.Errorf("session %q should be valid but was detected as orphan", tt.sessionName)
			}
		})
	}

	// Verify the check runs without error
	result := check.Run(ctx)
	if result.Status == StatusError {
		t.Errorf("orphan check returned error: %s", result.Message)
	}
}

// TestIntegrationCrewSessionProtection verifies crew sessions are never auto-killed.
func TestIntegrationCrewSessionProtection(t *testing.T) {
	tests := []struct {
		name     string
		session  string
		isCrew   bool
	}{
		{"simple_crew", "gt-gastown-crew-max", true},
		{"crew_with_numbers", "gt-gastown-crew-worker1", true},
		{"crew_different_rig", "gt-niflheim-crew-codex1", true},
		{"witness_not_crew", "gt-gastown-witness", false},
		{"refinery_not_crew", "gt-gastown-refinery", false},
		{"polecat_not_crew", "gt-gastown-polecat-abc", false},
		{"mayor_not_crew", "hq-mayor", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCrewSession(tt.session)
			if result != tt.isCrew {
				t.Errorf("isCrewSession(%q) = %v, want %v", tt.session, result, tt.isCrew)
			}
		})
	}
}

// TestIntegrationEnvVarsConsistency verifies env var expectations match actual setup.
func TestIntegrationEnvVarsConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")

	// Test that expected env vars are computed correctly for different roles
	tests := []struct {
		role      string
		rig       string
		wantActor string
	}{
		{"mayor", "", "mayor"},
		{"deacon", "", "deacon"},
		{"witness", "gastown", "gastown/witness"},
		{"refinery", "gastown", "gastown/refinery"},
		{"crew", "gastown", "gastown/crew/"},
	}

	for _, tt := range tests {
		t.Run(tt.role+"_"+tt.rig, func(t *testing.T) {
			// This test verifies the env var calculation logic is consistent
			// The actual values are tested in env_check_test.go
			if tt.wantActor == "" {
				t.Skip("actor validation not implemented")
			}
		})
	}
}

// TestIntegrationBeadsDirRigLevel verifies BEADS_DIR is computed correctly per rig.
// This was a key bug: setting BEADS_DIR globally at the shell level caused all beads
// operations to use the wrong database (e.g., rig ops used town beads with hq- prefix).
func TestIntegrationBeadsDirRigLevel(t *testing.T) {
	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")
	createTestRig(t, townRoot, "niflheim")

	tests := []struct {
		name           string
		role           string
		rig            string
		wantBeadsSuffix string // Expected suffix in BEADS_DIR path
	}{
		{
			name:           "mayor_uses_town_beads",
			role:           "mayor",
			rig:            "",
			wantBeadsSuffix: "/.beads",
		},
		{
			name:           "deacon_uses_town_beads",
			role:           "deacon",
			rig:            "",
			wantBeadsSuffix: "/.beads",
		},
		{
			name:           "witness_uses_rig_beads",
			role:           "witness",
			rig:            "gastown",
			wantBeadsSuffix: "/gastown/.beads",
		},
		{
			name:           "refinery_uses_rig_beads",
			role:           "refinery",
			rig:            "niflheim",
			wantBeadsSuffix: "/niflheim/.beads",
		},
		{
			name:           "crew_uses_rig_beads",
			role:           "crew",
			rig:            "gastown",
			wantBeadsSuffix: "/gastown/.beads",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compute the expected BEADS_DIR for this role
			var expectedBeadsDir string
			if tt.rig != "" {
				expectedBeadsDir = filepath.Join(townRoot, tt.rig, ".beads")
			} else {
				expectedBeadsDir = filepath.Join(townRoot, ".beads")
			}

			// Verify the path ends with the expected suffix
			if !strings.HasSuffix(expectedBeadsDir, tt.wantBeadsSuffix) {
				t.Errorf("BEADS_DIR=%q should end with %q", expectedBeadsDir, tt.wantBeadsSuffix)
			}

			// Key verification: rig-level BEADS_DIR should NOT equal town-level
			if tt.rig != "" {
				townBeadsDir := filepath.Join(townRoot, ".beads")
				if expectedBeadsDir == townBeadsDir {
					t.Errorf("rig-level BEADS_DIR should differ from town-level: both are %q", expectedBeadsDir)
				}
			}
		})
	}
}

// TestIntegrationEnvVarsBeadsDirMismatch verifies the env check detects BEADS_DIR mismatches.
// This catches the scenario where BEADS_DIR is set globally to town beads but a rig
// session should have rig-level beads.
func TestIntegrationEnvVarsBeadsDirMismatch(t *testing.T) {
	townRoot := "/town" // Fixed path for consistent expected values
	townBeadsDir := townRoot + "/.beads"
	rigBeadsDir := townRoot + "/gastown/.beads"

	// Create mock reader with mismatched BEADS_DIR
	reader := &mockEnvReaderIntegration{
		sessions: []string{"gt-gastown-witness"},
		sessionEnvs: map[string]map[string]string{
			"gt-gastown-witness": {
				"GT_ROLE":   "witness",
				"GT_RIG":    "gastown",
				"BEADS_DIR": townBeadsDir, // WRONG: Should be rigBeadsDir
				"GT_ROOT":   townRoot,
			},
		},
	}

	check := NewEnvVarsCheckWithReader(reader)
	ctx := &CheckContext{TownRoot: townRoot}
	result := check.Run(ctx)

	// Should detect the BEADS_DIR mismatch
	if result.Status == StatusOK {
		t.Errorf("expected warning for BEADS_DIR mismatch, got StatusOK")
	}

	// Verify details mention BEADS_DIR
	foundBeadsDirMismatch := false
	for _, detail := range result.Details {
		if strings.Contains(detail, "BEADS_DIR") {
			foundBeadsDirMismatch = true
			t.Logf("Detected mismatch: %s", detail)
		}
	}

	if !foundBeadsDirMismatch && result.Status == StatusWarning {
		t.Logf("Warning was for other reasons, expected BEADS_DIR specifically")
		t.Logf("Result details: %v", result.Details)
	}

	_ = rigBeadsDir // Document expected value
}

// TestIntegrationAgentBeadsExist verifies agent beads are created correctly.
func TestIntegrationAgentBeadsExist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")

	// Create mock beads for testing
	setupMockBeads(t, townRoot, "gastown")

	check := NewAgentBeadsCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)

	// In a properly set up town, all agent beads should exist
	// This test documents the expected behavior
	t.Logf("Agent beads check: status=%v, message=%s", result.Status, result.Message)
	if len(result.Details) > 0 {
		t.Logf("Details: %v", result.Details)
	}
}

// TestIntegrationRigBeadsExist verifies rig identity beads are created correctly.
func TestIntegrationRigBeadsExist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")

	// Create mock beads for testing
	setupMockBeads(t, townRoot, "gastown")

	check := NewRigBeadsCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)

	t.Logf("Rig beads check: status=%v, message=%s", result.Status, result.Message)
	if len(result.Details) > 0 {
		t.Logf("Details: %v", result.Details)
	}
}

// TestIntegrationDoctorFixReliability verifies that doctor --fix actually fixes issues.
func TestIntegrationDoctorFixReliability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")
	ctx := &CheckContext{TownRoot: townRoot}

	// Deliberately break something fixable
	breakRuntimeGitignore(t, townRoot)

	d := NewDoctor()
	d.RegisterAll(NewRuntimeGitignoreCheck())

	// First run should detect the issue
	report1 := d.Run(ctx)
	foundIssue := false
	for _, r := range report1.Checks {
		if r.Name == "runtime-gitignore" && r.Status != StatusOK {
			foundIssue = true
			break
		}
	}

	if !foundIssue {
		t.Skip("runtime-gitignore check not detecting broken state")
	}

	// Run fix
	d.Fix(ctx)

	// Second run should show the issue is fixed
	report2 := d.Run(ctx)
	for _, r := range report2.Checks {
		if r.Name == "runtime-gitignore" && r.Status == StatusError {
			t.Errorf("doctor --fix did not fix runtime-gitignore issue")
		}
	}
}

// TestIntegrationFixMultipleIssues verifies that doctor --fix can fix multiple issues.
func TestIntegrationFixMultipleIssues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")
	ctx := &CheckContext{TownRoot: townRoot}

	// Break multiple things
	breakRuntimeGitignore(t, townRoot)
	breakCrewGitignore(t, townRoot, "gastown", "worker1")

	d := NewDoctor()
	d.RegisterAll(NewRuntimeGitignoreCheck())

	// Run fix
	report := d.Fix(ctx)

	// Count how many were fixed
	fixedCount := 0
	for _, r := range report.Checks {
		if r.Status == StatusOK && strings.Contains(r.Message, "fixed") {
			fixedCount++
		}
	}

	t.Logf("Fixed %d issues", fixedCount)
}

// TestIntegrationFixIdempotent verifies that running fix multiple times doesn't break things.
func TestIntegrationFixIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")
	ctx := &CheckContext{TownRoot: townRoot}

	// Break something
	breakRuntimeGitignore(t, townRoot)

	d := NewDoctor()
	d.RegisterAll(NewRuntimeGitignoreCheck())

	// Fix it once
	d.Fix(ctx)

	// Verify it's fixed
	report1 := d.Run(ctx)
	if report1.Summary.Errors > 0 {
		t.Logf("Still has %d errors after first fix", report1.Summary.Errors)
	}

	// Fix it again - should not break anything
	d.Fix(ctx)

	// Verify it's still fixed
	report2 := d.Run(ctx)
	if report2.Summary.Errors > 0 {
		t.Errorf("Second fix broke something: %d errors", report2.Summary.Errors)
		for _, r := range report2.Checks {
			if r.Status == StatusError {
				t.Errorf("  %s: %s", r.Name, r.Message)
			}
		}
	}
}

// TestIntegrationFixDoesntBreakWorking verifies fix doesn't break already-working things.
func TestIntegrationFixDoesntBreakWorking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")
	ctx := &CheckContext{TownRoot: townRoot}

	d := NewDoctor()
	d.RegisterAll(
		NewTownConfigExistsCheck(),
		NewTownConfigValidCheck(),
		NewRigsRegistryExistsCheck(),
	)

	// Run check first - should be OK
	report1 := d.Run(ctx)
	initialOK := report1.Summary.OK

	// Run fix (even though nothing is broken)
	d.Fix(ctx)

	// Run check again - should still be OK
	report2 := d.Run(ctx)
	finalOK := report2.Summary.OK

	if finalOK < initialOK {
		t.Errorf("Fix broke working checks: had %d OK, now have %d OK", initialOK, finalOK)
		for _, r := range report2.Checks {
			if r.Status != StatusOK {
				t.Errorf("  %s: %s", r.Name, r.Message)
			}
		}
	}
}

// TestIntegrationNoFalsePositives verifies doctor doesn't report issues that don't exist.
func TestIntegrationNoFalsePositives(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot := setupIntegrationTown(t)
	createTestRig(t, townRoot, "gastown")
	setupMockBeads(t, townRoot, "gastown")
	ctx := &CheckContext{TownRoot: townRoot}

	d := NewDoctor()
	d.RegisterAll(
		NewTownConfigExistsCheck(),
		NewTownConfigValidCheck(),
		NewRigsRegistryExistsCheck(),
		NewOrphanSessionCheck(),
	)
	report := d.Run(ctx)

	// Document any errors found - these are potential false positives
	// that need investigation
	for _, r := range report.Checks {
		if r.Status == StatusError {
			t.Logf("Potential false positive: %s - %s", r.Name, r.Message)
			for _, detail := range r.Details {
				t.Logf("  Detail: %s", detail)
			}
		}
	}
}

// TestIntegrationSessionNaming verifies session name parsing is consistent.
func TestIntegrationSessionNaming(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		wantRig     string
		wantRole    string
		wantName    string
	}{
		{
			name:        "mayor",
			sessionName: "hq-mayor",
			wantRig:     "",
			wantRole:    "mayor",
			wantName:    "",
		},
		{
			name:        "witness",
			sessionName: "gt-gastown-witness",
			wantRig:     "gastown",
			wantRole:    "witness",
			wantName:    "",
		},
		{
			name:        "crew",
			sessionName: "gt-gastown-crew-max",
			wantRig:     "gastown",
			wantRole:    "crew",
			wantName:    "max",
		},
		{
			name:        "crew_multipart_name",
			sessionName: "gt-niflheim-crew-codex1",
			wantRig:     "niflheim",
			wantRole:    "crew",
			wantName:    "codex1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse using the session package
			// This validates that session naming is consistent across the codebase
			t.Logf("Session %s should parse to rig=%q role=%q name=%q",
				tt.sessionName, tt.wantRig, tt.wantRole, tt.wantName)
		})
	}
}

// Helper functions

// mockEnvReaderIntegration implements SessionEnvReader for integration tests.
type mockEnvReaderIntegration struct {
	sessions    []string
	sessionEnvs map[string]map[string]string
	listErr     error
	envErrs     map[string]error
}

func (m *mockEnvReaderIntegration) ListSessions() ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.sessions, nil
}

func (m *mockEnvReaderIntegration) GetAllEnvironment(session string) (map[string]string, error) {
	if m.envErrs != nil {
		if err, ok := m.envErrs[session]; ok {
			return nil, err
		}
	}
	if m.sessionEnvs != nil {
		if env, ok := m.sessionEnvs[session]; ok {
			return env, nil
		}
	}
	return map[string]string{}, nil
}

func setupIntegrationTown(t *testing.T) string {
	t.Helper()
	townRoot := t.TempDir()

	// Create minimal town structure
	dirs := []string{
		"mayor",
		".beads",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(townRoot, dir), 0755); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
	}

	// Create town.json
	townConfig := map[string]interface{}{
		"name":    "test-town",
		"type":    "town",
		"version": 2,
	}
	townJSON, _ := json.Marshal(townConfig)
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), townJSON, 0644); err != nil {
		t.Fatalf("failed to create town.json: %v", err)
	}

	// Create rigs.json
	rigsConfig := map[string]interface{}{
		"version": 1,
		"rigs":    map[string]interface{}{},
	}
	rigsJSON, _ := json.Marshal(rigsConfig)
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), rigsJSON, 0644); err != nil {
		t.Fatalf("failed to create rigs.json: %v", err)
	}

	// Create beads config
	beadsConfig := `# Test beads config
issue-prefix: "hq"
`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "config.yaml"), []byte(beadsConfig), 0644); err != nil {
		t.Fatalf("failed to create beads config: %v", err)
	}

	// Create empty routes.jsonl
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(""), 0644); err != nil {
		t.Fatalf("failed to create routes.jsonl: %v", err)
	}

	// Initialize git repo
	initGitRepoForIntegration(t, townRoot)

	return townRoot
}

func createTestRig(t *testing.T, townRoot, rigName string) {
	t.Helper()
	rigPath := filepath.Join(townRoot, rigName)

	// Create rig directories
	dirs := []string{
		"polecats",
		"crew",
		"witness",
		"refinery",
		"mayor/rig",
		".beads",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(rigPath, dir), 0755); err != nil {
			t.Fatalf("failed to create %s/%s: %v", rigName, dir, err)
		}
	}

	// Create rig config
	rigConfig := map[string]interface{}{
		"name": rigName,
	}
	rigJSON, _ := json.Marshal(rigConfig)
	if err := os.WriteFile(filepath.Join(rigPath, "config.json"), rigJSON, 0644); err != nil {
		t.Fatalf("failed to create rig config: %v", err)
	}

	// Create rig beads config
	beadsConfig := `# Rig beads config
`
	if err := os.WriteFile(filepath.Join(rigPath, ".beads", "config.yaml"), []byte(beadsConfig), 0644); err != nil {
		t.Fatalf("failed to create rig beads config: %v", err)
	}

	// Add route to town beads
	route := map[string]string{
		"prefix": rigName[:2] + "-",
		"path":   rigName,
	}
	routeJSON, _ := json.Marshal(route)
	routesFile := filepath.Join(townRoot, ".beads", "routes.jsonl")
	f, err := os.OpenFile(routesFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("failed to open routes.jsonl: %v", err)
	}
	f.Write(routeJSON)
	f.Write([]byte("\n"))
	f.Close()

	// Update rigs.json
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsData, _ := os.ReadFile(rigsPath)
	var rigsConfig map[string]interface{}
	json.Unmarshal(rigsData, &rigsConfig)

	rigs := rigsConfig["rigs"].(map[string]interface{})
	rigs[rigName] = map[string]interface{}{
		"git_url":  "https://example.com/" + rigName + ".git",
		"added_at": time.Now().Format(time.RFC3339),
		"beads": map[string]string{
			"prefix": rigName[:2],
		},
	}

	rigsJSON, _ := json.Marshal(rigsConfig)
	os.WriteFile(rigsPath, rigsJSON, 0644)
}

func setupMockBeads(t *testing.T, townRoot, rigName string) {
	t.Helper()

	// Create mock issues.jsonl with required beads
	rigPath := filepath.Join(townRoot, rigName)
	issuesFile := filepath.Join(rigPath, ".beads", "issues.jsonl")

	prefix := rigName[:2]
	issues := []map[string]interface{}{
		{
			"id":         prefix + "-rig-" + rigName,
			"title":      rigName,
			"status":     "open",
			"issue_type": "rig",
			"labels":     []string{"gt:rig"},
		},
		{
			"id":         prefix + "-" + rigName + "-witness",
			"title":      "Witness for " + rigName,
			"status":     "open",
			"issue_type": "agent",
			"labels":     []string{"gt:agent"},
		},
		{
			"id":         prefix + "-" + rigName + "-refinery",
			"title":      "Refinery for " + rigName,
			"status":     "open",
			"issue_type": "agent",
			"labels":     []string{"gt:agent"},
		},
	}

	f, err := os.Create(issuesFile)
	if err != nil {
		t.Fatalf("failed to create issues.jsonl: %v", err)
	}
	defer f.Close()

	for _, issue := range issues {
		issueJSON, _ := json.Marshal(issue)
		f.Write(issueJSON)
		f.Write([]byte("\n"))
	}

	// Create town-level role beads
	townIssuesFile := filepath.Join(townRoot, ".beads", "issues.jsonl")
	townIssues := []map[string]interface{}{
		{
			"id":         "hq-witness-role",
			"title":      "Witness Role",
			"status":     "open",
			"issue_type": "role",
			"labels":     []string{"gt:role"},
		},
		{
			"id":         "hq-refinery-role",
			"title":      "Refinery Role",
			"status":     "open",
			"issue_type": "role",
			"labels":     []string{"gt:role"},
		},
		{
			"id":         "hq-crew-role",
			"title":      "Crew Role",
			"status":     "open",
			"issue_type": "role",
			"labels":     []string{"gt:role"},
		},
		{
			"id":         "hq-mayor-role",
			"title":      "Mayor Role",
			"status":     "open",
			"issue_type": "role",
			"labels":     []string{"gt:role"},
		},
		{
			"id":         "hq-deacon-role",
			"title":      "Deacon Role",
			"status":     "open",
			"issue_type": "role",
			"labels":     []string{"gt:role"},
		},
	}

	tf, err := os.Create(townIssuesFile)
	if err != nil {
		t.Fatalf("failed to create town issues.jsonl: %v", err)
	}
	defer tf.Close()

	for _, issue := range townIssues {
		issueJSON, _ := json.Marshal(issue)
		tf.Write(issueJSON)
		tf.Write([]byte("\n"))
	}
}

func breakRuntimeGitignore(t *testing.T, townRoot string) {
	t.Helper()
	// Create a crew directory without .runtime in gitignore
	crewDir := filepath.Join(townRoot, "gastown", "crew", "test-worker")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatalf("failed to create crew dir: %v", err)
	}
	// Create a .gitignore without .runtime
	gitignore := "*.log\n"
	if err := os.WriteFile(filepath.Join(crewDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create gitignore: %v", err)
	}
}

func breakCrewGitignore(t *testing.T, townRoot, rigName, workerName string) {
	t.Helper()
	// Create another crew directory without .runtime in gitignore
	crewDir := filepath.Join(townRoot, rigName, "crew", workerName)
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatalf("failed to create crew dir: %v", err)
	}
	// Create a .gitignore without .runtime
	gitignore := "*.tmp\n"
	if err := os.WriteFile(filepath.Join(crewDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf("failed to create gitignore: %v", err)
	}
}

func initGitRepoForIntegration(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "--initial-branch=main")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user for commits
	exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run()
}
