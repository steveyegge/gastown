package doltserver

import (
	"strings"
	"testing"
)

// wlCommonsConformance is a shared test suite that validates any WLCommonsStore
// implementation against the expected behavioral contract. It runs against the
// fake (always) and can run against the real Dolt server with build tags.
func wlCommonsConformance(t *testing.T, newStore func(t *testing.T) WLCommonsStore) {
	t.Run("InsertAndQuery", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		item := &WantedItem{
			ID:          "w-conf01",
			Title:       "Conformance test item",
			Description: "Test description",
			Project:     "test-project",
			Type:        "feature",
			Priority:    1,
			PostedBy:    "test-rig",
			EffortLevel: "medium",
		}
		if err := store.InsertWanted(item); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}

		got, err := store.QueryWanted("w-conf01")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if got.ID != "w-conf01" {
			t.Errorf("ID = %q, want %q", got.ID, "w-conf01")
		}
		if got.Title != "Conformance test item" {
			t.Errorf("Title = %q, want %q", got.Title, "Conformance test item")
		}
		if got.Status != "open" {
			t.Errorf("Status = %q, want %q", got.Status, "open")
		}
	})

	t.Run("ClaimOpenItem", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_ = store.InsertWanted(&WantedItem{ID: "w-conf02", Title: "Claimable"})
		if err := store.ClaimWanted("w-conf02", "claimer-rig"); err != nil {
			t.Fatalf("ClaimWanted() error: %v", err)
		}

		got, _ := store.QueryWanted("w-conf02")
		if got.Status != "claimed" {
			t.Errorf("Status = %q, want %q", got.Status, "claimed")
		}
		if got.ClaimedBy != "claimer-rig" {
			t.Errorf("ClaimedBy = %q, want %q", got.ClaimedBy, "claimer-rig")
		}
	})

	t.Run("ClaimNonOpenItem", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_ = store.InsertWanted(&WantedItem{ID: "w-conf03", Title: "Already claimed"})
		_ = store.ClaimWanted("w-conf03", "rig-1")

		// Second claim on non-open item
		err := store.ClaimWanted("w-conf03", "rig-2")
		// The fake returns an error; the real SQL silently succeeds (0 rows affected).
		// Conformance: we verify the item is still claimed by rig-1 either way.
		_ = err

		got, _ := store.QueryWanted("w-conf03")
		if got.ClaimedBy != "rig-1" {
			t.Errorf("ClaimedBy = %q, want %q (should not be overwritten)", got.ClaimedBy, "rig-1")
		}
	})

	t.Run("SubmitCompletionLifecycle", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_ = store.InsertWanted(&WantedItem{ID: "w-conf04", Title: "Completable"})
		_ = store.ClaimWanted("w-conf04", "worker-rig")

		if err := store.SubmitCompletion("c-conf01", "w-conf04", "worker-rig", "https://pr/1"); err != nil {
			t.Fatalf("SubmitCompletion() error: %v", err)
		}

		got, _ := store.QueryWanted("w-conf04")
		if got.Status != "in_review" {
			t.Errorf("Status = %q, want %q", got.Status, "in_review")
		}
	})

	t.Run("QueryNotFound", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_, err := store.QueryWanted("w-nonexistent")
		if err == nil {
			t.Fatal("QueryWanted() expected error for missing item")
		}
	})

	t.Run("InsertEmptyIDFails", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		err := store.InsertWanted(&WantedItem{ID: "", Title: "No ID"})
		if err == nil {
			t.Fatal("InsertWanted() expected error for empty ID")
		}
	})

	t.Run("InsertEmptyTitleFails", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		err := store.InsertWanted(&WantedItem{ID: "w-nope", Title: ""})
		if err == nil {
			t.Fatal("InsertWanted() expected error for empty title")
		}
	})

	t.Run("EnsureDBIdempotent", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.EnsureDB(); err != nil {
			t.Fatalf("first EnsureDB() error: %v", err)
		}
		if err := store.EnsureDB(); err != nil {
			t.Fatalf("second EnsureDB() error: %v", err)
		}
	})

	t.Run("DatabaseExistsAfterEnsure", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_ = store.EnsureDB()
		if !store.DatabaseExists(WLCommonsDB) {
			t.Error("DatabaseExists() = false after EnsureDB()")
		}
	})

	t.Run("DefaultStatusIsOpen", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_ = store.InsertWanted(&WantedItem{ID: "w-conf05", Title: "Default status"})
		got, _ := store.QueryWanted("w-conf05")
		if got.Status != "open" {
			t.Errorf("default Status = %q, want %q", got.Status, "open")
		}
	})

	t.Run("InsertWithExplicitStatus", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_ = store.InsertWanted(&WantedItem{ID: "w-conf06", Title: "Explicit status", Status: "withdrawn"})
		got, _ := store.QueryWanted("w-conf06")
		if got.Status != "withdrawn" {
			t.Errorf("explicit Status = %q, want %q", got.Status, "withdrawn")
		}
	})

	t.Run("ClaimSetsClaimedBy", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		_ = store.InsertWanted(&WantedItem{ID: "w-conf07", Title: "Check claimer"})
		_ = store.ClaimWanted("w-conf07", "specific-rig")

		got, _ := store.QueryWanted("w-conf07")
		if !strings.Contains(got.ClaimedBy, "specific-rig") {
			t.Errorf("ClaimedBy = %q, want to contain %q", got.ClaimedBy, "specific-rig")
		}
	})
}

// TestFakeWLCommonsStore_Conformance runs the conformance suite against the fake.
func TestFakeWLCommonsStore_Conformance(t *testing.T) {
	wlCommonsConformance(t, func(t *testing.T) WLCommonsStore {
		return NewFakeWLCommonsStore()
	})
}
