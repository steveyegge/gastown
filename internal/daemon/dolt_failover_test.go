package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// startTCPListener starts a TCP listener on a random port and returns
// the "host:port" string and a cleanup function.
func startTCPListener(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func newFailoverTestManager(t *testing.T, config *DoltServerConfig) (*DoltServerManager, string) {
	t.Helper()
	tmpDir := t.TempDir()
	daemonDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if config == nil {
		config = DefaultDoltServerConfig(tmpDir)
	}
	m := &DoltServerManager{
		config:   config,
		townRoot: tmpDir,
		logger:   func(format string, v ...interface{}) { t.Logf(format, v...) },
	}
	return m, tmpDir
}

func TestFailover_SwitchesToFallbackOnPrimaryFailure(t *testing.T) {
	// Start a "fallback" TCP listener to simulate a reachable fallback host.
	fallbackAddr, cleanup := startTCPListener(t)
	defer cleanup()

	m, _ := newFailoverTestManager(t, nil)
	m.config.Enabled = true
	m.config.External = true
	m.config.Host = "192.0.2.1" // unreachable (TEST-NET)
	m.config.Port = 9999
	m.config.FallbackHosts = []string{fallbackAddr}

	err := m.tryFailover()
	if err != nil {
		t.Fatalf("expected failover to succeed, got: %v", err)
	}

	if !m.inFailover {
		t.Error("expected inFailover to be true")
	}
	if m.primaryHost != "192.0.2.1" {
		t.Errorf("expected primaryHost=192.0.2.1, got %s", m.primaryHost)
	}

	// Config should now point to the fallback.
	activeAddr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)
	if activeAddr != fallbackAddr {
		t.Errorf("expected active host=%s, got %s", fallbackAddr, activeAddr)
	}
}

func TestFailover_NoFallbackHosts(t *testing.T) {
	m, _ := newFailoverTestManager(t, nil)
	m.config.FallbackHosts = nil

	err := m.tryFailover()
	if err == nil {
		t.Error("expected error when no fallback hosts configured")
	}
}

func TestFailover_AllFallbacksUnreachable(t *testing.T) {
	m, _ := newFailoverTestManager(t, nil)
	m.config.Host = "192.0.2.1"
	m.config.Port = 9999
	m.config.FallbackHosts = []string{"192.0.2.2:9998", "192.0.2.3:9997"}

	err := m.tryFailover()
	if err == nil {
		t.Error("expected error when all fallbacks unreachable")
	}
	if m.inFailover {
		t.Error("should not be in failover when all fallbacks failed")
	}
}

func TestFailback_RevertsWhenPrimaryRecovers(t *testing.T) {
	// Start a "primary" listener.
	primaryAddr, cleanup := startTCPListener(t)
	defer cleanup()

	primaryHost, primaryPortStr, _ := net.SplitHostPort(primaryAddr)
	var primaryPort int
	fmt.Sscanf(primaryPortStr, "%d", &primaryPort)

	m, _ := newFailoverTestManager(t, nil)
	// Simulate being in failover already.
	m.config.Host = "10.0.0.99"
	m.config.Port = 3307
	m.primaryHost = primaryHost
	m.primaryPort = primaryPort
	m.inFailover = true

	reverted := m.tryFailback()
	if !reverted {
		t.Fatal("expected failback to succeed")
	}
	if m.inFailover {
		t.Error("expected inFailover=false after failback")
	}
	if m.config.Host != primaryHost {
		t.Errorf("expected host reverted to %s, got %s", primaryHost, m.config.Host)
	}
	if m.config.Port != primaryPort {
		t.Errorf("expected port reverted to %d, got %d", primaryPort, m.config.Port)
	}
}

func TestFailback_NoRevertWhenPrimaryStillDown(t *testing.T) {
	m, _ := newFailoverTestManager(t, nil)
	m.config.Host = "10.0.0.99"
	m.config.Port = 3307
	m.primaryHost = "192.0.2.1"
	m.primaryPort = 9999
	m.inFailover = true

	reverted := m.tryFailback()
	if reverted {
		t.Error("should not revert when primary is still unreachable")
	}
	if !m.inFailover {
		t.Error("should still be in failover")
	}
}

func TestFailoverState_WrittenAndReadable(t *testing.T) {
	m, tmpDir := newFailoverTestManager(t, nil)
	m.config.Host = "10.0.0.50"
	m.config.Port = 3307
	m.primaryHost = "100.111.197.110"
	m.primaryPort = 3307
	m.inFailover = true
	m.nowFn = func() time.Time { return time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC) }

	m.writeFailoverState()

	// Verify the file exists and is parseable.
	state := ReadFailoverState(tmpDir)
	if state == nil {
		t.Fatal("expected failover state to be readable")
	}
	if state.ActiveHost != "10.0.0.50" {
		t.Errorf("expected active host 10.0.0.50, got %s", state.ActiveHost)
	}
	if state.PrimaryHost != "100.111.197.110" {
		t.Errorf("expected primary host 100.111.197.110, got %s", state.PrimaryHost)
	}
	if !state.InFailover {
		t.Error("expected InFailover=true")
	}
}

