package cmd

import (
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/doltserver"
)

func TestReleaseExpiredClaims_ReleasesOldClaims(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	// Insert and claim an item
	_ = store.InsertWanted(&doltserver.WantedItem{
		ID:    "w-old",
		Title: "Old claim",
	})
	_ = store.ClaimWanted("w-old", "stale-rig")

	// Backdate the claimed_at to 4 days ago
	store.mu.Lock()
	old := time.Now().Add(-96 * time.Hour)
	store.items["w-old"].ClaimedAt = &old
	store.mu.Unlock()

	// Release with 72h timeout
	released, err := store.ReleaseExpiredClaims(72 * time.Hour)
	if err != nil {
		t.Fatalf("ReleaseExpiredClaims() error: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want 1", released)
	}

	// Verify item is now open
	item, _ := store.QueryWanted("w-old")
	if item.Status != "open" {
		t.Errorf("Status = %q, want %q", item.Status, "open")
	}
	if item.ClaimedBy != "" {
		t.Errorf("ClaimedBy = %q, want empty", item.ClaimedBy)
	}
	if item.ClaimedAt != nil {
		t.Errorf("ClaimedAt = %v, want nil", item.ClaimedAt)
	}
}

func TestReleaseExpiredClaims_KeepsFreshClaims(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	// Insert and claim an item (claimed just now)
	_ = store.InsertWanted(&doltserver.WantedItem{
		ID:    "w-fresh",
		Title: "Fresh claim",
	})
	_ = store.ClaimWanted("w-fresh", "active-rig")

	// Release with 72h timeout — should not release
	released, err := store.ReleaseExpiredClaims(72 * time.Hour)
	if err != nil {
		t.Fatalf("ReleaseExpiredClaims() error: %v", err)
	}
	if released != 0 {
		t.Errorf("released = %d, want 0", released)
	}

	// Verify item is still claimed
	item, _ := store.QueryWanted("w-fresh")
	if item.Status != "claimed" {
		t.Errorf("Status = %q, want %q", item.Status, "claimed")
	}
	if item.ClaimedBy != "active-rig" {
		t.Errorf("ClaimedBy = %q, want %q", item.ClaimedBy, "active-rig")
	}
}

func TestReleaseExpiredClaims_MixedItems(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	// One old claim, one fresh claim, one open item
	_ = store.InsertWanted(&doltserver.WantedItem{ID: "w-old", Title: "Old"})
	_ = store.ClaimWanted("w-old", "stale-rig")
	store.mu.Lock()
	old := time.Now().Add(-96 * time.Hour)
	store.items["w-old"].ClaimedAt = &old
	store.mu.Unlock()

	_ = store.InsertWanted(&doltserver.WantedItem{ID: "w-fresh", Title: "Fresh"})
	_ = store.ClaimWanted("w-fresh", "active-rig")

	_ = store.InsertWanted(&doltserver.WantedItem{ID: "w-open", Title: "Open"})

	released, err := store.ReleaseExpiredClaims(72 * time.Hour)
	if err != nil {
		t.Fatalf("ReleaseExpiredClaims() error: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want 1", released)
	}

	// w-old should be open, w-fresh still claimed, w-open still open
	oldItem, _ := store.QueryWanted("w-old")
	if oldItem.Status != "open" {
		t.Errorf("w-old Status = %q, want open", oldItem.Status)
	}
	freshItem, _ := store.QueryWanted("w-fresh")
	if freshItem.Status != "claimed" {
		t.Errorf("w-fresh Status = %q, want claimed", freshItem.Status)
	}
	openItem, _ := store.QueryWanted("w-open")
	if openItem.Status != "open" {
		t.Errorf("w-open Status = %q, want open", openItem.Status)
	}
}

func TestQueryExpiredClaims_ReturnsOnlyExpired(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	_ = store.InsertWanted(&doltserver.WantedItem{ID: "w-old", Title: "Old"})
	_ = store.ClaimWanted("w-old", "stale-rig")
	store.mu.Lock()
	old := time.Now().Add(-96 * time.Hour)
	store.items["w-old"].ClaimedAt = &old
	store.mu.Unlock()

	_ = store.InsertWanted(&doltserver.WantedItem{ID: "w-fresh", Title: "Fresh"})
	_ = store.ClaimWanted("w-fresh", "active-rig")

	expired, err := store.QueryExpiredClaims(72 * time.Hour)
	if err != nil {
		t.Fatalf("QueryExpiredClaims() error: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expired count = %d, want 1", len(expired))
	}
	if expired[0].ID != "w-old" {
		t.Errorf("expired[0].ID = %q, want %q", expired[0].ID, "w-old")
	}
}

func TestLifecycle_ClaimReleaseReclaim(t *testing.T) {
	t.Parallel()
	store := newFakeWLCommonsStore()

	// Post an item
	_ = store.InsertWanted(&doltserver.WantedItem{
		ID:    "w-cycle",
		Title: "Claim cycle test",
	})

	// First rig claims it
	_, err := claimWanted(store, "w-cycle", "rig-1")
	if err != nil {
		t.Fatalf("first claim error: %v", err)
	}

	// Backdate claim to expire it
	store.mu.Lock()
	old := time.Now().Add(-96 * time.Hour)
	store.items["w-cycle"].ClaimedAt = &old
	store.mu.Unlock()

	// Sweep releases it
	released, err := store.ReleaseExpiredClaims(72 * time.Hour)
	if err != nil {
		t.Fatalf("sweep error: %v", err)
	}
	if released != 1 {
		t.Errorf("released = %d, want 1", released)
	}

	// Second rig can now claim it
	_, err = claimWanted(store, "w-cycle", "rig-2")
	if err != nil {
		t.Fatalf("second claim error: %v", err)
	}

	item, _ := store.QueryWanted("w-cycle")
	if item.ClaimedBy != "rig-2" {
		t.Errorf("ClaimedBy = %q, want %q", item.ClaimedBy, "rig-2")
	}
}
