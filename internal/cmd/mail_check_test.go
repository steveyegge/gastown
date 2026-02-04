package cmd

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMailCheckCacheRoundTrip(t *testing.T) {
	mailCheckCacheDir = t.TempDir()
	defer func() { mailCheckCacheDir = "" }()

	address := "mayor/"

	// No cache initially
	if got := loadMailCheckCache(address); got != nil {
		t.Fatal("expected nil cache, got entry")
	}

	// Save and reload
	entry := &mailCheckCacheEntry{
		Timestamp: time.Now(),
		Address:   address,
		Unread:    3,
		Subjects:  []string{"- hq--abc from deacon/: test"},
	}
	saveMailCheckCache(entry)

	got := loadMailCheckCache(address)
	if got == nil {
		t.Fatal("expected cached entry, got nil")
	}
	if got.Unread != 3 {
		t.Errorf("unread = %d, want 3", got.Unread)
	}
	if len(got.Subjects) != 1 {
		t.Errorf("subjects len = %d, want 1", len(got.Subjects))
	}
}

func TestMailCheckCacheExpiry(t *testing.T) {
	mailCheckCacheDir = t.TempDir()
	defer func() { mailCheckCacheDir = "" }()

	address := "mayor/"

	// Save an old entry
	entry := &mailCheckCacheEntry{
		Timestamp: time.Now().Add(-60 * time.Second), // 60s ago, past 30s TTL
		Address:   address,
		Unread:    1,
	}
	saveMailCheckCache(entry)

	// Should be expired
	if got := loadMailCheckCache(address); got != nil {
		t.Fatal("expected expired cache to return nil")
	}
}

func TestMailCheckCacheAddressMismatch(t *testing.T) {
	mailCheckCacheDir = t.TempDir()
	defer func() { mailCheckCacheDir = "" }()

	// Save entry for one address
	entry := &mailCheckCacheEntry{
		Timestamp: time.Now(),
		Address:   "deacon/",
		Unread:    1,
	}
	saveMailCheckCache(entry)

	// Try to load for a different address
	if got := loadMailCheckCache("mayor/"); got != nil {
		t.Fatal("expected nil for different address")
	}
}

func TestMailCheckCachePath(t *testing.T) {
	mailCheckCacheDir = t.TempDir()
	defer func() { mailCheckCacheDir = "" }()

	path := mailCheckCachePath("mayor/")
	expected := filepath.Join(mailCheckCacheDir, "mayor_.json")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

func TestMailCheckCacheZeroUnread(t *testing.T) {
	mailCheckCacheDir = t.TempDir()
	defer func() { mailCheckCacheDir = "" }()

	address := "mayor/"

	entry := &mailCheckCacheEntry{
		Timestamp: time.Now(),
		Address:   address,
		Unread:    0,
	}
	saveMailCheckCache(entry)

	got := loadMailCheckCache(address)
	if got == nil {
		t.Fatal("expected cached entry for zero unread, got nil")
	}
	if got.Unread != 0 {
		t.Errorf("unread = %d, want 0", got.Unread)
	}
}
