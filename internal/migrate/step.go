package migrate

// Step is the unit of migration work.
// Each step should be idempotent - it can check if it's already been applied
// and skip itself if so. Steps should also be reversible via Rollback.
type Step interface {
	// ID returns a unique identifier for this step.
	// Used for logging, tracking, and rollback purposes.
	ID() string

	// Description returns a human-readable description of what this step does.
	Description() string

	// Check returns true if this step needs to run.
	// This enables idempotency - if the step has already been applied,
	// Check should return (false, nil).
	Check(ctx *Context) (needed bool, err error)

	// Execute performs the migration step.
	// Should only be called if Check returned (true, nil).
	Execute(ctx *Context) error

	// Rollback undoes the migration step.
	// Should restore the workspace to its state before Execute was called.
	Rollback(ctx *Context) error

	// Verify confirms the step was applied successfully.
	// Called after Execute to ensure the migration worked.
	Verify(ctx *Context) error
}

// BaseStep provides a base implementation with common functionality.
// Embed this in custom steps to get default implementations.
type BaseStep struct {
	StepID          string
	StepDescription string
}

// ID returns the step identifier.
func (s *BaseStep) ID() string {
	return s.StepID
}

// Description returns the step description.
func (s *BaseStep) Description() string {
	return s.StepDescription
}

// Check returns true by default (step needs to run).
// Override this in specific steps to add idempotency checks.
func (s *BaseStep) Check(ctx *Context) (bool, error) {
	return true, nil
}

// Rollback does nothing by default.
// Override this in specific steps to add rollback logic.
func (s *BaseStep) Rollback(ctx *Context) error {
	return nil
}

// Verify does nothing by default (assumes success).
// Override this in specific steps to add verification logic.
func (s *BaseStep) Verify(ctx *Context) error {
	return nil
}

// FuncStep is a Step implementation that wraps simple functions.
// Useful for simple migration steps that don't need complex state.
type FuncStep struct {
	StepID          string
	StepDescription string
	CheckFunc       func(ctx *Context) (bool, error)
	ExecuteFunc     func(ctx *Context) error
	RollbackFunc    func(ctx *Context) error
	VerifyFunc      func(ctx *Context) error
}

// ID returns the step identifier.
func (s *FuncStep) ID() string {
	return s.StepID
}

// Description returns the step description.
func (s *FuncStep) Description() string {
	return s.StepDescription
}

// Check runs the check function if provided.
func (s *FuncStep) Check(ctx *Context) (bool, error) {
	if s.CheckFunc != nil {
		return s.CheckFunc(ctx)
	}
	return true, nil
}

// Execute runs the execute function.
func (s *FuncStep) Execute(ctx *Context) error {
	if s.ExecuteFunc != nil {
		return s.ExecuteFunc(ctx)
	}
	return nil
}

// Rollback runs the rollback function if provided.
func (s *FuncStep) Rollback(ctx *Context) error {
	if s.RollbackFunc != nil {
		return s.RollbackFunc(ctx)
	}
	return nil
}

// Verify runs the verify function if provided.
func (s *FuncStep) Verify(ctx *Context) error {
	if s.VerifyFunc != nil {
		return s.VerifyFunc(ctx)
	}
	return nil
}
