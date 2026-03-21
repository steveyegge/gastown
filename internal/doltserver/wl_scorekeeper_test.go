package doltserver

import (
	"testing"
)

func TestRunScorekeeper_Empty(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	entries, err := RunScorekeeper(store)
	if err != nil {
		t.Fatalf("RunScorekeeper() error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 for empty DB", len(entries))
	}
}

func TestRunScorekeeper_MultipleRigs(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	// Alice: 4 stamps → contributor
	store.stamps = []StampRecord{
		{ID: "s-a1", Author: "val-1", Subject: "alice", Valence: `{"quality":4}`, Confidence: 0.7, Severity: "leaf", ContextType: "completion", SkillTags: `["go"]`},
		{ID: "s-a2", Author: "val-2", Subject: "alice", Valence: `{"quality":3}`, Confidence: 0.7, Severity: "leaf", ContextType: "completion", SkillTags: `["go"]`},
		{ID: "s-a3", Author: "val-1", Subject: "alice", Valence: `{"quality":5}`, Confidence: 0.7, Severity: "leaf", ContextType: "endorsement", SkillTags: `["federation"]`},
		{ID: "s-a4", Author: "val-3", Subject: "alice", Valence: `{"quality":4}`, Confidence: 0.7, Severity: "leaf", ContextType: "completion"},
		// Bob: 1 stamp → newcomer
		{ID: "s-b1", Author: "val-1", Subject: "bob", Valence: `{"quality":3}`, Confidence: 0.5, Severity: "leaf", ContextType: "completion"},
	}

	entries, err := RunScorekeeper(store)
	if err != nil {
		t.Fatalf("RunScorekeeper() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	// Find alice and bob entries
	var alice, bob *LeaderboardEntry
	for _, e := range entries {
		switch e.Handle {
		case "alice":
			alice = e
		case "bob":
			bob = e
		}
	}

	if alice == nil {
		t.Fatal("alice entry not found")
	}
	if alice.StampCount != 4 {
		t.Errorf("alice StampCount = %d, want 4", alice.StampCount)
	}
	if alice.Tier != "contributor" {
		t.Errorf("alice Tier = %q, want contributor", alice.Tier)
	}

	if bob == nil {
		t.Fatal("bob entry not found")
	}
	if bob.StampCount != 1 {
		t.Errorf("bob StampCount = %d, want 1", bob.StampCount)
	}
	if bob.Tier != "newcomer" {
		t.Errorf("bob Tier = %q, want newcomer", bob.Tier)
	}
}

func TestRunScorekeeper_SkillsInLeaderboard(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	store.stamps = []StampRecord{
		{ID: "s-1", Author: "v", Subject: "dev", Valence: `{"quality":4}`, Confidence: 0.7, Severity: "leaf", ContextType: "completion", SkillTags: `["go","api"]`},
		{ID: "s-2", Author: "v", Subject: "dev", Valence: `{"quality":3}`, Confidence: 0.7, Severity: "leaf", ContextType: "completion", SkillTags: `["go"]`},
		{ID: "s-3", Author: "v", Subject: "dev", Valence: `{"quality":5}`, Confidence: 0.7, Severity: "leaf", ContextType: "completion", SkillTags: `["testing"]`},
	}

	entries, err := RunScorekeeper(store)
	if err != nil {
		t.Fatalf("RunScorekeeper() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}

	entry := entries[0]
	if entry.TopSkills == "" {
		t.Error("TopSkills should not be empty")
	}
	// Should contain "go" as a skill
	if !containsStr(entry.TopSkills, "go") {
		t.Errorf("TopSkills %q should contain 'go'", entry.TopSkills)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 && haystack != "" && needle != "" &&
		(haystack == needle || len(haystack) > len(needle) && containsSubstr(haystack, needle))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
