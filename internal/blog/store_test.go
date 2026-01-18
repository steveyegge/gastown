package blog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "blog-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "blog.db")

	// Test NewStore
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Test CreatePost
	post, err := store.CreatePost("Test Title", "Test content", "author1", false)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if post.ID == 0 {
		t.Error("CreatePost: expected non-zero ID")
	}
	if post.Title != "Test Title" {
		t.Errorf("CreatePost: expected title 'Test Title', got %q", post.Title)
	}
	if post.Published {
		t.Error("CreatePost: expected unpublished post")
	}

	// Test GetPost
	retrieved, err := store.GetPost(post.ID)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if retrieved.Title != post.Title {
		t.Errorf("GetPost: expected title %q, got %q", post.Title, retrieved.Title)
	}

	// Test GetPost not found
	_, err = store.GetPost(999)
	if err != ErrPostNotFound {
		t.Errorf("GetPost: expected ErrPostNotFound, got %v", err)
	}

	// Test UpdatePost
	updated, err := store.UpdatePost(post.ID, "Updated Title", "Updated content", true)
	if err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("UpdatePost: expected title 'Updated Title', got %q", updated.Title)
	}
	if !updated.Published {
		t.Error("UpdatePost: expected published post")
	}

	// Test UpdatePost not found
	_, err = store.UpdatePost(999, "Title", "Content", false)
	if err != ErrPostNotFound {
		t.Errorf("UpdatePost: expected ErrPostNotFound, got %v", err)
	}

	// Create more posts for list testing
	_, err = store.CreatePost("Post 2", "Content 2", "author1", true)
	if err != nil {
		t.Fatalf("CreatePost 2: %v", err)
	}
	_, err = store.CreatePost("Post 3", "Content 3", "author2", true)
	if err != nil {
		t.Fatalf("CreatePost 3: %v", err)
	}

	// Test ListPosts
	posts, err := store.ListPosts(ListOptions{})
	if err != nil {
		t.Fatalf("ListPosts: %v", err)
	}
	if len(posts) != 3 {
		t.Errorf("ListPosts: expected 3 posts, got %d", len(posts))
	}

	// Test ListPosts with author filter
	posts, err = store.ListPosts(ListOptions{Author: "author1"})
	if err != nil {
		t.Fatalf("ListPosts with author: %v", err)
	}
	if len(posts) != 2 {
		t.Errorf("ListPosts with author: expected 2 posts, got %d", len(posts))
	}

	// Test ListPosts with published filter
	// We have 3 published posts: the updated first post, post 2, and post 3
	posts, err = store.ListPosts(ListOptions{PublishedOnly: true})
	if err != nil {
		t.Fatalf("ListPosts published: %v", err)
	}
	if len(posts) != 3 {
		t.Errorf("ListPosts published: expected 3 posts, got %d", len(posts))
	}

	// Test ListPosts with limit
	posts, err = store.ListPosts(ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("ListPosts limit: %v", err)
	}
	if len(posts) != 1 {
		t.Errorf("ListPosts limit: expected 1 post, got %d", len(posts))
	}

	// Test CountPosts
	count, err := store.CountPosts(ListOptions{})
	if err != nil {
		t.Fatalf("CountPosts: %v", err)
	}
	if count != 3 {
		t.Errorf("CountPosts: expected 3, got %d", count)
	}

	count, err = store.CountPosts(ListOptions{PublishedOnly: true})
	if err != nil {
		t.Fatalf("CountPosts published: %v", err)
	}
	if count != 3 {
		t.Errorf("CountPosts published: expected 3, got %d", count)
	}

	// Test DeletePost
	err = store.DeletePost(post.ID)
	if err != nil {
		t.Fatalf("DeletePost: %v", err)
	}

	// Verify deletion
	_, err = store.GetPost(post.ID)
	if err != ErrPostNotFound {
		t.Errorf("GetPost after delete: expected ErrPostNotFound, got %v", err)
	}

	// Test DeletePost not found
	err = store.DeletePost(999)
	if err != ErrPostNotFound {
		t.Errorf("DeletePost: expected ErrPostNotFound, got %v", err)
	}
}

func TestStoreNotOpen(t *testing.T) {
	store := &Store{} // nil db

	_, err := store.CreatePost("Title", "Content", "author", false)
	if err != ErrStoreNotOpen {
		t.Errorf("CreatePost: expected ErrStoreNotOpen, got %v", err)
	}

	_, err = store.GetPost(1)
	if err != ErrStoreNotOpen {
		t.Errorf("GetPost: expected ErrStoreNotOpen, got %v", err)
	}

	_, err = store.UpdatePost(1, "Title", "Content", false)
	if err != ErrStoreNotOpen {
		t.Errorf("UpdatePost: expected ErrStoreNotOpen, got %v", err)
	}

	err = store.DeletePost(1)
	if err != ErrStoreNotOpen {
		t.Errorf("DeletePost: expected ErrStoreNotOpen, got %v", err)
	}

	_, err = store.ListPosts(ListOptions{})
	if err != ErrStoreNotOpen {
		t.Errorf("ListPosts: expected ErrStoreNotOpen, got %v", err)
	}

	_, err = store.CountPosts(ListOptions{})
	if err != ErrStoreNotOpen {
		t.Errorf("CountPosts: expected ErrStoreNotOpen, got %v", err)
	}
}
