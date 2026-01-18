// Package blog provides a simple blog post storage backed by SQLite.
// This serves as an example of how to implement SQLite persistence in Go.
package blog

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Common errors
var (
	ErrPostNotFound = errors.New("post not found")
	ErrStoreNotOpen = errors.New("store not open")
)

// Post represents a blog post.
type Post struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Published bool      `json:"published"`
}

// Store provides SQLite-backed storage for blog posts.
type Store struct {
	db   *sql.DB
	path string
}

// NewStore creates a new blog store at the given path.
// The database file will be created if it doesn't exist.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	store := &Store{
		db:   db,
		path: dbPath,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database tables if they don't exist.
func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		author TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		published INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_posts_author ON posts(author);
	CREATE INDEX IF NOT EXISTS idx_posts_published ON posts(published);
	CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts(created_at);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// CreatePost creates a new blog post and returns the created post with its ID.
func (s *Store) CreatePost(title, content, author string, published bool) (*Post, error) {
	if s.db == nil {
		return nil, ErrStoreNotOpen
	}

	now := time.Now()
	result, err := s.db.Exec(
		`INSERT INTO posts (title, content, author, created_at, updated_at, published)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		title, content, author, now, now, published,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting post: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting insert ID: %w", err)
	}

	return &Post{
		ID:        id,
		Title:     title,
		Content:   content,
		Author:    author,
		CreatedAt: now,
		UpdatedAt: now,
		Published: published,
	}, nil
}

// GetPost retrieves a post by ID.
func (s *Store) GetPost(id int64) (*Post, error) {
	if s.db == nil {
		return nil, ErrStoreNotOpen
	}

	post := &Post{}
	err := s.db.QueryRow(
		`SELECT id, title, content, author, created_at, updated_at, published
		 FROM posts WHERE id = ?`,
		id,
	).Scan(&post.ID, &post.Title, &post.Content, &post.Author,
		&post.CreatedAt, &post.UpdatedAt, &post.Published)

	if err == sql.ErrNoRows {
		return nil, ErrPostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying post: %w", err)
	}

	return post, nil
}

// UpdatePost updates an existing post.
func (s *Store) UpdatePost(id int64, title, content string, published bool) (*Post, error) {
	if s.db == nil {
		return nil, ErrStoreNotOpen
	}

	now := time.Now()
	result, err := s.db.Exec(
		`UPDATE posts SET title = ?, content = ?, updated_at = ?, published = ?
		 WHERE id = ?`,
		title, content, now, published, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating post: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return nil, ErrPostNotFound
	}

	return s.GetPost(id)
}

// DeletePost deletes a post by ID.
func (s *Store) DeletePost(id int64) error {
	if s.db == nil {
		return ErrStoreNotOpen
	}

	result, err := s.db.Exec(`DELETE FROM posts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting post: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrPostNotFound
	}

	return nil
}

// ListPosts returns posts with optional filtering.
func (s *Store) ListPosts(opts ListOptions) ([]*Post, error) {
	if s.db == nil {
		return nil, ErrStoreNotOpen
	}

	query := `SELECT id, title, content, author, created_at, updated_at, published
		      FROM posts WHERE 1=1`
	var args []interface{}

	if opts.Author != "" {
		query += " AND author = ?"
		args = append(args, opts.Author)
	}

	if opts.PublishedOnly {
		query += " AND published = 1"
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying posts: %w", err)
	}
	defer rows.Close()

	var posts []*Post
	for rows.Next() {
		post := &Post{}
		if err := rows.Scan(&post.ID, &post.Title, &post.Content, &post.Author,
			&post.CreatedAt, &post.UpdatedAt, &post.Published); err != nil {
			return nil, fmt.Errorf("scanning post: %w", err)
		}
		posts = append(posts, post)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating posts: %w", err)
	}

	return posts, nil
}

// ListOptions specifies options for listing posts.
type ListOptions struct {
	Author        string
	PublishedOnly bool
	Limit         int
	Offset        int
}

// CountPosts returns the total number of posts matching the filter.
func (s *Store) CountPosts(opts ListOptions) (int, error) {
	if s.db == nil {
		return 0, ErrStoreNotOpen
	}

	query := `SELECT COUNT(*) FROM posts WHERE 1=1`
	var args []interface{}

	if opts.Author != "" {
		query += " AND author = ?"
		args = append(args, opts.Author)
	}

	if opts.PublishedOnly {
		query += " AND published = 1"
	}

	var count int
	if err := s.db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting posts: %w", err)
	}

	return count, nil
}
