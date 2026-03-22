package telegram

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface checks.
var _ Sender = (*CLISender)(nil)
var _ Sender = (*DirectSender)(nil)

// --- DirectSender tests ---

func TestDirectSender_SendMail_CallsFunc(t *testing.T) {
	var gotCtx context.Context
	var gotFrom, gotTo, gotSubject, gotBody string

	sendFn := func(ctx context.Context, from, to, subject, body string) error {
		gotCtx = ctx
		gotFrom = from
		gotTo = to
		gotSubject = subject
		gotBody = body
		return nil
	}

	s := NewDirectSender("/town", sendFn, nil)
	ctx := context.Background()
	err := s.SendMail(ctx, "alice", "Hello", "World")
	require.NoError(t, err)

	assert.Equal(t, ctx, gotCtx)
	assert.Equal(t, "overseer", gotFrom)
	assert.Equal(t, "alice", gotTo)
	assert.Equal(t, "Hello", gotSubject)
	assert.Equal(t, "World", gotBody)
}

func TestDirectSender_SendMail_NilFunc_ReturnsError(t *testing.T) {
	s := NewDirectSender("/town", nil, nil)
	err := s.SendMail(context.Background(), "alice", "Hello", "World")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sendMail")
}

func TestDirectSender_SendMail_PropagatesError(t *testing.T) {
	want := errors.New("smtp down")
	s := NewDirectSender("/town", func(_ context.Context, _, _, _, _ string) error {
		return want
	}, nil)
	err := s.SendMail(context.Background(), "bob", "sub", "body")
	assert.Equal(t, want, err)
}

func TestDirectSender_Nudge_CallsFunc(t *testing.T) {
	var gotRoot, gotSession, gotMessage string

	nudgeFn := func(townRoot, session, message string) error {
		gotRoot = townRoot
		gotSession = session
		gotMessage = message
		return nil
	}

	s := NewDirectSender("/mytown", nil, nudgeFn)
	err := s.Nudge(context.Background(), "sess1", "ping")
	require.NoError(t, err)

	assert.Equal(t, "/mytown", gotRoot)
	assert.Equal(t, "sess1", gotSession)
	assert.Equal(t, "ping", gotMessage)
}

func TestDirectSender_Nudge_NilFunc_ReturnsError(t *testing.T) {
	s := NewDirectSender("/town", nil, nil)
	err := s.Nudge(context.Background(), "sess1", "ping")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nudge")
}

func TestDirectSender_Nudge_PropagatesError(t *testing.T) {
	want := errors.New("session not found")
	s := NewDirectSender("/town", nil, func(_, _, _ string) error {
		return want
	})
	err := s.Nudge(context.Background(), "sess1", "ping")
	assert.Equal(t, want, err)
}

// --- CLISender tests ---

func TestCLISender_buildMailCmd_Args(t *testing.T) {
	s := NewCLISender("/mytown")
	cmd := s.buildMailCmd(context.Background(), "alice", "Hello World", "Body text")

	assert.Equal(t, []string{"gt", "mail", "send", "alice", "-s", "Hello World", "-m", "Body text"}, cmd.Args)
}

func TestCLISender_buildMailCmd_Dir(t *testing.T) {
	s := NewCLISender("/mytown")
	cmd := s.buildMailCmd(context.Background(), "alice", "subj", "body")
	assert.Equal(t, "/mytown", cmd.Dir)
}

func TestCLISender_buildMailCmd_BdActorEnv(t *testing.T) {
	s := NewCLISender("/mytown")
	cmd := s.buildMailCmd(context.Background(), "alice", "subj", "body")
	assert.True(t, hasBdActor(cmd, "overseer"), "expected BD_ACTOR=overseer in env")
}

func TestCLISender_buildNudgeCmd_Args(t *testing.T) {
	s := NewCLISender("/mytown")
	cmd := s.buildNudgeCmd(context.Background(), "sess1", "hello there")

	assert.Equal(t, []string{"gt", "nudge", "sess1", "hello there", "--mode=queue"}, cmd.Args)
}

func TestCLISender_buildNudgeCmd_Dir(t *testing.T) {
	s := NewCLISender("/mytown")
	cmd := s.buildNudgeCmd(context.Background(), "sess1", "msg")
	assert.Equal(t, "/mytown", cmd.Dir)
}

func TestCLISender_buildNudgeCmd_BdActorEnv(t *testing.T) {
	s := NewCLISender("/mytown")
	cmd := s.buildNudgeCmd(context.Background(), "sess1", "msg")
	assert.True(t, hasBdActor(cmd, "overseer"), "expected BD_ACTOR=overseer in env")
}

// hasBdActor checks whether cmd.Env contains BD_ACTOR=<val>.
func hasBdActor(cmd *exec.Cmd, val string) bool {
	target := "BD_ACTOR=" + val
	for _, e := range cmd.Env {
		if e == target {
			return true
		}
	}
	return false
}
