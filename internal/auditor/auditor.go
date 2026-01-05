// Package auditor provides independent code verification using alternate AI models.
// The Auditor agent reviews work before merge to catch issues that the original
// model might have missed.
package auditor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
)

// Common errors
var (
	ErrNoRuntime      = errors.New("no verification runtime available")
	ErrBeadNotFound   = errors.New("bead not found")
	ErrInvalidBead    = errors.New("invalid bead for verification")
	ErrParseResponse  = errors.New("failed to parse verification response")
)

// Verdict represents the outcome of a verification.
type Verdict string

const (
	// VerdictPass indicates the work passed verification.
	VerdictPass Verdict = "PASS"

	// VerdictFail indicates the work failed verification.
	VerdictFail Verdict = "FAIL"

	// VerdictNeedsHuman indicates human review is required.
	VerdictNeedsHuman Verdict = "NEEDS_HUMAN"
)

// VerificationResult contains the outcome of a verification check.
type VerificationResult struct {
	// BeadID is the ID of the bead that was verified.
	BeadID string `json:"bead_id"`

	// Verdict is the verification outcome (PASS, FAIL, NEEDS_HUMAN).
	Verdict Verdict `json:"verdict"`

	// Confidence is how confident the auditor is in the verdict (0.0-1.0).
	Confidence float64 `json:"confidence"`

	// Issues contains any problems found during verification.
	Issues []string `json:"issues,omitempty"`

	// Suggestions contains improvement suggestions (not blocking).
	Suggestions []string `json:"suggestions,omitempty"`

	// ReviewedBy is the name of the runtime that performed the review.
	ReviewedBy string `json:"reviewed_by"`

	// IsIndependent indicates if the review was by a different model than Claude.
	// True independent verification provides stronger guarantees.
	IsIndependent bool `json:"is_independent"`

	// ReviewedAt is when the verification was performed.
	ReviewedAt time.Time `json:"reviewed_at"`

	// Duration is how long the verification took.
	Duration time.Duration `json:"duration"`
}

// IsPass returns true if the verification passed.
func (r *VerificationResult) IsPass() bool {
	return r.Verdict == VerdictPass
}

// IsFail returns true if the verification failed.
func (r *VerificationResult) IsFail() bool {
	return r.Verdict == VerdictFail
}

// NeedsHuman returns true if human review is required.
func (r *VerificationResult) NeedsHuman() bool {
	return r.Verdict == VerdictNeedsHuman
}

// Auditor performs independent verification of work using an alternate AI model.
type Auditor struct {
	runtime  agent.Runtime
	beadsDB  *beads.Beads
	registry *agent.RuntimeRegistry
}

// New creates a new Auditor with the appropriate runtime for verification.
// Uses the registry's GetForRole("auditor") to select the best available runtime.
// Returns an error if no runtime is available - verification is mandatory.
func New(registry *agent.RuntimeRegistry, db *beads.Beads) (*Auditor, error) {
	runtime, err := registry.RequireRuntime("auditor")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoRuntime, err)
	}

	return &Auditor{
		runtime:  runtime,
		beadsDB:  db,
		registry: registry,
	}, nil
}

// MustNew creates a new Auditor, panicking if no runtime is available.
// Use this when verification is mandatory and the system cannot proceed without it.
func MustNew(registry *agent.RuntimeRegistry, db *beads.Beads) *Auditor {
	aud, err := New(registry, db)
	if err != nil {
		panic(fmt.Sprintf("mandatory verification requires an LLM runtime: %v", err))
	}
	return aud
}

// NewWithRuntime creates an Auditor with a specific runtime.
// This is useful for testing or when a specific runtime is required.
func NewWithRuntime(runtime agent.Runtime, db *beads.Beads) *Auditor {
	return &Auditor{
		runtime: runtime,
		beadsDB: db,
	}
}

// RuntimeName returns the name of the runtime being used for verification.
func (a *Auditor) RuntimeName() string {
	if a.runtime == nil {
		return ""
	}
	return a.runtime.Name()
}

