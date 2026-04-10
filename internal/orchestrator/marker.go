// Package orchestrator provides the step orchestrator for multi-turn polecat
// sessions. The orchestrator is a Go daemon component (no Claude) that detects
// STEP_COMPLETE markers in tmux output, pattern matches outcomes, and routes
// to the next action.
package orchestrator

import (
	"regexp"
	"strings"
)

// StepCompleteMarker is the marker polecats output when a formula step finishes.
const StepCompleteMarker = "STEP_COMPLETE"

// markerPattern matches "STEP_COMPLETE <step-id>" with optional whitespace.
var markerPattern = regexp.MustCompile(`^STEP_COMPLETE\s+(\S+)`)

// MarkerResult holds the parsed result of a STEP_COMPLETE detection.
type MarkerResult struct {
	// StepID is the step that completed.
	StepID string

	// Body is the tmux output between the last marker (or start) and this marker.
	Body string
}

// DetectStepComplete scans output for the last STEP_COMPLETE marker.
// Returns nil if no marker is found.
func DetectStepComplete(output string) *MarkerResult {
	lines := strings.Split(output, "\n")

	var lastMarkerIdx int = -1
	var lastStepID string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if matches := markerPattern.FindStringSubmatch(trimmed); len(matches) >= 2 {
			lastMarkerIdx = i
			lastStepID = matches[1]
		}
	}

	if lastMarkerIdx < 0 {
		return nil
	}

	// Find previous marker to extract body between markers.
	prevMarkerIdx := -1
	for i := lastMarkerIdx - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if markerPattern.MatchString(trimmed) {
			prevMarkerIdx = i
			break
		}
	}

	// Body is everything between previous marker (or start) and this marker.
	startIdx := prevMarkerIdx + 1
	bodyLines := lines[startIdx:lastMarkerIdx]

	// Trim trailing empty lines.
	for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[len(bodyLines)-1]) == "" {
		bodyLines = bodyLines[:len(bodyLines)-1]
	}

	return &MarkerResult{
		StepID: lastStepID,
		Body:   strings.Join(bodyLines, "\n"),
	}
}

// ExtractOutputSinceLastPrompt returns the output after the last occurrence
// of promptMark in the output. If no prompt is found, returns the full output
// with leading/trailing whitespace trimmed.
func ExtractOutputSinceLastPrompt(output, promptMark string) string {
	if output == "" {
		return ""
	}

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	lastPromptIdx := -1
	for i, line := range lines {
		if strings.Contains(line, promptMark) {
			lastPromptIdx = i
		}
	}

	if lastPromptIdx < 0 {
		return strings.TrimRight(output, "\n")
	}

	// Include the prompt line (strip the prompt marker prefix) and everything after.
	result := lines[lastPromptIdx:]
	if len(result) > 0 {
		// Remove the prompt marker from the first line.
		first := result[0]
		idx := strings.Index(first, promptMark)
		if idx >= 0 {
			first = strings.TrimSpace(first[idx+len(promptMark):])
		}
		result[0] = first
	}

	return strings.Join(result, "\n")
}
