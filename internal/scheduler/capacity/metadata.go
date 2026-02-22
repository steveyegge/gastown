package capacity

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SchedulerMetadata holds scheduler dispatch parameters stored in a bead's description.
// Delimited by ---gt:scheduler:v1--- so it can be cleanly parsed without conflicting
// with existing description content. The namespaced delimiter avoids collision
// with user content that might contain generic markdown separators.
type SchedulerMetadata struct {
	TargetRig        string `json:"target_rig"`
	Formula          string `json:"formula,omitempty"`
	Args             string `json:"args,omitempty"`
	Vars             string `json:"vars,omitempty"` // newline-separated key=value pairs
	EnqueuedAt       string `json:"enqueued_at"`
	Merge            string `json:"merge,omitempty"`
	Convoy           string `json:"convoy,omitempty"`
	BaseBranch       string `json:"base_branch,omitempty"`
	NoMerge          bool   `json:"no_merge,omitempty"`
	Account          string `json:"account,omitempty"`
	Agent            string `json:"agent,omitempty"`
	HookRawBead      bool   `json:"hook_raw_bead,omitempty"`
	Owned            bool   `json:"owned,omitempty"`
	Mode             string `json:"mode,omitempty"`
	DispatchFailures int    `json:"dispatch_failures,omitempty"`
	LastFailure      string `json:"last_failure,omitempty"`
}

// MetadataDelimiter is the versioned delimiter for scheduler metadata blocks.
const MetadataDelimiter = "---gt:scheduler:v1---"

// LegacyMetadataDelimiter is the old queue metadata delimiter, supported for
// backward compatibility during migration from gt queue to gt scheduler.
const LegacyMetadataDelimiter = "---gt:queue:v1---"

// LabelScheduled marks a bead as scheduled for dispatch.
const LabelScheduled = "gt:queued"

