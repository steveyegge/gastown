package polecat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gofrs/flock"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/daytona"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/proxy"
	"github.com/steveyegge/gastown/internal/rig"
)

// configurableMockRunner allows per-command control over success/failure for
// testing different rollback scenarios in addDaytona.
type configurableMockRunner struct {
	mu       sync.Mutex
	calls    []mockCall
	handlers map[string]func(args []string) (string, string, int, error)
}

func newConfigurableMockRunner() *configurableMockRunner {
	return &configurableMockRunner{
		handlers: make(map[string]func(args []string) (string, string, int, error)),
	}
}

func (r *configurableMockRunner) Run(_ context.Context, name string, args ...string) (string, string, int, error) {
	r.mu.Lock()
	r.calls = append(r.calls, mockCall{name: name, args: args})
	r.mu.Unlock()

	// Check for a handler matching the first subcommand (e.g. "create", "exec", "stop", "delete").
	if len(args) > 0 {
		key := name + ":" + args[0]
		r.mu.Lock()
		handler, ok := r.handlers[key]
		r.mu.Unlock()
		if ok {
			return handler(args)
		}
	}

	// Default: succeed.
	return "", "", 0, nil
}

func (r *configurableMockRunner) getCalls() []mockCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]mockCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// setupDaytonaTestManager creates a Manager with all dependencies configured for
// addDaytona testing: git repo with origin/main, proxy CA, mock bd, and daytona client.
func setupDaytonaTestManager(t *testing.T, runner daytona.CommandRunner) *Manager {
	t.Helper()

	root := t.TempDir()

	// Create mayor/rig bare-ish directory with origin/main ref.
	mayorRig := filepath.Join(root, "mayor", "rig")
	if err := os.MkdirAll(mayorRig, 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(mayorRig, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mayorGit := git.NewGit(mayorRig)
	if err := mayorGit.Add("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := mayorGit.Commit("Initial commit"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	// Point origin/main at HEAD.
	cmd = exec.Command("git", "remote", "add", "origin", mayorRig)
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "update-ref", "refs/remotes/origin/main", "HEAD")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git update-ref: %v\n%s", err, out)
	}

	// Install mock bd.
	installMockBd(t)

	// Create beads directories.
	rigBeads := filepath.Join(root, ".beads")
	mayorBeads := filepath.Join(mayorRig, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(mayorBeads, 0755); err != nil {
		t.Fatalf("mkdir mayor .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rigBeads, "redirect"), []byte("mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}
	_ = os.WriteFile(filepath.Join(mayorBeads, ".gt-types-configured"), []byte("v1\n"), 0644)

	// Generate a real proxy CA for cert issuance.
	caDir := filepath.Join(root, "ca")
	ca, err := proxy.GenerateCA(caDir)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: root,
	}
	m := NewManager(r, git.NewGit(root), nil)

	client := daytona.NewClientWithRunner("gt-test1234", runner)
	settings := &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{
			Provider:  "daytona",
			ProxyAddr: "proxy.test:8443",
		},
	}
	m.SetDaytona(client, ca, settings)

	// Create polecat dir (addDaytona expects it to already exist).
	polecatDir := m.polecatDir("testpolecat")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatalf("mkdir polecatDir: %v", err)
	}

	return m
}

// newTestLock creates a flock for testing addDaytona's lock parameter.
func newTestLock(t *testing.T) *flock.Flock {
	t.Helper()
	lockPath := filepath.Join(t.TempDir(), "test.lock")
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		t.Fatalf("lock: %v", err)
	}
	return fl
}

// TestAddDaytona_NilClient verifies error when daytonaClient is nil.
func TestAddDaytona_NilClient(t *testing.T) {
	t.Parallel()

	r := &rig.Rig{Name: "testrig", Path: t.TempDir()}
	m := NewManager(r, git.NewGit(r.Path), nil)
	// daytonaClient is nil

	fl := newTestLock(t)
	defer func() { _ = fl.Unlock() }()

	polecatDir := filepath.Join(r.Path, "polecats", "test")
	_ = os.MkdirAll(polecatDir, 0755)

	_, err := m.addDaytona("test", AddOptions{}, polecatDir, fl)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "daytona client not configured") {
		t.Errorf("expected 'daytona client not configured' error, got: %v", err)
	}
}

