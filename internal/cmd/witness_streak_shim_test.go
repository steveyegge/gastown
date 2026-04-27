package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempStreakShim sets up a fake town root + bd-with-streak.sh source
// script in a temp dir, points the install dir at a separate temp dir, and
// returns the (source, target) the install command will resolve. Restores
// global flags + cwd on cleanup.
func withTempStreakShim(t *testing.T) (source, targetDir string) {
	t.Helper()

	town := t.TempDir()
	scriptDir := filepath.Join(town, "occultfusion", "witness", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir script dir: %v", err)
	}
	source = filepath.Join(scriptDir, "bd-with-streak.sh")
	if err := os.WriteFile(source, []byte("#!/bin/sh\necho noop\n"), 0o755); err != nil {
		t.Fatalf("write source: %v", err)
	}
	// Mark the temp dir as a Gas Town workspace via the mayor/town.json
	// PrimaryMarker that workspace.Find recognizes.
	mayorDir := filepath.Join(town, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	targetDir = t.TempDir()

	prevCwd, _ := os.Getwd()
	if err := os.Chdir(town); err != nil {
		t.Fatalf("chdir town: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevCwd) })

	prevDir := streakShimInstallDir
	prevDry := streakShimDryRun
	streakShimInstallDir = targetDir
	streakShimDryRun = false
	t.Cleanup(func() {
		streakShimInstallDir = prevDir
		streakShimDryRun = prevDry
	})

	return source, targetDir
}

func TestInstallStreakShimCreatesSymlink(t *testing.T) {
	source, targetDir := withTempStreakShim(t)

	if err := runWitnessInstallStreakShim(nil, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	target := filepath.Join(targetDir, streakShimName)
	got, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink %s: %v", target, err)
	}
	if got != source {
		t.Errorf("symlink target = %q, want %q", got, source)
	}
}

func TestInstallStreakShimIsIdempotent(t *testing.T) {
	source, targetDir := withTempStreakShim(t)

	if err := runWitnessInstallStreakShim(nil, nil); err != nil {
		t.Fatalf("first install: %v", err)
	}
	// Second invocation — same args — should be a no-op (no error,
	// symlink unchanged).
	if err := runWitnessInstallStreakShim(nil, nil); err != nil {
		t.Errorf("second install (idempotency): %v", err)
	}
	got, err := os.Readlink(filepath.Join(targetDir, streakShimName))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if got != source {
		t.Errorf("symlink target after re-install = %q, want %q", got, source)
	}
}

func TestInstallStreakShimRefusesToClobberRegularFile(t *testing.T) {
	_, targetDir := withTempStreakShim(t)

	target := filepath.Join(targetDir, streakShimName)
	if err := os.WriteFile(target, []byte("preexisting\n"), 0o755); err != nil {
		t.Fatalf("seed regular file: %v", err)
	}

	err := runWitnessInstallStreakShim(nil, nil)
	if err == nil {
		t.Fatal("expected error when target is a regular file, got nil")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Errorf("error should explain non-symlink collision; got: %v", err)
	}
	// File contents must be untouched (fail-soft).
	got, _ := os.ReadFile(target)
	if string(got) != "preexisting\n" {
		t.Errorf("regular file was clobbered; contents = %q", got)
	}
}

func TestInstallStreakShimRefusesToClobberDifferentSymlinkTarget(t *testing.T) {
	_, targetDir := withTempStreakShim(t)

	target := filepath.Join(targetDir, streakShimName)
	otherSrc := filepath.Join(t.TempDir(), "other.sh")
	if err := os.WriteFile(otherSrc, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed other src: %v", err)
	}
	if err := os.Symlink(otherSrc, target); err != nil {
		t.Fatalf("seed symlink: %v", err)
	}

	err := runWitnessInstallStreakShim(nil, nil)
	if err == nil {
		t.Fatal("expected error when symlink points at a different source, got nil")
	}
	if !strings.Contains(err.Error(), "uninstall-streak-shim") {
		t.Errorf("error should suggest uninstall; got: %v", err)
	}
	// Existing symlink target must be unchanged.
	got, _ := os.Readlink(target)
	if got != otherSrc {
		t.Errorf("existing symlink was clobbered; target = %q want %q", got, otherSrc)
	}
}

func TestInstallStreakShimDryRunMakesNoFilesystemChange(t *testing.T) {
	_, targetDir := withTempStreakShim(t)

	streakShimDryRun = true
	defer func() { streakShimDryRun = false }()

	if err := runWitnessInstallStreakShim(nil, nil); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	target := filepath.Join(targetDir, streakShimName)
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("dry-run created %s (err=%v); should be no-op", target, err)
	}
}

func TestInstallStreakShimRejectsMissingSourceScript(t *testing.T) {
	source, _ := withTempStreakShim(t)

	if err := os.Remove(source); err != nil {
		t.Fatalf("remove source: %v", err)
	}

	err := runWitnessInstallStreakShim(nil, nil)
	if err == nil {
		t.Fatal("expected error when source script is missing, got nil")
	}
	if !strings.Contains(err.Error(), "source script not found") {
		t.Errorf("error should explain missing source; got: %v", err)
	}
}

func TestInstallStreakShimRejectsNonExecutableSource(t *testing.T) {
	source, _ := withTempStreakShim(t)
	if err := os.Chmod(source, 0o644); err != nil {
		t.Fatalf("chmod -x: %v", err)
	}

	err := runWitnessInstallStreakShim(nil, nil)
	if err == nil {
		t.Fatal("expected error when source script is not executable, got nil")
	}
	if !strings.Contains(err.Error(), "not executable") {
		t.Errorf("error should explain executable bit; got: %v", err)
	}
}

func TestUninstallStreakShimRemovesOwnSymlink(t *testing.T) {
	_, targetDir := withTempStreakShim(t)

	if err := runWitnessInstallStreakShim(nil, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := runWitnessUninstallStreakShim(nil, nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	target := filepath.Join(targetDir, streakShimName)
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("uninstall left %s in place (err=%v)", target, err)
	}
}

func TestUninstallStreakShimNoOpWhenAbsent(t *testing.T) {
	_, _ = withTempStreakShim(t)

	if err := runWitnessUninstallStreakShim(nil, nil); err != nil {
		t.Errorf("uninstall on absent shim should be no-op; got: %v", err)
	}
}

func TestUninstallStreakShimRefusesForeignSymlink(t *testing.T) {
	_, targetDir := withTempStreakShim(t)

	target := filepath.Join(targetDir, streakShimName)
	otherSrc := filepath.Join(t.TempDir(), "other.sh")
	if err := os.WriteFile(otherSrc, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("seed other src: %v", err)
	}
	if err := os.Symlink(otherSrc, target); err != nil {
		t.Fatalf("seed symlink: %v", err)
	}

	err := runWitnessUninstallStreakShim(nil, nil)
	if err == nil {
		t.Fatal("expected error when symlink is foreign, got nil")
	}
	if !strings.Contains(err.Error(), "not installed by this command") {
		t.Errorf("error should explain ownership refusal; got: %v", err)
	}
	got, _ := os.Readlink(target)
	if got != otherSrc {
		t.Errorf("foreign symlink was clobbered; target = %q want %q", got, otherSrc)
	}
}
