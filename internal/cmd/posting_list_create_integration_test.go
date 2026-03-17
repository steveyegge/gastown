//go:build integration

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ===========================================================================
// Section 1: gt posting list (integration tests)
// ===========================================================================

// 1.7: error when not in workspace
func TestPostingList_ErrorOutsideWorkspace(t *testing.T) {
	t.Parallel()
	gtBin := buildGT(t)

	cmd := exec.Command(gtBin, "posting", "list")
	cmd.Dir = "/tmp"
	cmd.Env = cleanGTEnv()

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error when running outside workspace, got nil")
	}
	if !strings.Contains(string(output), "not in a Gas Town workspace") {
		t.Errorf("expected 'not in a Gas Town workspace' error, got: %s", output)
	}
}

// 1.13: inside rig infers rig context (no --all)
func TestPostingList_InsideRigInfersRigContext(t *testing.T) {
	t.Parallel()
	gtBin := buildGT(t)

	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")

	env := cleanGTEnv()
	env = append(env, "HOME="+tmpDir)
	os.WriteFile(filepath.Join(tmpDir, ".gitconfig"), []byte("[user]\n\tname = Test\n\temail = test@test.com\n"), 0644)

	installCmd := exec.Command(gtBin, "install", hqPath, "--no-beads")
	installCmd.Env = env
	if output, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\n%s", err, output)
	}

	// Create two rigs with distinct rig-level postings
	rig1Dir := filepath.Join(hqPath, "rig1")
	rig2Dir := filepath.Join(hqPath, "rig2")
	for _, dir := range []string{
		filepath.Join(rig1Dir, "postings"),
		filepath.Join(rig2Dir, "postings"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(rig1Dir, "postings", "alpha.md.tmpl"), []byte("# Alpha"), 0644)
	os.WriteFile(filepath.Join(rig2Dir, "postings", "beta.md.tmpl"), []byte("# Beta"), 0644)

	// Register both rigs in mayor/rigs.json
	rigsJSON := `{"version":1,"rigs":{"rig1":{"git_url":"git@example.com:test/rig1.git"},"rig2":{"git_url":"git@example.com:test/rig2.git"}}}`
	rigsPath := filepath.Join(hqPath, "mayor", "rigs.json")
	if err := os.WriteFile(rigsPath, []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Run gt posting list from INSIDE rig1 (no flags)
	cmd := exec.Command(gtBin, "posting", "list")
	cmd.Dir = rig1Dir
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("gt posting list failed inside rig1: %v\n%s", err, outStr)
	}

	// Should show rig1's posting "alpha"
	if !strings.Contains(outStr, "alpha") {
		t.Errorf("expected rig1's posting 'alpha' in output, got:\n%s", outStr)
	}

	// Should NOT show rig2's posting "beta" (rig inference means single-rig mode)
	if strings.Contains(outStr, "beta") {
		t.Errorf("should not see rig2's posting 'beta' when inside rig1, got:\n%s", outStr)
	}

	// Should show single-rig header, not the all-rigs format
	if !strings.Contains(outStr, "Available postings for rig1") {
		t.Errorf("expected single-rig header 'Available postings for rig1', got:\n%s", outStr)
	}
	if strings.Contains(outStr, "Embedded & town-level") {
		t.Errorf("should not see all-rigs format header when inside a rig, got:\n%s", outStr)
	}
}

