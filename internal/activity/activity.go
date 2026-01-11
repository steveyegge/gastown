// Package activity provides last-activity tracking and color-coding for the dashboard.
package activity

import (
	"time"
)

// Color class constants for activity status.
const (
	ColorGreen   = "green"   // Active: <2 minutes
	ColorYellow  = "yellow"  // Stale: 2-5 minutes
	ColorRed     = "red"     // Stuck: >5 minutes
	ColorUnknown = "unknown" // No activity data
)

// Thresholds for activity color coding.
const (
	ThresholdActive = 2 * time.Minute  // Green threshold
	ThresholdStale  = 5 * time.Minute  // Yellow threshold (beyond this is red)
)

// Info holds activity information for display.
type Info struct {
	LastActivity time.Time // Raw timestamp of last activity
	Duration     time.Duration // Time since last activity
	FormattedAge string    // Human-readable age (e.g., "2m", "1h")
	ColorClass   string    // CSS class for coloring (green, yellow, red, unknown)
}

// Calculate computes activity info from a last-activity timestamp.
// Returns color-coded info based on thresholds:
//   - Green:   <2 minutes (active)
//   - Yellow:  2-5 minutes (stale)
//   - Red:     >5 minutes (stuck)
//   - Unknown: zero time value
func Calculate(lastActivity time.Time) Info {
	info := Info{
		LastActivity: lastActivity,
	}

	// Handle zero time (no activity data)
	if lastActivity.IsZero() {
		info.FormattedAge = "unknown"
		info.ColorClass = ColorUnknown
		return info
	}

	// Calculate duration since last activity
	info.Duration = time.Since(lastActivity)

	// Handle future time (clock skew) - treat as just now
	if info.Duration < 0 {
		info.Duration = 0
	}

	// Format age string
	info.FormattedAge = formatAge(info.Duration)

	// Determine color class
	info.ColorClass = colorForDuration(info.Duration)

	return info
}

// formatAge formats a duration as a short human-readable string.
// Examples: "<1m", "5m", "2h", "1d"
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return formatMinutes(d)
	}
	if d < 24*time.Hour {
		return formatHours(d)
	}
	return formatDays(d)
}

func formatMinutes(d time.Duration) string {
	mins := int(d.Minutes())
	return formatInt(mins) + "m"
}

func formatHours(d time.Duration) string {
	hours := int(d.Hours())
	return formatInt(hours) + "h"
}

func formatDays(d time.Duration) string {
	days := int(d.Hours() / 24)
	return formatInt(days) + "d"
}

// formatInt converts a non-negative integer to its decimal string representation.
// For single digits (0-9), it uses direct rune conversion for efficiency.
// For larger numbers, it extracts digits iteratively from least to most significant.
// This avoids importing strconv for simple integer formatting in the activity package.
func formatInt(n int) string {
	if n < 10 {
		return string(rune('0'+n))
	}
	// For larger numbers, use standard conversion
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}

// colorForDuration returns the color class for a given duration.
func colorForDuration(d time.Duration) string {
	switch {
	case d < ThresholdActive:
		return ColorGreen
	case d < ThresholdStale:
		return ColorYellow
	default:
		return ColorRed
	}
}

// IsActive returns true if the activity is within the active threshold (green).
func (i Info) IsActive() bool {
	return i.ColorClass == ColorGreen
}

// IsStale returns true if the activity is in the stale range (yellow).
func (i Info) IsStale() bool {
	return i.ColorClass == ColorYellow
}

// IsStuck returns true if the activity is beyond the stale threshold (red).
func (i Info) IsStuck() bool {
	return i.ColorClass == ColorRed
}
