package db

import (
	"context"
	"os"
	"testing"
)

func openAccountsTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_accounts_test?parseTime=true"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.ExecContext(context.Background(), "DELETE FROM auth_sessions")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM accounts")
		_ = d.Close()
	})
	return d
}

func TestOwnerCount(t *testing.T) {
	d := openAccountsTestDB(t)
	ctx := context.Background()

	// No accounts → 0 owners.
	count, err := d.OwnerCount(ctx)
	if err != nil {
		t.Fatalf("owner count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 owners, got %d", count)
	}

	// Create one owner and one member.
	_, _ = d.CreateAccount(ctx, "owner@test.com", "Owner", "pass", "owner")
	_, _ = d.CreateAccount(ctx, "member@test.com", "Member", "pass", "member")

	count, err = d.OwnerCount(ctx)
	if err != nil {
		t.Fatalf("owner count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 owner, got %d", count)
	}
}

func TestUpdateAccountRole(t *testing.T) {
	d := openAccountsTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "user@test.com", "User", "pass", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Update to admin.
	err = d.UpdateAccountRole(ctx, acct.ID, "admin")
	if err != nil {
		t.Fatalf("update role: %v", err)
	}

	// Verify via ListAccounts.
	accounts, err := d.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].Role != "admin" {
		t.Errorf("expected role 'admin', got %q", accounts[0].Role)
	}
}

func TestUpdateAccountRoleInvalidRole(t *testing.T) {
	d := openAccountsTestDB(t)
	ctx := context.Background()

	acct, _ := d.CreateAccount(ctx, "user@test.com", "User", "pass", "member")

	err := d.UpdateAccountRole(ctx, acct.ID, "superuser")
	if err == nil {
		t.Error("expected error for invalid role, got nil")
	}
}