// IsIndependent returns true if the verification uses a different model than Claude.
// True independent verification provides a second opinion from a different AI model.
func (a *Auditor) IsIndependent() bool {
	if a.runtime == nil {
		return false
	}
	return a.runtime.Name() != "claude"
}

// Verify performs verification on a bead's associated work.
// It fetches the bead details, constructs a verification prompt,
// and executes it using the configured runtime.
func (a *Auditor) Verify(ctx context.Context, beadID string, workdir string) (*VerificationResult, error) {
	if a.runtime == nil {
		return nil, ErrNoRuntime
	}

	startTime := time.Now()

	// Get bead details
	bead, err := a.beadsDB.Show(beadID)
	if err != nil {
		if errors.Is(err, beads.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrBeadNotFound, beadID)
		}
		return nil, fmt.Errorf("fetching bead: %w", err)
	}

	// Build verification prompt
	prompt := a.buildPrompt(bead, workdir)

	// Execute with configured runtime
	response, err := a.runtime.Execute(ctx, prompt, workdir)
	if err != nil {
		return &VerificationResult{
			BeadID:     beadID,
			Verdict:    VerdictNeedsHuman,
			Confidence: 0,
			Issues:     []string{fmt.Sprintf("Verification execution failed: %v", err)},
			ReviewedBy: a.runtime.Name(),
			ReviewedAt: time.Now(),
			Duration:   time.Since(startTime),
		}, nil
	}

	// Parse response
	result, err := a.parseResponse(response, beadID)
	if err != nil {
		// If parsing fails, treat as needs human review
		result = &VerificationResult{
			BeadID:     beadID,
			Verdict:    VerdictNeedsHuman,
			Confidence: 0,
			Issues:     []string{"Failed to parse verification response", err.Error()},
		}
	}

	result.ReviewedBy = a.runtime.Name()
	result.IsIndependent = a.IsIndependent()
	result.ReviewedAt = time.Now()
	result.Duration = time.Since(startTime)

	return result, nil
}

// VerifyMR performs verification specifically for a merge request.
// It uses additional context about the MR such as the branch and target.
func (a *Auditor) VerifyMR(ctx context.Context, mrID string, branch string, targetBranch string, workdir string) (*VerificationResult, error) {
	if a.runtime == nil {
		return nil, ErrNoRuntime
	}

	startTime := time.Now()

	// Build MR-specific verification prompt
	prompt := a.buildMRPrompt(mrID, branch, targetBranch, workdir)

	// Execute with configured runtime
	response, err := a.runtime.Execute(ctx, prompt, workdir)
	if err != nil {
		return &VerificationResult{
			BeadID:     mrID,
			Verdict:    VerdictNeedsHuman,
			Confidence: 0,
			Issues:     []string{fmt.Sprintf("Verification execution failed: %v", err)},
			ReviewedBy: a.runtime.Name(),
			ReviewedAt: time.Now(),
			Duration:   time.Since(startTime),
		}, nil
	}

	// Parse response
	result, err := a.parseResponse(response, mrID)
	if err != nil {
		result = &VerificationResult{
			BeadID:     mrID,
			Verdict:    VerdictNeedsHuman,
			Confidence: 0,
			Issues:     []string{"Failed to parse verification response"},
		}
	}

	result.ReviewedBy = a.runtime.Name()
	result.IsIndependent = a.IsIndependent()
	result.ReviewedAt = time.Now()
	result.Duration = time.Since(startTime)

	return result, nil
}

