package db

import (
	"context"
	"os"
	"testing"
)

func openNotifTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_notif_test?parseTime=true"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.ExecContext(context.Background(), "DELETE FROM notifications")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM auth_sessions")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM accounts")
		_ = d.Close()
	})
	return d
}

func TestCreateAndListNotifications(t *testing.T) {
	d := openNotifTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "user@test.com", "Test User", "pass123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	actor, err := d.CreateAccount(ctx, "actor@test.com", "Actor", "pass123", "member")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	// Create notifications.
	n1, err := d.CreateNotification(ctx, acct.ID, "mention", "issue-1", nil, actor.ID, actor.Name, "You were mentioned")
	if err != nil {
		t.Fatalf("create notification 1: %v", err)
	}
	if n1.ID == "" {
		t.Fatal("expected non-empty notification ID")
	}
	if n1.Type != "mention" {
		t.Errorf("expected type 'mention', got %q", n1.Type)
	}

	commentID := "comment-123"
	n2, err := d.CreateNotification(ctx, acct.ID, "comment", "issue-1", &commentID, actor.ID, actor.Name, "New comment on issue")
	if err != nil {
		t.Fatalf("create notification 2: %v", err)
	}
	if n2.CommentID == nil || *n2.CommentID != commentID {
		t.Errorf("expected comment_id %q, got %v", commentID, n2.CommentID)
	}

	// List notifications — newest first.
	list, err := d.ListNotifications(ctx, acct.ID, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(list))
	}
	// n2 created after n1, should be first.
	if list[0].ID != n2.ID {
		t.Errorf("expected newest first, got %s", list[0].ID)
	}
}

func TestUnreadNotificationCount(t *testing.T) {
	d := openNotifTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "count@test.com", "Counter", "pass123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	actor, err := d.CreateAccount(ctx, "actor2@test.com", "Actor2", "pass123", "member")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	// Initially zero.
	count, err := d.UnreadNotificationCount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread, got %d", count)
	}

	// Create two notifications.
	_, _ = d.CreateNotification(ctx, acct.ID, "mention", "issue-1", nil, actor.ID, actor.Name, "Mentioned")
	n2, _ := d.CreateNotification(ctx, acct.ID, "assignment", "issue-2", nil, actor.ID, actor.Name, "Assigned")

	count, err = d.UnreadNotificationCount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unread, got %d", count)
	}

	// Mark one as read.
	if err := d.MarkNotificationsRead(ctx, acct.ID, []string{n2.ID}); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	count, err = d.UnreadNotificationCount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 unread, got %d", count)
	}
}

func TestMarkAllNotificationsRead(t *testing.T) {
	d := openNotifTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "all@test.com", "All Reader", "pass123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	actor, err := d.CreateAccount(ctx, "actor3@test.com", "Actor3", "pass123", "member")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	_, _ = d.CreateNotification(ctx, acct.ID, "mention", "issue-1", nil, actor.ID, actor.Name, "One")
	_, _ = d.CreateNotification(ctx, acct.ID, "comment", "issue-2", nil, actor.ID, actor.Name, "Two")

	if err := d.MarkAllNotificationsRead(ctx, acct.ID); err != nil {
		t.Fatalf("mark all read: %v", err)
	}

	count, err := d.UnreadNotificationCount(ctx, acct.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread after mark-all, got %d", count)
	}

	// Verify read_at is set.
	list, err := d.ListNotifications(ctx, acct.ID, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, n := range list {
		if n.ReadAt == nil {
			t.Errorf("notification %s should have read_at set", n.ID)
		}
	}
}

func TestMarkNotificationsRead_Empty(t *testing.T) {
	d := openNotifTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "empty@test.com", "Empty", "pass123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Should not error with empty slice.
	if err := d.MarkNotificationsRead(ctx, acct.ID, nil); err != nil {
		t.Fatalf("mark empty: %v", err)
	}
}

func TestAccountsByNames(t *testing.T) {
	d := openNotifTestDB(t)
	ctx := context.Background()

	_, err := d.CreateAccount(ctx, "alice@test.com", "Alice", "pass123", "member")
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	_, err = d.CreateAccount(ctx, "bob@test.com", "Bob", "pass123", "member")
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	// Lookup by name (case-insensitive).
	accounts, err := d.AccountsByNames(ctx, []string{"alice", "Bob"})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	// Non-existent name returns empty.
	accounts, err = d.AccountsByNames(ctx, []string{"nobody"})
	if err != nil {
		t.Fatalf("lookup nobody: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts for 'nobody', got %d", len(accounts))
	}

	// Empty input returns nil.
	accounts, err = d.AccountsByNames(ctx, nil)
	if err != nil {
		t.Fatalf("lookup nil: %v", err)
	}
	if accounts != nil {
		t.Errorf("expected nil for empty input, got %v", accounts)
	}
}
