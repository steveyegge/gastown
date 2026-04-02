package db

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

func openAssignmentTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3307)/faultline_assign_test?parseTime=true"
	}
	d, err := Open(dsn)
	if err != nil {
		t.Skipf("Dolt not available: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.ExecContext(context.Background(), "DELETE FROM issue_assignments")
		_ = d.Close()
	})
	return d
}

func TestAssignAndGetIssue(t *testing.T) {
	d := openAssignmentTestDB(t)
	ctx := context.Background()

	groupID := uuid.New().String()
	projectID := int64(1)

	// No assignment initially.
	a, err := d.GetIssueAssignment(ctx, groupID, projectID)
	if err != nil {
		t.Fatalf("get before assign: %v", err)
	}
	if a != nil {
		t.Fatal("expected nil assignment before assign")
	}

	// Assign.
	if err := d.AssignIssue(ctx, groupID, projectID, "alice@test.com", "bob@test.com"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Get assignment.
	a, err = d.GetIssueAssignment(ctx, groupID, projectID)
	if err != nil {
		t.Fatalf("get after assign: %v", err)
	}
	if a == nil {
		t.Fatal("expected assignment, got nil")
	}
	if a.GroupID != groupID {
		t.Errorf("group_id: got %q, want %q", a.GroupID, groupID)
	}
	if a.ProjectID != projectID {
		t.Errorf("project_id: got %d, want %d", a.ProjectID, projectID)
	}
	if a.AssignedTo != "alice@test.com" {
		t.Errorf("assigned_to: got %q, want %q", a.AssignedTo, "alice@test.com")
	}
	if a.AssignedBy != "bob@test.com" {
		t.Errorf("assigned_by: got %q, want %q", a.AssignedBy, "bob@test.com")
	}
	if a.AssignedAt.IsZero() {
		t.Error("assigned_at should not be zero")
	}
}

func TestAssignIssueReassign(t *testing.T) {
	d := openAssignmentTestDB(t)
	ctx := context.Background()

	groupID := uuid.New().String()
	projectID := int64(1)

	// First assignment.
	if err := d.AssignIssue(ctx, groupID, projectID, "alice@test.com", "bob@test.com"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// Reassign via REPLACE INTO.
	if err := d.AssignIssue(ctx, groupID, projectID, "charlie@test.com", "bob@test.com"); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	a, err := d.GetIssueAssignment(ctx, groupID, projectID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.AssignedTo != "charlie@test.com" {
		t.Errorf("expected reassigned to charlie, got %q", a.AssignedTo)
	}
}

func TestUnassignIssue(t *testing.T) {
	d := openAssignmentTestDB(t)
	ctx := context.Background()

	groupID := uuid.New().String()
	projectID := int64(1)

	// Assign then unassign.
	if err := d.AssignIssue(ctx, groupID, projectID, "alice@test.com", "bob@test.com"); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := d.UnassignIssue(ctx, groupID, projectID); err != nil {
		t.Fatalf("unassign: %v", err)
	}

	a, err := d.GetIssueAssignment(ctx, groupID, projectID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a != nil {
		t.Error("expected nil after unassign")
	}

	// Unassigning a non-existent assignment should not error.
	if err := d.UnassignIssue(ctx, groupID, projectID); err != nil {
		t.Fatalf("unassign non-existent: %v", err)
	}
}

func TestAssignmentsForIssues(t *testing.T) {
	d := openAssignmentTestDB(t)
	ctx := context.Background()

	projectID := int64(1)
	g1 := uuid.New().String()
	g2 := uuid.New().String()
	g3 := uuid.New().String()

	// Assign two of three.
	if err := d.AssignIssue(ctx, g1, projectID, "alice@test.com", "admin@test.com"); err != nil {
		t.Fatalf("assign g1: %v", err)
	}
	if err := d.AssignIssue(ctx, g2, projectID, "bob@test.com", "admin@test.com"); err != nil {
		t.Fatalf("assign g2: %v", err)
	}

	// Batch query all three.
	result, err := d.AssignmentsForIssues(ctx, projectID, []string{g1, g2, g3})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(result))
	}
	if result[g1].AssignedTo != "alice@test.com" {
		t.Errorf("g1 assigned_to: got %q", result[g1].AssignedTo)
	}
	if result[g2].AssignedTo != "bob@test.com" {
		t.Errorf("g2 assigned_to: got %q", result[g2].AssignedTo)
	}
	if _, ok := result[g3]; ok {
		t.Error("g3 should not have an assignment")
	}
}

func TestAssignmentsForIssuesEmpty(t *testing.T) {
	d := openAssignmentTestDB(t)
	ctx := context.Background()

	result, err := d.AssignmentsForIssues(ctx, 1, nil)
	if err != nil {
		t.Fatalf("empty batch: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
}

func TestAssignmentsIsolatedByProject(t *testing.T) {
	d := openAssignmentTestDB(t)
	ctx := context.Background()

	groupID := uuid.New().String()

	// Assign same group in two projects.
	if err := d.AssignIssue(ctx, groupID, 1, "alice@test.com", "admin@test.com"); err != nil {
		t.Fatalf("assign p1: %v", err)
	}
	if err := d.AssignIssue(ctx, groupID, 2, "bob@test.com", "admin@test.com"); err != nil {
		t.Fatalf("assign p2: %v", err)
	}

	a1, err := d.GetIssueAssignment(ctx, groupID, 1)
	if err != nil {
		t.Fatalf("get p1: %v", err)
	}
	a2, err := d.GetIssueAssignment(ctx, groupID, 2)
	if err != nil {
		t.Fatalf("get p2: %v", err)
	}

	if a1.AssignedTo != "alice@test.com" {
		t.Errorf("p1: got %q, want alice", a1.AssignedTo)
	}
	if a2.AssignedTo != "bob@test.com" {
		t.Errorf("p2: got %q, want bob", a2.AssignedTo)
	}
}
