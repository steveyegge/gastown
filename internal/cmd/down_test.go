package cmd

import (
	"os"
	"testing"
)

func TestIsProcessRunning_CurrentProcess(t *testing.T) {
	if !isProcessRunning(os.Getpid()) {
		t.Error("current process should be detected as running")
	}
}

func TestIsProcessRunning_InvalidPID(t *testing.T) {
	if isProcessRunning(99999999) {
		t.Error("invalid PID should not be detected as running")
	}
}

func TestIsProcessRunning_MaxPID(t *testing.T) {
	if isProcessRunning(2147483647) {
		t.Error("max PID should not be running")
	}
}
