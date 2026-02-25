package doltserver

import (
	"net"
	"testing"
)

func TestCheckPortAvailable_Free(t *testing.T) {
	// Use port 0 to get an ephemeral port that's guaranteed free
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Port should be free now
	if err := checkPortAvailable(port); err != nil {
		t.Errorf("checkPortAvailable(%d) returned error for free port: %v", port, err)
	}
}

func TestCheckPortAvailable_InUse(t *testing.T) {
	// Bind a port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	// Should fail since port is in use
	if err := checkPortAvailable(port); err == nil {
		t.Errorf("checkPortAvailable(%d) returned nil for in-use port", port)
	}
}
