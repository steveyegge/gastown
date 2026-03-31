package db

import (
	"context"
	"os"
	"testing"
	"time"
)

func openTokenTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_token_test?parseTime=true"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		d.ExecContext(context.Background(), "DELETE FROM api_tokens")
		d.ExecContext(context.Background(), "DELETE FROM auth_sessions")
		d.ExecContext(context.Background(), "DELETE FROM accounts")
		d.Close()
	})
	return d
}

func TestCreateAndValidateAPIToken(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	// Create an account to own the token.
	acct, err := d.CreateAccount(ctx, "agent@test.com", "Agent", "password123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Create a token with no expiry and no project scope.
	plaintext, token, err := d.CreateAPIToken(ctx, acct.ID, nil, "ci-token", nil)
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}

	if plaintext == "" {
		t.Fatal("expected non-empty plaintext token")
	}
	if len(plaintext) < 20 {
		t.Errorf("token too short: %d chars", len(plaintext))
	}
	if plaintext[:3] != "fl_" {
		t.Errorf("expected fl_ prefix, got %q", plaintext[:3])
	}
	if token.Name != "ci-token" {
		t.Errorf("expected name 'ci-token', got %q", token.Name)
	}
	if token.Prefix == "" {
		t.Error("expected non-empty prefix")
	}

	// Validate the token.
	account, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if account == nil {
		t.Fatal("expected account, got nil")
	}
	if account.ID != acct.ID {
		t.Errorf("expected account %d, got %d", acct.ID, account.ID)
	}

	// Invalid token should return nil.
	account, err = d.ValidateAPIToken(ctx, "fl_invalid_token_value")
	if err != nil {
		t.Fatalf("validate invalid: %v", err)
	}
	if account != nil {
		t.Error("expected nil for invalid token")
	}
}

func TestAPITokenWithProjectScope(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "scoped@test.com", "Scoped", "password123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	projectID := int64(42)
	_, token, err := d.CreateAPIToken(ctx, acct.ID, &projectID, "project-token", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token.ProjectID == nil || *token.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %v", token.ProjectID)
	}
}

func TestAPITokenExpiry(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "expiry@test.com", "Expiry", "password123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Create a token that already expired.
	past := time.Now().UTC().Add(-1 * time.Hour)
	plaintext, _, err := d.CreateAPIToken(ctx, acct.ID, nil, "expired-token", &past)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Validation should fail.
	account, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if account != nil {
		t.Error("expected nil for expired token")
	}
}

func TestAPITokenRevoke(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "revoke@test.com", "Revoke", "password123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	plaintext, token, err := d.CreateAPIToken(ctx, acct.ID, nil, "to-revoke", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Revoke the token.
	if err := d.RevokeAPIToken(ctx, token.ID, acct.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Validation should fail.
	account, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if account != nil {
		t.Error("expected nil for revoked token")
	}

	// Revoking again should fail.
	if err := d.RevokeAPIToken(ctx, token.ID, acct.ID); err == nil {
		t.Error("expected error revoking already-revoked token")
	}
}

func TestListAPITokens(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "list@test.com", "List", "password123", "member")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Create multiple tokens.
	_, _, err = d.CreateAPIToken(ctx, acct.ID, nil, "token-1", nil)
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	_, tok2, err := d.CreateAPIToken(ctx, acct.ID, nil, "token-2", nil)
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}

	tokens, err := d.ListAPITokens(ctx, acct.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}

	// Revoke one.
	d.RevokeAPIToken(ctx, tok2.ID, acct.ID)

	tokens, err = d.ListAPITokens(ctx, acct.ID)
	if err != nil {
		t.Fatalf("list after revoke: %v", err)
	}
	if len(tokens) != 1 {
		t.Errorf("expected 1 active token, got %d", len(tokens))
	}
}

func TestRevokeTokenWrongAccount(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	acct1, err := d.CreateAccount(ctx, "owner@test.com", "Owner", "password123", "member")
	if err != nil {
		t.Fatalf("create account 1: %v", err)
	}
	acct2, err := d.CreateAccount(ctx, "other@test.com", "Other", "password123", "member")
	if err != nil {
		t.Fatalf("create account 2: %v", err)
	}

	_, token, err := d.CreateAPIToken(ctx, acct1.ID, nil, "my-token", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// acct2 should not be able to revoke acct1's token.
	if err := d.RevokeAPIToken(ctx, token.ID, acct2.ID); err == nil {
		t.Error("expected error revoking another account's token")
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	h1 := hashToken("fl_abc123")
	h2 := hashToken("fl_abc123")
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q != %q", h1, h2)
	}
	h3 := hashToken("fl_different")
	if h1 == h3 {
		t.Error("different tokens should have different hashes")
	}
}
