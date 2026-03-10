package architect

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/mail"
)

func TestArchitectAddress(t *testing.T) {
	tests := []struct {
		rig  string
		want string
	}{
		{"gastown", "gastown/architect"},
		{"myrig", "myrig/architect"},
	}
	for _, tt := range tests {
		got := ArchitectAddress(tt.rig)
		if got != tt.want {
			t.Errorf("ArchitectAddress(%q) = %q, want %q", tt.rig, got, tt.want)
		}
	}
}

func TestNewExamineRequest(t *testing.T) {
	req := NewExamineRequest("user-profile", "gt-abc", "Add user profile page", []string{"src/pages/**"})

	if req.Type != RequestExamine {
		t.Errorf("Type = %q, want %q", req.Type, RequestExamine)
	}
	if req.FeatureName != "user-profile" {
		t.Errorf("FeatureName = %q", req.FeatureName)
	}
	if req.BeadID != "gt-abc" {
		t.Errorf("BeadID = %q", req.BeadID)
	}
	if len(req.AffectedPaths) != 1 {
		t.Errorf("AffectedPaths = %v", req.AffectedPaths)
	}
}

func TestNewAPIContractRequest(t *testing.T) {
	req := NewAPIContractRequest("user-profile", "gt-abc", "Define user API endpoints")
	if req.Type != RequestAPIContract {
		t.Errorf("Type = %q, want %q", req.Type, RequestAPIContract)
	}
}

func TestNewModelSpecRequest(t *testing.T) {
	req := NewModelSpecRequest("user-profile", "gt-abc", "User table schema")
	if req.Type != RequestModelSpec {
		t.Errorf("Type = %q, want %q", req.Type, RequestModelSpec)
	}
}

func TestNewQuestion(t *testing.T) {
	req := NewQuestion("user-profile", "How is auth middleware structured?")
	if req.Type != RequestQuestion {
		t.Errorf("Type = %q, want %q", req.Type, RequestQuestion)
	}
	if req.BeadID != "" {
		t.Errorf("Question should not require BeadID, got %q", req.BeadID)
	}
}

func TestFormatRequest_Examine(t *testing.T) {
	req := NewExamineRequest("user-profile", "gt-abc", "Add user profile with avatar", []string{"src/pages/**", "internal/api/**"})
	body := FormatRequest(req)

	checks := []string{
		"## Architect Consultation: examine",
		"**Feature:** user-profile",
		"**Bead:** gt-abc",
		"**Request Type:** examine",
		"### Description",
		"Add user profile with avatar",
		"### Affected Paths",
		"`src/pages/**`",
		"`internal/api/**`",
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("FormatRequest missing %q in:\n%s", check, body)
		}
	}
}

func TestFormatRequest_Question(t *testing.T) {
	req := NewQuestion("general", "How does the auth middleware work?")
	body := FormatRequest(req)

	if !strings.Contains(body, "## Architect Consultation: question") {
		t.Error("missing question header")
	}
	if !strings.Contains(body, "How does the auth middleware work?") {
		t.Error("missing question body")
	}
	// Should NOT have affected paths section
	if strings.Contains(body, "### Affected Paths") {
		t.Error("question should not have affected paths")
	}
}

func TestFormatRequest_WithContext(t *testing.T) {
	req := Request{
		Type:        RequestExamine,
		FeatureName: "feature",
		Description: "desc",
		Context:     "The conductor plans to split this into 3 PRs",
	}
	body := FormatRequest(req)

	if !strings.Contains(body, "### Additional Context") {
		t.Error("missing context section")
	}
	if !strings.Contains(body, "The conductor plans to split this into 3 PRs") {
		t.Error("missing context content")
	}
}