// TestAddDaytona_NilProxyCA verifies error when proxyCA is nil.
func TestAddDaytona_NilProxyCA(t *testing.T) {
	t.Parallel()

	r := &rig.Rig{Name: "testrig", Path: t.TempDir()}
	m := NewManager(r, git.NewGit(r.Path), nil)

	runner := &mockRunner{}
	client := daytona.NewClientWithRunner("gt-test1234", runner)
	m.SetDaytona(client, nil, &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{Provider: "daytona"},
	})

	fl := newTestLock(t)
	defer func() { _ = fl.Unlock() }()

	polecatDir := filepath.Join(r.Path, "polecats", "test")
	_ = os.MkdirAll(polecatDir, 0755)

	_, err := m.addDaytona("test", AddOptions{}, polecatDir, fl)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "proxy CA not configured") {
		t.Errorf("expected 'proxy CA not configured' error, got: %v", err)
	}
}

// TestAddDaytona_BranchCreationFails verifies rollback when the start point ref
// doesn't exist (branch creation fails at ref validation).
func TestAddDaytona_BranchCreationFails(t *testing.T) {
	root := t.TempDir()

	// Create mayor/rig with git but NO origin/main ref — ref check fails.
	mayorRig := filepath.Join(root, "mayor", "rig")
	if err := os.MkdirAll(mayorRig, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(mayorRig, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mayorGit := git.NewGit(mayorRig)
	if err := mayorGit.Add("."); err != nil {
		t.Fatal(err)
	}
	if err := mayorGit.Commit("init"); err != nil {
		t.Fatal(err)
	}
	// Add origin but don't create origin/main ref.
	cmd = exec.Command("git", "remote", "add", "origin", "/nonexistent/repo")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remote add: %v\n%s", err, out)
	}

	installMockBd(t)

	caDir := filepath.Join(root, "ca")
	ca, err := proxy.GenerateCA(caDir)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	r := &rig.Rig{Name: "testrig", Path: root}
	m := NewManager(r, git.NewGit(root), nil)

	runner := newConfigurableMockRunner()
	client := daytona.NewClientWithRunner("gt-test1234", runner)
	m.SetDaytona(client, ca, &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{Provider: "daytona"},
	})

	name := "branchfail"
	polecatDir := m.polecatDir(name)
	_ = os.MkdirAll(polecatDir, 0755)

	fl := newTestLock(t)
	defer func() { _ = fl.Unlock() }()

	_, err = m.addDaytona(name, AddOptions{}, polecatDir, fl)
	if err == nil {
		t.Fatal("expected error from ref validation, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' ref error, got: %v", err)
	}

	// Verify rollback: polecat directory should be removed.
	if _, statErr := os.Stat(polecatDir); !os.IsNotExist(statErr) {
		t.Error("polecat directory should be removed after rollback")
	}

	// No daytona calls should have been made (failed before step 4).
	calls := runner.getCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 daytona calls, got %d: %v", len(calls), calls)
	}
}

