package doltserver

import (
	"testing"
)

func TestAssembleCharacterSheet_Normal(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	// Insert stamps for alice-dev (based on spec fixture)
	store.stamps = []StampRecord{
		{ID: "s-001", Author: "julian-k", Subject: "alice-dev", Valence: `{"quality":5,"reliability":5}`, Confidence: 1.0, Severity: "leaf", ContextType: "completion", SkillTags: `["go"]`, Message: "Exceptional Go work on role parser", CreatedAt: "2026-01-15"},
		{ID: "s-002", Author: "steveyegge", Subject: "alice-dev", Valence: `{"quality":4,"creativity":4}`, Confidence: 1.0, Severity: "leaf", ContextType: "completion", SkillTags: `["federation"]`, Message: "Solid federation design", CreatedAt: "2026-01-20"},
		{ID: "s-003", Author: "rust-sarah", Subject: "alice-dev", Valence: `{"quality":5,"reliability":4}`, Confidence: 1.0, Severity: "leaf", ContextType: "completion", SkillTags: `["federation","go"]`, Message: "Clean federation interfaces", CreatedAt: "2026-02-01"},
		{ID: "s-004", Author: "ux-maria", Subject: "alice-dev", Valence: `{"quality":4,"creativity":5}`, Confidence: 1.0, Severity: "leaf", ContextType: "endorsement", SkillTags: `["docs"]`, Message: "Thoughtful API design", CreatedAt: "2026-02-05"},
	}
	store.badges = []BadgeRecord{
		{ID: "b-001", Type: "first_blood", AwardedAt: "2025-11-15", Evidence: "Completion c-001"},
		{ID: "b-002", Type: "polyglot", AwardedAt: "2026-01-01", Evidence: "Stamps in go, federation, docs"},
	}

	sheet, err := AssembleCharacterSheet(store, "alice-dev")
	if err != nil {
		t.Fatalf("AssembleCharacterSheet() error: %v", err)
	}

	if sheet.Handle != "alice-dev" {
		t.Errorf("Handle = %q, want alice-dev", sheet.Handle)
	}
	if sheet.StampCount != 4 {
		t.Errorf("StampCount = %d, want 4", sheet.StampCount)
	}

	// Check stamp geometry
	if g, ok := sheet.StampGeometry["quality"]; !ok {
		t.Error("missing quality geometry")
	} else {
		if g.Count != 4 {
			t.Errorf("quality count = %d, want 4", g.Count)
		}
		if g.Avg != 4.5 {
			t.Errorf("quality avg = %f, want 4.5", g.Avg)
		}
	}

	if g, ok := sheet.StampGeometry["reliability"]; !ok {
		t.Error("missing reliability geometry")
	} else if g.Count != 2 {
		t.Errorf("reliability count = %d, want 2", g.Count)
	}

	if g, ok := sheet.StampGeometry["creativity"]; !ok {
		t.Error("missing creativity geometry")
	} else if g.Count != 2 {
		t.Errorf("creativity count = %d, want 2", g.Count)
	}

	// Check skill coverage
	if len(sheet.SkillCoverage) == 0 {
		t.Fatal("expected skill coverage entries")
	}
	// go should be the top skill (2 stamps)
	topSkill := sheet.SkillCoverage[0]
	if topSkill.Skill != "federation" && topSkill.Skill != "go" {
		t.Errorf("top skill = %q, want federation or go", topSkill.Skill)
	}

	// Check top stamps — should include quality >= 4 completion/endorsement stamps
	if len(sheet.TopStamps) == 0 {
		t.Fatal("expected top stamps")
	}
	// All 4 stamps qualify (quality >= 4, completion or endorsement)
	if len(sheet.TopStamps) != 4 {
		t.Errorf("top stamps count = %d, want 4", len(sheet.TopStamps))
	}
	// Highest effective weight should be first (quality 5 * confidence 1.0 = 5.0)
	if sheet.TopStamps[0].Author != "julian-k" && sheet.TopStamps[0].Author != "rust-sarah" {
		t.Errorf("top stamp author = %q, want julian-k or rust-sarah", sheet.TopStamps[0].Author)
	}

	// Check badges
	if len(sheet.Badges) != 2 {
		t.Errorf("badge count = %d, want 2", len(sheet.Badges))
	}

	// Check warnings (none expected)
	if len(sheet.Warnings) != 0 {
		t.Errorf("warning count = %d, want 0", len(sheet.Warnings))
	}

	// Phase 2 stubs should be nil
	if sheet.NearestNeighbors != nil {
		t.Error("expected nil NearestNeighbors")
	}
	if sheet.CrossCluster != nil {
		t.Error("expected nil CrossCluster")
	}
}

