package tmux

import (
	"strings"
	"testing"
	"time"
)

// TestNewGroupedSession_Basic verifies that a grouped session joins the anchor's
// window group and creates a new window visible to both sessions.
func TestNewGroupedSession_Basic(t *testing.T) {
	tm := newTestTmux(t)

	anchor := "gt-test-group-anchor-" + t.Name()
	grouped := "gt-test-group-member-" + t.Name()
	_ = tm.KillSession(anchor)
	_ = tm.KillSession(grouped)
	defer func() {
		_ = tm.KillSession(grouped)
		_ = tm.KillSession(anchor)
	}()

	// Create anchor session
	if err := tm.NewSessionWithCommand(anchor, "", `sh -c 'echo "ANCHOR"; sleep 30'`); err != nil {
		t.Fatalf("anchor creation: %v", err)
	}

	// Create grouped session joining anchor's group
	err := tm.NewGroupedSessionWithCommandAndEnv(grouped, "", `sh -c 'echo "GROUPED"; sleep 30'`, nil, anchor)
	if err != nil {
		t.Fatalf("grouped creation: %v", err)
	}

	// Both sessions should exist
	hasAnchor, _ := tm.HasSession(anchor)
	hasGrouped, _ := tm.HasSession(grouped)
	if !hasAnchor {
		t.Error("anchor session not found")
	}
	if !hasGrouped {
		t.Error("grouped session not found")
	}

	// The grouped session should be in a group
	if !tm.IsGroupedSession(grouped) {
		t.Error("grouped session should report as grouped")
	}
	// The anchor should also be in a group now (it was joined)
	if !tm.IsGroupedSession(anchor) {
		t.Error("anchor session should report as grouped after member joins")
	}

	// Verify the grouped session's command is running
	time.Sleep(500 * time.Millisecond)
	output, _ := tm.CapturePane(grouped, 50)
	if !strings.Contains(output, "GROUPED") {
		t.Errorf("expected grouped session output to contain GROUPED, got: %q", strings.TrimSpace(output))
	}
}

// TestNewGroupedSession_EnvVars verifies that environment variables are properly
// set in the grouped session.
func TestNewGroupedSession_EnvVars(t *testing.T) {
	tm := newTestTmux(t)

	anchor := "gt-test-groupenv-anchor-" + t.Name()
	grouped := "gt-test-groupenv-member-" + t.Name()
	_ = tm.KillSession(anchor)
	_ = tm.KillSession(grouped)
	defer func() {
		_ = tm.KillSession(grouped)
		_ = tm.KillSession(anchor)
	}()

	if err := tm.NewSessionWithCommand(anchor, "", "sleep 30"); err != nil {
		t.Fatalf("anchor creation: %v", err)
	}

	env := map[string]string{
		"GT_ROLE":  "crew",
		"GT_RIG":   "testrig",
		"GT_AGENT": "testmember",
	}
	err := tm.NewGroupedSessionWithCommandAndEnv(grouped, "", `sh -c 'echo "ROLE=$GT_ROLE RIG=$GT_RIG"; sleep 30'`, env, anchor)
	if err != nil {
		t.Fatalf("grouped creation: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	output, _ := tm.CapturePane(grouped, 50)
	if !strings.Contains(output, "ROLE=crew") || !strings.Contains(output, "RIG=testrig") {
		t.Errorf("expected env vars in output, got: %q", strings.TrimSpace(output))
	}
}

// TestNewGroupedSession_MissingGroupTarget verifies that empty groupTarget is rejected.
func TestNewGroupedSession_MissingGroupTarget(t *testing.T) {
	tm := newTestTmux(t)
	err := tm.NewGroupedSessionWithCommandAndEnv("gt-test-nogroup", "", "sleep 1", nil, "")
	if err == nil {
		t.Error("expected error for empty groupTarget")
	}
}

// TestNewGroupedSession_NonexistentGroupTarget verifies that tmux creates a new
// group when the target session doesn't exist (tmux -t names a group, not just
// a session). The session is still created — it's just in a standalone group.
func TestNewGroupedSession_NonexistentGroupTarget(t *testing.T) {
	tm := newTestTmux(t)
	sess := "gt-test-badgroup-" + t.Name()
	_ = tm.KillSession(sess)
	defer func() { _ = tm.KillSession(sess) }()

	// tmux -t with a nonexistent target creates a new group (not an error)
	err := tm.NewGroupedSessionWithCommandAndEnv(sess, "", "sleep 5", nil, "gt-nonexistent-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	has, _ := tm.HasSession(sess)
	if !has {
		t.Error("session should have been created even with nonexistent group target")
	}
}

// TestKillGroupedSessionWindow verifies that killing a grouped session's window
// removes it from the group without affecting other sessions.
func TestKillGroupedSessionWindow(t *testing.T) {
	tm := newTestTmux(t)

	anchor := "gt-test-killwin-anchor-" + t.Name()
	grouped := "gt-test-killwin-member-" + t.Name()
	_ = tm.KillSession(anchor)
	_ = tm.KillSession(grouped)
	defer func() {
		_ = tm.KillSession(grouped)
		_ = tm.KillSession(anchor)
	}()

	if err := tm.NewSessionWithCommand(anchor, "", "sleep 30"); err != nil {
		t.Fatalf("anchor creation: %v", err)
	}

	if err := tm.NewGroupedSessionWithCommandAndEnv(grouped, "", "sleep 30", nil, anchor); err != nil {
		t.Fatalf("grouped creation: %v", err)
	}

	// Kill the grouped session's window
	if err := tm.KillGroupedSessionWindow(grouped); err != nil {
		t.Fatalf("killing grouped window: %v", err)
	}

	// Anchor should still be running
	hasAnchor, _ := tm.HasSession(anchor)
	if !hasAnchor {
		t.Error("anchor session should still exist after killing grouped window")
	}
}

// TestNewGroupedSession_MultipleMembers verifies that multiple sessions can join
// the same group and each gets its own window.
func TestNewGroupedSession_MultipleMembers(t *testing.T) {
	tm := newTestTmux(t)

	anchor := "gt-test-multi-anchor-" + t.Name()
	member1 := "gt-test-multi-m1-" + t.Name()
	member2 := "gt-test-multi-m2-" + t.Name()
	_ = tm.KillSession(anchor)
	_ = tm.KillSession(member1)
	_ = tm.KillSession(member2)
	defer func() {
		_ = tm.KillSession(member2)
		_ = tm.KillSession(member1)
		_ = tm.KillSession(anchor)
	}()

	if err := tm.NewSessionWithCommand(anchor, "", "sleep 30"); err != nil {
		t.Fatalf("anchor creation: %v", err)
	}

	if err := tm.NewGroupedSessionWithCommandAndEnv(member1, "", "sleep 30", nil, anchor); err != nil {
		t.Fatalf("member1 creation: %v", err)
	}

	if err := tm.NewGroupedSessionWithCommandAndEnv(member2, "", "sleep 30", nil, anchor); err != nil {
		t.Fatalf("member2 creation: %v", err)
	}

	// All three should exist and be grouped
	for _, name := range []string{anchor, member1, member2} {
		has, _ := tm.HasSession(name)
		if !has {
			t.Errorf("session %s not found", name)
		}
		if !tm.IsGroupedSession(name) {
			t.Errorf("session %s should be grouped", name)
		}
	}
}