// sanitizeMetadataValue escapes the delimiter string and newlines in field values
// to prevent metadata block corruption from user-supplied content.
func sanitizeMetadataValue(s string) string {
	s = strings.ReplaceAll(s, MetadataDelimiter, "---gt_scheduler_v1---")
	s = strings.ReplaceAll(s, LegacyMetadataDelimiter, "---gt_queue_v1---")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// FormatMetadata formats metadata as key-value lines for bead description.
func FormatMetadata(m *SchedulerMetadata) string {
	var lines []string
	lines = append(lines, MetadataDelimiter)

	if m.TargetRig != "" {
		lines = append(lines, fmt.Sprintf("target_rig: %s", m.TargetRig))
	}
	if m.Formula != "" {
		lines = append(lines, fmt.Sprintf("formula: %s", sanitizeMetadataValue(m.Formula)))
	}
	if m.Args != "" {
		lines = append(lines, fmt.Sprintf("args: %s", sanitizeMetadataValue(m.Args)))
	}
	// Vars are stored as repeated "var:" lines to avoid lossy delimiters.
	// Values may contain commas, so one line per var is the safe format.
	for _, v := range strings.Split(m.Vars, "\n") {
		v = strings.TrimSpace(v)
		if v != "" {
			lines = append(lines, fmt.Sprintf("var: %s", sanitizeMetadataValue(v)))
		}
	}
	if m.EnqueuedAt != "" {
		lines = append(lines, fmt.Sprintf("enqueued_at: %s", m.EnqueuedAt))
	}
	if m.Merge != "" {
		lines = append(lines, fmt.Sprintf("merge: %s", sanitizeMetadataValue(m.Merge)))
	}
	if m.Convoy != "" {
		lines = append(lines, fmt.Sprintf("convoy: %s", sanitizeMetadataValue(m.Convoy)))
	}
	if m.BaseBranch != "" {
		lines = append(lines, fmt.Sprintf("base_branch: %s", sanitizeMetadataValue(m.BaseBranch)))
	}
	if m.NoMerge {
		lines = append(lines, "no_merge: true")
	}
	if m.Account != "" {
		lines = append(lines, fmt.Sprintf("account: %s", sanitizeMetadataValue(m.Account)))
	}
	if m.Agent != "" {
		lines = append(lines, fmt.Sprintf("agent: %s", sanitizeMetadataValue(m.Agent)))
	}
	if m.HookRawBead {
		lines = append(lines, "hook_raw_bead: true")
	}
	if m.Owned {
		lines = append(lines, "owned: true")
	}
	if m.Mode != "" {
		lines = append(lines, fmt.Sprintf("mode: %s", sanitizeMetadataValue(m.Mode)))
	}
	if m.DispatchFailures > 0 {
		lines = append(lines, fmt.Sprintf("dispatch_failures: %d", m.DispatchFailures))
	}
	if m.LastFailure != "" {
		lines = append(lines, fmt.Sprintf("last_failure: %s", sanitizeMetadataValue(m.LastFailure)))
	}

	return strings.Join(lines, "\n")
}

// ParseMetadata extracts scheduler metadata from a bead description.
// Returns nil if no metadata section is found. Supports both the current
// ---gt:scheduler:v1--- and legacy ---gt:queue:v1--- delimiters.
func ParseMetadata(description string) *SchedulerMetadata {
	// Try current delimiter first, then legacy
	delimiter := MetadataDelimiter
	idx := strings.Index(description, delimiter)
	if idx < 0 {
		delimiter = LegacyMetadataDelimiter
		idx = strings.Index(description, delimiter)
		if idx < 0 {
			return nil
		}
	}

	section := description[idx+len(delimiter):]
	m := &SchedulerMetadata{}
	var varLines []string

	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Stop at a second delimiter or non-kv line
		if line == MetadataDelimiter || line == LegacyMetadataDelimiter {
			break
		}

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "target_rig":
			m.TargetRig = val
		case "formula":
			m.Formula = val
		case "args":
			m.Args = val
		case "var":
			varLines = append(varLines, val)
		case "vars":
			// Legacy: comma-separated format for backward compatibility
			varLines = append(varLines, strings.Split(val, ",")...)
		case "enqueued_at":
			m.EnqueuedAt = val
		case "merge":
			m.Merge = val
		case "convoy":
			m.Convoy = val
		case "base_branch":
			m.BaseBranch = val
		case "no_merge":
			m.NoMerge = val == "true"
		case "account":
			m.Account = val
		case "agent":
			m.Agent = val
		case "hook_raw_bead":
			m.HookRawBead = val == "true"
		case "no_boot":
			// Legacy: ignored. Dispatch always sets NoBoot=true.
		case "owned":
			m.Owned = val == "true"
		case "mode":
			m.Mode = val
		case "dispatch_failures":
			if n, err := strconv.Atoi(val); err == nil {
				m.DispatchFailures = n
			}
			// On parse error, DispatchFailures stays 0. The gt:dispatch-failed
			// label (added when counter hits max) acts as an independent guard
			// since quarantine also removes the scheduled label.
		case "last_failure":
			m.LastFailure = val
		}
	}

	if len(varLines) > 0 {
		m.Vars = strings.Join(varLines, "\n")
	}

	return m
}

// StripMetadata removes the scheduler metadata section from a bead description.
// Used when descheduling a bead for dispatch (clean up the metadata).
// Supports both current and legacy delimiters.
func StripMetadata(description string) string {
	// Try current delimiter first, then legacy
	idx := strings.Index(description, MetadataDelimiter)
	if idx < 0 {
		idx = strings.Index(description, LegacyMetadataDelimiter)
		if idx < 0 {
			return description
		}
	}
	return strings.TrimRight(description[:idx], "\n")
}

// NewMetadata creates a SchedulerMetadata with the current timestamp.
func NewMetadata(rigName string) *SchedulerMetadata {
	return &SchedulerMetadata{
		TargetRig:  rigName,
		EnqueuedAt: time.Now().UTC().Format(time.RFC3339),
	}
}