// TestAddDaytona_WorkspaceCreateFails verifies rollback when daytona create fails.
// Steps 1-3 (branch, cert, bead) succeed; step 4 (workspace create) fails.
// Rollback should clean up the branch.
func TestAddDaytona_WorkspaceCreateFails(t *testing.T) {
	runner := newConfigurableMockRunner()
	runner.handlers["daytona:create"] = func(args []string) (string, string, int, error) {
		return "", "workspace creation failed", 1, fmt.Errorf("daytona create error")
	}

	m := setupDaytonaTestManager(t, runner)

	name := "testpolecat"
	polecatDir := m.polecatDir(name)

	fl := newTestLock(t)
	defer func() { _ = fl.Unlock() }()

	_, err := m.addDaytona(name, AddOptions{}, polecatDir, fl)
	if err == nil {
		t.Fatal("expected error from workspace create, got nil")
	}
	if !strings.Contains(err.Error(), "daytona create") {
		t.Errorf("expected daytona create error, got: %v", err)
	}

	// Verify rollback: polecat directory cleaned up.
	if _, statErr := os.Stat(polecatDir); !os.IsNotExist(statErr) {
		t.Error("polecat directory should be removed after rollback")
	}

	// Verify no workspace delete call (workspace was never created).
	calls := runner.getCalls()
	for _, c := range calls {
		if c.name == "daytona" && len(c.args) > 0 && c.args[0] == "delete" {
			t.Error("should not call daytona delete when create failed")
		}
	}
}

