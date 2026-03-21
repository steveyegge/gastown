package daemon

import (
	"net"
	"strconv"
	"testing"
	"time"
)

func TestInitFailoverHosts_NoDuplicates(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host: "10.0.0.1",
			Port: 3307,
			FallbackHosts: []string{
				"10.0.0.2:3307",
				"10.0.0.1:3307", // duplicate of primary
				"10.0.0.3:3307",
			},
		},
	}
	m.initFailoverHosts()

	if len(m.allHosts) != 3 {
		t.Fatalf("expected 3 hosts (primary + 2 unique fallbacks), got %d: %v", len(m.allHosts), m.allHosts)
	}
	if m.allHosts[0] != "10.0.0.1:3307" {
		t.Errorf("expected primary 10.0.0.1:3307, got %s", m.allHosts[0])
	}
}

func TestInitFailoverHosts_BareHostGetsPort(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host:          "10.0.0.1",
			Port:          3307,
			FallbackHosts: []string{"10.0.0.2"}, // no port
		},
	}
	m.initFailoverHosts()

	if len(m.allHosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d: %v", len(m.allHosts), m.allHosts)
	}
	if m.allHosts[1] != "10.0.0.2:3307" {
		t.Errorf("expected fallback to get default port, got %s", m.allHosts[1])
	}
}

func TestInitFailoverHosts_Empty(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host: "127.0.0.1",
			Port: 3307,
		},
	}
	m.initFailoverHosts()

	if len(m.allHosts) != 1 {
		t.Fatalf("expected 1 host (primary only), got %d: %v", len(m.allHosts), m.allHosts)
	}
}

func TestActiveHostAndPort(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host: "10.0.0.1",
			Port: 3307,
			FallbackHosts: []string{
				"10.0.0.2:3308",
			},
		},
	}
	m.initFailoverHosts()

	host, port := m.activeHostAndPort()
	if host != "10.0.0.1" || port != 3307 {
		t.Errorf("expected 10.0.0.1:3307, got %s:%d", host, port)
	}

	// Switch to fallback
	m.activeHostIdx = 1
	host, port = m.activeHostAndPort()
	if host != "10.0.0.2" || port != 3308 {
		t.Errorf("expected 10.0.0.2:3308, got %s:%d", host, port)
	}
}

func TestTryFailover_NoFallbacks(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host: "10.0.0.1",
			Port: 3307,
		},
		logger: func(format string, v ...interface{}) {},
	}
	m.initFailoverHosts()

	if m.tryFailover() {
		t.Error("tryFailover should return false with no fallbacks")
	}
}

func TestTryFailover_WithLiveServer(t *testing.T) {
	// Start a TCP listener to simulate a live fallback server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}
	defer ln.Close()

	// Accept connections in background so probeHost succeeds.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host: "10.0.0.99", // unreachable primary
			Port: 3307,
			FallbackHosts: []string{
				"127.0.0.1:" + portStr, // live fallback
			},
		},
		logger: func(format string, v ...interface{}) {},
	}
	m.initFailoverHosts()

	if !m.tryFailover() {
		t.Fatal("tryFailover should succeed when a fallback is reachable")
	}
	if m.activeHostIdx != 1 {
		t.Errorf("expected activeHostIdx=1, got %d", m.activeHostIdx)
	}
	host, gotPort := m.activeHostAndPort()
	if host != "127.0.0.1" || gotPort != port {
		t.Errorf("expected 127.0.0.1:%d, got %s:%d", port, host, gotPort)
	}
}

func TestTryFailback_ProbesAndSwitches(t *testing.T) {
	// Start a TCP listener to simulate recovered primary.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())

	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host:                  "127.0.0.1",
			Port:                  0, // will be overridden
			FallbackHosts:         []string{"10.0.0.2:3307"},
			FailbackProbeInterval: 0, // probe immediately
		},
		logger: func(format string, v ...interface{}) {},
	}
	// Manually set allHosts to use the test listener port as primary.
	m.allHosts = []string{"127.0.0.1:" + portStr, "10.0.0.2:3307"}
	m.activeHostIdx = 1 // on fallback

	if !m.tryFailback() {
		t.Fatal("tryFailback should succeed when primary is reachable")
	}
	if m.activeHostIdx != 0 {
		t.Errorf("expected activeHostIdx=0 after failback, got %d", m.activeHostIdx)
	}
}

func TestTryFailback_RateLimited(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host:                  "127.0.0.1",
			Port:                  3307,
			FallbackHosts:         []string{"10.0.0.2:3307"},
			FailbackProbeInterval: 60 * time.Second,
		},
		logger:       func(format string, v ...interface{}) {},
		lastFailback: time.Now(), // just probed
	}
	m.initFailoverHosts()
	m.activeHostIdx = 1

	if m.tryFailback() {
		t.Error("tryFailback should be rate-limited and return false")
	}
}

func TestIsOnFallback(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Host:          "10.0.0.1",
			Port:          3307,
			FallbackHosts: []string{"10.0.0.2:3307"},
		},
		logger: func(format string, v ...interface{}) {},
	}
	m.initFailoverHosts()

	if m.IsOnFallback() {
		t.Error("should not be on fallback initially")
	}
	m.mu.Lock()
	m.activeHostIdx = 1
	m.mu.Unlock()
	if !m.IsOnFallback() {
		t.Error("should be on fallback after switching")
	}
}
