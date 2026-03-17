package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
)

// ---------------------------------------------------------------------------
// Section 9: Identity and bracket notation — CLI-level tests
// Tests 9.9–9.14, 9.16–9.17 from postings-test-spec.md
//
// These test detectSender() and ActorString() with posting bracket notation
// in contexts that simulate mail inbox, mail send, bead creation, and routing.
// ---------------------------------------------------------------------------

// TestBracketNotation_9_9_CrewMailInboxShowsBrackets verifies that a crew member
// with an active posting has bracket notation in detectSender() output, which is
// used in mail inbox headers.
func TestBracketNotation_9_9_CrewMailInboxShowsBrackets(t *testing.T) {
	t.Setenv("GT_ROLE", "crew")
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_CREW", "diesel")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_POSTING", "scout")

	got := detectSender()
	want := "gastown/crew/diesel[scout]"
	if got != want {
		t.Errorf("detectSender() = %q, want %q (mail inbox should show bracket notation)", got, want)
	}
}

// TestBracketNotation_9_10_PolecatMailInboxShowsBrackets verifies that a polecat
// with an active posting has bracket notation in detectSender() output.
func TestBracketNotation_9_10_PolecatMailInboxShowsBrackets(t *testing.T) {
	t.Setenv("GT_ROLE", "polecat")
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_POLECAT", "Toast")
	t.Setenv("GT_CREW", "")
	t.Setenv("GT_POSTING", "inspector")

	got := detectSender()
	want := "gastown/Toast[inspector]"
	if got != want {
		t.Errorf("detectSender() = %q, want %q (mail inbox should show bracket notation)", got, want)
	}
}

// TestBracketNotation_9_11_CrewMailSendFromIncludesBrackets verifies that
// mail sent by a crew member with a posting includes bracket notation in the
// From line (via detectSender).
func TestBracketNotation_9_11_CrewMailSendFromIncludesBrackets(t *testing.T) {
	t.Setenv("GT_ROLE", "crew")
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_CREW", "diesel")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_POSTING", "dispatcher")

	from := detectSender()
	want := "gastown/crew/diesel[dispatcher]"
	if from != want {
		t.Errorf("detectSender() = %q, want %q (mail From line should include brackets)", from, want)
	}
}

// TestBracketNotation_9_11b_PersistentPostingMailSendFromIncludesBrackets verifies
// that mail sent by a crew member whose posting is resolved from WorkerPostings
// config (persistent posting) includes bracket notation in the From line.
// This is the counterpart to TestBracketNotation_9_11 which uses GT_POSTING
// directly; here we resolve from config → detectSender end-to-end.
func TestBracketNotation_9_11b_PersistentPostingMailSendFromIncludesBrackets(t *testing.T) {
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"

	// Set up rig config with persistent posting via WorkerPostings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{crewName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create work directory (no .runtime/posting — only config path)
	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Resolve posting from WorkerPostings config (persistent path)
	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}
	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Fatalf("resolvePostingName() name = %q, want %q", name, "dispatcher")
	}
	if level != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", level, "config")
	}

	// Simulate the env that a session manager would set after resolving from config
	t.Setenv("GT_ROLE", "crew")
	t.Setenv("GT_RIG", rigName)
	t.Setenv("GT_CREW", crewName)
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_POSTING", name) // Set from config resolution, not .runtime/posting

	// Verify bracket notation appears in mail From line
	from := detectSender()
	want := rigName + "/crew/" + crewName + "[dispatcher]"
	if from != want {
		t.Errorf("detectSender() = %q, want %q (persistent posting via WorkerPostings should produce brackets in mail From)", from, want)
	}
}

