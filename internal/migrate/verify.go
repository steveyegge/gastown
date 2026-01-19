package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VerificationResult describes the outcome of post-migration verification.
type VerificationResult struct {
	// Success indicates all verification checks passed.
	Success bool `json:"success"`

	// Checks lists individual verification results.
	Checks []VerificationCheck `json:"checks"`

	// Errors lists critical issues found.
	Errors []string `json:"errors,omitempty"`

	// Warnings lists non-critical issues found.
	Warnings []string `json:"warnings,omitempty"`
}

// VerificationCheck describes a single verification check.
type VerificationCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// Verifier performs post-migration verification checks.
type Verifier struct {
	townRoot string
}

// NewVerifier creates a new verifier for the given workspace.
func NewVerifier(townRoot string) *Verifier {
	return &Verifier{townRoot: townRoot}
}

// Verify runs all post-migration verification checks.
func (v *Verifier) Verify() *VerificationResult {
	result := &VerificationResult{
		Success: true,
		Checks:  []VerificationCheck{},
	}

	// Run individual checks
	checks := []func() VerificationCheck{
		v.checkTownConfig,
		v.checkRigsConfig,
		v.checkMayorDirectory,
		v.checkBeadsAccessible,
		v.checkRigStructure,
	}

	for _, check := range checks {
		checkResult := check()
		result.Checks = append(result.Checks, checkResult)
		if !checkResult.Passed {
			result.Success = false
			result.Errors = append(result.Errors, checkResult.Message)
		}
	}

	return result
}

// VerifyQuick runs a fast subset of verification checks.
func (v *Verifier) VerifyQuick() *VerificationResult {
	result := &VerificationResult{
		Success: true,
		Checks:  []VerificationCheck{},
	}

	// Only run essential checks
	checks := []func() VerificationCheck{
		v.checkTownConfig,
		v.checkMayorDirectory,
	}

	for _, check := range checks {
		checkResult := check()
		result.Checks = append(result.Checks, checkResult)
		if !checkResult.Passed {
			result.Success = false
			result.Errors = append(result.Errors, checkResult.Message)
		}
	}

	return result
}

// checkTownConfig verifies town.json is accessible and valid.
func (v *Verifier) checkTownConfig() VerificationCheck {
	check := VerificationCheck{Name: "town-config"}

	// Check for mayor/town.json (0.2.x location)
	configPath := filepath.Join(v.townRoot, "mayor", "town.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Cannot read town.json: %v", err)
		return check
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Invalid town.json JSON: %v", err)
		return check
	}

	// Check for required fields
	if _, ok := config["name"]; !ok {
		check.Passed = false
		check.Message = "town.json missing 'name' field"
		return check
	}

	check.Passed = true
	check.Message = "town.json is valid"
	return check
}

// checkRigsConfig verifies rigs.json is accessible and valid.
func (v *Verifier) checkRigsConfig() VerificationCheck {
	check := VerificationCheck{Name: "rigs-config"}

	// Check for mayor/rigs.json (0.2.x location)
	configPath := filepath.Join(v.townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// rigs.json is optional
		check.Passed = true
		check.Message = "rigs.json not found (optional)"
		return check
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Invalid rigs.json JSON: %v", err)
		return check
	}

	check.Passed = true
	check.Message = "rigs.json is valid"
	return check
}

// checkMayorDirectory verifies the mayor/ directory structure.
func (v *Verifier) checkMayorDirectory() VerificationCheck {
	check := VerificationCheck{Name: "mayor-directory"}

	mayorDir := filepath.Join(v.townRoot, "mayor")
	info, err := os.Stat(mayorDir)
	if err != nil {
		check.Passed = false
		check.Message = "mayor/ directory not found"
		return check
	}

	if !info.IsDir() {
		check.Passed = false
		check.Message = "mayor/ is not a directory"
		return check
	}

	check.Passed = true
	check.Message = "mayor/ directory exists"
	return check
}

// checkBeadsAccessible verifies beads database is accessible.
func (v *Verifier) checkBeadsAccessible() VerificationCheck {
	check := VerificationCheck{Name: "beads-accessible"}

	// Check if bd command is available
	_, err := exec.LookPath("bd")
	if err != nil {
		check.Passed = true
		check.Message = "beads not installed (skipped)"
		return check
	}

	// Try to run bd list in the workspace
	cmd := exec.Command("bd", "list", "--limit", "1")
	cmd.Dir = v.townRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is just "no beads found" vs actual failure
		if strings.Contains(string(output), "No beads found") ||
			strings.Contains(string(output), "no beads") {
			check.Passed = true
			check.Message = "beads accessible (empty)"
			return check
		}
		check.Passed = false
		check.Message = fmt.Sprintf("beads database error: %s", strings.TrimSpace(string(output)))
		return check
	}

	check.Passed = true
	check.Message = "beads database accessible"
	return check
}

// checkRigStructure verifies rig directories have expected structure.
func (v *Verifier) checkRigStructure() VerificationCheck {
	check := VerificationCheck{Name: "rig-structure"}

	rigs := detectRigs(v.townRoot)
	if len(rigs) == 0 {
		check.Passed = true
		check.Message = "no rigs found (ok)"
		return check
	}

	var issues []string
	for _, rigPath := range rigs {
		rigName := filepath.Base(rigPath)

		// Check for settings/ directory (0.2.x requirement)
		settingsDir := filepath.Join(rigPath, "settings")
		if _, err := os.Stat(settingsDir); os.IsNotExist(err) {
			issues = append(issues, fmt.Sprintf("%s: missing settings/", rigName))
		}

		// Check for mayor/rig/ directory
		mayorRigDir := filepath.Join(rigPath, "mayor", "rig")
		if _, err := os.Stat(mayorRigDir); os.IsNotExist(err) {
			// This is expected for some rig types, so just note it
			// issues = append(issues, fmt.Sprintf("%s: missing mayor/rig/", rigName))
		}
	}

	if len(issues) > 0 {
		check.Passed = false
		check.Message = strings.Join(issues, "; ")
		return check
	}

	check.Passed = true
	check.Message = fmt.Sprintf("%d rig(s) verified", len(rigs))
	return check
}

// RunDoctorQuick runs 'gt doctor --quick' for comprehensive verification.
func RunDoctorQuick(townRoot string) error {
	cmd := exec.Command("gt", "doctor", "--quick")
	cmd.Dir = townRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gt doctor failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
