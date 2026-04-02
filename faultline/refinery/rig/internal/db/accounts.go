package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Account represents a user account.
type Account struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"` // owner, admin, member, viewer
	CreatedAt time.Time `json:"created_at"`
}

// Session represents an active login session.
type Session struct {
	Token     string    `json:"token"`
	AccountID int64     `json:"account_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// migrateAccounts creates the accounts and auth_sessions tables.
func (d *DB) migrateAccounts(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id           BIGINT AUTO_INCREMENT PRIMARY KEY,
			email        VARCHAR(256) NOT NULL UNIQUE,
			name         VARCHAR(256) NOT NULL,
			password_hash VARCHAR(256) NOT NULL,
			role         VARCHAR(16) NOT NULL DEFAULT 'member',
			created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
		)`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			token        VARCHAR(64) PRIMARY KEY,
			account_id   BIGINT NOT NULL,
			expires_at   DATETIME(6) NOT NULL,
			INDEX idx_account (account_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate accounts: %w", err)
		}
	}
	return nil
}

// CreateAccount creates a new account with a bcrypt-hashed password.
func (d *DB) CreateAccount(ctx context.Context, email, name, password, role string) (*Account, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	res, err := d.ExecContext(ctx,
		`INSERT INTO accounts (email, name, password_hash, role) VALUES (?, ?, ?, ?)`,
		email, name, string(hash), role,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	d.MarkDirty()
	return &Account{ID: id, Email: email, Name: name, Role: role, CreatedAt: time.Now().UTC()}, nil
}

// Authenticate checks email/password and returns the account if valid.
func (d *DB) Authenticate(ctx context.Context, email, password string) (*Account, error) {
	var a Account
	var hash string
	err := d.QueryRowContext(ctx,
		`SELECT id, email, name, password_hash, role, created_at FROM accounts WHERE email = ?`, email,
	).Scan(&a.ID, &a.Email, &a.Name, &hash, &a.Role, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid credentials")
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &a, nil
}

// CreateSession creates a new session token for an account (30-day expiry).
func (d *DB) CreateSession(ctx context.Context, accountID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	expires := time.Now().UTC().Add(30 * 24 * time.Hour)
	if _, err := d.ExecContext(ctx,
		`INSERT INTO auth_sessions (token, account_id, expires_at) VALUES (?, ?, ?)`,
		token, accountID, expires,
	); err != nil {
		return "", err
	}
	d.MarkDirty()
	return token, nil
}

// GetSession validates a session token and returns the associated account.
func (d *DB) GetSession(ctx context.Context, token string) (*Account, error) {
	var s Session
	err := d.QueryRowContext(ctx,
		`SELECT token, account_id, expires_at FROM auth_sessions WHERE token = ?`, token,
	).Scan(&s.Token, &s.AccountID, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(s.ExpiresAt) {
		_, _ = d.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token = ?`, token)
		return nil, nil
	}
	var a Account
	err = d.QueryRowContext(ctx,
		`SELECT id, email, name, role, created_at FROM accounts WHERE id = ?`, s.AccountID,
	).Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// DeleteSession removes a session token (logout).
func (d *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token = ?`, token)
	return err
}

// ListAccounts returns all accounts ordered by name.
func (d *DB) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, email, name, role, created_at FROM accounts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

// AccountCount returns the number of accounts (used to detect first-run setup).
func (d *DB) AccountCount(ctx context.Context) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&n)
	return n, err
}

// OwnerCount returns the number of accounts with the "owner" role.
func (d *DB) OwnerCount(ctx context.Context) (int, error) {
	var n int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE role = 'owner'`).Scan(&n)
	return n, err
}

// validRoles is the set of allowed account roles.
var validRoles = map[string]bool{
	"viewer": true,
	"member": true,
	"admin":  true,
	"owner":  true,
}

// UpdateAccountRole changes an account's role.
func (d *DB) UpdateAccountRole(ctx context.Context, accountID int64, role string) error {
	if !validRoles[role] {
		return fmt.Errorf("invalid role: %q", role)
	}
	_, err := d.ExecContext(ctx, `UPDATE accounts SET role = ? WHERE id = ?`, role, accountID)
	if err != nil {
		return err
	}
	d.MarkDirty()
	return nil
}