// buildPrompt constructs the verification prompt for a bead.
func (a *Auditor) buildPrompt(bead *beads.Issue, workdir string) string {
	return fmt.Sprintf(`You are a senior code reviewer performing MANDATORY independent verification.
Your review is required before any code can be merged. Be thorough and rigorous.

## Task Being Verified

Task ID: %s
Title: %s
Description: %s
Working Directory: %s

## Verification Steps

You MUST perform ALL of the following checks:

### 1. Requirements Verification
- Read the task description carefully
- Check if ALL requirements are implemented
- Verify no scope creep (no unrelated changes)
- Ensure the implementation matches the intent

### 2. Code Quality Analysis
- Run: git diff HEAD~1 (or appropriate command to see changes)
- Check code structure and organization
- Verify naming conventions are followed
- Look for code duplication
- Check for proper error handling
- Verify logging is appropriate

### 3. Bug Detection
- Look for off-by-one errors
- Check null/nil handling
- Verify edge cases are handled
- Check for race conditions in concurrent code
- Verify resource cleanup (files, connections, etc.)

### 4. Security Review
- Check for injection vulnerabilities (SQL, command, XSS)
- Verify authentication/authorization is correct
- Look for hardcoded secrets or credentials
- Check for sensitive data exposure
- Verify input validation

### 5. Test Coverage
- Check if tests exist for new code
- Verify tests cover main success paths
- Check for edge case tests
- Verify tests actually assert correct behavior
- Run: go test ./... (or appropriate test command)

### 6. Build Verification
- Run: go build ./... (or appropriate build command)
- Check for compiler warnings
- Verify no linting errors

## Response Format

After completing ALL checks, respond with ONLY valid JSON:

{
  "verdict": "PASS" | "FAIL" | "NEEDS_HUMAN",
  "confidence": 0.0-1.0,
  "issues": ["issue1", "issue2"],
  "suggestions": ["suggestion1", "suggestion2"]
}

## Verdict Guidelines

**PASS** (confidence >= 0.8):
- All requirements are met
- No bugs found
- No security issues
- Tests are adequate
- Code quality is good

**FAIL** (any of these):
- Missing required functionality
- Bugs that affect correctness
- Security vulnerabilities
- Build or test failures
- Critical code quality issues

**NEEDS_HUMAN** (confidence < 0.7 or):
- Unable to verify requirements (unclear spec)
- Complex logic that needs human review
- Architectural decisions needed
- Cannot run tests/build

Be strict but fair. Only mark PASS if you are confident the code is production-ready.`, bead.ID, bead.Title, bead.Description, workdir)
}

// buildMRPrompt constructs a verification prompt specifically for MRs.
func (a *Auditor) buildMRPrompt(mrID, branch, targetBranch, workdir string) string {
	return fmt.Sprintf(`You are a senior code reviewer performing MANDATORY verification of a merge request.
This review is REQUIRED before the code can be merged. Be thorough and rigorous.

## Merge Request Details

MR ID: %s
Source Branch: %s
Target Branch: %s
Working Directory: %s

## Required Verification Steps

Execute these commands and analyze the output:

### Step 1: View the Changes
'''bash
git fetch origin
git diff origin/%s...origin/%s
git log origin/%s..origin/%s --oneline
'''

### Step 2: Check for Conflicts
'''bash
git checkout %s
git merge --no-commit --no-ff origin/%s
git merge --abort  # Clean up after check
'''

### Step 3: Run Tests
'''bash
go test ./...  # Or appropriate test command
'''

### Step 4: Run Build
'''bash
go build ./...  # Or appropriate build command
'''

### Step 5: Run Linter (if available)
'''bash
golangci-lint run ./...  # Or appropriate lint command
'''

## Review Checklist

You MUST verify ALL of the following:

### Code Quality
- [ ] Code is well-organized and readable
- [ ] Naming conventions are followed
- [ ] No unnecessary code duplication
- [ ] Error handling is appropriate
- [ ] No debug/console statements left in

### Correctness
- [ ] Logic is correct and handles edge cases
- [ ] No off-by-one errors
- [ ] Null/nil cases are handled
- [ ] Resource cleanup is proper
- [ ] Concurrent code is thread-safe

### Security
- [ ] No injection vulnerabilities
- [ ] No hardcoded secrets
- [ ] Input is validated
- [ ] Auth/authz is correct
- [ ] Sensitive data is protected

### Testing
- [ ] Tests exist for new functionality
- [ ] Tests cover success and error paths
- [ ] Tests actually verify behavior (not just coverage)
- [ ] All tests pass

### Integration
- [ ] Changes merge cleanly with no conflicts
- [ ] Build succeeds
- [ ] No regressions in existing functionality

## Response Format

After completing ALL verification steps, respond with ONLY this JSON:

{
  "verdict": "PASS" | "FAIL" | "NEEDS_HUMAN",
  "confidence": 0.0-1.0,
  "issues": ["issue1", "issue2"],
  "suggestions": ["suggestion1", "suggestion2"]
}

## Verdict Criteria

**PASS** (confidence >= 0.8):
- All checklist items verified
- Tests pass
- Build succeeds
- No merge conflicts
- Code is production-ready

**FAIL** (immediately if any):
- Tests fail
- Build fails
- Security vulnerability found
- Critical bug found
- Merge conflicts exist

**NEEDS_HUMAN**:
- Confidence < 0.7
- Complex architectural changes
- Cannot verify some items
- Unclear requirements

Be strict. Only PASS if genuinely production-ready.`, mrID, branch, targetBranch, workdir, targetBranch, branch, targetBranch, branch, targetBranch, branch)
}

