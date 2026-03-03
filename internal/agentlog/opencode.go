package agentlog

import (
	"context"
	"fmt"
	"time"
)

// OpenCodeAdapter is a placeholder for future OpenCode conversation log support.
// OpenCode stores conversations differently from Claude Code; implement Watch
// here when adding OpenCode telemetry.
//
// See: https://github.com/sst/opencode for OpenCode's storage format.
type OpenCodeAdapter struct{}

func (a *OpenCodeAdapter) AgentType() string { return "opencode" }

// Watch is not yet implemented for OpenCode.
func (a *OpenCodeAdapter) Watch(_ context.Context, _, _ string, _ time.Time) (<-chan AgentEvent, error) {
	return nil, fmt.Errorf("opencode adapter not yet implemented")
}