// TestBracketNotation_9_12_PolecatMailSendFromIncludesBrackets verifies that
// mail sent by a polecat with a posting includes bracket notation in the From line.
func TestBracketNotation_9_12_PolecatMailSendFromIncludesBrackets(t *testing.T) {
	t.Setenv("GT_ROLE", "polecat")
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_POLECAT", "Toast")
	t.Setenv("GT_CREW", "")
	t.Setenv("GT_POSTING", "scout")

	from := detectSender()
	want := "gastown/Toast[scout]"
	if from != want {
		t.Errorf("detectSender() = %q, want %q (mail From line should include brackets)", from, want)
	}
}

// TestBracketNotation_9_13_CrewBeadCreatedByUsesBrackets verifies that bead
// created_by for a crew member with a posting uses the full ActorString with brackets.
func TestBracketNotation_9_13_CrewBeadCreatedByUsesBrackets(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RoleCrew,
		Rig:     "gastown",
		Polecat: "diesel",
		Posting: "inspector",
	}
	got := info.ActorString()
	want := "gastown/crew/diesel[inspector]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q (bead created_by should use brackets)", got, want)
	}
}

// TestBracketNotation_9_13b_CrewBeadCreatedByFromWorkerPostings verifies that bead
// created_by for a crew member with a persistent posting (from WorkerPostings config)
// uses the full ActorString with brackets. Counterpart to TestBracketNotation_9_13
// which sets the Posting field directly; this test resolves it from rig config.
func TestBracketNotation_9_13b_CrewBeadCreatedByFromWorkerPostings(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"

	// Set up rig config with persistent posting via WorkerPostings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{crewName: "inspector"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create work dir (no .runtime/posting — only config/persistent)
	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	// Resolve posting from config (persistent path)
	name, level := resolvePostingName(ctx)
	if name != "inspector" {
		t.Fatalf("resolvePostingName() name = %q, want %q", name, "inspector")
	}
	if level != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", level, "config")
	}

	// Set the resolved posting on the context and verify ActorString
	ctx.Posting = name
	got := ctx.ActorString()
	want := "testrig/crew/diesel[inspector]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q (bead created_by should use brackets from WorkerPostings config)", got, want)
	}
}

// TestBracketNotation_9_13b_PolecatBeadCreatedByFromWorkerPostings verifies that bead
// created_by for a polecat with a persistent posting (from WorkerPostings config)
// uses the full ActorString with brackets.
func TestBracketNotation_9_13b_PolecatBeadCreatedByFromWorkerPostings(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "Toast"

	// Set up rig config with persistent posting via WorkerPostings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{polecatName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create work dir (no .runtime/posting — only config/persistent)
	workDir := filepath.Join(rigPath, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	// Resolve posting from config (persistent path)
	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Fatalf("resolvePostingName() name = %q, want %q", name, "dispatcher")
	}
	if level != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", level, "config")
	}

	// Set the resolved posting on the context and verify ActorString
	ctx.Posting = name
	got := ctx.ActorString()
	want := "testrig/polecats/Toast[dispatcher]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q (bead created_by should use brackets from WorkerPostings config)", got, want)
	}
}

// TestBracketNotation_9_14_PolecatBeadCreatedByUsesBrackets verifies that bead
// created_by for a polecat with a posting uses the full ActorString with brackets.
func TestBracketNotation_9_14_PolecatBeadCreatedByUsesBrackets(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "Toast",
		Posting: "dispatcher",
	}
	got := info.ActorString()
	want := "gastown/polecats/Toast[dispatcher]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q (bead created_by should use brackets)", got, want)
	}
}

