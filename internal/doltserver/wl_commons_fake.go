package doltserver

import (
	"fmt"
	"sync"
)

// FakeWLCommonsStore is an in-memory implementation of WLCommonsStore for testing.
type FakeWLCommonsStore struct {
	mu    sync.Mutex
	items map[string]*WantedItem
	dbOK  bool

	// Error injection fields
	EnsureDBErr         error
	InsertWantedErr     error
	ClaimWantedErr      error
	SubmitCompletionErr error
	QueryWantedErr      error
}

// NewFakeWLCommonsStore creates a ready-to-use fake store.
func NewFakeWLCommonsStore() *FakeWLCommonsStore {
	return &FakeWLCommonsStore{
		items: make(map[string]*WantedItem),
		dbOK:  true,
	}
}

func (f *FakeWLCommonsStore) EnsureDB() error {
	if f.EnsureDBErr != nil {
		return f.EnsureDBErr
	}
	f.dbOK = true
	return nil
}

func (f *FakeWLCommonsStore) DatabaseExists(_ string) bool {
	return f.dbOK
}

func (f *FakeWLCommonsStore) InsertWanted(item *WantedItem) error {
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

	// Copy to avoid aliasing
	stored := *item
	if stored.Status == "" {
		stored.Status = "open"
	}
	f.items[item.ID] = &stored
	return nil
}

func (f *FakeWLCommonsStore) ClaimWanted(wantedID, rigHandle string) error {
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

func (f *FakeWLCommonsStore) SubmitCompletion(completionID, wantedID, rigHandle, evidence string) error {
	if f.SubmitCompletionErr != nil {
		return f.SubmitCompletionErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	item, ok := f.items[wantedID]
	if !ok {
		return fmt.Errorf("wanted item %q not found", wantedID)
	}
	item.Status = "in_review"
	_ = completionID
	_ = rigHandle
	_ = evidence
	return nil
}

func (f *FakeWLCommonsStore) QueryWanted(wantedID string) (*WantedItem, error) {
	if f.QueryWantedErr != nil {
		return nil, f.QueryWantedErr
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	item, ok := f.items[wantedID]
	if !ok {
		return nil, fmt.Errorf("wanted item %q not found", wantedID)
	}
	// Return a copy to avoid aliasing
	cp := *item
	return &cp, nil
}
