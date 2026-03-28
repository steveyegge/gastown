package cmd

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunCrewStatus_NoArgsAggregatesJSONAcrossAllRigs(t *testing.T) {
	townRoot := setupTestTownForCrewList(t, map[string][]string{
		"rig-a": {"alice"},
		"rig-b": {"bob"},
	})

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	crewRig = ""
	crewJSON = true
	defer func() {
		crewRig = ""
		crewJSON = false
	}()

	output := captureStdout(t, func() {
		if err := runCrewStatus(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runCrewStatus failed: %v", err)
		}
	})

	var items []CrewStatusItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 crew workers, got %d", len(items))
	}

	rigs := map[string]bool{}
	for _, item := range items {
		rigs[item.Rig] = true
	}
	if !rigs["rig-a"] || !rigs["rig-b"] {
		t.Fatalf("expected crew from rig-a and rig-b, got: %#v", rigs)
	}
}

func TestRunCrewStatus_RigFlagFiltersJSONFromTownRoot(t *testing.T) {
	townRoot := setupTestTownForCrewList(t, map[string][]string{
		"rig-a": {"alice"},
		"rig-b": {"bob"},
	})

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	crewRig = "rig-b"
	crewJSON = true
	defer func() {
		crewRig = ""
		crewJSON = false
	}()

	output := captureStdout(t, func() {
		if err := runCrewStatus(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runCrewStatus failed: %v", err)
		}
	})

	var items []CrewStatusItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 crew worker, got %d", len(items))
	}
	if items[0].Rig != "rig-b" {
		t.Fatalf("expected rig-b, got %s", items[0].Rig)
	}
	if items[0].Name != "bob" {
		t.Fatalf("expected bob, got %s", items[0].Name)
	}
}
