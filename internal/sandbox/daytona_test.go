package sandbox

import (
	"context"
	"errors"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/proxy"
)

// mockWorkspaceClient implements WorkspaceClient for testing.
type mockWorkspaceClient struct {
	workspaces    map[string]bool // name -> running
	createCalls   []string
	startCalls    []string
	stopCalls     []string
	deleteCalls   []string
	injectCalls   []injectCall
	createErr     error
	startErr      error
	stopErr       error
	deleteErr     error
	existsErr     error
	injectErr     error
	installPrefix string
}

type injectCall struct {
	wsName, certDir string
	cert, key, ca   []byte
}

func newMockClient(prefix string) *mockWorkspaceClient {
	return &mockWorkspaceClient{
		workspaces:    make(map[string]bool),
		installPrefix: prefix,
	}
}

func (m *mockWorkspaceClient) WorkspaceName(rig, polecat string) string {
	return m.installPrefix + "-" + rig + "--" + polecat
}

func (m *mockWorkspaceClient) Create(ctx context.Context, name string, opts WorkspaceCreateOptions) error {
	m.createCalls = append(m.createCalls, name)
	if m.createErr != nil {
		return m.createErr
	}
	m.workspaces[name] = false
	return nil
}

func (m *mockWorkspaceClient) Start(ctx context.Context, name string) error {
	m.startCalls = append(m.startCalls, name)
	if m.startErr != nil {
		return m.startErr
	}
	m.workspaces[name] = true
	return nil
}

func (m *mockWorkspaceClient) Stop(ctx context.Context, name string) error {
	m.stopCalls = append(m.stopCalls, name)
	if m.stopErr != nil {
		return m.stopErr
	}
	m.workspaces[name] = false
	return nil
}

func (m *mockWorkspaceClient) Delete(ctx context.Context, name string) error {
	m.deleteCalls = append(m.deleteCalls, name)
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.workspaces, name)
	return nil
}

func (m *mockWorkspaceClient) Exists(ctx context.Context, name string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	_, ok := m.workspaces[name]
	return ok, nil
}

func (m *mockWorkspaceClient) InjectCerts(ctx context.Context, wsName, certDir string, cert, key, ca []byte) error {
	m.injectCalls = append(m.injectCalls, injectCall{wsName, certDir, cert, key, ca})
	return m.injectErr
}

// mockCertIssuer implements CertIssuer for testing.
type mockCertIssuer struct {
	issueCalls []issueCall
	denyCalls  []string
	issueErr   error
	denyErr    error
	serial     string
}

type issueCall struct {
	rig, name, ttl string
}

func (m *mockCertIssuer) IssueCert(ctx context.Context, rig, name, ttl string) (*CertResult, error) {
	m.issueCalls = append(m.issueCalls, issueCall{rig, name, ttl})
	if m.issueErr != nil {
		return nil, m.issueErr
	}
	serial := m.serial
	if serial == "" {
		serial = "abc123"
	}
	return &CertResult{
		CN:        "gt-" + rig + "-" + name,
		Cert:      "---CERT---",
		Key:       "---KEY---",
		CA:        "---CA---",
		Serial:    serial,
		ExpiresAt: "2026-04-12T04:36:00Z",
	}, nil
}

func (m *mockCertIssuer) DenyCert(ctx context.Context, serial string) error {
	m.denyCalls = append(m.denyCalls, serial)
	return m.denyErr
}

func defaultOpts(wsName string) SandboxOpts {
	return SandboxOpts{
		Rig:           "TestRig",
		Polecat:       "obsidian",
		InstallPrefix: "gt-abc",
		WorkspaceName: wsName,
		RigSettings: &config.RigSettings{
			RemoteBackend: &config.RemoteBackendConfig{
				Image:   "gastown:latest",
				Profile: "standard",
			},
		},
	}
}