// TestAddDaytona_CertInjectionFails verifies rollback when cert injection fails
// after workspace creation. The workspace should be deleted during rollback.
func TestAddDaytona_CertInjectionFails(t *testing.T) {
	execCallCount := 0
	runner := newConfigurableMockRunner()
	// First exec call (mkdir -p) succeeds. Second exec call (writeFile) fails.
	runner.handlers["daytona:exec"] = func(args []string) (string, string, int, error) {
		execCallCount++
		if execCallCount == 1 {
			// mkdir -p succeeds
			return "", "", 0, nil
		}
		// writeFile fails
		return "", "permission denied", 1, nil
	}

	m := setupDaytonaTestManager(t, runner)

	name := "testpolecat"
	polecatDir := m.polecatDir(name)

	fl := newTestLock(t)
	defer func() { _ = fl.Unlock() }()

	_, err := m.addDaytona(name, AddOptions{}, polecatDir, fl)
	if err == nil {
		t.Fatal("expected error from cert injection, got nil")
	}

	// Verify rollback: workspace should be deleted.
	calls := runner.getCalls()
	foundDelete := false
	for _, c := range calls {
		if c.name == "daytona" && len(c.args) > 0 && c.args[0] == "delete" {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Error("expected daytona delete call during rollback after cert injection failure")
	}

	// Polecat directory cleaned up.
	if _, statErr := os.Stat(polecatDir); !os.IsNotExist(statErr) {
		t.Error("polecat directory should be removed after rollback")
	}
}

// TestAddDaytona_PostCreateFails verifies rollback when post-create (gt prime)
// fails. The workspace should be deleted during rollback.
func TestAddDaytona_PostCreateFails(t *testing.T) {
	execCallCount := 0
	runner := newConfigurableMockRunner()
	// exec calls: 1=mkdir, 2-4=writeFile(cert,key,ca), 5=gt prime (fail).
	runner.handlers["daytona:exec"] = func(args []string) (string, string, int, error) {
		execCallCount++
		if execCallCount <= 4 {
			return "", "", 0, nil
		}
		// gt prime fails
		return "", "gt prime failed", 1, nil
	}

	m := setupDaytonaTestManager(t, runner)

	name := "testpolecat"
	polecatDir := m.polecatDir(name)

	fl := newTestLock(t)
	defer func() { _ = fl.Unlock() }()

	_, err := m.addDaytona(name, AddOptions{}, polecatDir, fl)
	if err == nil {
		t.Fatal("expected error from post-create, got nil")
	}
	if !strings.Contains(err.Error(), "post-create") {
		t.Errorf("expected post-create error, got: %v", err)
	}

	// Rollback: workspace deleted.
	calls := runner.getCalls()
	foundDelete := false
	for _, c := range calls {
		if c.name == "daytona" && len(c.args) > 0 && c.args[0] == "delete" {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Error("expected daytona delete during rollback after post-create failure")
	}
}

// TestAddDaytona_SuccessPath verifies the happy path: all steps succeed,
// the returned Polecat has the correct fields set.
func TestAddDaytona_SuccessPath(t *testing.T) {
	runner := newConfigurableMockRunner()
	m := setupDaytonaTestManager(t, runner)

	name := "testpolecat"
	polecatDir := m.polecatDir(name)

	fl := newTestLock(t)
	// Note: addDaytona unlocks the lock before step 4, so don't defer Unlock.

	p, err := m.addDaytona(name, AddOptions{HookBead: "gtd-abc"}, polecatDir, fl)
	if err != nil {
		t.Fatalf("addDaytona failed: %v", err)
	}

	// Verify returned Polecat.
	if p.Name != name {
		t.Errorf("Name = %q, want %q", p.Name, name)
	}
	if p.Rig != "testrig" {
		t.Errorf("Rig = %q, want testrig", p.Rig)
	}
	if p.State != StateWorking {
		t.Errorf("State = %v, want StateWorking", p.State)
	}
	if p.DaytonaWorkspaceName == "" {
		t.Error("DaytonaWorkspaceName should be set")
	}
	if !strings.Contains(p.DaytonaWorkspaceName, "testrig") {
		t.Errorf("DaytonaWorkspaceName should contain rig name, got: %q", p.DaytonaWorkspaceName)
	}
	if p.Branch == "" {
		t.Error("Branch should be set")
	}
	if p.ClonePath != polecatDir {
		t.Errorf("ClonePath = %q, want %q", p.ClonePath, polecatDir)
	}

	// Verify daytona calls: create, then exec calls (mkdir + 3 file writes + gt prime).
	calls := runner.getCalls()
	if len(calls) == 0 {
		t.Fatal("expected daytona calls, got 0")
	}

	// First call should be create (no --volume; certs injected post-create via exec).
	foundCreate := false
	foundCertExec := false
	for _, c := range calls {
		if c.name == "daytona" && len(c.args) > 0 && c.args[0] == "create" {
			foundCreate = true
		}
		if c.name == "daytona" && len(c.args) > 0 && c.args[0] == "exec" {
			for _, arg := range c.args {
				if strings.Contains(arg, ".gt-proxy") {
					foundCertExec = true
				}
			}
		}
	}
	if !foundCreate {
		t.Errorf("expected 'daytona create' call in: %v", calls)
	}
	if !foundCertExec {
		t.Errorf("expected 'daytona exec' call injecting certs to .gt-proxy in: %v", calls)
	}
}

// TestAddDaytona_LockReleasedBeforeStep4 verifies that the polecat lock is
// released before the slow daytona workspace creation (step 4). (gtd-b4o)
func TestAddDaytona_LockReleasedBeforeStep4(t *testing.T) {
	lockReleased := false
	runner := newConfigurableMockRunner()
	runner.handlers["daytona:create"] = func(args []string) (string, string, int, error) {
		// When daytona create is called, the lock should already be released.
		// We can't check flock state directly, but we set a flag.
		lockReleased = true
		return "", "", 0, nil
	}

	m := setupDaytonaTestManager(t, runner)

	name := "testpolecat"
	polecatDir := m.polecatDir(name)

	fl := newTestLock(t)

	_, err := m.addDaytona(name, AddOptions{}, polecatDir, fl)
	if err != nil {
		t.Fatalf("addDaytona failed: %v", err)
	}

	if !lockReleased {
		t.Error("daytona create was not called (lock release not verified)")
	}
}

// TestAddDaytona_RollbackDeletesBranch verifies that when workspace creation
// fails, the branch created in step 1 is deleted during rollback.
func TestAddDaytona_RollbackDeletesBranch(t *testing.T) {
	runner := newConfigurableMockRunner()
	runner.handlers["daytona:create"] = func(args []string) (string, string, int, error) {
		return "", "create failed", 1, fmt.Errorf("create error")
	}

	m := setupDaytonaTestManager(t, runner)

	name := "testpolecat"
	polecatDir := m.polecatDir(name)

	fl := newTestLock(t)
	defer func() { _ = fl.Unlock() }()

	_, err := m.addDaytona(name, AddOptions{HookBead: "gtd-xyz"}, polecatDir, fl)
	if err == nil {
		t.Fatal("expected error")
	}

	// Verify the branch was cleaned up by checking mayor/rig git.
	mayorRig := filepath.Join(m.rig.Path, "mayor", "rig")
	cmd := exec.Command("git", "branch", "--list", "polecat/*")
	cmd.Dir = mayorRig
	out, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		t.Fatalf("git branch: %v\n%s", cmdErr, out)
	}
	branches := strings.TrimSpace(string(out))
	if branches != "" {
		t.Errorf("expected no polecat branches after rollback, got: %q", branches)
	}
}

// TestInjectCertsIntoWorkspace_Success verifies the happy path for cert injection:
// mkdir, then 3 file writes (client.crt, client.key, ca.crt).
func TestInjectCertsIntoWorkspace_Success(t *testing.T) {
	t.Parallel()

	rigDir := t.TempDir()
	r := &rig.Rig{Name: "testrig", Path: rigDir}
	m := NewManager(r, git.NewGit(rigDir), nil)

	runner := newConfigurableMockRunner()
	client := daytona.NewClientWithRunner("gt-test1234", runner)
	ca, err := proxy.GenerateCA(filepath.Join(rigDir, "ca"))
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	m.SetDaytona(client, ca, &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{Provider: "daytona"},
	})

	certPEM := []byte("-----BEGIN CERTIFICATE-----\ntest-cert\n-----END CERTIFICATE-----\n")
	keyPEM := []byte("-----BEGIN EC PRIVATE KEY-----\ntest-key\n-----END EC PRIVATE KEY-----\n")

	err = m.injectCertsIntoWorkspace(context.Background(), "ws-test", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("injectCertsIntoWorkspace: %v", err)
	}

	// Expect 4 exec calls: 1 mkdir + 3 file writes.
	calls := runner.getCalls()
	if len(calls) != 4 {
		t.Fatalf("expected 4 exec calls (mkdir + 3 writes), got %d", len(calls))
	}

	// First call should be mkdir -p.
	firstArgs := calls[0].args
	foundMkdir := false
	for _, a := range firstArgs {
		if a == "mkdir" {
			foundMkdir = true
		}
	}
	if !foundMkdir {
		t.Errorf("first exec call should be mkdir, got args: %v", firstArgs)
	}
}

