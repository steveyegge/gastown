package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchivistDogInterval_Default(t *testing.T) {
	got := archivistDogInterval(nil)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultArchivistDogInterval, got)
	}
}

func TestArchivistDogInterval_NilPatrols(t *testing.T) {
	config := &DaemonPatrolConfig{}
	got := archivistDogInterval(config)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultArchivistDogInterval, got)
	}
}

func TestArchivistDogInterval_NilArchivistDog(t *testing.T) {
	config := &DaemonPatrolConfig{Patrols: &PatrolsConfig{}}
	got := archivistDogInterval(config)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultArchivistDogInterval, got)
	}
}

func TestArchivistDogInterval_Configured(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:     true,
				IntervalStr: "3m",
			},
		},
	}
	got := archivistDogInterval(config)
	if got != 3*time.Minute {
		t.Errorf("expected 3m, got %v", got)
	}
}

func TestArchivistDogInterval_InvalidFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:     true,
				IntervalStr: "not-a-duration",
			},
		},
	}
	got := archivistDogInterval(config)
	if got != defaultArchivistDogInterval {
		t.Errorf("expected default interval %v for invalid config, got %v", defaultArchivistDogInterval, got)
	}
}

func TestScanRigNotes_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	results := scanRigNotes(tmpDir)
	if len(results) != 0 {
		t.Errorf("expected no results for empty dir, got %d", len(results))
	}
}

func TestScanRigNotes_NoNotes(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "rigs", "my-rig", "domain")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	results := scanRigNotes(tmpDir)
	if len(results) != 0 {
		t.Errorf("expected no results for rig without notes dir, got %d", len(results))
	}
}

func TestScanRigNotes_WithMarkdownFiles(t *testing.T) {
	tmpDir := t.TempDir()
	notesDir := filepath.Join(tmpDir, "rigs", "backend", "domain", "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"auth-patterns.md", "api-design.md"} {
		if err := os.WriteFile(filepath.Join(notesDir, name), []byte("# test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(notesDir, "scratch.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatal(err)
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 1 {
		t.Fatalf("expected 1 rig with notes, got %d", len(results))
	}
	if results[0].Rig != "backend" {
		t.Errorf("expected rig 'backend', got %q", results[0].Rig)
	}
	if len(results[0].Files) != 2 {
		t.Errorf("expected 2 md files, got %d", len(results[0].Files))
	}
}

func TestScanRigNotes_MultipleRigs(t *testing.T) {
	tmpDir := t.TempDir()
	for _, rig := range []string{"backend", "frontend", "infra"} {
		notesDir := filepath.Join(tmpDir, "rigs", rig, "domain", "notes")
		if err := os.MkdirAll(notesDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(notesDir, "note.md"), []byte("# test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 3 {
		t.Errorf("expected 3 rigs with notes, got %d", len(results))
	}
}

func TestScanRigNotes_TopLevelLayout(t *testing.T) {
	tmpDir := t.TempDir()
	notesDir := filepath.Join(tmpDir, "my-rig", "domain", "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "note.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 1 {
		t.Fatalf("expected 1 rig with notes in top-level layout, got %d", len(results))
	}
	if results[0].Rig != "my-rig" {
		t.Errorf("expected rig 'my-rig', got %q", results[0].Rig)
	}
}

func TestScanRigNotes_HiddenFilesIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	notesDir := filepath.Join(tmpDir, "rigs", "backend", "domain", "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, ".hidden.md"), []byte("# hidden"), 0644); err != nil {
		t.Fatal(err)
	}

	results := scanRigNotes(tmpDir)
	if len(results) != 0 {
		t.Errorf("expected no results (hidden files only), got %d", len(results))
	}
}

func TestCooldown_Fresh(t *testing.T) {
	c := newArchivistDogCooldowns()
	if !c.canDispatch("backend") {
		t.Error("expected canDispatch=true for fresh rig")
	}
}

func TestCooldown_AfterDispatch(t *testing.T) {
	c := newArchivistDogCooldowns()
	c.markDispatched("backend")
	if c.canDispatch("backend") {
		t.Error("expected canDispatch=false immediately after dispatch")
	}
}

func TestCooldown_DifferentRigs(t *testing.T) {
	c := newArchivistDogCooldowns()
	c.markDispatched("backend")
	if !c.canDispatch("frontend") {
		t.Error("expected canDispatch=true for different rig")
	}
}

func TestCooldown_Expired(t *testing.T) {
	c := newArchivistDogCooldowns()
	c.mu.Lock()
	c.lastDispatched["backend"] = time.Now().Add(-archivistDogCooldownPerRig - time.Second)
	c.mu.Unlock()

	if !c.canDispatch("backend") {
		t.Error("expected canDispatch=true after cooldown expired")
	}
}

func TestArchivistAgentCommand_Default(t *testing.T) {
	got := archivistAgentCommand(nil)
	if got != defaultArchivistAgentCommand {
		t.Errorf("expected default %q, got %q", defaultArchivistAgentCommand, got)
	}
}

func TestArchivistAgentCommand_NilPatrols(t *testing.T) {
	config := &DaemonPatrolConfig{}
	got := archivistAgentCommand(config)
	if got != defaultArchivistAgentCommand {
		t.Errorf("expected default %q, got %q", defaultArchivistAgentCommand, got)
	}
}

func TestArchivistAgentCommand_Configured(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:      true,
				AgentCommand: "/usr/local/bin/my-agent",
			},
		},
	}
	got := archivistAgentCommand(config)
	if got != "/usr/local/bin/my-agent" {
		t.Errorf("expected configured command, got %q", got)
	}
}

func TestArchivistAgentCommand_EmptyFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{
				Enabled:      true,
				AgentCommand: "",
			},
		},
	}
	got := archivistAgentCommand(config)
	if got != defaultArchivistAgentCommand {
		t.Errorf("expected default %q for empty config, got %q", defaultArchivistAgentCommand, got)
	}
}