func TestFailoverState_ClearedOnFailback(t *testing.T) {
	m, tmpDir := newFailoverTestManager(t, nil)
	// Write a state file.
	m.inFailover = true
	m.primaryHost = "10.0.0.1"
	m.primaryPort = 3307
	m.writeFailoverState()

	// Clear it.
	m.clearFailoverState()

	state := ReadFailoverState(tmpDir)
	if state != nil {
		t.Error("expected nil state after clear")
	}
}

func TestReadFailoverState_NoFile(t *testing.T) {
	state := ReadFailoverState(t.TempDir())
	if state != nil {
		t.Error("expected nil state when file doesn't exist")
	}
}

func TestReadFailoverState_NotInFailover(t *testing.T) {
	tmpDir := t.TempDir()
	daemonDir := filepath.Join(tmpDir, "daemon")
	os.MkdirAll(daemonDir, 0755)

	// Write a state file with InFailover=false.
	state := failoverState{
		ActiveHost:  "10.0.0.1",
		ActivePort:  3307,
		PrimaryHost: "100.111.197.110",
		PrimaryPort: 3307,
		InFailover:  false,
	}
	data, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(daemonDir, "dolt-failover-state.json"), data, 0644)

	result := ReadFailoverState(tmpDir)
	if result != nil {
		t.Error("expected nil when InFailover=false")
	}
}

func TestFailover_SkipsPrimaryAndCurrentInFallbackList(t *testing.T) {
	// The fallback list might include the primary and current host.
	// tryFailover should skip both.
	fallbackAddr, cleanup := startTCPListener(t)
	defer cleanup()

	m, _ := newFailoverTestManager(t, nil)
	m.config.Host = "192.0.2.1"
	m.config.Port = 9999
	m.config.FallbackHosts = []string{
		"192.0.2.1:9999", // same as primary — should skip
		fallbackAddr,      // this should be picked
	}

	err := m.tryFailover()
	if err != nil {
		t.Fatalf("expected failover to succeed: %v", err)
	}

	activeAddr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)
	if activeAddr != fallbackAddr {
		t.Errorf("expected %s, got %s", fallbackAddr, activeAddr)
	}
}

func TestEnsureRunning_ExternalFailoverIntegration(t *testing.T) {
	// Integration test: EnsureRunning in external mode should failover
	// when the primary is unreachable and a fallback is available.
	fallbackAddr, cleanup := startTCPListener(t)
	defer cleanup()

	m, _ := newFailoverTestManager(t, nil)
	m.config.Enabled = true
	m.config.External = true
	m.config.Host = "192.0.2.1" // unreachable
	m.config.Port = 9999
	m.config.FallbackHosts = []string{fallbackAddr}

	// Health check should fail on primary, succeed on fallback (TCP-level).
	// But checkHealthLocked runs dolt sql, not just TCP. We need to mock it.
	callCount := 0
	m.healthCheckFn = func() error {
		callCount++
		addr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)
		if addr == "192.0.2.1:9999" {
			return fmt.Errorf("connection refused")
		}
		return nil // fallback is "healthy"
	}

	err := m.EnsureRunning()
	if err != nil {
		t.Fatalf("expected EnsureRunning to succeed after failover: %v", err)
	}
	if !m.inFailover {
		t.Error("expected to be in failover state")
	}
	activeAddr := fmt.Sprintf("%s:%d", m.config.Host, m.config.Port)
	if activeAddr != fallbackAddr {
		t.Errorf("expected active=%s, got %s", fallbackAddr, activeAddr)
	}
}

func TestEnsureRunning_ExternalFailbackIntegration(t *testing.T) {
	// When already in failover, EnsureRunning should try failback first.
	primaryAddr, cleanupPrimary := startTCPListener(t)
	defer cleanupPrimary()

	primaryHost, primaryPortStr, _ := net.SplitHostPort(primaryAddr)
	var primaryPort int
	fmt.Sscanf(primaryPortStr, "%d", &primaryPort)

	m, tmpDir := newFailoverTestManager(t, nil)
	m.config.Enabled = true
	m.config.External = true
	m.config.Host = "10.0.0.99"
	m.config.Port = 3307
	m.config.FallbackHosts = []string{"10.0.0.99:3307"}
	m.primaryHost = primaryHost
	m.primaryPort = primaryPort
	m.inFailover = true
	// Write initial failover state.
	m.writeFailoverState()

	m.healthCheckFn = func() error { return nil }

	err := m.EnsureRunning()
	if err != nil {
		t.Fatalf("expected EnsureRunning to succeed: %v", err)
	}
	if m.inFailover {
		t.Error("expected failback to have occurred")
	}
	if m.config.Host != primaryHost {
		t.Errorf("expected host=%s after failback, got %s", primaryHost, m.config.Host)
	}

	// State file should be cleared.
	state := ReadFailoverState(tmpDir)
	if state != nil {
		t.Error("expected failover state file to be cleared after failback")
	}
}
