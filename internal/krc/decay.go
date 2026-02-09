// Package krc provides the Key Record Chronicle - configurable TTL management
// and auto-pruning for Level 0 ephemeral operational data.
//
// This file implements forensic value decay modeling. Different event types
// lose forensic value at different rates. The decay model quantifies this
// so operators can see what data is losing value and make informed TTL decisions.
package krc

import (
	"math"
	"sort"
	"time"
)

// DecayCurve defines how forensic value decays over time for an event type.
type DecayCurve int

const (
	// DecayRapid: value drops quickly (e.g., heartbeats, pings).
	// Half-life at 25% of TTL. Near zero well before TTL.
	DecayRapid DecayCurve = iota

	// DecaySteady: value decays linearly (e.g., patrol events, status checks).
	// Value = 1.0 at age 0, 0.0 at TTL.
	DecaySteady

	// DecaySlow: value persists longer (e.g., errors, escalations).
	// Half-life at 75% of TTL. Still has value near TTL.
	DecaySlow

	// DecayFlat: value doesn't decay until TTL (e.g., audit events, deaths).
	// Value = 1.0 until TTL, then drops to 0.
	DecayFlat
)

// defaultDecayCurves maps event type patterns to their decay curves.
var defaultDecayCurves = map[string]DecayCurve{
	// Rapid decay: operational noise
	"patrol_*":        DecayRapid,
	"polecat_checked": DecayRapid,
	"polecat_nudged":  DecayRapid,
	"heartbeat":       DecayRapid,
	"ping":            DecayRapid,

	// Steady decay: session lifecycle
	"session_start": DecaySteady,
	"session_end":   DecaySteady,
	"nudge":         DecaySteady,
	"handoff":       DecaySteady,
	"gc_report":     DecaySteady,

	// Slow decay: higher-value operational events
	"hook":       DecaySlow,
	"unhook":     DecaySlow,
	"sling":      DecaySlow,
	"done":       DecaySlow,
	"error":      DecaySlow,
	"recovery":   DecaySlow,
	"escalation": DecaySlow,

	// Flat: audit-critical events that retain full value
	"mail":          DecayFlat,
	"session_death": DecayFlat,
	"mass_death":    DecayFlat,
	"merge_*":       DecayFlat,
}

// ForensicScore returns the forensic value score for an event of the given type
// and age, based on the configured TTL. Returns a value between 0.0 (no value)
// and 1.0 (full value).
func ForensicScore(eventType string, age, ttl time.Duration) float64 {
	if ttl <= 0 || age < 0 {
		return 1.0
	}
	ratio := float64(age) / float64(ttl)
	if ratio >= 1.0 {
		return 0.0
	}

	curve := getDecayCurve(eventType)
	return applyDecayCurve(curve, ratio)
}

// applyDecayCurve applies the decay function for the given curve type.
// ratio is age/TTL, clamped to [0, 1].
func applyDecayCurve(curve DecayCurve, ratio float64) float64 {
	switch curve {
	case DecayRapid:
		// Exponential decay with half-life at 25% of TTL.
		// f(x) = 2^(-x/0.25) = 2^(-4x)
		return math.Pow(2, -4*ratio)

	case DecaySteady:
		// Linear decay: 1.0 at 0, 0.0 at 1.0
		return 1.0 - ratio

	case DecaySlow:
		// Exponential decay with half-life at 75% of TTL.
		// f(x) = 2^(-x/0.75)
		return math.Pow(2, -ratio/0.75)

	case DecayFlat:
		// Full value until 90% of TTL, then cliff.
		if ratio < 0.9 {
			return 1.0
		}
		// Linear drop from 1.0 to 0.0 in the last 10%
		return (1.0 - ratio) / 0.1

	default:
		return 1.0 - ratio // linear fallback
	}
}

// getDecayCurve returns the decay curve for an event type, checking exact
// matches first, then glob patterns. Falls back to DecaySteady.
func getDecayCurve(eventType string) DecayCurve {
	// Exact match
	if curve, ok := defaultDecayCurves[eventType]; ok {
		return curve
	}

	// Glob match (longest pattern first for specificity)
	var patterns []string
	for p := range defaultDecayCurves {
		if containsGlob(p) {
			patterns = append(patterns, p)
		}
	}
	sort.Slice(patterns, func(i, j int) bool {
		return len(patterns[i]) > len(patterns[j])
	})
	for _, p := range patterns {
		if matchGlob(p, eventType) {
			return defaultDecayCurves[p]
		}
	}

	return DecaySteady
}