func TestArchivistPromptTemplate_Formatting(t *testing.T) {
	beadSection := renderBeadNotesSection(nil)
	prompt := fmt.Sprintf(archivistPromptTemplate,
		"backend",
		"rigs/backend/domain/notes",
		"rigs/backend/domain",
		"auth-patterns.md, api-design.md",
		beadSection,
		"rigs/backend/domain/notes",
	)
	if !strings.Contains(prompt, `"backend"`) {
		t.Error("prompt should contain rig name")
	}
	if !strings.Contains(prompt, "rigs/backend/domain/notes") {
		t.Error("prompt should contain notes dir")
	}
	if !strings.Contains(prompt, "auth-patterns.md, api-design.md") {
		t.Error("prompt should contain file list")
	}
	if !strings.Contains(prompt, "source B") {
		t.Error("prompt should describe source B (bead/wisp notes)")
	}
	if !strings.Contains(prompt, "DELETE each processed note FILE") {
		t.Error("prompt should preserve file-delete semantics")
	}
	if !strings.Contains(prompt, "Do NOT delete or modify any bead/wisp") {
		t.Error("prompt should instruct that bead/wisp notes are durable")
	}
	if !strings.Contains(prompt, "laws-prospector.formula.toml") {
		t.Error("prompt should reference the prospector formula section structure")
	}
}

func TestIsPatrolEnabled_ArchivistDog(t *testing.T) {
	if IsPatrolEnabled(nil, "archivist_dog") {
		t.Error("expected disabled for nil config")
	}

	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			ArchivistDog: &ArchivistDogConfig{Enabled: true},
		},
	}
	if !IsPatrolEnabled(config, "archivist_dog") {
		t.Error("expected enabled when config says enabled")
	}

	config.Patrols.ArchivistDog.Enabled = false
	if IsPatrolEnabled(config, "archivist_dog") {
		t.Error("expected disabled when config says disabled")
	}
}

// --- bead-notes pickup tests ---

func TestPickRigLabel_InfraSkipped(t *testing.T) {
	if got := pickRigLabel([]string{"prime", "backend"}); got != "backend" {
		t.Errorf("expected 'backend', got %q", got)
	}
	if got := pickRigLabel([]string{"wisp", "molecule", "ios"}); got != "ios" {
		t.Errorf("expected 'ios', got %q", got)
	}
}