func TestWorkspaceName(t *testing.T) {
	client := newMockClient("gt-abc")
	d := NewDaytonaSandbox(client, &mockCertIssuer{}, nil)

	got := d.WorkspaceName("MyRig", "obsidian")
	want := "gt-abc-MyRig--obsidian"
	if got != want {
		t.Errorf("WorkspaceName = %q, want %q", got, want)
	}
}

func TestPreStart_NewWorkspace(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	wsName := "gt-abc-TestRig--obsidian"
	opts := defaultOpts(wsName)

	env, err := d.PreStart(ctx, opts)
	if err != nil {
		t.Fatalf("PreStart() error: %v", err)
	}

	// Should have created the workspace.
	if len(client.createCalls) != 1 || client.createCalls[0] != wsName {
		t.Errorf("expected Create(%q), got %v", wsName, client.createCalls)
	}

	// Should have started the workspace.
	if len(client.startCalls) != 1 || client.startCalls[0] != wsName {
		t.Errorf("expected Start(%q), got %v", wsName, client.startCalls)
	}

	// Should have issued a cert.
	if len(issuer.issueCalls) != 1 {
		t.Fatalf("expected 1 IssueCert call, got %d", len(issuer.issueCalls))
	}
	ic := issuer.issueCalls[0]
	if ic.rig != "TestRig" || ic.name != "obsidian" || ic.ttl != "720h" {
		t.Errorf("IssueCert args = (%q, %q, %q), want (TestRig, obsidian, 720h)", ic.rig, ic.name, ic.ttl)
	}

	// Should have injected certs.
	if len(client.injectCalls) != 1 {
		t.Fatalf("expected 1 InjectCerts call, got %d", len(client.injectCalls))
	}
	ij := client.injectCalls[0]
	if ij.wsName != wsName {
		t.Errorf("InjectCerts workspace = %q, want %q", ij.wsName, wsName)
	}
	if ij.certDir != DefaultRemoteCertDir {
		t.Errorf("InjectCerts certDir = %q, want %q", ij.certDir, DefaultRemoteCertDir)
	}

	// Check inner env vars.
	expectedEnvKeys := []string{
		"GT_RIG", "GT_POLECAT", "GT_ROLE",
		"GT_PROXY_URL", "GT_PROXY_CERT", "GT_PROXY_KEY", "GT_PROXY_CA",
		"GIT_SSL_CERT", "GIT_SSL_KEY", "GIT_SSL_CAINFO",
		"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL",
		"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL",
		"BD_DOLT_AUTO_COMMIT",
		"GT_CERT_SERIAL",
	}
	for _, key := range expectedEnvKeys {
		if _, ok := env[key]; !ok {
			t.Errorf("missing inner env var %q", key)
		}
	}

	if env["GT_RIG"] != "TestRig" {
		t.Errorf("GT_RIG = %q, want %q", env["GT_RIG"], "TestRig")
	}
	if env["GT_POLECAT"] != "obsidian" {
		t.Errorf("GT_POLECAT = %q, want %q", env["GT_POLECAT"], "obsidian")
	}
	if env["GT_ROLE"] != "TestRig/polecats/obsidian" {
		t.Errorf("GT_ROLE = %q, want %q", env["GT_ROLE"], "TestRig/polecats/obsidian")
	}
	if env["GT_PROXY_URL"] != "https://"+DefaultProxyAddr {
		t.Errorf("GT_PROXY_URL = %q, want %q", env["GT_PROXY_URL"], "https://"+DefaultProxyAddr)
	}
	if env["BD_DOLT_AUTO_COMMIT"] != "off" {
		t.Errorf("BD_DOLT_AUTO_COMMIT = %q, want %q", env["BD_DOLT_AUTO_COMMIT"], "off")
	}
	if env["GT_CERT_SERIAL"] != "abc123" {
		t.Errorf("GT_CERT_SERIAL = %q, want %q", env["GT_CERT_SERIAL"], "abc123")
	}

	// Should NOT have GT_REPO_BRANCH (no branch set).
	if _, ok := env["GT_REPO_BRANCH"]; ok {
		t.Error("GT_REPO_BRANCH should not be set when opts.Branch is empty")
	}
}

