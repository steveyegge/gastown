package doctor

import (
	"testing"
)

// stubCheck implements Check for testing.
type stubCheck struct {
	BaseCheck
}

func (s *stubCheck) Run(ctx *CheckContext) *CheckResult {
	return &CheckResult{Name: s.CheckName, Status: StatusOK}
}

func newStub(name, category string) *stubCheck {
	return &stubCheck{
		BaseCheck: BaseCheck{
			CheckName:        name,
			CheckDescription: "stub: " + name,
			CheckCategory:    category,
		},
	}
}

func testChecks() []Check {
	return []Check{
		newStub("town-config-exists", CategoryCore),
		newStub("town-config-valid", CategoryCore),
		newStub("orphan-sessions", CategoryCleanup),
		newStub("orphan-processes", CategoryCleanup),
		newStub("wisp-garbage-collection", CategoryCleanup),
		newStub("stale-beads-redirect", CategoryCleanup),
		newStub("rig-is-git-repo", CategoryRig),
		newStub("session-hooks", CategoryConfig),
		newStub("beads-config-valid", CategoryBeads),
	}
}

// --- NormalizeName ---

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"orphan-sessions", "orphan-sessions"},
		{"Orphan-Sessions", "orphan-sessions"},
		{"ORPHAN-SESSIONS", "orphan-sessions"},
		{"orphan_sessions", "orphan-sessions"},
		{"Orphan_Sessions", "orphan-sessions"},
		{"orphan_Sessions", "orphan-sessions"},
		{"ORPHAN_SESSIONS", "orphan-sessions"},
		{"wisp-garbage-collection", "wisp-garbage-collection"},
		{"wisp_garbage_collection", "wisp-garbage-collection"},
		{"WispGc", "wispgc"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- FilterChecks ---

func TestFilterChecks_EmptyArgs(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, nil)
	if len(result.Matched) != len(checks) {
		t.Errorf("empty args: got %d matched, want %d", len(result.Matched), len(checks))
	}
	if len(result.Unmatched) != 0 {
		t.Errorf("empty args: got %d unmatched, want 0", len(result.Unmatched))
	}
}

func TestFilterChecks_EmptySliceArgs(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, []string{})
	if len(result.Matched) != len(checks) {
		t.Errorf("empty slice args: got %d matched, want %d", len(result.Matched), len(checks))
	}
}

func TestFilterChecks_ExactNameMatch(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, []string{"orphan-sessions"})
	if len(result.Matched) != 1 {
		t.Fatalf("exact name: got %d matched, want 1", len(result.Matched))
	}
	if result.Matched[0].Name() != "orphan-sessions" {
		t.Errorf("exact name: got %q, want %q", result.Matched[0].Name(), "orphan-sessions")
	}
	if len(result.Unmatched) != 0 {
		t.Errorf("exact name: got %d unmatched, want 0", len(result.Unmatched))
	}
}

func TestFilterChecks_NormalizedNameMatch(t *testing.T) {
	checks := testChecks()

	// Underscore variant
	result := FilterChecks(checks, []string{"orphan_sessions"})
	if len(result.Matched) != 1 || result.Matched[0].Name() != "orphan-sessions" {
		t.Errorf("underscore: got %v", checkNames(result.Matched))
	}

	// Mixed case
	result = FilterChecks(checks, []string{"Orphan-Sessions"})
	if len(result.Matched) != 1 || result.Matched[0].Name() != "orphan-sessions" {
		t.Errorf("mixed case: got %v", checkNames(result.Matched))
	}

	// Uppercase + underscore
	result = FilterChecks(checks, []string{"ORPHAN_SESSIONS"})
	if len(result.Matched) != 1 || result.Matched[0].Name() != "orphan-sessions" {
		t.Errorf("upper+underscore: got %v", checkNames(result.Matched))
	}
}

