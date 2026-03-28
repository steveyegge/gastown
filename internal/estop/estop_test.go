package estop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivateAndRead(t *testing.T) {
	townRoot := t.TempDir()

	if IsActive(townRoot) {
		t.Fatal("should not be active before activation")
	}

	if err := Activate(townRoot, TriggerManual, "test reason"); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if !IsActive(townRoot) {
		t.Fatal("should be active after activation")
	}

	info := Read(townRoot)
	if info == nil {
		t.Fatal("Read returned nil")
	}
	if info.Trigger != TriggerManual {
		t.Errorf("trigger = %q, want %q", info.Trigger, TriggerManual)
	}
	if info.Reason != "test reason" {
		t.Errorf("reason = %q, want %q", info.Reason, "test reason")
	}
}

func TestDeactivate(t *testing.T) {
	townRoot := t.TempDir()

	if err := Activate(townRoot, TriggerManual, ""); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if err := Deactivate(townRoot, false); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	if IsActive(townRoot) {
		t.Fatal("should not be active after deactivation")
	}
}

func TestDeactivateOnlyAutoSkipsManual(t *testing.T) {
	townRoot := t.TempDir()

	if err := Activate(townRoot, TriggerManual, "human triggered"); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	err := Deactivate(townRoot, true)
	if err == nil {
		t.Fatal("Deactivate(onlyAuto=true) should fail for manual E-stop")
	}

	if !IsActive(townRoot) {
		t.Fatal("manual E-stop should still be active")
	}
}

func TestDeactivateOnlyAutoRemovesAuto(t *testing.T) {
	townRoot := t.TempDir()

	if err := Activate(townRoot, TriggerAuto, "dolt-unreachable"); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if err := Deactivate(townRoot, true); err != nil {
		t.Fatalf("Deactivate(onlyAuto=true): %v", err)
	}

	if IsActive(townRoot) {
		t.Fatal("auto E-stop should be removed")
	}
}

func TestFilePath(t *testing.T) {
	got := FilePath("/tmp/mytown")
	want := filepath.Join("/tmp/mytown", FileName)
	if got != want {
		t.Errorf("FilePath = %q, want %q", got, want)
	}
}

func TestReadNonExistent(t *testing.T) {
	info := Read(t.TempDir())
	if info != nil {
		t.Error("Read should return nil for non-existent file")
	}
}

func TestPerRigActivateAndRead(t *testing.T) {
	townRoot := t.TempDir()

	if IsRigActive(townRoot, "gastown") {
		t.Fatal("rig should not be active before activation")
	}

	if err := ActivateRig(townRoot, "gastown", TriggerManual, "closing laptop"); err != nil {
		t.Fatalf("ActivateRig: %v", err)
	}

	if !IsRigActive(townRoot, "gastown") {
		t.Fatal("gastown should be active after activation")
	}
	if IsRigActive(townRoot, "beads") {
		t.Fatal("beads should not be active")
	}
	// Town-wide should not be active
	if IsActive(townRoot) {
		t.Fatal("town-wide should not be active from per-rig activation")
	}

	info := ReadRig(townRoot, "gastown")
	if info == nil {
		t.Fatal("ReadRig returned nil")
	}
	if info.Reason != "closing laptop" {
		t.Errorf("reason = %q, want %q", info.Reason, "closing laptop")
	}
}

func TestIsAnyActive(t *testing.T) {
	townRoot := t.TempDir()

	if IsAnyActive(townRoot, "gastown") {
		t.Fatal("nothing should be active")
	}

	// Per-rig activation
	if err := ActivateRig(townRoot, "gastown", TriggerManual, ""); err != nil {
		t.Fatal(err)
	}
	if !IsAnyActive(townRoot, "gastown") {
		t.Fatal("gastown should be active via per-rig")
	}
	if IsAnyActive(townRoot, "beads") {
		t.Fatal("beads should not be affected by gastown per-rig")
	}

	// Clean up and test town-wide
	_ = DeactivateRig(townRoot, "gastown")
	if err := Activate(townRoot, TriggerManual, ""); err != nil {
		t.Fatal(err)
	}
	if !IsAnyActive(townRoot, "gastown") {
		t.Fatal("gastown should be active via town-wide")
	}
	if !IsAnyActive(townRoot, "beads") {
		t.Fatal("beads should be active via town-wide")
	}
}

func TestPerRigDeactivate(t *testing.T) {
	townRoot := t.TempDir()
	if err := ActivateRig(townRoot, "gastown", TriggerManual, ""); err != nil {
		t.Fatal(err)
	}
	if err := DeactivateRig(townRoot, "gastown"); err != nil {
		t.Fatal(err)
	}
	if IsRigActive(townRoot, "gastown") {
		t.Fatal("gastown should not be active after deactivation")
	}
}

func TestParseBareFile(t *testing.T) {
	townRoot := t.TempDir()
	// Simulate a bare touch (no content)
	if err := os.WriteFile(FilePath(townRoot), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	info := Read(townRoot)
	if info == nil {
		t.Fatal("Read should handle empty file")
	}
	if info.Trigger != TriggerManual {
		t.Errorf("bare file trigger = %q, want %q", info.Trigger, TriggerManual)
	}
}

func TestDeactivateNonExistent(t *testing.T) {
	townRoot := t.TempDir()
	// Should not error on non-existent file
	if err := Deactivate(townRoot, false); err != nil {
		t.Fatalf("Deactivate non-existent: %v", err)
	}
}