func TestAssembleCharacterSheet_Empty(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	sheet, err := AssembleCharacterSheet(store, "new-rig")
	if err != nil {
		t.Fatalf("AssembleCharacterSheet() error: %v", err)
	}

	if sheet.StampCount != 0 {
		t.Errorf("StampCount = %d, want 0", sheet.StampCount)
	}
	if sheet.Tier != "newcomer" {
		t.Errorf("Tier = %q, want newcomer", sheet.Tier)
	}
	if len(sheet.StampGeometry) != 0 {
		t.Errorf("StampGeometry should be empty, got %d entries", len(sheet.StampGeometry))
	}
	if len(sheet.SkillCoverage) != 0 {
		t.Errorf("SkillCoverage should be empty, got %d entries", len(sheet.SkillCoverage))
	}
	if len(sheet.TopStamps) != 0 {
		t.Errorf("TopStamps should be empty, got %d entries", len(sheet.TopStamps))
	}
}

func TestAssembleCharacterSheet_BootBlocksOnly(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	store.stamps = []StampRecord{
		{ID: "s-bb-001", Author: "system/github-scraper", Subject: "github-dev", Valence: `{"quality":3}`, Confidence: 0.35, Severity: "leaf", ContextType: "boot_block", SkillTags: `["go","react"]`, CreatedAt: "2025-11-01"},
		{ID: "s-bb-002", Author: "system/github-scraper", Subject: "github-dev", Valence: `{"quality":2}`, Confidence: 0.35, Severity: "leaf", ContextType: "boot_block", SkillTags: `["go"]`, CreatedAt: "2025-11-02"},
		{ID: "s-bb-003", Author: "system/github-scraper", Subject: "github-dev", Valence: `{"quality":3}`, Confidence: 0.35, Severity: "leaf", ContextType: "boot_block", SkillTags: `["testing"]`, CreatedAt: "2025-11-03"},
	}

	sheet, err := AssembleCharacterSheet(store, "github-dev")
	if err != nil {
		t.Fatalf("AssembleCharacterSheet() error: %v", err)
	}

	if sheet.StampCount != 3 {
		t.Errorf("StampCount = %d, want 3", sheet.StampCount)
	}

	// All skills should be boot blocks
	for _, skill := range sheet.SkillCoverage {
		if skill.PeerStamps != 0 {
			t.Errorf("skill %q PeerStamps = %d, want 0 (all boot blocks)", skill.Skill, skill.PeerStamps)
		}
		if skill.BootBlocks == 0 {
			t.Errorf("skill %q BootBlocks = 0, want > 0", skill.Skill)
		}
	}

	// Top stamps should be empty (boot_block context type excluded)
	if len(sheet.TopStamps) != 0 {
		t.Errorf("TopStamps should be empty for boot blocks, got %d", len(sheet.TopStamps))
	}
}