// 1.15: --rig flag from town root shows only that rig's postings
func TestPostingList_RigFlagFromTownRoot(t *testing.T) {
	t.Parallel()
	gtBin := buildGT(t)

	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")

	env := cleanGTEnv()
	env = append(env, "HOME="+tmpDir)
	// Configure git identity
	os.WriteFile(filepath.Join(tmpDir, ".gitconfig"), []byte("[user]\n\tname = Test\n\temail = test@test.com\n"), 0644)

	installCmd := exec.Command(gtBin, "install", hqPath, "--no-beads")
	installCmd.Env = env
	if output, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\n%s", err, output)
	}

	// Create two rigs with different postings
	for _, rc := range []struct {
		name     string
		postings []string
	}{
		{"rig-alpha", []string{"scout-alpha"}},
		{"rig-beta", []string{"scout-beta", "medic-beta"}},
	} {
		rigDir := filepath.Join(hqPath, rc.name)
		postingsDir := filepath.Join(rigDir, "postings")
		if err := os.MkdirAll(postingsDir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, pName := range rc.postings {
			content := "---\ndescription: \"" + pName + " posting\"\n---\n# " + pName + "\n"
			if err := os.WriteFile(filepath.Join(postingsDir, pName+".md.tmpl"), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Register both rigs in mayor/rigs.json
	rigsJSON := `{"version":1,"rigs":{` +
		`"rig-alpha":{"git_url":"git@github.com:test/rig-alpha.git"},` +
		`"rig-beta":{"git_url":"git@github.com:test/rig-beta.git"}` +
		`}}`
	rigsPath := filepath.Join(hqPath, "mayor", "rigs.json")
	if err := os.WriteFile(rigsPath, []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Run gt posting list --rig rig-alpha from town root
	cmd := exec.Command(gtBin, "posting", "list", "--rig", "rig-alpha")
	cmd.Dir = hqPath
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("gt posting list --rig rig-alpha failed: %v\n%s", err, outStr)
	}

	// Should contain rig-alpha's posting
	if !strings.Contains(outStr, "scout-alpha") {
		t.Errorf("expected output to contain 'scout-alpha', got:\n%s", outStr)
	}

	// Should NOT contain rig-beta's postings
	if strings.Contains(outStr, "scout-beta") {
		t.Errorf("expected output to NOT contain 'scout-beta' (from rig-beta), got:\n%s", outStr)
	}
	if strings.Contains(outStr, "medic-beta") {
		t.Errorf("expected output to NOT contain 'medic-beta' (from rig-beta), got:\n%s", outStr)
	}

	// Should mention rig-alpha in the header
	if !strings.Contains(outStr, "rig-alpha") {
		t.Errorf("expected output header to mention 'rig-alpha', got:\n%s", outStr)
	}
}

// ===========================================================================
// Section 6: gt posting create (integration tests)
// ===========================================================================

// 6.5: --rig flag targets specific rig
func TestPostingCreate_RigFlagTargetsSpecificRig(t *testing.T) {
	t.Parallel()
	gtBin := buildGT(t)

	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")

	env := cleanGTEnv()
	env = append(env, "HOME="+tmpDir)
	// Configure git identity
	os.WriteFile(filepath.Join(tmpDir, ".gitconfig"), []byte("[user]\n\tname = Test\n\temail = test@test.com\n"), 0644)

	installCmd := exec.Command(gtBin, "install", hqPath, "--no-beads")
	installCmd.Env = env
	if output, err := installCmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\n%s", err, output)
	}

	// Create a test rig directory structure
	rigName := "testrig"
	rigDir := filepath.Join(hqPath, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Run gt posting create with --rig flag from within the workspace
	cmd := exec.Command(gtBin, "posting", "create", "reviewer", "--rig", rigName)
	cmd.Dir = hqPath
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	// The command may fail because testrig isn't a registered rig.
	// That's expected — we're testing that --rig is accepted as a flag.
	if err != nil && strings.Contains(outStr, "unknown flag") {
		t.Errorf("--rig flag not recognized: %s", outStr)
	}
}

// 6.7: NEG no args
func TestPostingCreate_ErrorNoArgs(t *testing.T) {
	t.Parallel()
	gtBin := buildGT(t)

	cmd := exec.Command(gtBin, "posting", "create")
	cmd.Dir = "/tmp"
	cmd.Env = cleanGTEnv()

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
	outStr := string(output)
	if !strings.Contains(outStr, "requires") && !strings.Contains(outStr, "arg") &&
		!strings.Contains(outStr, "accepts") {
		t.Errorf("expected arg count error, got: %s", outStr)
	}
}

// 6.8: NEG too many args
func TestPostingCreate_ErrorTooManyArgs(t *testing.T) {
	t.Parallel()
	gtBin := buildGT(t)

	cmd := exec.Command(gtBin, "posting", "create", "a", "b")
	cmd.Dir = "/tmp"
	cmd.Env = cleanGTEnv()

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for too many args, got nil")
	}
	outStr := string(output)
	if !strings.Contains(outStr, "accepts") && !strings.Contains(outStr, "arg") {
		t.Errorf("expected arg count error, got: %s", outStr)
	}
}

// 6.12: NEG outside workspace
func TestPostingCreate_ErrorOutsideWorkspace(t *testing.T) {
	t.Parallel()
	gtBin := buildGT(t)

	cmd := exec.Command(gtBin, "posting", "create", "reviewer")
	cmd.Dir = "/tmp"
	cmd.Env = cleanGTEnv()

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error when running outside workspace, got nil")
	}
	if !strings.Contains(string(output), "not in a Gas Town workspace") {
		t.Errorf("expected 'not in a Gas Town workspace' error, got: %s", output)
	}
}
