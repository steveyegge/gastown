package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestRunDaytonaPreflightChecks_MissingDaytonaCLI(t *testing.T) {
	// Override PATH to ensure daytona is not found
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	err := runDaytonaPreflightChecks(t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error when daytona CLI is not in PATH")
	}
	if want := "daytona CLI not found in PATH"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want to contain %q", err.Error(), want)
	}
}

func TestRunDaytonaPreflightChecks_CustomAdminAddr(t *testing.T) {
	// Skip if daytona is not installed
	if _, err := lookPathDaytona(); err != nil {
		t.Skip("daytona CLI not available")
	}

	settings := &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{
			Provider:       "daytona",
			ProxyAdminAddr: "10.0.0.5:9877",
		},
	}

	err := runDaytonaPreflightChecks(t.TempDir(), settings)
	if err == nil {
		t.Fatal("expected error (proxy not reachable at custom addr)")
	}
	// Error message should reference the custom address, not 127.0.0.1
	if !strings.Contains(err.Error(), "10.0.0.5:9877") {
		t.Errorf("error = %q, want to contain custom address 10.0.0.5:9877", err.Error())
	}
}

func TestRunDaytonaPreflightChecks_DefaultAdminAddr(t *testing.T) {
	// Skip if daytona is not installed
	if _, err := lookPathDaytona(); err != nil {
		t.Skip("daytona CLI not available")
	}

	// nil settings should use default 127.0.0.1:9877
	err := runDaytonaPreflightChecks(t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error (proxy not reachable)")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:9877") {
		t.Errorf("error = %q, want to contain default address 127.0.0.1:9877", err.Error())
	}
}

func TestRunDaytonaPreflightChecks_MissingCA(t *testing.T) {
	// Skip if daytona is not installed (CI environments)
	if _, err := lookPathDaytona(); err != nil {
		t.Skip("daytona CLI not available, skipping CA check test")
	}

	townRoot := t.TempDir()
	// Don't create CA files — should fail on CA check
	// Proxy check will also fail, but CA check should be reached via error ordering.
	// Since proxy check comes before CA check, this test validates proxy failure.
	err := runDaytonaPreflightChecks(townRoot, nil)
	if err == nil {
		t.Fatal("expected error when proxy is not running")
	}
	// Should fail on proxy check (comes before CA check)
	if want := "proxy server not reachable"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want to contain %q", err.Error(), want)
	}
}

func TestCheckCAFiles_MissingCert(t *testing.T) {
	townRoot := t.TempDir()
	caDir := filepath.Join(townRoot, ".runtime", "ca")
	if err := os.MkdirAll(caDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Only create key, not cert
	if err := os.WriteFile(filepath.Join(caDir, "ca.key"), []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}

	err := checkCAFiles(townRoot)
	if err == nil {
		t.Fatal("expected error when ca.crt is missing")
	}
	if !strings.Contains(err.Error(), "CA certificate not found") {
		t.Errorf("error = %q, want to contain 'CA certificate not found'", err.Error())
	}
}

func TestCheckCAFiles_MissingKey(t *testing.T) {
	townRoot := t.TempDir()
	caDir := filepath.Join(townRoot, ".runtime", "ca")
	if err := os.MkdirAll(caDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Only create cert, not key
	if err := os.WriteFile(filepath.Join(caDir, "ca.crt"), []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}

	err := checkCAFiles(townRoot)
	if err == nil {
		t.Fatal("expected error when ca.key is missing")
	}
	if !strings.Contains(err.Error(), "CA key not found") {
		t.Errorf("error = %q, want to contain 'CA key not found'", err.Error())
	}
}

func TestCheckCAFiles_BothPresent(t *testing.T) {
	townRoot := t.TempDir()
	caDir := filepath.Join(townRoot, ".runtime", "ca")
	if err := os.MkdirAll(caDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caDir, "ca.crt"), []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caDir, "ca.key"), []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := checkCAFiles(townRoot); err != nil {
		t.Errorf("expected no error when both CA files exist, got: %v", err)
	}
}

func TestCheckCAFiles_NoCaDir(t *testing.T) {
	townRoot := t.TempDir()
	// Don't create the CA directory at all
	err := checkCAFiles(townRoot)
	if err == nil {
		t.Fatal("expected error when CA directory does not exist")
	}
	if !strings.Contains(err.Error(), "CA certificate not found") {
		t.Errorf("error = %q, want to contain 'CA certificate not found'", err.Error())
	}
}

func TestShouldUseDaytona_ExplicitFlag(t *testing.T) {
	if !shouldUseDaytona(true, nil) {
		t.Error("expected true when explicit flag is set")
	}
}

func TestShouldUseDaytona_AutoDetectFromSettings(t *testing.T) {
	settings := &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{
			Provider: "daytona",
		},
	}
	if !shouldUseDaytona(false, settings) {
		t.Error("expected true when settings have daytona provider")
	}
}

func TestShouldUseDaytona_NilSettings(t *testing.T) {
	if shouldUseDaytona(false, nil) {
		t.Error("expected false for nil settings")
	}
}

func TestShouldUseDaytona_NonDaytonaProvider(t *testing.T) {
	settings := &config.RigSettings{
		RemoteBackend: &config.RemoteBackend{
			Provider: "other",
		},
	}
	if shouldUseDaytona(false, settings) {
		t.Error("expected false for non-daytona provider")
	}
}

func TestShouldUseDaytona_NilRemoteBackend(t *testing.T) {
	settings := &config.RigSettings{}
	if shouldUseDaytona(false, settings) {
		t.Error("expected false when RemoteBackend is nil")
	}
}

// lookPathDaytona checks if daytona is in PATH without modifying env.
func lookPathDaytona() (string, error) {
	return exec.LookPath("daytona")
}
