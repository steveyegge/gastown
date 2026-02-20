package doltserver

import (
	"fmt"
	"sync"
)

// fakeWLCommonsStore is an in-memory implementation of WLCommonsStore for testing.
// It enforces the same business rules as the real SQL implementation.
type fakeWLCommonsStore struct {
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

func newFakeWLCommonsStore() *fakeWLCommonsStore {
	return &fakeWLCommonsStore{
		items: make(map[string]*WantedItem),
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

func (f *fakeWLCommonsStore) DatabaseExists(_ string) bool {
	return f.dbOK
}

func (f *fakeWLCommonsStore) InsertWanted(item *WantedItem) error {
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
	item.Status = "in_review"
	_ = completionID
	_ = rigHandle
	_ = evidence
	return nil
}

func (f *fakeWLCommonsStore) QueryWanted(wantedID string) (*WantedItem, error) {
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
