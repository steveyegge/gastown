package rally

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ReportKind classifies what an agent is signalling about an entry.
type ReportKind string

const (
	ReportKindStale   ReportKind = "stale"   // entry is outdated / no longer accurate
	ReportKindWrong   ReportKind = "wrong"   // entry is factually incorrect
	ReportKindImprove ReportKind = "improve" // entry is correct but could be better
	ReportKindVerify  ReportKind = "verify"  // agent confirms entry is still valid
)

// Report is a signal from an agent about an existing knowledge entry.
type Report struct {
	// Target entry
	EntryID string `yaml:"entry_id"` // slug ID of the entry (e.g. "gas-town-upgrade-sequence")
	EntryTag string `yaml:"entry_tag,omitempty"` // alternative: tag to identify entry

	// What kind of signal
	Kind   ReportKind `yaml:"kind"` // stale | wrong | improve | verify
	Reason string     `yaml:"reason,omitempty"`      // why stale/wrong
	Improvement string `yaml:"improvement,omitempty"` // suggested text for improve kind

	// Provenance
	ReportedBy string `yaml:"reported_by"`
	ReportedAt string `yaml:"reported_at"`
	ReportID   string `yaml:"report_id"` // "rpt-<6hex>"
}

const reportMailBodyPrefix = "RALLY_REPORT_V1\n---\n"

// Validate checks that the report has required fields.
func (r *Report) Validate() error {
	if r.EntryID == "" && r.EntryTag == "" {
		return fmt.Errorf("entry_id or entry_tag is required")
	}
	switch r.Kind {
	case ReportKindStale, ReportKindWrong, ReportKindImprove, ReportKindVerify:
	default:
		return fmt.Errorf("kind must be one of: stale, wrong, improve, verify (got %q)", r.Kind)
	}
	if (r.Kind == ReportKindStale || r.Kind == ReportKindWrong) && strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("reason is required for kind=%s", r.Kind)
	}
	if r.Kind == ReportKindImprove && strings.TrimSpace(r.Improvement) == "" {
		return fmt.Errorf("improvement text is required for kind=improve")
	}
	return nil
}

// ToMailBody serializes the report to the wire format used in mail bodies.
func (r *Report) ToMailBody() (string, error) {
	data, err := yaml.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("serializing report: %w", err)
	}
	return reportMailBodyPrefix + string(data), nil
}

// ParseReportFromMailBody parses a report from a mail body.
func ParseReportFromMailBody(body string) (*Report, error) {
	if !strings.HasPrefix(body, reportMailBodyPrefix) {
		return nil, fmt.Errorf("not a report mail body (missing RALLY_REPORT_V1 sentinel)")
	}
	yamlPart := strings.TrimPrefix(body, reportMailBodyPrefix)
	var r Report
	if err := yaml.Unmarshal([]byte(yamlPart), &r); err != nil {
		return nil, fmt.Errorf("parsing report YAML: %w", err)
	}
	return &r, nil
}

// GenerateReportID returns a unique report ID in the form "rpt-<6hex>".
func GenerateReportID() string {
	// Reuse the same approach as GenerateNominationID but with rpt- prefix.
	id := GenerateNominationID()
	return "rpt-" + strings.TrimPrefix(id, "nom-")
}

// SubjectLine returns the mail subject for this report.
func (r *Report) SubjectLine() string {
	target := r.EntryID
	if target == "" {
		target = r.EntryTag
	}
	return fmt.Sprintf("RALLY_REPORT: %s [%s]", target, r.Kind)
}

// NowRFC3339 returns the current UTC time as an RFC3339 string.
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
