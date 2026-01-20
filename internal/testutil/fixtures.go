package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/deps"
	"github.com/steveyegge/gastown/internal/runtime"
)

var (
	envSetupOnce sync.Once
	envSetupErr  error
)

type TownFixture struct {
	Root  string
	Agent string
	t     *testing.T
}

func NewTownFixture(t *testing.T, agent string) *TownFixture {
	t.Helper()

	ensureTestEnv(t)

	townRoot := t.TempDir()
	f := &TownFixture{
		Root:  townRoot,
		Agent: agent,
		t:     t,
	}

	f.runGTInstall()
	f.configureAgent()
	return f
}

func ensureTestEnv(t *testing.T) {
	t.Helper()

	envSetupOnce.Do(func() {
		homeDir, _ := os.UserHomeDir()
		goBin := filepath.Join(homeDir, "go", "bin")

		if _, err := os.Stat(goBin); err == nil {
			os.Setenv("PATH", goBin+":"+os.Getenv("PATH"))
		}

		if projectRoot := findProjectRoot(); projectRoot != "" {
			os.Setenv("PATH", projectRoot+":"+os.Getenv("PATH"))
		}

		if _, err := exec.LookPath("gt"); err != nil {
			envSetupErr = err
		}
	})

	if envSetupErr != nil {
		t.Fatalf("Test environment setup failed: gt not found - run 'make build'")
	}
}

func FindProjectRoot() string {
	cwd, _ := os.Getwd()
	for dir := cwd; dir != "/"; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "cmd", "gt")); err == nil {
				return dir
			}
		}
	}
	return ""
}

func findProjectRoot() string {
	return FindProjectRoot()
}

func (f *TownFixture) runGTInstall() {
	f.t.Helper()

	cmd := exec.Command("gt", "install", f.Root, "--git")
	output, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("gt install failed: %v\n%s", err, output)
	}
}

func (f *TownFixture) configureAgent() {
	f.t.Helper()

	settingsPath := filepath.Join(f.Root, "settings", "config.json")
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		f.t.Fatalf("Failed to load settings: %v", err)
	}

	settings.DefaultAgent = f.Agent
	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		f.t.Fatalf("Failed to save settings: %v", err)
	}

	runtimeConfig := config.ResolveRoleAgentConfig("mayor", f.Root, "")
	if runtimeConfig != nil {
		runtimeConfig = config.NormalizeRuntimeConfig(runtimeConfig)
	}

	mayorDir := filepath.Join(f.Root, "mayor")
	if err := runtime.EnsureSettingsForRole(mayorDir, "mayor", runtimeConfig); err != nil {
		f.t.Fatalf("Failed to ensure settings for role: %v", err)
	}
}

func (f *TownFixture) Path(rel string) string {
	return filepath.Join(f.Root, rel)
}

func RequireBeads(t *testing.T) {
	t.Helper()
	ensureTestEnv(t)
	status, version := deps.CheckBeads()
	switch status {
	case deps.BeadsOK:
		return
	case deps.BeadsNotFound:
		t.Fatalf("beads (bd) not found in PATH")
	case deps.BeadsTooOld:
		t.Fatalf("beads %s too old (minimum: %s)", version, deps.MinBeadsVersion)
	}
}

func RequireBinary(t *testing.T, name string) {
	t.Helper()
	ensureTestEnv(t)
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found in PATH", name)
	}
}

func RequireGT(t *testing.T) {
	t.Helper()
	ensureTestEnv(t)
}