func TestPickRigLabel_EmptyWhenNoRigLike(t *testing.T) {
	if got := pickRigLabel([]string{"prime"}); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := pickRigLabel(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFilterBeadNotesForPickup_EmptyNotesSkipped(t *testing.T) {
	state := &archivistDogState{Collated: map[string]collatedEntry{}}
	entries := []rawBeadListEntry{
		{ID: "sbx-gastown-aaa", Notes: "", Labels: []string{"backend"}},
		{ID: "sbx-gastown-bbb", Notes: "   ", Labels: []string{"backend"}},
		{ID: "sbx-gastown-ccc", Notes: "turn 1: did a thing", Labels: []string{"backend"}},
	}
	got := filterBeadNotesForPickup(entries, state)
	if len(got["backend"]) != 1 {
		t.Fatalf("expected 1 bead note for backend, got %d", len(got["backend"]))
	}
	if got["backend"][0].ID != "sbx-gastown-ccc" {
		t.Errorf("expected ccc, got %q", got["backend"][0].ID)
	}
}

func TestFilterBeadNotesForPickup_CollatedGateSkipsRedispatch(t *testing.T) {
	state := &archivistDogState{Collated: map[string]collatedEntry{
		"sbx-gastown-aaa": {LastCollatedAt: "2026-04-20T00:00:00Z"},
	}}
	entries := []rawBeadListEntry{
		{ID: "sbx-gastown-aaa", Notes: "turn 1", Labels: []string{"backend"}},
		{ID: "sbx-gastown-bbb", Notes: "turn 1", Labels: []string{"backend"}},
	}
	got := filterBeadNotesForPickup(entries, state)
	if len(got["backend"]) != 1 {
		t.Fatalf("expected 1 bead (collated one filtered out), got %d", len(got["backend"]))
	}
	if got["backend"][0].ID != "sbx-gastown-bbb" {
		t.Errorf("expected bbb (non-collated), got %q", got["backend"][0].ID)
	}
}

func TestFilterBeadNotesForPickup_WispKindDetected(t *testing.T) {
	state := &archivistDogState{Collated: map[string]collatedEntry{}}
	entries := []rawBeadListEntry{
		{ID: "sbx-gastown-wisp-abc", Notes: "wisp turn", Labels: []string{"backend"}},
		{ID: "sbx-gastown-xyz", Notes: "bead turn", Labels: []string{"backend"}},
	}
	got := filterBeadNotesForPickup(entries, state)
	if len(got["backend"]) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(got["backend"]))
	}
	kinds := map[string]string{}
	for _, n := range got["backend"] {
		kinds[n.ID] = n.Kind
	}
	if kinds["sbx-gastown-wisp-abc"] != "wisp" {
		t.Errorf("expected wisp kind, got %q", kinds["sbx-gastown-wisp-abc"])
	}
	if kinds["sbx-gastown-xyz"] != "bead" {
		t.Errorf("expected bead kind, got %q", kinds["sbx-gastown-xyz"])
	}
}

func TestFilterBeadNotesForPickup_NoRigLabelSkipped(t *testing.T) {
	state := &archivistDogState{Collated: map[string]collatedEntry{}}
	entries := []rawBeadListEntry{
		{ID: "sbx-gastown-aaa", Notes: "turn 1", Labels: []string{"prime"}},
		{ID: "sbx-gastown-bbb", Notes: "turn 1", Labels: nil},
	}
	got := filterBeadNotesForPickup(entries, state)
	if len(got) != 0 {
		t.Errorf("expected no buckets (no rig label), got %d", len(got))
	}
}

func TestFilterBeadNotesForPickup_GroupsByRig(t *testing.T) {
	state := &archivistDogState{Collated: map[string]collatedEntry{}}
	entries := []rawBeadListEntry{
		{ID: "sbx-gastown-aaa", Notes: "note", Labels: []string{"backend"}},
		{ID: "sbx-gastown-bbb", Notes: "note", Labels: []string{"ios"}},
		{ID: "sbx-gastown-ccc", Notes: "note", Labels: []string{"backend"}},
	}
	got := filterBeadNotesForPickup(entries, state)
	if len(got["backend"]) != 2 {
		t.Errorf("expected 2 for backend, got %d", len(got["backend"]))
	}
	if len(got["ios"]) != 1 {
		t.Errorf("expected 1 for ios, got %d", len(got["ios"]))
	}
}

func TestMergeRigNotes_UnionOfSources(t *testing.T) {
	files := []rigNotes{
		{Rig: "backend", Files: []string{"a.md"}},
		{Rig: "ios", Files: []string{"b.md"}},
	}
	beads := map[string][]beadNote{
		"backend":  {{ID: "sbx-1", Notes: "x"}},
		"frontend": {{ID: "sbx-2", Notes: "y"}},
	}
	merged := mergeRigNotes(files, beads)
	if len(merged) != 3 {
		t.Fatalf("expected 3 rigs in merged set, got %d", len(merged))
	}
	byRig := map[string]rigNotes{}
	for _, rn := range merged {
		byRig[rn.Rig] = rn
	}
	if len(byRig["backend"].Files) != 1 || len(byRig["backend"].BeadNotes) != 1 {
		t.Errorf("backend should merge files + bead notes, got files=%d beads=%d",
			len(byRig["backend"].Files), len(byRig["backend"].BeadNotes))
	}
	if len(byRig["ios"].Files) != 1 || len(byRig["ios"].BeadNotes) != 0 {
		t.Errorf("ios should be file-only, got files=%d beads=%d",
			len(byRig["ios"].Files), len(byRig["ios"].BeadNotes))
	}
	if len(byRig["frontend"].Files) != 0 || len(byRig["frontend"].BeadNotes) != 1 {
		t.Errorf("frontend should be bead-only, got files=%d beads=%d",
			len(byRig["frontend"].Files), len(byRig["frontend"].BeadNotes))
	}
}