func TestFilterChecks_CategoryMatch(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, []string{"Cleanup"})

	// Should match all Cleanup checks
	want := []string{"orphan-sessions", "orphan-processes", "wisp-garbage-collection", "stale-beads-redirect"}
	if len(result.Matched) != len(want) {
		t.Fatalf("category: got %d matched (%v), want %d", len(result.Matched), checkNames(result.Matched), len(want))
	}
	for i, check := range result.Matched {
		if check.Name() != want[i] {
			t.Errorf("category[%d]: got %q, want %q", i, check.Name(), want[i])
		}
	}
	if len(result.Unmatched) != 0 {
		t.Errorf("category: got %d unmatched, want 0", len(result.Unmatched))
	}
}

func TestFilterChecks_CategoryCaseInsensitive(t *testing.T) {
	checks := testChecks()

	for _, cat := range []string{"cleanup", "CLEANUP", "Cleanup", "cLEANUP"} {
		result := FilterChecks(checks, []string{cat})
		if len(result.Matched) != 4 {
			t.Errorf("category %q: got %d matched, want 4", cat, len(result.Matched))
		}
	}
}

func TestFilterChecks_MultipleArgs(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, []string{"orphan-sessions", "wisp-garbage-collection"})
	if len(result.Matched) != 2 {
		t.Fatalf("multiple: got %d matched, want 2", len(result.Matched))
	}
	if result.Matched[0].Name() != "orphan-sessions" {
		t.Errorf("multiple[0]: got %q", result.Matched[0].Name())
	}
	if result.Matched[1].Name() != "wisp-garbage-collection" {
		t.Errorf("multiple[1]: got %q", result.Matched[1].Name())
	}
}

func TestFilterChecks_MixedNameAndCategory(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, []string{"town-config-exists", "Cleanup"})

	// Should have town-config-exists + all 4 cleanup checks = 5
	if len(result.Matched) != 5 {
		t.Fatalf("mixed: got %d matched (%v), want 5", len(result.Matched), checkNames(result.Matched))
	}
}

func TestFilterChecks_DeduplicateNameAndCategory(t *testing.T) {
	checks := testChecks()
	// orphan-sessions is in Cleanup, so specifying both should not duplicate
	result := FilterChecks(checks, []string{"orphan-sessions", "Cleanup"})

	// Should have orphan-sessions (from name) + orphan-processes, wisp-garbage-collection, stale-beads-redirect (from category) = 4
	if len(result.Matched) != 4 {
		t.Fatalf("dedup: got %d matched (%v), want 4", len(result.Matched), checkNames(result.Matched))
	}
}

func TestFilterChecks_UnknownName(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, []string{"nonexistent"})
	if len(result.Matched) != 0 {
		t.Errorf("unknown: got %d matched, want 0", len(result.Matched))
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0] != "nonexistent" {
		t.Errorf("unknown: got unmatched %v, want [nonexistent]", result.Unmatched)
	}
}

func TestFilterChecks_PartialUnknown(t *testing.T) {
	checks := testChecks()
	result := FilterChecks(checks, []string{"orphan-sessions", "bogus"})
	if len(result.Matched) != 1 {
		t.Errorf("partial unknown: got %d matched, want 1", len(result.Matched))
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0] != "bogus" {
		t.Errorf("partial unknown: got unmatched %v, want [bogus]", result.Unmatched)
	}
}

func TestFilterChecks_CheckNameTakesPrecedenceOverCategory(t *testing.T) {
	// If a check name matches, category matching is skipped for that arg
	checks := []Check{
		newStub("core", "Special"),    // check named "core"
		newStub("other-check", "Core"), // in category "Core"
	}
	result := FilterChecks(checks, []string{"core"})
	// Should match the check named "core", not the category "Core"
	if len(result.Matched) != 1 {
		t.Fatalf("precedence: got %d matched, want 1", len(result.Matched))
	}
	if result.Matched[0].Name() != "core" {
		t.Errorf("precedence: got %q, want 'core'", result.Matched[0].Name())
	}
}

