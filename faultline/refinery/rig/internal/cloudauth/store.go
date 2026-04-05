// Package cloudauth provides account management for cloud mode using SQLite.
package cloudauth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// Account represents a cloud user account.
type Account struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"` // owner, admin, member, viewer
	CreatedAt string `json:"created_at"`
}

// Store manages cloud accounts and sessions in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore opens the SQLite database at path and runs migrations.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if err := migrateAccounts(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrateAccounts(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS accounts (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			email         TEXT UNIQUE NOT NULL,
			name          TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'member',
			created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);
		CREATE TABLE IF NOT EXISTS auth_sessions (
			token      TEXT PRIMARY KEY,
			account_id INTEGER NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_account ON auth_sessions (account_id);
	`)
	return err
}

// AccountCount returns the number of accounts.
func (s *Store) AccountCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts`).Scan(&n)
	return n, err
}

// CreateAccount creates a new account with a bcrypt-hashed password.
func (s *Store) CreateAccount(ctx context.Context, email, name, password, role string) (*Account, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO accounts (email, name, password_hash, role) VALUES (?, ?, ?, ?)`,
		email, name, string(hash), role,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Account{
		ID:        id,
		Email:     email,
		Name:      name,
		Role:      role,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Authenticate checks email/password and returns the account if valid.
func (s *Store) Authenticate(ctx context.Context, email, password string) (*Account, error) {
	var a Account
	var hash string
	err := s.db.QueryRowContext(ctx,
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
func (s *Store) CreateSession(ctx context.Context, accountID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	expires := time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO auth_sessions (token, account_id, expires_at) VALUES (?, ?, ?)`,
		token, accountID, expires,
	); err != nil {
		return "", err
	}
	return token, nil
}

// GetSession validates a session token and returns the associated account.
func (s *Store) GetSession(ctx context.Context, token string) (*Account, error) {
	var accountID int64
	var expiresAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT account_id, expires_at FROM auth_sessions WHERE token = ?`, token,
	).Scan(&accountID, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	if time.Now().After(exp) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token = ?`, token)
		return nil, nil
	}
	var a Account
	err = s.db.QueryRowContext(ctx,
		`SELECT id, email, name, role, created_at FROM accounts WHERE id = ?`, accountID,
	).Scan(&a.ID, &a.Email, &a.Name, &a.Role, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}