// TestBracketNotation_9_16_CrewRoutingAddressExcludesPosting verifies that
// the routing address (GT_ROLE) for a crew member excludes the posting bracket
// notation, even when a posting is assigned. Mail delivery uses GT_ROLE, not
// BD_ACTOR, so brackets must not leak into the routing path.
func TestBracketNotation_9_16_CrewRoutingAddressExcludesPosting(t *testing.T) {
	t.Parallel()

	env := config.AgentEnv(config.AgentEnvConfig{
		Role:      "crew",
		Rig:       "gastown",
		AgentName: "diesel",
		Posting:   "scout",
	})

	// GT_ROLE is the routing address — must NOT contain brackets
	gtRole := env["GT_ROLE"]
	want := "gastown/crew/diesel"
	if gtRole != want {
		t.Errorf("GT_ROLE = %q, want %q (routing address should exclude posting brackets)", gtRole, want)
	}

	// BD_ACTOR should include brackets (display identity)
	bdActor := env["BD_ACTOR"]
	wantActor := "gastown/crew/diesel[scout]"
	if bdActor != wantActor {
		t.Errorf("BD_ACTOR = %q, want %q (display identity should include brackets)", bdActor, wantActor)
	}
}

// TestBracketNotation_9_17_PolecatRoutingAddressExcludesPosting verifies that
// the routing address (GT_ROLE) for a polecat excludes the posting bracket
// notation, even when a posting is assigned.
func TestBracketNotation_9_17_PolecatRoutingAddressExcludesPosting(t *testing.T) {
	t.Parallel()

	env := config.AgentEnv(config.AgentEnvConfig{
		Role:      "polecat",
		Rig:       "gastown",
		AgentName: "Toast",
		Posting:   "inspector",
	})

	// GT_ROLE is the routing address — must NOT contain brackets
	gtRole := env["GT_ROLE"]
	want := "gastown/polecats/Toast"
	if gtRole != want {
		t.Errorf("GT_ROLE = %q, want %q (routing address should exclude posting brackets)", gtRole, want)
	}

	// BD_ACTOR should include brackets
	bdActor := env["BD_ACTOR"]
	wantActor := "gastown/polecats/Toast[inspector]"
	if bdActor != wantActor {
		t.Errorf("BD_ACTOR = %q, want %q (display identity should include brackets)", bdActor, wantActor)
	}
}

// TestBracketNotation_9_9b_PersistentPostingMailInboxShowsBrackets verifies that
// a crew member whose posting comes from WorkerPostings config (persistent, not
// .runtime/posting) still gets bracket notation in detectSender() output, which
// drives the mail inbox header. This is the counterpart to 9.9 which uses
// GT_POSTING directly.
func TestBracketNotation_9_9b_PersistentPostingMailInboxShowsBrackets(t *testing.T) {
	// Set up rig config with persistent posting via WorkerPostings
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"

	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{crewName: "scout"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create the work directory (no .runtime/posting — posting is config-only)
	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Step 1: Resolve posting from config (persistent path)
	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}
	name, level := resolvePostingName(ctx)
	if name != "scout" {
		t.Fatalf("resolvePostingName() name = %q, want %q", name, "scout")
	}
	if level != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", level, "config")
	}

	// Step 2: Simulate the session manager setting GT_POSTING from config resolution
	t.Setenv("GT_ROLE", "crew")
	t.Setenv("GT_RIG", rigName)
	t.Setenv("GT_CREW", crewName)
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_POSTING", name) // Set from WorkerPostings config

	// Step 3: Verify detectSender() shows bracket notation in mail inbox header
	got := detectSender()
	want := rigName + "/crew/" + crewName + "[scout]"
	if got != want {
		t.Errorf("detectSender() = %q, want %q (persistent posting from WorkerPostings config should show bracket notation in mail inbox)", got, want)
	}
}