func TestPreStart_ExistingWorkspace(t *testing.T) {
	client := newMockClient("gt-abc")
	wsName := "gt-abc-TestRig--obsidian"
	client.workspaces[wsName] = false // exists but stopped

	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts(wsName)

	_, err := d.PreStart(ctx, opts)
	if err != nil {
		t.Fatalf("PreStart() error: %v", err)
	}

	// Should NOT have created (workspace already exists).
	if len(client.createCalls) != 0 {
		t.Errorf("expected no Create calls for existing workspace, got %v", client.createCalls)
	}

	// Should have started the workspace.
	if len(client.startCalls) != 1 {
		t.Errorf("expected 1 Start call, got %d", len(client.startCalls))
	}
}

func TestPreStart_WithBranch(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	wsName := "gt-abc-TestRig--obsidian"
	opts := defaultOpts(wsName)
	opts.Branch = "feat/daytona-two"

	env, err := d.PreStart(ctx, opts)
	if err != nil {
		t.Fatalf("PreStart() error: %v", err)
	}

	if env["GT_REPO_BRANCH"] != "feat/daytona-two" {
		t.Errorf("GT_REPO_BRANCH = %q, want %q", env["GT_REPO_BRANCH"], "feat/daytona-two")
	}
}

func TestPreStart_CustomProxyAddr(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	wsName := "gt-abc-TestRig--obsidian"
	opts := defaultOpts(wsName)
	opts.RigSettings.RemoteBackend.ProxyAddr = "proxy.example.com:9999"

	env, err := d.PreStart(ctx, opts)
	if err != nil {
		t.Fatalf("PreStart() error: %v", err)
	}

	if env["GT_PROXY_URL"] != "https://proxy.example.com:9999" {
		t.Errorf("GT_PROXY_URL = %q, want %q", env["GT_PROXY_URL"], "https://proxy.example.com:9999")
	}
}

func TestPreStart_WithProxyCA(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	wsName := "gt-abc-TestRig--obsidian"
	opts := defaultOpts(wsName)
	opts.ProxyCA = &proxy.CA{CertPEM: []byte("---REAL-CA---")}

	_, err := d.PreStart(ctx, opts)
	if err != nil {
		t.Fatalf("PreStart() error: %v", err)
	}

	// Should use ProxyCA.CertPEM for cert injection.
	if len(client.injectCalls) != 1 {
		t.Fatalf("expected 1 InjectCerts call, got %d", len(client.injectCalls))
	}
	if string(client.injectCalls[0].ca) != "---REAL-CA---" {
		t.Errorf("InjectCerts CA = %q, want %q", string(client.injectCalls[0].ca), "---REAL-CA---")
	}
}

func TestPreStart_CreateError(t *testing.T) {
	client := newMockClient("gt-abc")
	client.createErr = errors.New("quota exceeded")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")

	_, err := d.PreStart(ctx, opts)
	if err == nil {
		t.Fatal("PreStart() should return error when Create fails")
	}
	if !errors.Is(err, client.createErr) {
		t.Errorf("error should wrap create error, got: %v", err)
	}
}

func TestPreStart_StartError(t *testing.T) {
	client := newMockClient("gt-abc")
	client.startErr = errors.New("workspace unhealthy")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")

	_, err := d.PreStart(ctx, opts)
	if err == nil {
		t.Fatal("PreStart() should return error when Start fails")
	}
}

func TestPreStart_IssueCertError(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{issueErr: errors.New("CA unavailable")}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")

	_, err := d.PreStart(ctx, opts)
	if err == nil {
		t.Fatal("PreStart() should return error when IssueCert fails")
	}
}

func TestPreStart_InjectCertsError(t *testing.T) {
	client := newMockClient("gt-abc")
	client.injectErr = errors.New("exec failed")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")

	_, err := d.PreStart(ctx, opts)
	if err == nil {
		t.Fatal("PreStart() should return error when InjectCerts fails")
	}
}

