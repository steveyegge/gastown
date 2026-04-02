package relay

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store manages SQLite envelope storage for the relay.
type Store struct {
	db  *sql.DB
	ttl time.Duration
}

// NewStore opens (or creates) a SQLite database at path.
// ttl controls auto-purge of old envelopes (0 = no purge).
func NewStore(path string, ttl time.Duration) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// WAL mode for concurrent reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db, ttl: ttl}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS envelopes (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id  INTEGER NOT NULL,
			public_key  TEXT    NOT NULL,
			payload     BLOB   NOT NULL,
			received_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			pulled      INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_unpulled ON envelopes (pulled, received_at);
		CREATE INDEX IF NOT EXISTS idx_received ON envelopes (received_at);
	`)
	return err
}

// Envelope represents a stored envelope.
type Envelope struct {
	ID         int64  `json:"id"`
	ProjectID  int64  `json:"project_id"`
	PublicKey  string `json:"public_key"`
	Payload    []byte `json:"payload"`
	ReceivedAt string `json:"received_at"`
}

// Insert stores a raw envelope payload.
func (s *Store) Insert(projectID int64, publicKey string, payload []byte) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO envelopes (project_id, public_key, payload) VALUES (?, ?, ?)`,
		projectID, publicKey, payload,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Poll returns up to limit unpulled envelopes received after sinceID.
func (s *Store) Poll(sinceID int64, limit int) ([]Envelope, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, project_id, public_key, payload, received_at
		 FROM envelopes
		 WHERE pulled = 0 AND id > ?
		 ORDER BY id ASC
		 LIMIT ?`,
		sinceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var envelopes []Envelope
	for rows.Next() {
		var e Envelope
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.PublicKey, &e.Payload, &e.ReceivedAt); err != nil {
			return nil, err
		}
		envelopes = append(envelopes, e)
	}
	return envelopes, rows.Err()
}

// Ack marks envelopes as pulled (by IDs).
func (s *Store) Ack(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE envelopes SET pulled = 1 WHERE id = ?`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	var total int64
	for _, id := range ids {
		res, err := stmt.Exec(id)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		total += n
	}

	return total, tx.Commit()
}

// Purge deletes pulled envelopes older than the configured TTL.
// Returns the number of rows deleted.
func (s *Store) Purge() (int64, error) {
	if s.ttl <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-s.ttl).Format(time.RFC3339)
	res, err := s.db.Exec(
		`DELETE FROM envelopes WHERE pulled = 1 AND received_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Stats returns basic storage statistics.
func (s *Store) Stats() (total, unpulled int64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*) FROM envelopes`).Scan(&total)
	if err != nil {
		return
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM envelopes WHERE pulled = 0`).Scan(&unpulled)
	return
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}
