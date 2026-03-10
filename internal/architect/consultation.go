// Package architect provides the Architect consultation protocol.
//
// The Architect is a per-rig codebase oracle that other agents (conductor,
// artisans) can consult via mail. It handles:
//   - Phase 1 examination: deep codebase analysis for a proposed change
//   - API contract design: endpoint specifications for cross-specialty coordination
//   - Model/DB spec design: database schema changes
//   - On-demand Q&A: any agent can ask architecture questions
//
// All communication uses the mail system's threading support.
package architect

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/mail"
)

// RequestType categorizes what the requester needs from the Architect.
type RequestType string

const (
	// RequestExamine asks for Phase 1 codebase examination.
	RequestExamine RequestType = "examine"

	// RequestAPIContract asks for API endpoint contract design.
	RequestAPIContract RequestType = "api-contract"

	// RequestModelSpec asks for database/model specification.
	RequestModelSpec RequestType = "model-spec"

	// RequestSpecReview asks the Architect to review Phase 4 test specifications.
	RequestSpecReview RequestType = "spec-review"

	// RequestQuestion is a general architecture question.
	RequestQuestion RequestType = "question"
)

// Request represents a structured consultation request to the Architect.
type Request struct {
	// Type is the kind of consultation being requested.
	Type RequestType

	// FeatureName identifies the feature being worked on.
	FeatureName string

	// BeadID is the parent bead this consultation relates to.
	BeadID string

	// Description provides context for the request.
	Description string

	// AffectedPaths lists file paths or patterns the change will touch.
	AffectedPaths []string

	// Context is additional context (e.g., conductor's plan summary).
	Context string
}

// Response represents a structured response from the Architect.
type Response struct {
	// Type matches the request type this responds to.
	Type RequestType

	// Summary is a brief overview of findings.
	Summary string

	// ArchitectureReport is the detailed analysis (Phase 1 examine).
	ArchitectureReport string

	// APIContracts describes new/changed API endpoints (Phase 1 + on-demand).
	APIContracts string

	// ModelSpecs describes database/model changes (Phase 1 + on-demand).
	ModelSpecs string

	// Recommendations are actionable suggestions.
	Recommendations []string

	// Risks are identified concerns.
	Risks []string
}

// FormatRequest converts a Request into a mail message body.
// The body uses a structured markdown format that the Architect agent can parse.
func FormatRequest(req Request) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Architect Consultation: %s\n\n", req.Type))
	b.WriteString(fmt.Sprintf("**Feature:** %s\n", req.FeatureName))

	if req.BeadID != "" {
		b.WriteString(fmt.Sprintf("**Bead:** %s\n", req.BeadID))
	}

	b.WriteString(fmt.Sprintf("**Request Type:** %s\n\n", req.Type))

	if req.Description != "" {
		b.WriteString("### Description\n\n")
		b.WriteString(req.Description)
		b.WriteString("\n\n")
	}

	if len(req.AffectedPaths) > 0 {
		b.WriteString("### Affected Paths\n\n")
		for _, p := range req.AffectedPaths {
			b.WriteString(fmt.Sprintf("- `%s`\n", p))
		}
		b.WriteString("\n")
	}

	if req.Context != "" {
		b.WriteString("### Additional Context\n\n")
		b.WriteString(req.Context)
		b.WriteString("\n")
	}

	return b.String()
}

// FormatResponse converts a Response into a mail message body.
func FormatResponse(resp Response) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("## Architect Response: %s\n\n", resp.Type))

	if resp.Summary != "" {
		b.WriteString("### Summary\n\n")
		b.WriteString(resp.Summary)
		b.WriteString("\n\n")
	}

	if resp.ArchitectureReport != "" {
		b.WriteString("### Architecture Report\n\n")
		b.WriteString(resp.ArchitectureReport)
		b.WriteString("\n\n")
	}

	if resp.APIContracts != "" {
		b.WriteString("### API Contracts\n\n")
		b.WriteString(resp.APIContracts)
		b.WriteString("\n\n")
	}

	if resp.ModelSpecs != "" {
		b.WriteString("### Model Specifications\n\n")
		b.WriteString(resp.ModelSpecs)
		b.WriteString("\n\n")
	}

	if len(resp.Recommendations) > 0 {
		b.WriteString("### Recommendations\n\n")
		for _, r := range resp.Recommendations {
			b.WriteString(fmt.Sprintf("- %s\n", r))
		}
		b.WriteString("\n")
	}

	if len(resp.Risks) > 0 {
		b.WriteString("### Risks\n\n")
		for _, r := range resp.Risks {
			b.WriteString(fmt.Sprintf("- %s\n", r))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// NewExamineRequest creates a Phase 1 examination request.
func NewExamineRequest(featureName, beadID, description string, affectedPaths []string) Request {
	return Request{
		Type:          RequestExamine,
		FeatureName:   featureName,
		BeadID:        beadID,
		Description:   description,
		AffectedPaths: affectedPaths,
	}
}

// NewAPIContractRequest creates an API contract design request.
func NewAPIContractRequest(featureName, beadID, description string) Request {
	return Request{
		Type:        RequestAPIContract,
		FeatureName: featureName,
		BeadID:      beadID,
		Description: description,
	}
}

// NewModelSpecRequest creates a model/DB spec design request.
func NewModelSpecRequest(featureName, beadID, description string) Request {
	return Request{
		Type:        RequestModelSpec,
		FeatureName: featureName,
		BeadID:      beadID,
		Description: description,
	}
}

// NewQuestion creates a general architecture question.
func NewQuestion(featureName, question string) Request {
	return Request{
		Type:        RequestQuestion,
		FeatureName: featureName,
		Description: question,
	}
}

// ArchitectAddress returns the mail address for the Architect of a rig.
func ArchitectAddress(rigName string) string {
	return fmt.Sprintf("%s/architect", rigName)
}

// CreateConsultationMessage builds a mail message for sending to the Architect.
func CreateConsultationMessage(from, rigName string, req Request) *mail.Message {
	subject := consultationSubject(req)
	body := FormatRequest(req)

	msg := mail.NewMessage(from, ArchitectAddress(rigName), subject, body)
	msg.Type = mail.TypeTask
	msg.Priority = mail.PriorityHigh

	return msg
}

// CreateResponseMessage builds a reply message from the Architect.
func CreateResponseMessage(rigName, recipientAddress string, original *mail.Message, resp Response) *mail.Message {
	subject := fmt.Sprintf("Re: %s", original.Subject)
	body := FormatResponse(resp)

	return mail.NewReplyMessage(
		ArchitectAddress(rigName),
		recipientAddress,
		subject,
		body,
		original,
	)
}

// consultationSubject generates a subject line for a consultation request.
func consultationSubject(req Request) string {
	switch req.Type {
	case RequestExamine:
		return fmt.Sprintf("[examine] %s", req.FeatureName)
	case RequestAPIContract:
		return fmt.Sprintf("[api-contract] %s", req.FeatureName)
	case RequestModelSpec:
		return fmt.Sprintf("[model-spec] %s", req.FeatureName)
	case RequestSpecReview:
		return fmt.Sprintf("[spec-review] %s", req.FeatureName)
	case RequestQuestion:
		return fmt.Sprintf("[question] %s", req.FeatureName)
	default:
		return fmt.Sprintf("[consultation] %s", req.FeatureName)
	}
}