func TestPreStart_NoRemoteBackend(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")
	opts.RigSettings.RemoteBackend = nil

	_, err := d.PreStart(ctx, opts)
	if err == nil {
		t.Fatal("PreStart() should return error when RemoteBackend is nil")
	}
}

func TestPreStart_ExistsError(t *testing.T) {
	client := newMockClient("gt-abc")
	client.existsErr = errors.New("API timeout")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")

	_, err := d.PreStart(ctx, opts)
	if err == nil {
		t.Fatal("PreStart() should return error when Exists fails")
	}
	if !errors.Is(err, client.existsErr) {
		t.Errorf("error should wrap exists error, got: %v", err)
	}

	// Should NOT have attempted Create or Start.
	if len(client.createCalls) != 0 {
		t.Errorf("expected no Create calls after Exists error, got %v", client.createCalls)
	}
	if len(client.startCalls) != 0 {
		t.Errorf("expected no Start calls after Exists error, got %v", client.startCalls)
	}
}

func TestPostStop_CertRevocation(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")
	opts.CertSerial = "abc123"

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() error: %v", err)
	}

	// Should have denied cert with the serial number, not the CN.
	if len(issuer.denyCalls) != 1 {
		t.Fatalf("expected 1 DenyCert call, got %d", len(issuer.denyCalls))
	}
	if issuer.denyCalls[0] != "abc123" {
		t.Errorf("DenyCert serial = %q, want %q", issuer.denyCalls[0], "abc123")
	}
}

func TestPostStop_NoCertSerial_SkipsRevocation(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")
	// CertSerial is empty — should skip DenyCert call.

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() error: %v", err)
	}

	if len(issuer.denyCalls) != 0 {
		t.Errorf("expected 0 DenyCert calls when CertSerial is empty, got %d", len(issuer.denyCalls))
	}
}

func TestPostStop_AutoStop(t *testing.T) {
	client := newMockClient("gt-abc")
	wsName := "gt-abc-TestRig--obsidian"
	client.workspaces[wsName] = true
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts(wsName)
	opts.CertSerial = "abc123"
	opts.RigSettings.RemoteBackend.AutoStop = true

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() error: %v", err)
	}

	if len(client.stopCalls) != 1 || client.stopCalls[0] != wsName {
		t.Errorf("expected Stop(%q), got %v", wsName, client.stopCalls)
	}
}

func TestPostStop_AutoDelete(t *testing.T) {
	client := newMockClient("gt-abc")
	wsName := "gt-abc-TestRig--obsidian"
	client.workspaces[wsName] = true
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts(wsName)
	opts.CertSerial = "abc123"
	opts.RigSettings.RemoteBackend.AutoDelete = true

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() error: %v", err)
	}

	if len(client.deleteCalls) != 1 || client.deleteCalls[0] != wsName {
		t.Errorf("expected Delete(%q), got %v", wsName, client.deleteCalls)
	}
}

func TestPostStop_AutoStopAndAutoDelete(t *testing.T) {
	client := newMockClient("gt-abc")
	wsName := "gt-abc-TestRig--obsidian"
	client.workspaces[wsName] = true
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts(wsName)
	opts.CertSerial = "abc123"
	opts.RigSettings.RemoteBackend.AutoStop = true
	opts.RigSettings.RemoteBackend.AutoDelete = true

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() error: %v", err)
	}

	// Both stop and delete should be called.
	if len(client.stopCalls) != 1 {
		t.Errorf("expected 1 Stop call, got %d", len(client.stopCalls))
	}
	if len(client.deleteCalls) != 1 {
		t.Errorf("expected 1 Delete call, got %d", len(client.deleteCalls))
	}
}

func TestPostStop_NoAutoStopOrDelete(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")
	// AutoStop and AutoDelete default to false.

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() error: %v", err)
	}

	if len(client.stopCalls) != 0 {
		t.Errorf("expected no Stop calls, got %v", client.stopCalls)
	}
	if len(client.deleteCalls) != 0 {
		t.Errorf("expected no Delete calls, got %v", client.deleteCalls)
	}
}