func TestArchivistDogState_MarkAndIsCollated(t *testing.T) {
	s := &archivistDogState{Collated: map[string]collatedEntry{}}
	if s.IsCollated("sbx-1") {
		t.Error("expected not collated initially")
	}
	s.MarkCollated("sbx-1", time.Now())
	if !s.IsCollated("sbx-1") {
		t.Error("expected collated after marking")
	}
	if s.IsCollated("sbx-2") {
		t.Error("other IDs should remain uncollated")
	}
}

func TestArchivistDogState_NilSafe(t *testing.T) {
	var s *archivistDogState
	if s.IsCollated("anything") {
		t.Error("nil state should report uncollated")
	}
}

func TestArchivistDogState_LoadMissingReturnsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	state := loadArchivistDogState(tmpDir)
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Collated == nil {
		t.Error("expected initialized Collated map")
	}
	if len(state.Collated) != 0 {
		t.Errorf("expected empty map, got %d entries", len(state.Collated))
	}
}

func TestArchivistDogState_SaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	state := loadArchivistDogState(tmpDir)
	state.MarkCollated("sbx-gastown-aaa", time.Now())
	state.MarkCollated("sbx-gastown-wisp-bbb", time.Now())
	if err := saveArchivistDogState(tmpDir, state); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	reloaded := loadArchivistDogState(tmpDir)
	if !reloaded.IsCollated("sbx-gastown-aaa") {
		t.Error("expected aaa collated after reload")
	}
	if !reloaded.IsCollated("sbx-gastown-wisp-bbb") {
		t.Error("expected wisp-bbb collated after reload")
	}
	if reloaded.IsCollated("sbx-gastown-ccc") {
		t.Error("unexpected ccc collated")
	}
}

func TestArchivistDogState_SaveWritesJSON(t *testing.T) {
	tmpDir := t.TempDir()
	state := &archivistDogState{Collated: map[string]collatedEntry{
		"sbx-gastown-aaa": {LastCollatedAt: "2026-04-20T00:00:00Z"},
	}}
	if err := saveArchivistDogState(tmpDir, state); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "daemon", "archivist-state.json"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var parsed archivistDogState
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.Collated["sbx-gastown-aaa"].LastCollatedAt != "2026-04-20T00:00:00Z" {
		t.Errorf("timestamp not persisted, got %+v", parsed.Collated)
	}
}

func TestRenderBeadNotesSection_Empty(t *testing.T) {
	got := renderBeadNotesSection(nil)
	if !strings.Contains(got, "none this run") {
		t.Errorf("expected 'none' marker, got %q", got)
	}
}

func TestRenderBeadNotesSection_WithNotes(t *testing.T) {
	got := renderBeadNotesSection([]beadNote{
		{ID: "sbx-gastown-326f", Kind: "bead", Title: "Archivist pickup", Notes: "turn 1: read bead\nturn 2: wrote code"},
		{ID: "sbx-gastown-wisp-abc", Kind: "wisp", Title: "wispy", Notes: "turn 1: done"},
	})
	if !strings.Contains(got, "sbx-gastown-326f") {
		t.Error("expected bead ID in rendered section")
	}
	if !strings.Contains(got, "(bead)") {
		t.Error("expected kind label 'bead'")
	}
	if !strings.Contains(got, "(wisp)") {
		t.Error("expected kind label 'wisp'")
	}
	if !strings.Contains(got, "turn 1: read bead") {
		t.Error("expected note content")
	}
	if !strings.Contains(got, "2 entries") {
		t.Error("expected entry count")
	}
}

// TestDispatchPromptIncludesBeadNotes is an integration-style test that verifies
// the dispatch path stitches inline bead notes into the prompt the archivist
// agent will receive. It mimics the polecat -> bead-note -> archivist flow:
// a mock polecat has written turn notes on sbx-gastown-mock; the archivist
// dispatch prompt must contain those notes.
func TestDispatchPromptIncludesBeadNotes(t *testing.T) {
	rn := rigNotes{
		Rig:   "backend",
		Files: []string{"exit-summary.md"},
		BeadNotes: []beadNote{
			{
				ID:    "sbx-gastown-mock",
				Kind:  "bead",
				Title: "mock polecat work",
				Notes: "turn 1: primed from formula\nturn 2: read data/entities.md\nturn 3: applied edit to raw_sac.go\nexit: shipped",
			},
		},
	}
	notesDir := filepath.Join("rigs", rn.Rig, "domain", "notes")
	domainDir := filepath.Join("rigs", rn.Rig, "domain")
	section := renderBeadNotesSection(rn.BeadNotes)
	prompt := fmt.Sprintf(archivistPromptTemplate,
		rn.Rig, notesDir, domainDir, strings.Join(rn.Files, ", "), section, notesDir)

	for _, want := range []string{
		"sbx-gastown-mock",
		"turn 1: primed from formula",
		"turn 3: applied edit to raw_sac.go",
		"exit-summary.md",
		"source B",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing expected substring %q", want)
		}
	}
}
