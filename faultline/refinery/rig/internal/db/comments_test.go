package db

import (
	"context"
	"os"
	"testing"
)

func openCommentsTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_comments_test?parseTime=true"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.ExecContext(context.Background(), "DELETE FROM issue_comments")
		_ = d.Close()
	})
	return d
}

func TestParseMentions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"no mentions", "hello world", nil},
		{"single mention", "hey @Alice check this", []string{"Alice"}},
		{"multiple mentions", "@Bob and @Carol please review", []string{"Bob", "Carol"}},
		{"duplicate mention", "@Dave @Dave @Dave", []string{"Dave"}},
		{"dotted name", "cc @john.doe", []string{"john.doe"}},
		{"hyphenated name", "assigned to @agent-42", []string{"agent-42"}},
		{"email-like", "email user@host.com", []string{"host.com"}},
		{"at end of line", "thanks @Eve", []string{"Eve"}},
		{"underscore name", "@_internal_bot", []string{"_internal_bot"}},
		{"mixed", "Hi @Alice, @bob_smith and @Carol-1 should look at this", []string{"Alice", "bob_smith", "Carol-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMentions(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseMentions(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseMentions(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCreateAndListComments(t *testing.T) {
	d := openCommentsTestDB(t)
	ctx := context.Background()

	projectID := int64(1)
	groupID := "group-abc"
	authorID := int64(100)

	// Create two comments.
	c1, err := d.CreateComment(ctx, projectID, groupID, authorID, "First comment @Alice")
	if err != nil {
		t.Fatalf("create comment 1: %v", err)
	}
	if c1.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if len(c1.Mentions) != 1 || c1.Mentions[0] != "Alice" {
		t.Errorf("expected mentions [Alice], got %v", c1.Mentions)
	}

	c2, err := d.CreateComment(ctx, projectID, groupID, authorID, "Second comment, no mentions")
	if err != nil {
		t.Fatalf("create comment 2: %v", err)
	}
	if len(c2.Mentions) != 0 {
		t.Errorf("expected no mentions, got %v", c2.Mentions)
	}

	// List comments.
	comments, err := d.ListComments(ctx, projectID, groupID, 50, 0)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	// Should be oldest first.
	if comments[0].ID != c1.ID {
		t.Errorf("expected first comment %s, got %s", c1.ID, comments[0].ID)
	}
	if comments[1].ID != c2.ID {
		t.Errorf("expected second comment %s, got %s", c2.ID, comments[1].ID)
	}
}

func TestUpdateComment(t *testing.T) {
	d := openCommentsTestDB(t)
	ctx := context.Background()

	projectID := int64(1)
	groupID := "group-upd"
	authorID := int64(200)

	c, err := d.CreateComment(ctx, projectID, groupID, authorID, "Original body")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update by the same author.
	err = d.UpdateComment(ctx, c.ID, authorID, "Updated body @Bob")
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Verify update.
	comments, err := d.ListComments(ctx, projectID, groupID, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "Updated body @Bob" {
		t.Errorf("expected updated body, got %q", comments[0].Body)
	}
	if len(comments[0].Mentions) != 1 || comments[0].Mentions[0] != "Bob" {
		t.Errorf("expected mentions [Bob], got %v", comments[0].Mentions)
	}

	// Update by wrong author should fail.
	err = d.UpdateComment(ctx, c.ID, 999, "Hacked body")
	if err == nil {
		t.Error("expected error updating comment by wrong author")
	}
}

func TestDeleteComment(t *testing.T) {
	d := openCommentsTestDB(t)
	ctx := context.Background()

	projectID := int64(1)
	groupID := "group-del"
	authorID := int64(300)

	c, err := d.CreateComment(ctx, projectID, groupID, authorID, "To be deleted")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Delete by wrong author should fail.
	err = d.DeleteComment(ctx, c.ID, 999)
	if err == nil {
		t.Error("expected error deleting by wrong author")
	}

	// Delete by correct author.
	err = d.DeleteComment(ctx, c.ID, authorID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify deleted.
	comments, err := d.ListComments(ctx, projectID, groupID, 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments after delete, got %d", len(comments))
	}

	// Double delete should fail.
	err = d.DeleteComment(ctx, c.ID, authorID)
	if err == nil {
		t.Error("expected error deleting already-deleted comment")
	}
}

func TestListCommentsEmpty(t *testing.T) {
	d := openCommentsTestDB(t)
	ctx := context.Background()

	comments, err := d.ListComments(ctx, 999, "nonexistent-group", 50, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}