func TestPostStop_CertRevocationError_NonFatal(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{denyErr: errors.New("proxy down")}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")
	opts.CertSerial = "abc123"
	opts.RigSettings.RemoteBackend.AutoStop = true

	// PostStop should NOT return an error even if cert revocation fails.
	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() should be non-fatal on cert revocation error, got: %v", err)
	}

	// Should still attempt to stop the workspace.
	if len(client.stopCalls) != 1 {
		t.Errorf("expected workspace Stop even after cert failure, got %d calls", len(client.stopCalls))
	}
}

func TestPostStop_StopError_NonFatal(t *testing.T) {
	client := newMockClient("gt-abc")
	client.stopErr = errors.New("workspace not found")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")
	opts.CertSerial = "abc123"
	opts.RigSettings.RemoteBackend.AutoStop = true
	opts.RigSettings.RemoteBackend.AutoDelete = true

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() should be non-fatal on stop error, got: %v", err)
	}

	// Should still attempt delete even if stop failed.
	if len(client.deleteCalls) != 1 {
		t.Errorf("expected Delete even after Stop failure, got %d calls", len(client.deleteCalls))
	}
}

func TestPostStop_NilRemoteBackend(t *testing.T) {
	client := newMockClient("gt-abc")
	issuer := &mockCertIssuer{}
	d := NewDaytonaSandbox(client, issuer, nil)

	ctx := context.Background()
	opts := defaultOpts("gt-abc-TestRig--obsidian")
	opts.CertSerial = "abc123"
	opts.RigSettings.RemoteBackend = nil

	err := d.PostStop(ctx, opts)
	if err != nil {
		t.Fatalf("PostStop() error: %v", err)
	}

	// Should still revoke cert even without RemoteBackend config.
	if len(issuer.denyCalls) != 1 {
		t.Errorf("expected cert revocation even without RemoteBackend, got %d calls", len(issuer.denyCalls))
	}
}

func TestReconcile_WithFunction(t *testing.T) {
	called := false
	var capturedOpts ReconcileOpts

	reconcileFn := func(ctx context.Context, opts ReconcileOpts) error {
		called = true
		capturedOpts = opts
		return nil
	}

	d := NewDaytonaSandbox(newMockClient("gt-abc"), &mockCertIssuer{}, reconcileFn)

	ctx := context.Background()
	opts := ReconcileOpts{
		Rig:           "TestRig",
		InstallPrefix: "gt-abc",
	}

	err := d.Reconcile(ctx, opts)
	if err != nil {
		t.Fatalf("Reconcile() error: %v", err)
	}
	if !called {
		t.Error("reconcile function was not called")
	}
	if capturedOpts.Rig != "TestRig" {
		t.Errorf("reconcile opts.Rig = %q, want %q", capturedOpts.Rig, "TestRig")
	}
}

func TestReconcile_NilFunction(t *testing.T) {
	d := NewDaytonaSandbox(newMockClient("gt-abc"), &mockCertIssuer{}, nil)

	err := d.Reconcile(context.Background(), ReconcileOpts{})
	if err != nil {
		t.Fatalf("Reconcile() with nil function should return nil, got: %v", err)
	}
}

func TestReconcile_ErrorPropagation(t *testing.T) {
	reconcileErr := errors.New("reconciliation failed")
	reconcileFn := func(ctx context.Context, opts ReconcileOpts) error {
		return reconcileErr
	}

	d := NewDaytonaSandbox(newMockClient("gt-abc"), &mockCertIssuer{}, reconcileFn)

	err := d.Reconcile(context.Background(), ReconcileOpts{})
	if !errors.Is(err, reconcileErr) {
		t.Errorf("Reconcile() error = %v, want %v", err, reconcileErr)
	}
}

// Verify the compile-time interface assertion works.
func TestInterfaceCompliance(t *testing.T) {
	var _ Lifecycle = (*DaytonaSandbox)(nil)
}