func TestAssembleCharacterSheet_RootWarnings(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	store.stamps = []StampRecord{
		{ID: "s-001", Author: "validator", Subject: "trouble-rig", Valence: `{"quality":4}`, Confidence: 0.7, Severity: "leaf", ContextType: "completion", Message: "Good work", CreatedAt: "2026-01-10"},
		{ID: "s-002", Author: "guardian-node", Subject: "trouble-rig", Valence: `{"quality":1}`, Confidence: 0.9, Severity: "root", ContextType: "completion", Message: "Submitted plagiarized work to commons", CreatedAt: "2026-01-20"},
	}

	sheet, err := AssembleCharacterSheet(store, "trouble-rig")
	if err != nil {
		t.Fatalf("AssembleCharacterSheet() error: %v", err)
	}

	if len(sheet.Warnings) != 1 {
		t.Fatalf("Warnings count = %d, want 1", len(sheet.Warnings))
	}
	w := sheet.Warnings[0]
	if w.Severity != "root" {
		t.Errorf("Warning severity = %q, want root", w.Severity)
	}
	if w.Author != "guardian-node" {
		t.Errorf("Warning author = %q, want guardian-node", w.Author)
	}

	// Root-severity stamps should be excluded from geometry
	if g, ok := sheet.StampGeometry["quality"]; ok {
		if g.Count != 1 {
			t.Errorf("quality count = %d, want 1 (root excluded)", g.Count)
		}
	}
}

func TestComputeStampGeometry_ExcludesRoot(t *testing.T) {
	t.Parallel()
	stamps := []StampRecord{
		{Valence: `{"quality":5,"reliability":4}`, Severity: "leaf"},
		{Valence: `{"quality":1}`, Severity: "root"},
		{Valence: `{"quality":3,"reliability":2}`, Severity: "leaf"},
	}

	geo := computeStampGeometry(stamps)

	if g, ok := geo["quality"]; !ok {
		t.Error("missing quality")
	} else {
		if g.Count != 2 {
			t.Errorf("quality count = %d, want 2 (root excluded)", g.Count)
		}
		if g.Avg != 4.0 {
			t.Errorf("quality avg = %f, want 4.0", g.Avg)
		}
	}

	if g, ok := geo["reliability"]; !ok {
		t.Error("missing reliability")
	} else if g.Count != 2 {
		t.Errorf("reliability count = %d, want 2", g.Count)
	}
}

func TestComputeSkillCoverage_SplitsByContextType(t *testing.T) {
	t.Parallel()
	stamps := []StampRecord{
		{SkillTags: `["go","federation"]`, ContextType: "completion"},
		{SkillTags: `["go"]`, ContextType: "boot_block"},
		{SkillTags: `["go","testing"]`, ContextType: "completion"},
	}

	coverage := computeSkillCoverage(stamps)

	goEntry := findSkill(coverage, "go")
	if goEntry == nil {
		t.Fatal("missing go skill")
	}
	if goEntry.PeerStamps != 2 {
		t.Errorf("go PeerStamps = %d, want 2", goEntry.PeerStamps)
	}
	if goEntry.BootBlocks != 1 {
		t.Errorf("go BootBlocks = %d, want 1", goEntry.BootBlocks)
	}

	fedEntry := findSkill(coverage, "federation")
	if fedEntry == nil {
		t.Fatal("missing federation skill")
	}
	if fedEntry.PeerStamps != 1 {
		t.Errorf("federation PeerStamps = %d, want 1", fedEntry.PeerStamps)
	}
}

func TestComputeTopStamps_FiltersAndSorts(t *testing.T) {
	t.Parallel()
	stamps := []StampRecord{
		{Author: "a", Valence: `{"quality":5}`, Confidence: 1.0, ContextType: "completion", Message: "great"},
		{Author: "b", Valence: `{"quality":3}`, Confidence: 1.0, ContextType: "completion", Message: "ok"},           // quality < 4, excluded
		{Author: "c", Valence: `{"quality":4}`, Confidence: 0.5, ContextType: "endorsement", Message: "nice"},         // ew=2.0
		{Author: "d", Valence: `{"quality":4}`, Confidence: 1.0, ContextType: "boot_block", Message: "boot"},          // wrong context type
		{Author: "e", Valence: `{"quality":4}`, Confidence: 0.9, ContextType: "completion", Message: "solid"},          // ew=3.6
	}

	top := computeTopStamps(stamps, 5)

	if len(top) != 3 {
		t.Fatalf("top stamps count = %d, want 3", len(top))
	}
	// Ordered by effective weight: a(5.0) > e(3.6) > c(2.0)
	if top[0].Author != "a" {
		t.Errorf("top[0] author = %q, want a", top[0].Author)
	}
	if top[1].Author != "e" {
		t.Errorf("top[1] author = %q, want e", top[1].Author)
	}
	if top[2].Author != "c" {
		t.Errorf("top[2] author = %q, want c", top[2].Author)
	}
}

