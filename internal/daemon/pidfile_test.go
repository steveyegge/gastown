package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestWriteAndReadPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	nonce, err := writePIDFile(pidFile, 12345)
	if err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}
	if nonce == "" {
		t.Fatal("nonce should not be empty")
	}

	pid, readNonce, err := readPIDFile(pidFile)
	if err != nil {
		t.Fatalf("readPIDFile: %v", err)
	}
	if pid != 12345 {
		t.Errorf("expected PID 12345, got %d", pid)
	}
	if readNonce != nonce {
		t.Errorf("nonce mismatch: wrote %q, read %q", nonce, readNonce)
	}
}

func TestReadPIDFile_Legacy(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	// Write legacy format (PID only, no nonce)
	if err := os.WriteFile(pidFile, []byte("54321"), 0644); err != nil {
		t.Fatal(err)
	}

	pid, nonce, err := readPIDFile(pidFile)
	if err != nil {
		t.Fatalf("readPIDFile: %v", err)
	}
	if pid != 54321 {
		t.Errorf("expected PID 54321, got %d", pid)
	}
	if nonce != "" {
		t.Errorf("expected empty nonce for legacy format, got %q", nonce)
	}
}

func TestReadPIDFile_NotFound(t *testing.T) {
	_, _, err := readPIDFile("/nonexistent/path/test.pid")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadPIDFile_Invalid(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	if err := os.WriteFile(pidFile, []byte("notanumber"), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := readPIDFile(pidFile)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

func TestVerifyPIDOwnership_CurrentProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	// Write our own PID
	_, err := writePIDFile(pidFile, os.Getpid())
	if err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	pid, alive, err := verifyPIDOwnership(pidFile)
	if err != nil {
		t.Fatalf("verifyPIDOwnership: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), pid)
	}
	if !alive {
		t.Error("expected current process to be alive")
	}
}

func TestVerifyPIDOwnership_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	// Use a PID that's almost certainly not running (very high number)
	// Note: on some systems, max PID is 32768 or 4194304
	deadPID := 4194300
	if _, err := writePIDFile(pidFile, deadPID); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	pid, alive, err := verifyPIDOwnership(pidFile)
	if err != nil {
		t.Fatalf("verifyPIDOwnership: %v", err)
	}
	if pid != deadPID {
		t.Errorf("expected PID %d, got %d", deadPID, pid)
	}
	if alive {
		t.Error("expected dead process to not be alive")
	}
}

func TestVerifyPIDOwnership_NoFile(t *testing.T) {
	pid, alive, err := verifyPIDOwnership("/nonexistent/test.pid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid != 0 || alive {
		t.Errorf("expected pid=0 alive=false, got pid=%d alive=%v", pid, alive)
	}
}

func TestGenerateNonce_Unique(t *testing.T) {
	n1, err := generateNonce()
	if err != nil {
		t.Fatal(err)
	}
	n2, err := generateNonce()
	if err != nil {
		t.Fatal(err)
	}
	if n1 == n2 {
		t.Error("two nonces should not be equal")
	}
	if len(n1) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("expected 16 hex chars, got %d", len(n1))
	}
}

func TestWritePIDFile_Format(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	nonce, err := writePIDFile(pidFile, 99999)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}

	expected := "99999\n" + nonce
	if string(data) != expected {
		t.Errorf("file content mismatch:\ngot:  %q\nwant: %q", string(data), expected)
	}

	// Verify it's parseable by the legacy reader too (first line is the PID)
	firstLine := strconv.Itoa(99999)
	if !containsLine(string(data), firstLine) {
		t.Error("PID should be the first line for legacy compatibility")
	}
}

func containsLine(content, line string) bool {
	for _, l := range splitLines(content) {
		if l == line {
			return true
		}
	}
	return false
}