// containsGlob returns true if the pattern contains glob characters.
func containsGlob(pattern string) bool {
	for _, c := range pattern {
		if c == '*' || c == '?' {
			return true
		}
	}
	return false
}

// DecayInfo holds decay information for a single event type.
type DecayInfo struct {
	EventType    string        `json:"event_type"`
	Count        int           `json:"count"`
	TTL          time.Duration `json:"ttl"`
	Curve        string        `json:"curve"` // "rapid", "steady", "slow", "flat"
	AvgAge       time.Duration `json:"avg_age"`
	AvgScore     float64       `json:"avg_score"`
	MinScore     float64       `json:"min_score"`
	ExpiredCount int           `json:"expired_count"`
}

// DecayReport summarizes forensic value decay across all event types.
type DecayReport struct {
	Types       []DecayInfo   `json:"types"`
	TotalEvents int           `json:"total_events"`
	TotalScore  float64       `json:"total_score"` // weighted average
	AtRisk      int           `json:"at_risk"`     // events with score < 0.25
	Expired     int           `json:"expired"`     // events with score = 0
	Generated   time.Time     `json:"generated"`
}

// GenerateDecayReport builds a forensic value decay report from stats.
func GenerateDecayReport(stats *Stats, config *Config) *DecayReport {
	now := time.Now()
	report := &DecayReport{
		Generated: now,
	}

	// We need per-event detail which Stats.TTLBreakdown provides partially.
	// Build decay info from TTLBreakdown.
	for eventType, info := range stats.TTLBreakdown {
		ttl := config.GetTTL(eventType)
		curve := getDecayCurve(eventType)

		di := DecayInfo{
			EventType:    eventType,
			Count:        info.Count,
			TTL:          ttl,
			Curve:        curveToString(curve),
			ExpiredCount: info.Expired,
		}

		report.TotalEvents += info.Count
		report.Expired += info.Expired

		// Estimate average score from TTL breakdown.
		// We don't have individual event ages, so estimate from the TTL info.
		// Active (non-expired) events have avg age ~ TTL/2 as rough estimate.
		active := info.Count - info.Expired
		if active > 0 && ttl > 0 {
			// Use ExpiresIn as a proxy: soonest-to-expire gives us the oldest active event.
			// Average age of active events is roughly (TTL - ExpiresIn/2).
			estimatedAvgAge := ttl / 2
			if info.ExpiresIn > 0 {
				// The closest to expiring is (TTL - ExpiresIn) old.
				// Average active is somewhere between 0 and (TTL - ExpiresIn).
				oldestActive := ttl - info.ExpiresIn
				estimatedAvgAge = oldestActive / 2
			}
			di.AvgAge = estimatedAvgAge
			di.AvgScore = ForensicScore(eventType, estimatedAvgAge, ttl)
			di.MinScore = ForensicScore(eventType, ttl-info.ExpiresIn, ttl)
		}

		// Count at-risk (score < 0.25)
		if di.MinScore < 0.25 && active > 0 {
			// Rough estimate: fraction of active events at risk
			report.AtRisk += info.Expired // already expired are at zero
		}

		report.Types = append(report.Types, di)
	}

	// Sort by average score ascending (most decayed first)
	sort.Slice(report.Types, func(i, j int) bool {
		return report.Types[i].AvgScore < report.Types[j].AvgScore
	})

	// Calculate total weighted score
	if report.TotalEvents > 0 {
		var totalWeighted float64
		for _, di := range report.Types {
			totalWeighted += di.AvgScore * float64(di.Count)
		}
		report.TotalScore = totalWeighted / float64(report.TotalEvents)
	}

	return report
}

func curveToString(c DecayCurve) string {
	switch c {
	case DecayRapid:
		return "rapid"
	case DecaySteady:
		return "steady"
	case DecaySlow:
		return "slow"
	case DecayFlat:
		return "flat"
	default:
		return "unknown"
	}
}
