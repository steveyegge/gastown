package polecat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteReadyMarker(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	session := "rig001-polecat-alpha"

	if err := WriteReadyMarker(townRoot, session); err != nil {
		t.Fatalf("WriteReadyMarker() error = %v", err)
	}

	path := filepath.Join(townRoot, ".runtime", "ready", session+".ready")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("marker file not created: %v", err)
	}

	var marker readyMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatalf("marker file is not valid JSON: %v", err)
	}

	if marker.Timestamp.IsZero() {
		t.Error("marker timestamp should not be zero")
	}
	if time.Since(marker.Timestamp) > 5*time.Second {
		t.Errorf("marker timestamp too old: %v", marker.Timestamp)
	}
}

func TestReadyMarkerExists_Present(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	session := "rig001-polecat-alpha"

	if err := WriteReadyMarker(townRoot, session); err != nil {
		t.Fatalf("WriteReadyMarker() error = %v", err)
	}

	if !ReadyMarkerExists(townRoot, session) {
		t.Error("ReadyMarkerExists() = false, want true for freshly written marker")
	}
}

func TestReadyMarkerExists_Missing(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	if ReadyMarkerExists(townRoot, "nonexistent-session") {
		t.Error("ReadyMarkerExists() = true, want false for missing marker")
	}
}

func TestReadyMarkerExists_Stale(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	session := "rig001-polecat-alpha"

	// Write a marker with a timestamp far in the past
	dir := filepath.Join(townRoot, ".runtime", "ready")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	stale := readyMarker{Timestamp: time.Now().UTC().Add(-10 * time.Minute)}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(filepath.Join(dir, session+".ready"), data, 0644); err != nil {
		t.Fatal(err)
	}

	if ReadyMarkerExists(townRoot, session) {
		t.Error("ReadyMarkerExists() = true, want false for stale marker (>5min old)")
	}
}

func TestRemoveReadyMarker(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	session := "rig001-polecat-alpha"

	if err := WriteReadyMarker(townRoot, session); err != nil {
		t.Fatalf("WriteReadyMarker() error = %v", err)
	}

	RemoveReadyMarker(townRoot, session)

	if ReadyMarkerExists(townRoot, session) {
		t.Error("ReadyMarkerExists() = true after RemoveReadyMarker, want false")
	}
}

func TestRemoveReadyMarker_Missing(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	// Should not panic or error on missing marker
	RemoveReadyMarker(townRoot, "nonexistent-session")
}
