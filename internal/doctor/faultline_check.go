package doctor

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// FaultlineDSNCheck verifies that the FAULTLINE_DSN environment variable is set.
// Without it, error reporting to faultline is disabled for this workspace.
type FaultlineDSNCheck struct {
	BaseCheck
}

// NewFaultlineDSNCheck creates a new faultline DSN configuration check.
func NewFaultlineDSNCheck() *FaultlineDSNCheck {
	return &FaultlineDSNCheck{
		BaseCheck: BaseCheck{
			CheckName:        "faultline-dsn",
			CheckDescription: "Check that FAULTLINE_DSN is configured for error reporting",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if FAULTLINE_DSN is set.
func (c *FaultlineDSNCheck) Run(ctx *CheckContext) *CheckResult {
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "FAULTLINE_DSN not set — error reporting disabled",
			FixHint: "Set FAULTLINE_DSN to enable error reporting (e.g., http://gastown_key@localhost:8080/2)",
		}
	}
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "FAULTLINE_DSN configured",
	}
}

// Fix is not applicable — env vars must be set by the operator.
func (c *FaultlineDSNCheck) Fix(ctx *CheckContext) error { return nil }

// CanFix returns false — env var setup requires operator action.
func (c *FaultlineDSNCheck) CanFix() bool { return false }

// FaultlineReachableCheck verifies that the faultline server is reachable
// by hitting its health endpoint.
type FaultlineReachableCheck struct {
	BaseCheck
}

// NewFaultlineReachableCheck creates a new faultline server reachability check.
func NewFaultlineReachableCheck() *FaultlineReachableCheck {
	return &FaultlineReachableCheck{
		BaseCheck: BaseCheck{
			CheckName:        "faultline-reachable",
			CheckDescription: "Check that the faultline server is reachable",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if the faultline server responds to a health check.
func (c *FaultlineReachableCheck) Run(ctx *CheckContext) *CheckResult {
	dsn := os.Getenv("FAULTLINE_DSN")
	if dsn == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "skipped — FAULTLINE_DSN not set",
		}
	}

	// Extract base URL from DSN. DSN format: http://key@host:port/project_id
	// We just need to hit the host:port part.
	baseURL := os.Getenv("FAULTLINE_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("faultline server unreachable: %v", err),
			FixHint: "Start faultline server: faultline start",
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("faultline server returned status %d", resp.StatusCode),
			FixHint: "Check faultline server logs: faultline status",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "faultline server reachable",
	}
}

// Fix is not applicable — server must be started by the operator.
func (c *FaultlineReachableCheck) Fix(ctx *CheckContext) error { return nil }

// CanFix returns false — requires operator intervention to start the server.
func (c *FaultlineReachableCheck) CanFix() bool { return false }
