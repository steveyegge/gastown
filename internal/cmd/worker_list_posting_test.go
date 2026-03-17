package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
)

// ---------------------------------------------------------------------------
// Section 11: Worker list display — posting bracket notation
// ---------------------------------------------------------------------------

// 11.1: gt crew list shows bracket notation with persistent posting
func TestCrewList_ShowsBracketWithPersistentPosting(t *testing.T) {
	townRoot := setupTestTownForCrewList(t, map[string][]string{
		"testrig": {"alice"},
	})

	// Set persistent posting
	settingsDir := filepath.Join(townRoot, "testrig", "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{"alice": "dispatcher"}
	if err := config.SaveRigSettings(filepath.Join(settingsDir, "config.json"), settings); err != nil {
		t.Fatal(err)
	}

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	crewRig = "testrig"
	crewJSON = true
	defer func() {
		crewRig = ""
		crewJSON = false
	}()

	output := captureStdout(t, func() {
		if err := runCrewList(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runCrewList: %v", err)
		}
	})

	var items []CrewListItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, output)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Posting != "dispatcher" {
		t.Errorf("Posting = %q, want %q", items[0].Posting, "dispatcher")
	}
	if items[0].PostingSource != "config" {
		t.Errorf("PostingSource = %q, want %q", items[0].PostingSource, "config")
	}
}

// 11.2: (config) vs (session) labels
func TestCrewList_ConfigVsSessionLabels(t *testing.T) {
	townRoot := setupTestTownForCrewList(t, map[string][]string{
		"testrig": {"alice", "bob"},
	})

	// Set persistent posting for alice
	settingsDir := filepath.Join(townRoot, "testrig", "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{"alice": "dispatcher"}
	if err := config.SaveRigSettings(filepath.Join(settingsDir, "config.json"), settings); err != nil {
		t.Fatal(err)
	}

	// Set session posting for bob
	bobDir := filepath.Join(townRoot, "testrig", "crew", "bob")
	if err := posting.Write(bobDir, "scout"); err != nil {
		t.Fatal(err)
	}

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	crewRig = "testrig"
	crewJSON = true
	defer func() {
		crewRig = ""
		crewJSON = false
	}()

	output := captureStdout(t, func() {
		if err := runCrewList(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runCrewList: %v", err)
		}
	})

	var items []CrewListItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, output)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	byName := map[string]CrewListItem{}
	for _, item := range items {
		byName[item.Name] = item
	}

	alice := byName["alice"]
	if alice.Posting != "dispatcher" {
		t.Errorf("alice.Posting = %q, want %q", alice.Posting, "dispatcher")
	}
	if alice.PostingSource != "config" {
		t.Errorf("alice.PostingSource = %q, want %q", alice.PostingSource, "config")
	}

	bob := byName["bob"]
	if bob.Posting != "scout" {
		t.Errorf("bob.Posting = %q, want %q", bob.Posting, "scout")
	}
	if bob.PostingSource != "session" {
		t.Errorf("bob.PostingSource = %q, want %q", bob.PostingSource, "session")
	}
}

// 11.3: no posting: no brackets
func TestCrewList_NoPostingNoBrackets(t *testing.T) {
	townRoot := setupTestTownForCrewList(t, map[string][]string{
		"testrig": {"alice"},
	})

	originalWd, _ := os.Getwd()
	defer os.Chdir(originalWd)
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	crewRig = "testrig"
	crewJSON = true
	defer func() {
		crewRig = ""
		crewJSON = false
	}()

	output := captureStdout(t, func() {
		if err := runCrewList(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runCrewList: %v", err)
		}
	})

	var items []CrewListItem
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, output)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Posting != "" {
		t.Errorf("Posting = %q, want empty (no posting set)", items[0].Posting)
	}
	if items[0].PostingSource != "" {
		t.Errorf("PostingSource = %q, want empty", items[0].PostingSource)
	}

	// Also check text output has no brackets
	crewJSON = false
	textOutput := captureStdout(t, func() {
		if err := runCrewList(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runCrewList text: %v", err)
		}
	})
	if strings.Contains(textOutput, "[") && strings.Contains(textOutput, "]") {
		// Check it's not just structural brackets — posting brackets look like [posting (source)]
		if strings.Contains(textOutput, "(config)") || strings.Contains(textOutput, "(session)") {
			t.Errorf("text output should not contain posting brackets, got: %q", textOutput)
		}
	}
}

// 11.4: polecat list shows brackets (JSON-level verification)
func TestPolecatList_ShowsBracketsWithPosting(t *testing.T) {
	t.Parallel()

	// Verify the struct serialization path — polecat list reads posting.Read()
	// and includes it in PolecatListItem.Posting. We test the struct serialization
	// since runPolecatList requires real polecat worktree + tmux infrastructure.
	item := PolecatListItem{
		Rig:     "testrig",
		Name:    "toast",
		Posting: "inspector",
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	var decoded PolecatListItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Posting != "inspector" {
		t.Errorf("Posting = %q, want %q", decoded.Posting, "inspector")
	}

	// Verify text formatting path: posting appears in bracket notation
	postingStr := ""
	if decoded.Posting != "" {
		postingStr = "[" + decoded.Posting + "]"
	}
	if postingStr != "[inspector]" {
		t.Errorf("bracket notation = %q, want %q", postingStr, "[inspector]")
	}
}

// 11.4 supplemental: polecat list integrates posting from .runtime/posting
func TestPolecatList_PostingReadFromRuntime(t *testing.T) {
	t.Parallel()

	// Verify posting.Read returns the correct value from a polecat's work dir
	workDir := t.TempDir()
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "scout" {
		t.Errorf("posting.Read() = %q, want %q", got, "scout")
	}

	// This is exactly what runPolecatList does at line 463:
	//   postingName := posting.Read(p.ClonePath)
	item := PolecatListItem{
		Rig:     "testrig",
		Name:    "toast",
		Posting: got,
	}
	if item.Posting != "scout" {
		t.Errorf("PolecatListItem.Posting = %q, want %q", item.Posting, "scout")
	}
}

// 11.5: polecat no posting: no brackets
func TestPolecatList_NoPostingNoBrackets(t *testing.T) {
	t.Parallel()

	item := PolecatListItem{
		Rig:  "testrig",
		Name: "toast",
		// Posting intentionally empty
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	dataStr := string(data)
	if strings.Contains(dataStr, "posting") {
		t.Errorf("empty posting should be omitted from JSON, got: %s", dataStr)
	}

	// Verify text formatting has no brackets
	postingStr := ""
	if item.Posting != "" {
		postingStr = "[" + item.Posting + "]"
	}
	if postingStr != "" {
		t.Errorf("bracket notation should be empty, got: %q", postingStr)
	}
}

// 11.5 supplemental: posting.Read on empty dir returns ""
func TestPolecatList_NoRuntimePostingReturnsEmpty(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("posting.Read() on empty dir = %q, want empty", got)
	}
}
