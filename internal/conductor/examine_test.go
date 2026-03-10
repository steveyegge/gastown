package conductor

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/mail"
)

func TestConductorAddress(t *testing.T) {
	if got := ConductorAddress("gastown"); got != "gastown/conductor" {
		t.Errorf("ConductorAddress(\"gastown\") = %q", got)
	}
}

func TestRequestExamination(t *testing.T) {
	msg := RequestExamination("gastown", "user-profile", "gt-abc",
		"Add user profile page", []string{"src/pages/**"})

	if msg.From != "gastown/conductor" {
		t.Errorf("From = %q", msg.From)
	}
	if msg.To != "gastown/architect" {
		t.Errorf("To = %q", msg.To)
	}
	if msg.Subject != "[examine] user-profile" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if msg.Type != mail.TypeTask {
		t.Errorf("Type = %q", msg.Type)
	}
	if !strings.Contains(msg.Body, "Add user profile page") {
		t.Error("body missing description")
	}
	if !strings.Contains(msg.Body, "`src/pages/**`") {
		t.Error("body missing affected paths")
	}
}

func TestRequestAPIContracts(t *testing.T) {
	msg := RequestAPIContracts("gastown", "user-profile", "gt-abc",
		"Define user CRUD endpoints")

	if msg.Subject != "[api-contract] user-profile" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "Define user CRUD endpoints") {
		t.Error("body missing description")
	}
}

func TestRequestModelSpecs(t *testing.T) {
	msg := RequestModelSpecs("gastown", "user-profile", "gt-abc",
		"User table with avatar support")

	if msg.Subject != "[model-spec] user-profile" {
		t.Errorf("Subject = %q", msg.Subject)
	}
}

func TestAskArchitect(t *testing.T) {
	msg := AskArchitect("gastown", "gastown/artisans/frontend-1",
		"user-profile", "How is auth middleware structured?")

	if msg.From != "gastown/artisans/frontend-1" {
		t.Errorf("From = %q (should preserve caller address)", msg.From)
	}
	if msg.To != "gastown/architect" {
		t.Errorf("To = %q", msg.To)
	}
	if msg.Subject != "[question] user-profile" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "How is auth middleware structured?") {
		t.Error("body missing question")
	}
}

func TestAskArchitect_FromArtisan(t *testing.T) {
	// Any agent can consult the architect
	msg := AskArchitect("gastown", "gastown/artisans/backend-1",
		"payments", "What's the transaction isolation level?")

	if msg.From != "gastown/artisans/backend-1" {
		t.Errorf("From = %q", msg.From)
	}
	if msg.Type != mail.TypeTask {
		t.Errorf("Type = %q, want task", msg.Type)
	}
}
