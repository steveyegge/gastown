package cmd

import (
	"fmt"
	"sync"

	"github.com/steveyegge/gastown/internal/doltserver"
)

// fakeWLCommonsStore is a local in-memory WLCommonsStore for cmd package tests.
// Duplicated from doltserver's test fake following the codebase convention
// of per-package private mocks (see mockTmux in deacon, quota, doctor).
type fakeWLCommonsStore struct {
	mu    sync.Mutex
	items map[string]*doltserver.WantedItem
	dbOK  bool

	// Error injection fields
	EnsureDBErr         error
	InsertWantedErr     error
	ClaimWantedErr      error
	SubmitCompletionErr error
	QueryWantedErr      error
}

func newFakeWLCommonsStore() *fakeWLCommonsStore {
	return &fakeWLCommonsStore{
		items: make(map[string]*doltserver.WantedItem),
		dbOK:  true,
	}
}

func (f *fakeWLCommonsStore) EnsureDB() error {
	if f.EnsureDBErr != nil {
		return f.EnsureDBErr
	}
	f.dbOK = true
	return nil
}

func (f *fakeWLCommonsStore) DatabaseExists(dbName string) bool {
	return f.dbOK && dbName == doltserver.WLCommonsDB
}

func (f *fakeWLCommonsStore) InsertWanted(item *doltserver.WantedItem) error {
	if f.InsertWantedErr != nil {
		return f.InsertWantedErr
	}
	if item.ID == "" {
		return fmt.Errorf("wanted item ID cannot be empty")
	}
	if item.Title == "" {
		return fmt.Errorf("wanted item title cannot be empty")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.items[item.ID]; exists {
		return fmt.Errorf("duplicate wanted ID %q", item.ID)
	}

	stored := *item
	if stored.Status == "" {
		stored.Status = "open"
	}
	f.items[item.ID] = &stored
	return nil
}

func (f *fakeWLCommonsStore) ClaimWanted(wantedID, rigHandle string) error {
	if f.ClaimWantedErr != nil {
		return f.ClaimWantedErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	item, ok := f.items[wantedID]
	if !ok {
		return fmt.Errorf("wanted item %q not found", wantedID)
	}
	if item.Status != "open" {
		return fmt.Errorf("wanted item %q is not open (status: %s)", wantedID, item.Status)
	}
	item.Status = "claimed"
	item.ClaimedBy = rigHandle
	return nil
}

func (f *fakeWLCommonsStore) SubmitCompletion(completionID, wantedID, rigHandle, evidence string) error {
	if f.SubmitCompletionErr != nil {
		return f.SubmitCompletionErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	item, ok := f.items[wantedID]
	if !ok {
		return fmt.Errorf("wanted item %q not found", wantedID)
	}
	if item.Status != "claimed" {
		return fmt.Errorf("wanted item %q is not claimed (status: %s)", wantedID, item.Status)
	}
	if item.ClaimedBy != rigHandle {
		return fmt.Errorf("wanted item %q is not claimed by %q (claimed by %q)", wantedID, rigHandle, item.ClaimedBy)
	}
	item.Status = "in_review"
	return nil
}

func (f *fakeWLCommonsStore) QueryWanted(wantedID string) (*doltserver.WantedItem, error) {
	if f.QueryWantedErr != nil {
		return nil, f.QueryWantedErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	item, ok := f.items[wantedID]
	if !ok {
		return nil, fmt.Errorf("wanted item %q not found", wantedID)
	}
	cp := *item
	return &cp, nil
}
