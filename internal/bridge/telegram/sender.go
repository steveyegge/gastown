// Package telegram provides the Telegram bridge for Gastown.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Sender abstracts the two outbound operations the Telegram bridge needs:
// delivering mail and nudging an agent session.
type Sender interface {
	SendMail(ctx context.Context, to, subject, body string) error
	Nudge(ctx context.Context, session, message string) error
}

// --- Function types for dependency injection ---

// SendMailFunc is the signature of the mail-sending function injected into DirectSender.
type SendMailFunc func(ctx context.Context, from, to, subject, body string) error

// NudgeFunc is the signature of the nudge function injected into DirectSender.
type NudgeFunc func(townRoot, session, message string) error

// --- DirectSender (daemon mode) ---

// DirectSender calls injected functions directly, avoiding import cycles.
type DirectSender struct {
	townRoot string
	sendMail SendMailFunc
	nudge    NudgeFunc
}

// NewDirectSender creates a DirectSender with the given town root and injected functions.
func NewDirectSender(townRoot string, sendMail SendMailFunc, nudge NudgeFunc) *DirectSender {
	return &DirectSender{
		townRoot: townRoot,
		sendMail: sendMail,
		nudge:    nudge,
	}
}

// SendMail sends mail from "overseer" to the given recipient.
func (s *DirectSender) SendMail(ctx context.Context, to, subject, body string) error {
	if s.sendMail == nil {
		return errors.New("sendMail function is nil")
	}
	return s.sendMail(ctx, "overseer", to, subject, body)
}

// Nudge delivers a message to the given session queue.
func (s *DirectSender) Nudge(ctx context.Context, session, message string) error {
	if s.nudge == nil {
		return errors.New("nudge function is nil")
	}
	return s.nudge(s.townRoot, session, message)
}

// --- CLISender (standalone mode) ---

// CLISender invokes the gt CLI to send mail and nudges.
type CLISender struct {
	townRoot string
}

// NewCLISender creates a CLISender rooted at townRoot.
func NewCLISender(townRoot string) *CLISender {
	return &CLISender{townRoot: townRoot}
}

// buildMailCmd constructs the exec.Cmd for sending mail without running it.
func (s *CLISender) buildMailCmd(ctx context.Context, to, subject, body string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gt", "mail", "send", to, "-s", subject, "-m", body)
	cmd.Dir = s.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")
	return cmd
}

// buildNudgeCmd constructs the exec.Cmd for nudging a session without running it.
func (s *CLISender) buildNudgeCmd(ctx context.Context, session, message string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gt", "nudge", session, message, "--mode=queue")
	cmd.Dir = s.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")
	return cmd
}

// SendMail execs `gt mail send` with BD_ACTOR=overseer.
func (s *CLISender) SendMail(ctx context.Context, to, subject, body string) error {
	cmd := s.buildMailCmd(ctx, to, subject, body)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gt mail send: %w: %s", err, out)
	}
	return nil
}

// Nudge execs `gt nudge --mode=queue` with BD_ACTOR=overseer.
func (s *CLISender) Nudge(ctx context.Context, session, message string) error {
	cmd := s.buildNudgeCmd(ctx, session, message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gt nudge: %w: %s", err, out)
	}
	return nil
}