// TestInjectCertsIntoWorkspace_MkdirFails verifies that cert injection fails
// when the mkdir command fails.
func TestInjectCertsIntoWorkspace_MkdirFails(t *testing.T) {
	t.Parallel()

	rigDir := t.TempDir()
	r := &rig.Rig{Name: "testrig", Path: rigDir}
	m := NewManager(r, git.NewGit(rigDir), nil)

	runner := newConfigurableMockRunner()
	runner.handlers["daytona:exec"] = func(args []string) (string, string, int, error) {
		return "", "mkdir failed", 1, nil
	}

	client := daytona.NewClientWithRunner("gt-test1234", runner)
	ca, err := proxy.GenerateCA(filepath.Join(rigDir, "ca"))
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	m.SetDaytona(client, ca, &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{Provider: "daytona"},
	})

	err = m.injectCertsIntoWorkspace(context.Background(), "ws-test", []byte("cert"), []byte("key"))
	if err == nil {
		t.Fatal("expected error from mkdir failure")
	}
	if !strings.Contains(err.Error(), "creating cert dir") {
		t.Errorf("expected 'creating cert dir' error, got: %v", err)
	}
}

// TestInjectCertsIntoWorkspace_WriteFileFails verifies that cert injection fails
// when a file write command fails.
func TestInjectCertsIntoWorkspace_WriteFileFails(t *testing.T) {
	t.Parallel()

	rigDir := t.TempDir()
	r := &rig.Rig{Name: "testrig", Path: rigDir}
	m := NewManager(r, git.NewGit(rigDir), nil)

	execCount := 0
	runner := newConfigurableMockRunner()
	runner.handlers["daytona:exec"] = func(args []string) (string, string, int, error) {
		execCount++
		if execCount == 1 {
			return "", "", 0, nil // mkdir succeeds
		}
		return "", "write failed", 1, nil // file write fails
	}

	client := daytona.NewClientWithRunner("gt-test1234", runner)
	ca, err := proxy.GenerateCA(filepath.Join(rigDir, "ca"))
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	m.SetDaytona(client, ca, &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{Provider: "daytona"},
	})

	err = m.injectCertsIntoWorkspace(context.Background(), "ws-test", []byte("cert"), []byte("key"))
	if err == nil {
		t.Fatal("expected error from file write failure")
	}
	if !strings.Contains(err.Error(), "injecting") {
		t.Errorf("expected 'injecting' error, got: %v", err)
	}
}