// TestBracketNotation_9_16b_PersistentPostingRoutingAddressExcludesPosting
// verifies that when a posting is resolved from WorkerPostings config (persistent
// path) rather than .runtime/posting (session path), the routing address (GT_ROLE)
// still excludes the posting bracket notation. This is the persistent-config
// counterpart to TestBracketNotation_9_16.
func TestBracketNotation_9_16b_PersistentPostingRoutingAddressExcludesPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"
	postingName := "scout"

	// Set up rig config with persistent posting via WorkerPostings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings2 := config.NewRigSettings()
	settings2.WorkerPostings = map[string]string{crewName: postingName}
	data2, _ := json.Marshal(settings2)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data2, 0644); err != nil {
		t.Fatal(err)
	}

	// Create work directory (no .runtime/posting — config-only)
	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Resolve posting from persistent config
	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}
	name2, level2 := resolvePostingName(ctx)
	if name2 != postingName {
		t.Fatalf("resolvePostingName() name = %q, want %q", name2, postingName)
	}
	if level2 != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", level2, "config")
	}

	// Build env using the config-resolved posting
	env := config.AgentEnv(config.AgentEnvConfig{
		Role:      "crew",
		Rig:       rigName,
		AgentName: crewName,
		Posting:   name2,
	})

	// GT_ROLE is the routing address — must NOT contain brackets
	gtRole := env["GT_ROLE"]
	wantRole := rigName + "/crew/" + crewName
	if gtRole != wantRole {
		t.Errorf("GT_ROLE = %q, want %q (persistent posting routing address should exclude brackets)", gtRole, wantRole)
	}

	// BD_ACTOR should include brackets (display identity)
	bdActor := env["BD_ACTOR"]
	wantActor := rigName + "/crew/" + crewName + "[" + postingName + "]"
	if bdActor != wantActor {
		t.Errorf("BD_ACTOR = %q, want %q (persistent posting display identity should include brackets)", bdActor, wantActor)
	}
}

// ---------------------------------------------------------------------------
// Section 9.1b: Persistent config → unambiguous bracket notation
// Verifies the full path: WorkerPostings config → resolvePostingName →
// resolvePostingLevel (unambiguous, embedded-only) → ActorString produces
// simple bracket notation like gastown/crew/alice[inspector].
// ---------------------------------------------------------------------------

// TestBracketNotation_9_1b_PersistentCrewUnambiguousBracket verifies that a
// crew member whose posting is resolved from WorkerPostings config (persistent
// path) and whose posting template exists only at the embedded level (unambiguous)
// produces the simple bracket notation in ActorString — e.g.,
// "testrig/crew/alice[inspector]" without a level prefix.
func TestBracketNotation_9_1b_PersistentCrewUnambiguousBracket(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "alice"

	// Set up rig config with persistent posting via WorkerPostings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{crewName: "inspector"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create work directory (no .runtime/posting — config path only)
	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// No rig-level posting template override — only embedded exists → unambiguous

	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	// Step 1: Resolve posting name from WorkerPostings config
	name, nameLevel := resolvePostingName(ctx)
	if name != "inspector" {
		t.Fatalf("resolvePostingName() name = %q, want %q", name, "inspector")
	}
	if nameLevel != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", nameLevel, "config")
	}
	ctx.Posting = name

	// Step 2: Resolve template level — only embedded exists → unambiguous
	ctx.PostingLevel, ctx.PostingAmbiguous = resolvePostingLevel(ctx)
	if ctx.PostingLevel != "embedded" {
		t.Errorf("PostingLevel = %q, want %q (only embedded template exists)", ctx.PostingLevel, "embedded")
	}
	if ctx.PostingAmbiguous {
		t.Error("PostingAmbiguous = true, want false (only one level)")
	}

	// Step 3: Verify ActorString produces simple bracket notation (no level prefix)
	got := ctx.ActorString()
	want := "testrig/crew/alice[inspector]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q (persistent unambiguous posting should use simple brackets)", got, want)
	}
}