// parseResponse extracts a VerificationResult from the runtime's response.
func (a *Auditor) parseResponse(response, beadID string) (*VerificationResult, error) {
	result := &VerificationResult{
		BeadID: beadID,
	}

	// Try to extract JSON from the response
	// The response might have text before/after the JSON
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("%w: no JSON found in response", ErrParseResponse)
	}

	// Parse the JSON
	var parsed struct {
		Verdict     string   `json:"verdict"`
		Confidence  float64  `json:"confidence"`
		Issues      []string `json:"issues"`
		Suggestions []string `json:"suggestions"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseResponse, err)
	}

	// Validate and convert verdict
	switch strings.ToUpper(parsed.Verdict) {
	case "PASS":
		result.Verdict = VerdictPass
	case "FAIL":
		result.Verdict = VerdictFail
	case "NEEDS_HUMAN":
		result.Verdict = VerdictNeedsHuman
	default:
		// Unknown verdict, treat as needs human
		result.Verdict = VerdictNeedsHuman
		result.Issues = append(result.Issues, fmt.Sprintf("Unknown verdict: %s", parsed.Verdict))
	}

	// Clamp confidence to 0-1 range
	result.Confidence = parsed.Confidence
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}

	result.Issues = parsed.Issues
	result.Suggestions = parsed.Suggestions

	return result, nil
}

// extractJSON attempts to find and extract a JSON object from text.
// It looks for the first { and last } to extract the JSON.
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	if start == -1 {
		return ""
	}

	// Find matching closing brace
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}

	return ""
}

// VerificationConfig holds configuration for the verification process.
// Note: Verification is always mandatory - it cannot be disabled.
type VerificationConfig struct {
	// RequiredConfidence is the minimum confidence for auto-approval.
	// Results below this threshold will be escalated to NEEDS_HUMAN.
	RequiredConfidence float64 `json:"required_confidence" yaml:"required_confidence"`

	// PreferredRuntime is the name of the preferred runtime for verification.
	// If not available, fallback order is: codex > opencode > claude.
	PreferredRuntime string `json:"preferred_runtime" yaml:"preferred_runtime"`

	// TimeoutSeconds is the maximum time for a verification operation.
	TimeoutSeconds int `json:"timeout_seconds" yaml:"timeout_seconds"`

	// RequireIndependent if true, requires a different model than Claude.
	// If no alternate model is available, verification will fail rather than
	// falling back to Claude.
	RequireIndependent bool `json:"require_independent" yaml:"require_independent"`
}

// DefaultVerificationConfig returns the default verification configuration.
// Verification is always enabled and mandatory.
func DefaultVerificationConfig() VerificationConfig {
	return VerificationConfig{
		RequiredConfidence: 0.7,
		PreferredRuntime:   "codex",
		TimeoutSeconds:     300, // 5 minutes
		RequireIndependent: false, // Allow Claude fallback by default
	}
}

// StrictVerificationConfig returns a configuration requiring independent verification.
// This ensures a different model reviews the work, providing stronger guarantees.
func StrictVerificationConfig() VerificationConfig {
	return VerificationConfig{
		RequiredConfidence: 0.8,
		PreferredRuntime:   "codex",
		TimeoutSeconds:     300,
		RequireIndependent: true, // Must be a different model
	}
}