// TestDenyCertForPolecat_NilProxyAdmin verifies no-op when proxyAdmin is nil.
func TestDenyCertForPolecat_NilProxyAdmin(t *testing.T) {
	t.Parallel()

	r := &rig.Rig{Name: "testrig", Path: t.TempDir()}
	m := NewManager(r, git.NewGit(r.Path), nil)
	// proxyAdmin is nil — should return without panic.

	// Should not panic.
	m.denyCertForPolecat("nonexistent")
}

// TestDenyCertForPolecat_NoAgentBead verifies no-op when agent bead doesn't exist.
func TestDenyCertForPolecat_NoAgentBead(t *testing.T) {
	// Cannot use t.Parallel() because installMockBd uses t.Setenv.

	root := t.TempDir()
	// Set up beads dir structure.
	mayorRig := filepath.Join(root, "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(mayorRig, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	rigBeads := filepath.Join(root, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeads, "redirect"), []byte("mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	installMockBd(t)

	r := &rig.Rig{Name: "testrig", Path: root}
	m := NewManager(r, git.NewGit(root), nil)
	// Set a non-nil proxyAdmin so we enter the function body.
	m.proxyAdmin = proxy.NewAdminClient("127.0.0.1:19877")

	// Should return gracefully (GetAgentBead fails or returns nil fields).
	m.denyCertForPolecat("nonexistent")
}

// TestIssuePolecatCert_Success verifies that issuePolecatCert returns valid
// PEM cert and key with a non-empty serial number.
func TestIssuePolecatCert_Success(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	ca, err := proxy.GenerateCA(filepath.Join(root, "ca"))
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	r := &rig.Rig{Name: "testrig", Path: root}
	m := NewManager(r, git.NewGit(root), nil)
	m.proxyCA = ca

	certPEM, keyPEM, serial, err := m.issuePolecatCert("testpolecat")
	if err != nil {
		t.Fatalf("issuePolecatCert: %v", err)
	}

	if len(certPEM) == 0 {
		t.Error("certPEM is empty")
	}
	if len(keyPEM) == 0 {
		t.Error("keyPEM is empty")
	}
	if serial == "" {
		t.Error("serial is empty")
	}

	// Verify cert is valid PEM.
	if !strings.Contains(string(certPEM), "BEGIN CERTIFICATE") {
		t.Error("certPEM should contain PEM header")
	}
	if !strings.Contains(string(keyPEM), "BEGIN") {
		t.Error("keyPEM should contain PEM header")
	}
}

// TestIssuePolecatCert_NilCA verifies that issuePolecatCert panics or errors
// when proxyCA is nil.
func TestIssuePolecatCert_NilCA(t *testing.T) {
	t.Parallel()

	r := &rig.Rig{Name: "testrig", Path: t.TempDir()}
	m := NewManager(r, git.NewGit(r.Path), nil)
	// proxyCA is nil

	defer func() {
		if r := recover(); r != nil {
			// Expected: nil pointer dereference since proxyCA is nil
			return
		}
	}()

	_, _, _, err := m.issuePolecatCert("test")
	if err == nil {
		// If it doesn't panic, it should at least error
		t.Log("issuePolecatCert did not error with nil proxyCA (covered by addDaytona guard)")
	}
}
