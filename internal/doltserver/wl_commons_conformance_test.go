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

		if err := store.InsertWanted(&WantedItem{ID: "w-conf02", Title: "Claimable"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		if err := store.ClaimWanted("w-conf02", "claimer-rig"); err != nil {
			t.Fatalf("ClaimWanted() error: %v", err)
		}

		got, err := store.QueryWanted("w-conf02")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
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

		if err := store.InsertWanted(&WantedItem{ID: "w-conf03", Title: "Already claimed"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		if err := store.ClaimWanted("w-conf03", "rig-1"); err != nil {
			t.Fatalf("first ClaimWanted() error: %v", err)
		}

		// Second claim on non-open item must return an error.
		// Both fake and real now enforce this: the real SQL checks
		// ROW_COUNT() after the UPDATE to detect 0 rows affected.
		err := store.ClaimWanted("w-conf03", "rig-2")
		if err == nil {
			t.Error("ClaimWanted on non-open item should return an error")
		}

		got, err := store.QueryWanted("w-conf03")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if got.ClaimedBy != "rig-1" {
			t.Errorf("ClaimedBy = %q, want %q (should not be overwritten)", got.ClaimedBy, "rig-1")
		}
	})

	t.Run("SubmitCompletionLifecycle", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf04", Title: "Completable"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		if err := store.ClaimWanted("w-conf04", "worker-rig"); err != nil {
			t.Fatalf("ClaimWanted() error: %v", err)
		}

		if err := store.SubmitCompletion("c-conf01", "w-conf04", "worker-rig", "https://pr/1"); err != nil {
			t.Fatalf("SubmitCompletion() error: %v", err)
		}

		got, err := store.QueryWanted("w-conf04")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
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

		if err := store.EnsureDB(); err != nil {
			t.Fatalf("EnsureDB() error: %v", err)
		}
		if !store.DatabaseExists(WLCommonsDB) {
			t.Error("DatabaseExists() = false after EnsureDB()")
		}
	})

	t.Run("DefaultStatusIsOpen", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf05", Title: "Default status"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		got, err := store.QueryWanted("w-conf05")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if got.Status != "open" {
			t.Errorf("default Status = %q, want %q", got.Status, "open")
		}
	})

	t.Run("InsertWithExplicitStatus", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf06", Title: "Explicit status", Status: "withdrawn"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		got, err := store.QueryWanted("w-conf06")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if got.Status != "withdrawn" {
			t.Errorf("explicit Status = %q, want %q", got.Status, "withdrawn")
		}
	})

	t.Run("SubmitCompletionOnOpenItem", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf08", Title: "Open item"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}

		// SubmitCompletion on an open (unclaimed) item should fail
		err := store.SubmitCompletion("c-conf02", "w-conf08", "some-rig", "https://pr/2")
		if err == nil {
			t.Error("SubmitCompletion on open item should return an error")
		}

		// Status should remain open
		got, err := store.QueryWanted("w-conf08")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if got.Status != "open" {
			t.Errorf("Status = %q, want %q (should not change on failed completion)", got.Status, "open")
		}
	})

	t.Run("SubmitCompletionByWrongRig", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf09", Title: "Wrong rig item"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		if err := store.ClaimWanted("w-conf09", "rig-alpha"); err != nil {
			t.Fatalf("ClaimWanted() error: %v", err)
		}

		// SubmitCompletion by a different rig should fail
		err := store.SubmitCompletion("c-conf03", "w-conf09", "rig-beta", "https://pr/3")
		if err == nil {
			t.Error("SubmitCompletion by wrong rig should return an error")
		}

		// Status should remain claimed
		got, err := store.QueryWanted("w-conf09")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if got.Status != "claimed" {
			t.Errorf("Status = %q, want %q (should not change on wrong-rig completion)", got.Status, "claimed")
		}
	})

	t.Run("SubmitCompletionAlreadyDone", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf11", Title: "Already done"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		if err := store.ClaimWanted("w-conf11", "worker-rig"); err != nil {
			t.Fatalf("ClaimWanted() error: %v", err)
		}
		if err := store.SubmitCompletion("c-conf04", "w-conf11", "worker-rig", "https://pr/4"); err != nil {
			t.Fatalf("first SubmitCompletion() error: %v", err)
		}

		// Second completion on an already in_review item must fail
		err := store.SubmitCompletion("c-conf05", "w-conf11", "worker-rig", "https://pr/5")
		if err == nil {
			t.Error("second SubmitCompletion on already-completed item should return an error")
		}

		// Status should remain in_review
		got, err := store.QueryWanted("w-conf11")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if got.Status != "in_review" {
			t.Errorf("Status = %q, want %q", got.Status, "in_review")
		}
	})

	t.Run("DatabaseExistsWrongName", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.EnsureDB(); err != nil {
			t.Fatalf("EnsureDB() error: %v", err)
		}
		if store.DatabaseExists("wrong_db_name") {
			t.Error("DatabaseExists(wrong_db_name) = true, want false")
		}
	})

	t.Run("InsertDuplicateIDFails", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf10", Title: "First insert"}); err != nil {
			t.Fatalf("first InsertWanted() error: %v", err)
		}

		err := store.InsertWanted(&WantedItem{ID: "w-conf10", Title: "Duplicate insert"})
		if err == nil {
			t.Fatal("InsertWanted() expected error for duplicate ID")
		}
	})

	t.Run("ClaimSetsClaimedBy", func(t *testing.T) {
		t.Parallel()
		store := newStore(t)

		if err := store.InsertWanted(&WantedItem{ID: "w-conf07", Title: "Check claimer"}); err != nil {
			t.Fatalf("InsertWanted() error: %v", err)
		}
		if err := store.ClaimWanted("w-conf07", "specific-rig"); err != nil {
			t.Fatalf("ClaimWanted() error: %v", err)
		}

		got, err := store.QueryWanted("w-conf07")
		if err != nil {
			t.Fatalf("QueryWanted() error: %v", err)
		}
		if !strings.Contains(got.ClaimedBy, "specific-rig") {
			t.Errorf("ClaimedBy = %q, want to contain %q", got.ClaimedBy, "specific-rig")
		}
	})
}

// TestFakeWLCommonsStore_Conformance runs the conformance suite against the fake.
func TestFakeWLCommonsStore_Conformance(t *testing.T) {
	wlCommonsConformance(t, func(t *testing.T) WLCommonsStore {
		return newFakeWLCommonsStore()
	})
}
