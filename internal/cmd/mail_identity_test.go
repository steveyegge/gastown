package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSenderFromCwdUsesAgentFileWitnessIdentity(t *testing.T) {
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	tmp := t.TempDir()
	witnessDir := filepath.Join(tmp, "x267", "witness")
	if err := os.MkdirAll(filepath.Join(witnessDir, "rig"), 0o755); err != nil {
		t.Fatalf("mkdir witness dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(witnessDir, ".gt-agent"),
		[]byte(`{"role":"witness","rig":"x267"}`),
		0o644,
	); err != nil {
		t.Fatalf("write .gt-agent: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(filepath.Join(witnessDir, "rig")); err != nil {
		t.Fatalf("chdir witness rig dir: %v", err)
	}

	got := detectSender()
	if got != "x267/witness" {
		t.Fatalf("detectSender() = %q, want %q", got, "x267/witness")
	}
}

func TestDetectSenderFromCwdUsesAgentFileRefineryIdentity(t *testing.T) {
	t.Setenv("GT_ROLE", "")
	t.Setenv("GT_RIG", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")

	tmp := t.TempDir()
	refineryDir := filepath.Join(tmp, "x267", "refinery")
	if err := os.MkdirAll(filepath.Join(refineryDir, "rig"), 0o755); err != nil {
		t.Fatalf("mkdir refinery dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(refineryDir, ".gt-agent"),
		[]byte(`{"role":"refinery","rig":"x267"}`),
		0o644,
	); err != nil {
		t.Fatalf("write .gt-agent: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(filepath.Join(refineryDir, "rig")); err != nil {
		t.Fatalf("chdir refinery rig dir: %v", err)
	}

	got := detectSender()
	if got != "x267/refinery" {
		t.Fatalf("detectSender() = %q, want %q", got, "x267/refinery")
	}
}
