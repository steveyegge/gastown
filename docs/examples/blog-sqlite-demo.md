# Blog SQLite Demo

A reference implementation demonstrating SQLite persistence patterns in Go.

## Overview

The `internal/blog` package provides a simple blog post storage system backed by
SQLite. It demonstrates best practices for:

1. **Database initialization** - Opening connections and creating schema
2. **CRUD operations** - Create, Read, Update, Delete for posts
3. **Query building** - Parameterized queries with optional filters
4. **Error handling** - Proper error wrapping and sentinel errors
5. **Resource management** - Connection lifecycle and cleanup

## Usage

```go
package main

import (
    "fmt"
    "log"

    "github.com/steveyegge/gastown/internal/blog"
)

func main() {
    // Create a new store (creates database file if needed)
    store, err := blog.NewStore("./blog.db")
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()

    // Create a post
    post, err := store.CreatePost(
        "My First Post",
        "Hello, world!",
        "author@example.com",
        true, // published
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created post: %d\n", post.ID)

    // List published posts
    posts, err := store.ListPosts(blog.ListOptions{
        PublishedOnly: true,
        Limit:         10,
    })
    if err != nil {
        log.Fatal(err)
    }
    for _, p := range posts {
        fmt.Printf("- %s by %s\n", p.Title, p.Author)
    }
}
```

## API Reference

### Store

```go
// NewStore creates a new blog store at the given path.
// The database file will be created if it doesn't exist.
func NewStore(dbPath string) (*Store, error)

// Close closes the database connection.
func (s *Store) Close() error
```

### Post Operations

```go
// CreatePost creates a new blog post.
func (s *Store) CreatePost(title, content, author string, published bool) (*Post, error)

// GetPost retrieves a post by ID.
func (s *Store) GetPost(id int64) (*Post, error)

// UpdatePost updates an existing post.
func (s *Store) UpdatePost(id int64, title, content string, published bool) (*Post, error)

// DeletePost deletes a post by ID.
func (s *Store) DeletePost(id int64) error

// ListPosts returns posts with optional filtering.
func (s *Store) ListPosts(opts ListOptions) ([]*Post, error)

// CountPosts returns the total number of posts matching the filter.
func (s *Store) CountPosts(opts ListOptions) (int, error)
```

### Types

```go
type Post struct {
    ID        int64
    Title     string
    Content   string
    Author    string
    CreatedAt time.Time
    UpdatedAt time.Time
    Published bool
}

type ListOptions struct {
    Author        string // Filter by author
    PublishedOnly bool   // Only return published posts
    Limit         int    // Maximum number of results
    Offset        int    // Skip first N results
}
```

### Errors

```go
var (
    ErrPostNotFound = errors.New("post not found")
    ErrStoreNotOpen = errors.New("store not open")
)
```

## Design Patterns

### Schema Initialization

The store automatically creates tables on first use:

```sql
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
```

### Parameterized Queries

All queries use parameterized inputs to prevent SQL injection:

```go
result, err := s.db.Exec(
    `INSERT INTO posts (title, content, author) VALUES (?, ?, ?)`,
    title, content, author,
)
```

### Dynamic Query Building

The `ListPosts` method demonstrates building queries with optional filters:

```go
query := `SELECT ... FROM posts WHERE 1=1`
var args []interface{}

if opts.Author != "" {
    query += " AND author = ?"
    args = append(args, opts.Author)
}
```

### Error Wrapping

Errors include context for debugging:

```go
if err != nil {
    return nil, fmt.Errorf("querying post: %w", err)
}
```

## Testing

Run the tests:

```bash
go test -v ./internal/blog/...
```

The tests use a temporary directory to avoid polluting the filesystem.

## Dependencies

- `github.com/mattn/go-sqlite3` - CGo-based SQLite driver

Note: This requires CGo and a C compiler. On most systems this is available
by default. If you encounter build issues, ensure `gcc` is installed.
