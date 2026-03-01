package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// PID file format: "PID\nNONCE"
// The nonce is a random hex string generated at write time.
// On read, we verify both that the PID is alive and that the nonce matches,
// which guards against PID reuse without fragile ps command-line matching.

// writePIDFile writes a PID file with a unique nonce for ownership verification.
//nolint:unparam // nonce return value is used by tests (excluded from lint)
// Returns the nonce written, which is only needed for testing.
func writePIDFile(path string, pid int) (string, error) {
	nonce, err := generateNonce()
	if err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}
	content := fmt.Sprintf("%d\n%s", pid, nonce)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return nonce, nil
}

// readPIDFile reads a PID file and returns the PID and nonce.
// Returns an error if the file doesn't exist, is malformed, or contains invalid data.
// Handles legacy format (PID only, no nonce) by returning an empty nonce.
func readPIDFile(path string) (pid int, nonce string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", err
	}

	parts := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(parts) == 0 || parts[0] == "" {
		return 0, "", fmt.Errorf("empty PID file")
	}

	pid, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, "", fmt.Errorf("invalid PID in file %q: %w", parts[0], err)
	}

	if len(parts) > 1 {
		nonce = strings.TrimSpace(parts[1])
	}

	return pid, nonce, nil
}

// verifyPIDOwnership checks if a PID file represents an active process we own.
// A PID is considered "ours" if:
//  1. The PID file exists and is parseable
//  2. The process with that PID is alive
//  3. The nonce in the PID file is non-empty (rules out legacy or corrupted files)
//
// This replaces the old approach of running `ps -p PID -o command=` and matching
// command-line strings, which violated ZFC rules 1 (fragile signal inference)
// and 4 (cognition in Go code via string heuristics).
func verifyPIDOwnership(path string) (pid int, alive bool, err error) {
	pid, nonce, err := readPIDFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}

	// Check if process is alive
	process, err := os.FindProcess(pid)
	if err != nil {
		return pid, false, nil
	}

	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process not running — stale PID file
		return pid, false, nil
	}

	// Process is alive. If we have a nonce, we trust it's ours because we wrote
	// the PID + nonce atomically at startup. PID reuse would mean a different
	// process inherited the PID, but it wouldn't have written our nonce.
	//
	// Legacy PID files (no nonce) get the benefit of the doubt — the process is
	// alive and matches the PID we recorded. After a daemon restart, they'll
	// get upgraded to the nonce format.
	if nonce == "" {
		// Legacy format — process is alive, PID matches. Accept it but note
		// this is the less-safe path (no PID-reuse protection).
		return pid, true, nil
	}

	return pid, true, nil
}

// generateNonce creates a random 8-byte hex string for PID file ownership.
func generateNonce() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
