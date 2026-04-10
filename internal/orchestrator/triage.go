package orchestrator

import (
	"context"
	"fmt"
)

// TriageVerdict is the outcome of Haiku triage.
type TriageVerdict string

const (
	TriageSuccess TriageVerdict = "success"
	TriageFailure TriageVerdict = "failure"
	TriageUnsure  TriageVerdict = "unsure"
)

// TriageResult holds the Haiku triage assessment.
type TriageResult struct {
	Verdict TriageVerdict
	Reason  string
}

// ToAction converts a triage verdict to a routing action.
func (r TriageResult) ToAction() Action {
	switch r.Verdict {
	case TriageSuccess:
		return ActionAdvance
	case TriageFailure:
		return ActionRetry
	default:
		return ActionEscalate
	}
}

// TriageClient sends ambiguous output to Haiku for classification.
type TriageClient interface {
	Triage(ctx context.Context, body string, stepID string) (TriageResult, error)
}

// BuildTriagePrompt constructs the prompt sent to Haiku for triage.
func BuildTriagePrompt(body, stepID string) string {
	return fmt.Sprintf(`A polecat agent completed formula step %q and output STEP_COMPLETE, but
the daemon's pattern matcher could not classify the outcome as success or failure.

Review the polecat's output below and classify:
- "success" if the step completed successfully
- "failure" if the step failed (tests failed, build error, etc.)
- "unsure" if you genuinely cannot determine the outcome

Respond with exactly one word: success, failure, or unsure.
Then on the next line, a brief reason.

--- Polecat output ---
%s
--- End output ---`, stepID, body)
}