// TestBracketNotation_9_1b_PersistentPolecatUnambiguousBracket is the polecat
// counterpart to TestBracketNotation_9_1b_PersistentCrewUnambiguousBracket.
// Verifies persistent config → resolvePostingName → resolvePostingLevel
// (unambiguous) → ActorString for a polecat role.
func TestBracketNotation_9_1b_PersistentPolecatUnambiguousBracket(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "Toast"

	// Set up rig config with persistent posting via WorkerPostings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{polecatName: "inspector"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create work directory (no .runtime/posting — config path only)
	workDir := filepath.Join(rigPath, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// No rig-level posting template override — only embedded exists → unambiguous

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	// Step 1: Resolve posting name from WorkerPostings config
	name, nameLevel := resolvePostingName(ctx)
	if name != "inspector" {
		t.Fatalf("resolvePostingName() name = %q, want %q", name, "inspector")
	}
	if nameLevel != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", nameLevel, "config")
	}
	ctx.Posting = name

	// Step 2: Resolve template level — only embedded exists → unambiguous
	ctx.PostingLevel, ctx.PostingAmbiguous = resolvePostingLevel(ctx)
	if ctx.PostingLevel != "embedded" {
		t.Errorf("PostingLevel = %q, want %q (only embedded template exists)", ctx.PostingLevel, "embedded")
	}
	if ctx.PostingAmbiguous {
		t.Error("PostingAmbiguous = true, want false (only one level)")
	}

	// Step 3: Verify ActorString produces simple bracket notation (no level prefix)
	got := ctx.ActorString()
	want := "testrig/polecats/Toast[inspector]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q (persistent unambiguous posting should use simple brackets)", got, want)
	}
}

// ---------------------------------------------------------------------------
// Section 13: Environment variables — GT_POSTING for persistent posting
// Test 13.7 from postings-test-spec.md
// ---------------------------------------------------------------------------

// TestEnvVar_13_7_PersistentPostingSetsGT_POSTING verifies that a persistent
// posting (from rig config, not session) sets GT_POSTING in AgentEnv.
// The config path should set GT_POSTING the same as the session path.
func TestEnvVar_13_7_PersistentPostingSetsGT_POSTING(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"

	// Set up rig config with persistent posting
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{crewName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// No session posting — only config/persistent
	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	// Resolve posting from config (persistent path)
	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Fatalf("resolvePostingName() name = %q, want %q", name, "dispatcher")
	}
	if level != "config" {
		t.Fatalf("resolvePostingName() level = %q, want %q", level, "config")
	}

	// Now verify that AgentEnv includes GT_POSTING when posting is set via config path
	env := config.AgentEnv(config.AgentEnvConfig{
		Role:      "crew",
		Rig:       rigName,
		AgentName: crewName,
		Posting:   name, // from config resolution
	})

	if got := env["GT_POSTING"]; got != "dispatcher" {
		t.Errorf("GT_POSTING = %q, want %q (persistent posting should set GT_POSTING)", got, "dispatcher")
	}
}

// TestEnvVar_13_7_PersistentPostingResolvedViaDetectSender verifies that
// detectSender picks up posting from GT_POSTING env var (which would be set
// from the persistent config path by the session manager).
func TestEnvVar_13_7_PersistentPostingResolvedViaDetectSender(t *testing.T) {
	// Simulate the env that a session manager would set for a crew member
	// whose posting was resolved from persistent config
	t.Setenv("GT_ROLE", "crew")
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_CREW", "diesel")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_POSTING", "dispatcher") // Set from config path

	got := detectSender()
	want := "gastown/crew/diesel[dispatcher]"
	if got != want {
		t.Errorf("detectSender() = %q, want %q (persistent posting via GT_POSTING should appear in sender)", got, want)
	}
}

// TestDetectSender_PostingFromRuntimeFile verifies that detectSender picks up
// posting from .runtime/posting when GT_POSTING is not set (cwd fallback).
func TestDetectSender_PostingFromRuntimeFile(t *testing.T) {
	t.Setenv("GT_ROLE", "crew")
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_CREW", "diesel")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_POSTING", "") // Not set — should fall back to .runtime/posting

	// Create a temp dir with .runtime/posting
	tmpDir := t.TempDir()
	if err := posting.Write(tmpDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	// Change to the temp dir so detectPosting reads from cwd
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	got := detectSender()
	want := "gastown/crew/diesel[inspector]"
	if got != want {
		t.Errorf("detectSender() = %q, want %q (.runtime/posting fallback should work)", got, want)
	}
}
