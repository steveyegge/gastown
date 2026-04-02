// Package integrations defines the plugin architecture for external service
// integrations (GitHub Issues, PagerDuty, Jira, Linear, etc.).
package integrations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/outdoorsea/faultline/internal/notify"
)

// IntegrationType identifies an integration provider.
type IntegrationType string

const (
	TypeGitHubIssues IntegrationType = "github_issues"
	TypePagerDuty    IntegrationType = "pagerduty"
	TypeJira         IntegrationType = "jira"
	TypeLinear       IntegrationType = "linear"
	TypeSlackBot     IntegrationType = "slack_bot"
)

// ValidTypes returns all recognized integration types.
func ValidTypes() []IntegrationType {
	return []IntegrationType{TypeGitHubIssues, TypePagerDuty, TypeJira, TypeLinear, TypeSlackBot}
}

// IsValidType checks if t is a recognized integration type.
func IsValidType(t IntegrationType) bool {
	for _, v := range ValidTypes() {
		if v == t {
			return true
		}
	}
	return false
}

// Integration is the interface that all external integrations must implement.
// Each method corresponds to a faultline lifecycle event.
type Integration interface {
	// Type returns the integration provider identifier.
	Type() IntegrationType

	// OnNewIssue is called when a new issue group is created.
	OnNewIssue(ctx context.Context, event notify.Event) error

	// OnResolved is called when an issue is resolved.
	OnResolved(ctx context.Context, event notify.Event) error

	// OnRegression is called when a resolved issue regresses.
	OnRegression(ctx context.Context, event notify.Event) error
}

// Factory creates an Integration instance from a JSON config blob.
type Factory func(config json.RawMessage) (Integration, error)

// Registry maps integration types to their factory functions.
var registry = map[IntegrationType]Factory{}

// Register adds a factory for the given integration type.
func Register(t IntegrationType, f Factory) {
	registry[t] = f
}

// New creates an integration of the given type from its config.
func New(t IntegrationType, config json.RawMessage) (Integration, error) {
	f, ok := registry[t]
	if !ok {
		return nil, fmt.Errorf("unknown integration type: %s", t)
	}
	return f(config)
}

// Registered returns true if a factory exists for the type.
func Registered(t IntegrationType) bool {
	_, ok := registry[t]
	return ok
}

// Dispatch sends a notify.Event to an integration based on event type.
func Dispatch(ctx context.Context, intg Integration, event notify.Event) error {
	switch event.Type {
	case notify.EventNewIssue:
		return intg.OnNewIssue(ctx, event)
	case notify.EventResolved:
		return intg.OnResolved(ctx, event)
	case notify.EventRegression:
		return intg.OnRegression(ctx, event)
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}
