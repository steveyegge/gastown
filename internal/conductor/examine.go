package conductor

import (
	"github.com/steveyegge/gastown/internal/architect"
	"github.com/steveyegge/gastown/internal/mail"
)

// ConductorAddress returns the mail address for the conductor of a rig.
func ConductorAddress(rigName string) string {
	return rigName + "/conductor"
}

// RequestExamination creates a mail message from the conductor to the architect
// requesting a Phase 1 codebase examination for a feature.
func RequestExamination(rigName, featureName, beadID, description string, affectedPaths []string) *mail.Message {
	req := architect.NewExamineRequest(featureName, beadID, description, affectedPaths)
	return architect.CreateConsultationMessage(ConductorAddress(rigName), rigName, req)
}

// RequestAPIContracts creates a mail message requesting API contract design.
func RequestAPIContracts(rigName, featureName, beadID, description string) *mail.Message {
	req := architect.NewAPIContractRequest(featureName, beadID, description)
	return architect.CreateConsultationMessage(ConductorAddress(rigName), rigName, req)
}

// RequestModelSpecs creates a mail message requesting model/DB spec design.
func RequestModelSpecs(rigName, featureName, beadID, description string) *mail.Message {
	req := architect.NewModelSpecRequest(featureName, beadID, description)
	return architect.CreateConsultationMessage(ConductorAddress(rigName), rigName, req)
}

// AskArchitect creates a general question message to the architect.
func AskArchitect(rigName, fromAddress, featureName, question string) *mail.Message {
	req := architect.NewQuestion(featureName, question)
	return architect.CreateConsultationMessage(fromAddress, rigName, req)
}
