package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// APIToken represents a long-lived API token for agents and CI systems.
type APIToken struct {
	ID         int64      `json:"id"`
	AccountID  int64      `json:"account_id"`
	ProjectID  *int64     `json:"project_id,omitempty"` // nil = org-wide
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`      // first 8 chars, for identification
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"` // nil = no expiry
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// migrateAPITokens creates the api_tokens table.
func (d *DB) migrateAPITokens(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS api_tokens (
		id           BIGINT AUTO_INCREMENT PRIMARY KEY,
		account_id   BIGINT NOT NULL,
		project_id   BIGINT,
		name         VARCHAR(256) NOT NULL,
		token_hash   VARCHAR(64) NOT NULL UNIQUE,
		prefix       VARCHAR(12) NOT NULL,
		last_used_at DATETIME(6),
		expires_at   DATETIME(6),
		created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		revoked_at   DATETIME(6),
		INDEX idx_account (account_id),
		INDEX idx_hash (token_hash)
	)`)
	return err
}

// CreateAPIToken generates a new API token and stores a SHA-256 hash.
// Returns the plaintext token (shown once) and the token metadata.
func (d *DB) CreateAPIToken(ctx context.Context, accountID int64, projectID *int64, name string, expiresAt *time.Time) (string, *APIToken, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	plaintext := "fl_" + hex.EncodeToString(b)
	hash := hashToken(plaintext)
	prefix := plaintext[:11] // "fl_" + first 8 hex chars

	var pid sql.NullInt64
	if projectID != nil {
		pid = sql.NullInt64{Int64: *projectID, Valid: true}
	}

	var exp sql.NullTime
	if expiresAt != nil {
		exp = sql.NullTime{Time: *expiresAt, Valid: true}
	}

	res, err := d.ExecContext(ctx,
		`INSERT INTO api_tokens (account_id, project_id, name, token_hash, prefix, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		accountID, pid, name, hash, prefix, exp,
	)
	if err != nil {
		return "", nil, err
	}

	id, _ := res.LastInsertId()
	d.MarkDirty()

	token := &APIToken{
		ID:        id,
		AccountID: accountID,
		ProjectID: projectID,
		Name:      name,
		Prefix:    prefix,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	return plaintext, token, nil
}

// ValidateAPIToken checks a plaintext token against stored hashes.
// Returns the associated account if valid, nil otherwise.
// Updates last_used_at on successful validation.
func (d *DB) ValidateAPIToken(ctx context.Context, plaintext string) (*Account, error) {
	hash := hashToken(plaintext)

	var tok struct {
		id        int64
		accountID int64
		expiresAt *string
		revokedAt *string
	}
	err := d.QueryRowContext(ctx,
		`SELECT id, account_id,
		        CAST(expires_at AS CHAR) AS expires_at,
		        CAST(revoked_at AS CHAR) AS revoked_at
		 FROM api_tokens WHERE token_hash = ?`, hash,
	).Scan(&tok.id, &tok.accountID, &tok.expiresAt, &tok.revokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Check revoked.
	if tok.revokedAt != nil && *tok.revokedAt != "" {
		return nil, nil
	}

	// Check expired.
	if tok.expiresAt != nil && *tok.expiresAt != "" {
		if exp, err := time.Parse("2006-01-02 15:04:05.999999", *tok.expiresAt); err == nil {
			if time.Now().After(exp) {
				return nil, nil
			}
		}
	}

	// Update last_used_at (fire-and-forget).
	d.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, time.Now().UTC(), tok.id)
	d.MarkDirty()

	// Fetch the account.
	var a Account
	var createdStr *string
	err = d.QueryRowContext(ctx,
		`SELECT id, email, name, role, CAST(created_at AS CHAR) FROM accounts WHERE id = ?`, tok.accountID,
	).Scan(&a.ID, &a.Email, &a.Name, &a.Role, &createdStr)
	if err != nil {
		return nil, err
	}
	if createdStr != nil {
		a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999", *createdStr)
	}
	return &a, nil
}

// ListAPITokens returns all non-revoked tokens for an account.
func (d *DB) ListAPITokens(ctx context.Context, accountID int64) ([]APIToken, error) {
	rows, err := d.QueryContext(ctx,
		`SELECT id, account_id, project_id, name, prefix,
		        CAST(last_used_at AS CHAR), CAST(expires_at AS CHAR),
		        CAST(created_at AS CHAR), CAST(revoked_at AS CHAR)
		 FROM api_tokens WHERE account_id = ? AND revoked_at IS NULL ORDER BY created_at DESC`, accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var t APIToken
		var pid sql.NullInt64
		var lastUsed, expires, createdAt, revoked *string
		if err := rows.Scan(&t.ID, &t.AccountID, &pid, &t.Name, &t.Prefix, &lastUsed, &expires, &createdAt, &revoked); err != nil {
			return nil, err
		}
		if pid.Valid {
			t.ProjectID = &pid.Int64
		}
		if lastUsed != nil {
			if ts, err := time.Parse("2006-01-02 15:04:05.999999", *lastUsed); err == nil {
				t.LastUsedAt = &ts
			}
		}
		if expires != nil {
			if ts, err := time.Parse("2006-01-02 15:04:05.999999", *expires); err == nil {
				t.ExpiresAt = &ts
			}
		}
		if createdAt != nil {
			t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999", *createdAt)
		}
		if revoked != nil {
			if ts, err := time.Parse("2006-01-02 15:04:05.999999", *revoked); err == nil {
				t.RevokedAt = &ts
			}
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// RevokeAPIToken marks a token as revoked.
func (d *DB) RevokeAPIToken(ctx context.Context, tokenID, accountID int64) error {
	res, err := d.ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND account_id = ? AND revoked_at IS NULL`,
		time.Now().UTC(), tokenID, accountID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token not found or already revoked")
	}
	d.MarkDirty()
	return nil
}

// purgeRevokedAPITokens removes tokens that were revoked more than 90 days ago.
func (w *RetentionWorker) purgeRevokedAPITokens(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	res, err := w.db.ExecContext(ctx,
		`DELETE FROM api_tokens WHERE revoked_at IS NOT NULL AND revoked_at < ?`, cutoff)
	if err != nil {
		return 0, nil // table may not exist yet
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		w.db.MarkDirty()
	}
	return n, nil
}

// hashToken produces a hex-encoded SHA-256 hash of a plaintext token.
func hashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}