func TestFormatResponse_Full(t *testing.T) {
	resp := Response{
		Type:               RequestExamine,
		Summary:            "The user module is well-structured",
		ArchitectureReport: "Auth uses JWT middleware...",
		APIContracts:       "GET /api/users/:id returns UserProfile",
		ModelSpecs:         "users table needs avatar_url column",
		Recommendations:    []string{"Add input validation", "Use transactions"},
		Risks:              []string{"Breaking change to API"},
	}
	body := FormatResponse(resp)

	checks := []string{
		"## Architect Response: examine",
		"### Summary",
		"The user module is well-structured",
		"### Architecture Report",
		"Auth uses JWT middleware",
		"### API Contracts",
		"GET /api/users/:id",
		"### Model Specifications",
		"avatar_url column",
		"### Recommendations",
		"- Add input validation",
		"- Use transactions",
		"### Risks",
		"- Breaking change to API",
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("FormatResponse missing %q", check)
		}
	}
}

func TestFormatResponse_Minimal(t *testing.T) {
	resp := Response{
		Type:    RequestQuestion,
		Summary: "The auth middleware is at internal/middleware/auth.go",
	}
	body := FormatResponse(resp)

	if !strings.Contains(body, "### Summary") {
		t.Error("missing summary")
	}
	// Should NOT have empty sections
	if strings.Contains(body, "### Architecture Report") {
		t.Error("should not have empty architecture report section")
	}
	if strings.Contains(body, "### Recommendations") {
		t.Error("should not have empty recommendations section")
	}
}

func TestConsultationSubject(t *testing.T) {
	tests := []struct {
		reqType RequestType
		feature string
		want    string
	}{
		{RequestExamine, "user-profile", "[examine] user-profile"},
		{RequestAPIContract, "auth", "[api-contract] auth"},
		{RequestModelSpec, "payments", "[model-spec] payments"},
		{RequestSpecReview, "search", "[spec-review] search"},
		{RequestQuestion, "general", "[question] general"},
	}
	for _, tt := range tests {
		req := Request{Type: tt.reqType, FeatureName: tt.feature}
		got := consultationSubject(req)
		if got != tt.want {
			t.Errorf("consultationSubject(%s, %s) = %q, want %q", tt.reqType, tt.feature, got, tt.want)
		}
	}
}

func TestCreateConsultationMessage(t *testing.T) {
	req := NewExamineRequest("user-profile", "gt-abc", "Add user profile", nil)
	msg := CreateConsultationMessage("gastown/conductor", "gastown", req)

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
		t.Errorf("Type = %q, want %q", msg.Type, mail.TypeTask)
	}
	if msg.Priority != mail.PriorityHigh {
		t.Errorf("Priority = %q, want %q", msg.Priority, mail.PriorityHigh)
	}
	if msg.ThreadID == "" {
		t.Error("ThreadID should be set")
	}
	if !strings.Contains(msg.Body, "## Architect Consultation") {
		t.Error("body should contain formatted request")
	}
}

func TestCreateResponseMessage(t *testing.T) {
	// Simulate the full flow: request → response
	req := NewExamineRequest("user-profile", "gt-abc", "Add user profile", nil)
	original := CreateConsultationMessage("gastown/conductor", "gastown", req)

	resp := Response{
		Type:    RequestExamine,
		Summary: "Codebase is ready for this change",
	}
	reply := CreateResponseMessage("gastown", "gastown/conductor", original, resp)

	if reply.From != "gastown/architect" {
		t.Errorf("From = %q", reply.From)
	}
	if reply.To != "gastown/conductor" {
		t.Errorf("To = %q", reply.To)
	}
	if reply.Subject != "Re: [examine] user-profile" {
		t.Errorf("Subject = %q", reply.Subject)
	}
	if reply.Type != mail.TypeReply {
		t.Errorf("Type = %q, want %q", reply.Type, mail.TypeReply)
	}
	if reply.ThreadID != original.ThreadID {
		t.Errorf("ThreadID = %q, want %q (inherited)", reply.ThreadID, original.ThreadID)
	}
	if reply.ReplyTo != original.ID {
		t.Errorf("ReplyTo = %q, want %q", reply.ReplyTo, original.ID)
	}
}

func TestRequestTypes_AllDefined(t *testing.T) {
	types := []RequestType{
		RequestExamine,
		RequestAPIContract,
		RequestModelSpec,
		RequestSpecReview,
		RequestQuestion,
	}
	for _, rt := range types {
		if rt == "" {
			t.Error("found empty RequestType")
		}
	}
}
