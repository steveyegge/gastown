package orchestrator

import "regexp"

// Outcome represents a classified outcome from polecat output.
type Outcome string

const (
	OutcomeTestPass  Outcome = "test_pass"
	OutcomeTestFail  Outcome = "test_fail"
	OutcomeBuildOK   Outcome = "build_ok"
	OutcomeBuildFail Outcome = "build_fail"
	OutcomeGitPush   Outcome = "git_push"
	OutcomePRCreated Outcome = "pr_created"
	OutcomeCommit    Outcome = "commit"
	OutcomeAmbiguous Outcome = "ambiguous"
)

// OutcomeCategory groups outcomes into routing categories.
type OutcomeCategory string

const (
	CategorySuccess   OutcomeCategory = "success"
	CategoryFailure   OutcomeCategory = "failure"
	CategoryAmbiguous OutcomeCategory = "ambiguous"
)

// MatchResult is the output of pattern matching.
type MatchResult struct {
	Outcome  Outcome
	Category OutcomeCategory
	Pattern  string // The pattern that matched (for debugging).
}

type rule struct {
	pattern  *regexp.Regexp
	outcome  Outcome
	category OutcomeCategory
}

// Matcher classifies polecat output into known outcomes using regex patterns.
type Matcher struct {
	rules []rule
}

// NewMatcher creates a Matcher with the standard pattern set.
// Rules are checked in order — first match wins. Failure patterns are checked
// before success patterns so that mixed output (e.g., some tests pass, some fail)
// is classified as failure.
func NewMatcher() *Matcher {
	return &Matcher{
		rules: []rule{
			// --- Failure patterns (checked first) ---

			// Go test failure
			{regexp.MustCompile(`(?m)^FAIL\t`), OutcomeTestFail, CategoryFailure},
			// Bats test failure
			{regexp.MustCompile(`(?m)^not ok \d+`), OutcomeTestFail, CategoryFailure},
			// Pytest failure
			{regexp.MustCompile(`(?m)^FAILED\s+\S+`), OutcomeTestFail, CategoryFailure},
			// Jest failure
			{regexp.MustCompile(`(?m)Tests:\s+\d+ failed`), OutcomeTestFail, CategoryFailure},

			// Build errors
			{regexp.MustCompile(`(?m)^\S+\.go:\d+:\d+:.*`), OutcomeBuildFail, CategoryFailure},
			{regexp.MustCompile(`(?m)^#\s+\S+\ncompilation error`), OutcomeBuildFail, CategoryFailure},
			{regexp.MustCompile(`(?m)cannot find module`), OutcomeBuildFail, CategoryFailure},

			// --- Success patterns ---

			// Go test pass
			{regexp.MustCompile(`(?m)^ok\s+\t\S+\t[\d.]+s`), OutcomeTestPass, CategorySuccess},
			// Bats test pass
			{regexp.MustCompile(`(?m)All \d+ tests passed`), OutcomeTestPass, CategorySuccess},
			// Pytest pass
			{regexp.MustCompile(`(?m)\d+ passed in [\d.]+s`), OutcomeTestPass, CategorySuccess},
			// Jest pass
			{regexp.MustCompile(`(?m)Tests:\s+\d+ passed`), OutcomeTestPass, CategorySuccess},

			// Go build success
			{regexp.MustCompile(`(?m)^go:\s+no errors`), OutcomeBuildOK, CategorySuccess},
			{regexp.MustCompile(`(?m)^go build\s+`), OutcomeBuildOK, CategorySuccess},

			// Git push
			{regexp.MustCompile(`(?m)\[new branch\]\s+\S+\s+->\s+\S+`), OutcomeGitPush, CategorySuccess},
			{regexp.MustCompile(`(?m)set up to track remote branch`), OutcomeGitPush, CategorySuccess},
			{regexp.MustCompile(`(?m)^To\s+\S+\.git$`), OutcomeGitPush, CategorySuccess},

			// PR created
			{regexp.MustCompile(`(?m)https://github\.com/\S+/pull/\d+`), OutcomePRCreated, CategorySuccess},

			// Git commit
			{regexp.MustCompile(`(?m)^\[\S+\s+[0-9a-f]+\]`), OutcomeCommit, CategorySuccess},
		},
	}
}

// Match classifies the given output. Returns OutcomeAmbiguous if no pattern matches.
func (m *Matcher) Match(output string) MatchResult {
	for _, r := range m.rules {
		if r.pattern.MatchString(output) {
			return MatchResult{
				Outcome:  r.outcome,
				Category: r.category,
				Pattern:  r.pattern.String(),
			}
		}
	}
	return MatchResult{
		Outcome:  OutcomeAmbiguous,
		Category: CategoryAmbiguous,
	}
}
