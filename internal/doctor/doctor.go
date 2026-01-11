package doctor

// Doctor manages and executes health checks.
type Doctor struct {
	checks []Check
}

// NewDoctor creates a new Doctor with no registered checks.
func NewDoctor() *Doctor {
	return &Doctor{
		checks: make([]Check, 0),
	}
}

// Register adds a check to the doctor's check list.
func (d *Doctor) Register(check Check) {
	d.checks = append(d.checks, check)
}

// RegisterAll adds multiple checks to the doctor's check list.
func (d *Doctor) RegisterAll(checks ...Check) {
	d.checks = append(d.checks, checks...)
}

// Checks returns the list of registered checks.
func (d *Doctor) Checks() []Check {
	return d.checks
}

// categoryGetter interface for checks that provide a category
type categoryGetter interface {
	Category() string
}

// Run executes all registered checks and returns a report.
func (d *Doctor) Run(ctx *CheckContext) *Report {
	report := NewReport()

	for _, check := range d.checks {
		result := check.Run(ctx)
		// Ensure check name is populated
		if result.Name == "" {
			result.Name = check.Name()
		}
		// Set category from check if available
		if cg, ok := check.(categoryGetter); ok && result.Category == "" {
			result.Category = cg.Category()
		}
		report.Add(result)
	}

	return report
}

// Fix runs all checks with auto-fix enabled where possible.
// It first runs the check, then if it fails and can be fixed, attempts the fix.
func (d *Doctor) Fix(ctx *CheckContext) *Report {
	report := NewReport()

	for _, check := range d.checks {
		result := check.Run(ctx)
		if result.Name == "" {
			result.Name = check.Name()
		}
		// Set category from check if available
		if cg, ok := check.(categoryGetter); ok && result.Category == "" {
			result.Category = cg.Category()
		}

		// Attempt fix if check failed and is fixable
		if result.Status != StatusOK && check.CanFix() {
			err := check.Fix(ctx)
			if err == nil {
				// Re-run check to verify fix worked
				result = check.Run(ctx)
				if result.Name == "" {
					result.Name = check.Name()
				}
				// Set category again after re-run
				if cg, ok := check.(categoryGetter); ok && result.Category == "" {
					result.Category = cg.Category()
				}
				// Update message to indicate fix was applied
				if result.Status == StatusOK {
					result.Message = result.Message + " (fixed)"
				}
			} else {
				// Fix failed, add error to details
				result.Details = append(result.Details, "Fix failed: "+err.Error())
			}
		}

		report.Add(result)
	}

	return report
}

// BaseCheck provides a base implementation for checks that don't support auto-fix.
// Embed this in custom checks to get default CanFix() and Fix() implementations.
type BaseCheck struct {
	CheckName        string
	CheckDescription string
	CheckCategory    string // Category for grouping (e.g., CategoryCore)
}

// Category returns the check's category for grouping in output.
func (b *BaseCheck) Category() string {
	return b.CheckCategory
}

// Name returns the check name.
func (b *BaseCheck) Name() string {
	return b.CheckName
}

// Description returns the check description.
func (b *BaseCheck) Description() string {
	return b.CheckDescription
}

// CanFix returns false by default.
func (b *BaseCheck) CanFix() bool {
	return false
}

// Fix returns an error indicating this check cannot be auto-fixed.
func (b *BaseCheck) Fix(ctx *CheckContext) error {
	return ErrCannotFix
}

// FixableCheck provides a base implementation for checks that support auto-fix.
// Embed this and override CanFix() to return true, and implement Fix().
type FixableCheck struct {
	BaseCheck
}

// CanFix returns true for fixable checks.
func (f *FixableCheck) CanFix() bool {
	return true
}
