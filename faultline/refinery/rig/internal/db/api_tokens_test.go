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
		_, _ = d.ExecContext(context.Background(), "DELETE FROM api_tokens")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM auth_sessions")
		_, _ = d.ExecContext(context.Background(), "DELETE FROM accounts")
		_ = d.Close()
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
	plaintext, token, err := d.CreateAPIToken(ctx, acct.ID, nil, "member", "ci-token", nil)
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
	if token.Role != "member" {
		t.Errorf("expected role 'member', got %q", token.Role)
	}
	if token.Prefix == "" {
		t.Error("expected non-empty prefix")
	}

	// Validate the token.
	result, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Account.ID != acct.ID {
		t.Errorf("expected account %d, got %d", acct.ID, result.Account.ID)
	}
	if result.TokenRole != "member" {
		t.Errorf("expected token role 'member', got %q", result.TokenRole)
	}

	// Invalid token should return nil.
	result, err = d.ValidateAPIToken(ctx, "fl_invalid_token_value")
	if err != nil {
		t.Fatalf("validate invalid: %v", err)
	}
	if result != nil {
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
	plaintext, token, err := d.CreateAPIToken(ctx, acct.ID, &projectID, "viewer", "project-token", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token.ProjectID == nil || *token.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %v", token.ProjectID)
	}
	if token.Role != "viewer" {
		t.Errorf("expected role 'viewer', got %q", token.Role)
	}

	// Validate returns project scope and token role.
	result, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.TokenRole != "viewer" {
		t.Errorf("expected token role 'viewer', got %q", result.TokenRole)
	}
	if result.ProjectID == nil || *result.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %v", result.ProjectID)
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
	plaintext, _, err := d.CreateAPIToken(ctx, acct.ID, nil, "member", "expired-token", &past)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Validation should fail.
	result, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result != nil {
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

	plaintext, token, err := d.CreateAPIToken(ctx, acct.ID, nil, "member", "to-revoke", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Revoke the token.
	if err := d.RevokeAPIToken(ctx, token.ID, acct.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Validation should fail.
	result, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result != nil {
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
	_, _, err = d.CreateAPIToken(ctx, acct.ID, nil, "member", "token-1", nil)
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	_, tok2, err := d.CreateAPIToken(ctx, acct.ID, nil, "member", "token-2", nil)
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
	if err := d.RevokeAPIToken(ctx, tok2.ID, acct.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

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

	_, token, err := d.CreateAPIToken(ctx, acct1.ID, nil, "member", "my-token", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// acct2 should not be able to revoke acct1's token.
	if err := d.RevokeAPIToken(ctx, token.ID, acct2.ID); err == nil {
		t.Error("expected error revoking another account's token")
	}
}

func TestAPITokenRoleScoping(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	// Admin account creates a viewer-scoped token.
	acct, err := d.CreateAccount(ctx, "admin@role.test", "Admin", "password123", "admin")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	plaintext, token, err := d.CreateAPIToken(ctx, acct.ID, nil, "viewer", "read-only", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token.Role != "viewer" {
		t.Errorf("expected token role 'viewer', got %q", token.Role)
	}

	result, err := d.ValidateAPIToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// Account role is admin, token role is viewer.
	if result.Account.Role != "admin" {
		t.Errorf("expected account role 'admin', got %q", result.Account.Role)
	}
	if result.TokenRole != "viewer" {
		t.Errorf("expected token role 'viewer', got %q", result.TokenRole)
	}
	if result.ProjectID != nil {
		t.Errorf("expected nil project_id, got %v", result.ProjectID)
	}
}

func TestAPITokenDefaultRole(t *testing.T) {
	d := openTokenTestDB(t)
	ctx := context.Background()

	acct, err := d.CreateAccount(ctx, "default@role.test", "Default", "password123", "owner")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Empty role should default to member.
	_, token, err := d.CreateAPIToken(ctx, acct.ID, nil, "", "default-role", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token.Role != "member" {
		t.Errorf("expected default role 'member', got %q", token.Role)
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