func TestFilterChecks_NoChecks(t *testing.T) {
	result := FilterChecks(nil, []string{"something"})
	if len(result.Matched) != 0 {
		t.Errorf("no checks: got %d matched, want 0", len(result.Matched))
	}
	if len(result.Unmatched) != 1 {
		t.Errorf("no checks: got %d unmatched, want 1", len(result.Unmatched))
	}
}

// --- SuggestCheck ---

func TestSuggestCheck_CloseTypo(t *testing.T) {
	checks := testChecks()
	// "orphen-sessions" is distance 1 from "orphan-sessions"
	suggestions := SuggestCheck(checks, "orphen-sessions")
	if len(suggestions) == 0 {
		t.Fatal("close typo: got 0 suggestions")
	}
	if suggestions[0] != "orphan-sessions" {
		t.Errorf("close typo: got %q, want 'orphan-sessions'", suggestions[0])
	}
}

func TestSuggestCheck_ExactMatchExcluded(t *testing.T) {
	checks := testChecks()
	// Exact match has distance 0, should not appear in suggestions
	suggestions := SuggestCheck(checks, "orphan-sessions")
	for _, s := range suggestions {
		if s == "orphan-sessions" {
			t.Error("exact match should not appear in suggestions")
		}
	}
}

func TestSuggestCheck_NoCloseMatch(t *testing.T) {
	checks := testChecks()
	suggestions := SuggestCheck(checks, "completely-different-name")
	if len(suggestions) != 0 {
		t.Errorf("no close match: got %v, want empty", suggestions)
	}
}

func TestSuggestCheck_MaxThree(t *testing.T) {
	// Create checks that are all distance 1-2 from input
	checks := []Check{
		newStub("aaa", "Test"),
		newStub("aab", "Test"),
		newStub("aac", "Test"),
		newStub("aad", "Test"),
		newStub("aae", "Test"),
	}
	suggestions := SuggestCheck(checks, "aaz")
	if len(suggestions) > 3 {
		t.Errorf("max three: got %d suggestions, want ≤3", len(suggestions))
	}
}

func TestSuggestCheck_NormalizedInput(t *testing.T) {
	checks := testChecks()
	// "orphen_sessions" normalized → "orphen-sessions", distance 1 from "orphan-sessions"
	suggestions := SuggestCheck(checks, "orphen_sessions")
	if len(suggestions) == 0 {
		t.Fatal("normalized input: got 0 suggestions")
	}
	if suggestions[0] != "orphan-sessions" {
		t.Errorf("normalized input: got %q, want 'orphan-sessions'", suggestions[0])
	}
}

func TestSuggestCheck_DistanceTwo(t *testing.T) {
	checks := testChecks()
	// "wisp-garbage-collectian" is distance 1 from "wisp-garbage-collection" (a≠o)
	suggestions := SuggestCheck(checks, "wisp-garbage-collectian")
	found := false
	for _, s := range suggestions {
		if s == "wisp-garbage-collection" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("distance 2: 'wisp-garbage-collection' not in suggestions %v", suggestions)
	}
}

func TestSuggestCheck_DistanceThreeExcluded(t *testing.T) {
	checks := []Check{
		newStub("abcdef", "Test"),
	}
	// "xyzdef" vs "abcdef": a→x, b→y, c→z = distance 3
	suggestions := SuggestCheck(checks, "xyzdef")
	if len(suggestions) != 0 {
		t.Errorf("distance 3: got %v, want empty", suggestions)
	}
}

func TestSuggestCheck_EmptyChecks(t *testing.T) {
	suggestions := SuggestCheck(nil, "anything")
	if len(suggestions) != 0 {
		t.Errorf("empty checks: got %v, want empty", suggestions)
	}
}

// --- levenshtein ---

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"orphan", "orphen", 1},
		{"orphan-sessions", "orphen-sessions", 1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"→"+tt.b, func(t *testing.T) {
			got := levenshtein(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// helper
func checkNames(checks []Check) []string {
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = c.Name()
	}
	return names
}
