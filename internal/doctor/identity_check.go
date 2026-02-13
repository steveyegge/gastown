package doctor

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/lock"
)

// IdentityCollisionCheck checks for agent identity collisions and stale locks.
type IdentityCollisionCheck struct {
	BaseCheck
}

// NewIdentityCollisionCheck creates a new identity collision check.
func NewIdentityCollisionCheck() *IdentityCollisionCheck {
	return &IdentityCollisionCheck{
		BaseCheck: BaseCheck{
			CheckName:        "identity-collision",
			CheckDescription: "Check for agent identity collisions and stale locks",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

func (c *IdentityCollisionCheck) CanFix() bool {
	return true // Can fix stale locks
}

func (c *IdentityCollisionCheck) Run(ctx *CheckContext) *CheckResult {
	// Find all locks
	locks, err := lock.FindAllLocks(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("could not scan for locks: %v", err),
		}
	}

	if len(locks) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "no worker locks found",
		}
	}

	var staleLocks []string
	var healthyLocks int

	for workerDir, info := range locks {
		if info.IsStale() {
			staleLocks = append(staleLocks,
				fmt.Sprintf("%s (dead PID %d)", workerDir, info.PID))
			continue
		}

		healthyLocks++
	}

	// Build result
	if len(staleLocks) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("%d worker lock(s), all healthy", healthyLocks),
		}
	}

	result := &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d stale lock(s) found", len(staleLocks)),
		FixHint: "Run 'gt doctor --fix' or 'gt agents fix' to clean up",
	}
	result.Details = append(result.Details, "Stale locks (dead PIDs):")
	for _, s := range staleLocks {
		result.Details = append(result.Details, "  "+s)
	}

	return result
}

func (c *IdentityCollisionCheck) Fix(ctx *CheckContext) error {
	cleaned, err := lock.CleanStaleLocks(ctx.TownRoot)
	if err != nil {
		return fmt.Errorf("cleaning stale locks: %w", err)
	}

	if cleaned > 0 {
		fmt.Printf("  Cleaned %d stale lock(s)\n", cleaned)
	}

	return nil
}
