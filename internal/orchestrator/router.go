package orchestrator

// Action represents what the daemon should do after classifying an outcome.
type Action string

const (
	// ActionAdvance sends the next formula step to the polecat.
	ActionAdvance Action = "advance"

	// ActionComplete signals that the formula is finished (no more steps).
	ActionComplete Action = "complete"

	// ActionRetry re-sends the current step with a retry instruction.
	ActionRetry Action = "retry"

	// ActionTriage sends the output to Haiku for classification.
	ActionTriage Action = "triage"

	// ActionEscalate sends the situation to the mayor for human review.
	ActionEscalate Action = "escalate"
)

// Decision is the routing decision made by the Router.
type Decision struct {
	Action  Action
	Reason  string
	Attempt int // Current attempt number (for retries).
}

// Router makes routing decisions based on match results and workflow state.
type Router struct {
	// MaxRetries is the maximum number of retry attempts before escalating.
	MaxRetries int
}

// NewRouter creates a Router with default settings.
func NewRouter() *Router {
	return &Router{
		MaxRetries: 3,
	}
}

// Route determines the next action given a match result.
// hasNext indicates whether there are more formula steps after the current one.
func (r *Router) Route(match MatchResult, hasNext bool) Decision {
	return r.RouteWithAttempt(match, hasNext, 1)
}

// RouteWithAttempt determines the next action given a match result and attempt count.
func (r *Router) RouteWithAttempt(match MatchResult, hasNext bool, attempt int) Decision {
	switch match.Category {
	case CategorySuccess:
		if hasNext {
			return Decision{Action: ActionAdvance, Reason: string(match.Outcome)}
		}
		return Decision{Action: ActionComplete, Reason: string(match.Outcome)}

	case CategoryFailure:
		if attempt > r.MaxRetries {
			return Decision{
				Action:  ActionEscalate,
				Reason:  "max retries exceeded: " + string(match.Outcome),
				Attempt: attempt,
			}
		}
		return Decision{
			Action:  ActionRetry,
			Reason:  string(match.Outcome),
			Attempt: attempt,
		}

	case CategoryAmbiguous:
		return Decision{
			Action: ActionTriage,
			Reason: "pattern match failed, needs Haiku triage",
		}

	default:
		return Decision{
			Action: ActionEscalate,
			Reason: "unknown category: " + string(match.Category),
		}
	}
}

// RouteAfterTriage determines the next action when Haiku triage was inconclusive.
// If triage itself couldn't determine the outcome, escalate to the mayor.
func (r *Router) RouteAfterTriage(match MatchResult, hasNext bool) Decision {
	// After triage fails, always escalate — Haiku was the last automated backstop.
	return Decision{
		Action: ActionEscalate,
		Reason: "Haiku triage inconclusive, escalating to mayor",
	}
}