func TestComputeTopStamps_LimitsResults(t *testing.T) {
	t.Parallel()
	var stamps []StampRecord
	for i := 0; i < 10; i++ {
		stamps = append(stamps, StampRecord{
			Author: "author", Valence: `{"quality":5}`, Confidence: 1.0,
			ContextType: "completion", Message: "stamp",
		})
	}

	top := computeTopStamps(stamps, 3)
	if len(top) != 3 {
		t.Errorf("top stamps count = %d, want 3 (limited)", len(top))
	}
}

func TestComputeWarnings_OnlyRoot(t *testing.T) {
	t.Parallel()
	stamps := []StampRecord{
		{Author: "a", Severity: "leaf", Message: "good"},
		{Author: "b", Severity: "branch", Message: "warning"},
		{Author: "c", Severity: "root", Message: "bad", CreatedAt: "2026-01-20"},
	}

	warnings := computeWarnings(stamps)
	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}
	if warnings[0].Author != "c" {
		t.Errorf("warning author = %q, want c", warnings[0].Author)
	}
}

func TestParseValenceJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int // expected number of keys
	}{
		{`{"quality":4,"reliability":3,"creativity":2}`, 3},
		{`{"quality":5}`, 1},
		{`{}`, 0},
		{``, 0},
		{`invalid`, 0},
	}

	for _, tt := range tests {
		vals := parseValenceJSON(tt.input)
		got := len(vals)
		if got != tt.want {
			t.Errorf("parseValenceJSON(%q) = %d keys, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseSkillTagsJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int
	}{
		{`["go","federation","testing"]`, 3},
		{`["rust"]`, 1},
		{`[]`, 0},
		{``, 0},
		{`invalid`, 0},
	}

	for _, tt := range tests {
		tags := parseSkillTagsJSON(tt.input)
		if len(tags) != tt.want {
			t.Errorf("parseSkillTagsJSON(%q) = %d tags, want %d", tt.input, len(tags), tt.want)
		}
	}
}

func TestComputeTier_Newcomer(t *testing.T) {
	t.Parallel()
	sheet := &CharacterSheet{StampCount: 0}
	if tier := computeTier(sheet); tier != "newcomer" {
		t.Errorf("tier = %q, want newcomer", tier)
	}
}

func TestComputeTier_Contributor(t *testing.T) {
	t.Parallel()
	sheet := &CharacterSheet{
		StampCount:    5,
		StampGeometry: map[string]GeometryBar{"quality": {Avg: 2.0, Count: 5}},
	}
	if tier := computeTier(sheet); tier != "contributor" {
		t.Errorf("tier = %q, want contributor", tier)
	}
}

func TestComputeTier_Trusted(t *testing.T) {
	t.Parallel()
	sheet := &CharacterSheet{
		StampCount:     12,
		ClusterBreadth: 1,
		StampGeometry:  map[string]GeometryBar{"quality": {Avg: 3.5, Count: 12}},
	}
	if tier := computeTier(sheet); tier != "trusted" {
		t.Errorf("tier = %q, want trusted", tier)
	}
}

// findSkill is a test helper to find a SkillEntry by name.
func findSkill(entries []SkillEntry, name string) *SkillEntry {
	for i := range entries {
		if entries[i].Skill == name {
			return &entries[i]
		}
	}
	return nil
}
