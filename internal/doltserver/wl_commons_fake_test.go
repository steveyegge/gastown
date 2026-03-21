package doltserver

import (
	"fmt"
	"sync"
)

// fakeWLCommonsStore is an in-memory implementation of WLCommonsStore for testing.
// It enforces the same business rules as the real SQL implementation.
type fakeWLCommonsStore struct {
	mu     sync.Mutex
	items  map[string]*WantedItem
	stamps []StampRecord
	badges []BadgeRecord
	dbOK   bool

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

func (f *fakeWLCommonsStore) DatabaseExists(dbName string) bool {
	return f.dbOK && dbName == WLCommonsDB
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

func (f *fakeWLCommonsStore) QueryWantedFull(wantedID string) (*WantedItem, error) {
	return f.QueryWanted(wantedID)
}

func (f *fakeWLCommonsStore) InsertStamp(stamp *StampRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stamps = append(f.stamps, *stamp)
	return nil
}

func (f *fakeWLCommonsStore) QueryLastStampForSubject(subject string) (*StampRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.stamps) - 1; i >= 0; i-- {
		if f.stamps[i].Subject == subject {
			cp := f.stamps[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeWLCommonsStore) QueryStampsForSubject(subject string) ([]StampRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []StampRecord
	for _, s := range f.stamps {
		if s.Subject == subject {
			result = append(result, s)
		}
	}
	return result, nil
}

func (f *fakeWLCommonsStore) QueryBadges(handle string) ([]BadgeRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []BadgeRecord
	for _, b := range f.badges {
		result = append(result, b)
	}
	return result, nil
}

func (f *fakeWLCommonsStore) QueryAllSubjects() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := make(map[string]bool)
	var subjects []string
	for _, s := range f.stamps {
		if !seen[s.Subject] {
			seen[s.Subject] = true
			subjects = append(subjects, s.Subject)
		}
	}
	return subjects, nil
}

func (f *fakeWLCommonsStore) UpsertLeaderboard(entry *LeaderboardEntry) error {
	return nil
}
