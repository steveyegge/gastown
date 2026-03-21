package doltserver

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// CharacterSheet is the assembled data model for a rig's reputation profile.
// It is the intermediate representation consumed by all format renderers
// (ASCII, JSON, HTML) as specified in the character sheet spec Section 9.1.
type CharacterSheet struct {
	Handle         string
	DisplayName    string
	Tier           string
	StampCount     int
	ClusterBreadth int
	ComputedAt     time.Time

	StampGeometry map[string]GeometryBar // "quality", "reliability", "creativity"
	SkillCoverage []SkillEntry
	TopStamps     []StampHighlight
	Badges        []BadgeRecord
	Warnings      []Warning

	// Stub fields for Phase 2 (cluster-dependent features)
	NearestNeighbors  []Neighbor
	BridgeConnections []BridgeConnection
	CrossCluster      []CrossClusterStamp
}

// GeometryBar holds the average and count for a single valence dimension.
type GeometryBar struct {
	Avg   float64
	Count int
}

// SkillEntry holds aggregated skill tag data split by context type.
type SkillEntry struct {
	Skill      string
	PeerStamps int
	BootBlocks int
}

// StampHighlight is a top-rated stamp for display.
type StampHighlight struct {
	Author      string
	Message     string
	Valence     map[string]float64
	Confidence  float64
	ContextType string
}

// BadgeRecord represents a row from the badges table.
type BadgeRecord struct {
	ID        string
	Type      string
	AwardedAt string
	Evidence  string
}

// Warning represents a root-severity negative stamp.
type Warning struct {
	Severity string
	Author   string
	Message  string
	Date     string
}

// Neighbor represents a rig with high validator overlap (Phase 2 stub).
type Neighbor struct {
	Handle     string
	OverlapPct int
}

// BridgeConnection represents a cross-cluster connection (Phase 2 stub).
type BridgeConnection struct {
	Cluster      string
	StampSources int
	OverlapPct   int
}

// CrossClusterStamp is a stamp from an author in a different cluster (Phase 2 stub).
type CrossClusterStamp struct {
	Author        string
	Message       string
	Quality       int
	AuthorCluster string
}

// LeaderboardEntry represents a row in the leaderboard table.
type LeaderboardEntry struct {
	Handle         string
	DisplayName    string
	Tier           string
	StampCount     int
	AvgQuality     float64
	ClusterBreadth int
	TopSkills      string // JSON
	Badges         string // JSON
}

// TierThreshold defines the requirements for a tier level.
type TierThreshold struct {
	Name             string
	MinStamps        int
	MinAvgEffWeight  float64
	MinClusterBreadth int
}

// TierThresholds are the tier progression requirements from tier-engine.sql.
var TierThresholds = []TierThreshold{
	{Name: "governor", MinStamps: 100, MinAvgEffWeight: 4.5, MinClusterBreadth: 4},
	{Name: "admin", MinStamps: 50, MinAvgEffWeight: 4.0, MinClusterBreadth: 3},
	{Name: "maintainer", MinStamps: 25, MinAvgEffWeight: 3.5, MinClusterBreadth: 2},
	{Name: "trusted", MinStamps: 10, MinAvgEffWeight: 3.0, MinClusterBreadth: 1},
	{Name: "contributor", MinStamps: 3, MinAvgEffWeight: 0, MinClusterBreadth: 0},
	{Name: "newcomer", MinStamps: 0, MinAvgEffWeight: 0, MinClusterBreadth: 0},
}

// AssembleCharacterSheet builds a CharacterSheet from store queries.
// This implements the pilot-viable subset: stamp geometry (4.2), skill coverage (4.3),
// top stamps (4.7), badges (4.8), and warnings. Sections 4.4-4.6 (cluster-dependent)
// return empty results.
func AssembleCharacterSheet(store WLCommonsStore, handle string) (*CharacterSheet, error) {
	stamps, err := store.QueryStampsForSubject(handle)
	if err != nil {
		return nil, fmt.Errorf("querying stamps: %w", err)
	}

	badges, err := store.QueryBadges(handle)
	if err != nil {
		// Non-fatal: proceed without badges
		badges = nil
	}

	sheet := &CharacterSheet{
		Handle:     handle,
		ComputedAt: time.Now().UTC(),
	}

	sheet.StampCount = len(stamps)
	sheet.StampGeometry = computeStampGeometry(stamps)
	sheet.SkillCoverage = computeSkillCoverage(stamps)
	sheet.TopStamps = computeTopStamps(stamps, 5)
	sheet.Warnings = computeWarnings(stamps)
	sheet.Badges = badges
	sheet.Tier = computeTier(sheet)

	// Phase 2 stubs — return empty
	sheet.NearestNeighbors = nil
	sheet.BridgeConnections = nil
	sheet.CrossCluster = nil

	return sheet, nil
}

// computeStampGeometry computes average quality, reliability, and creativity
// from stamp valence JSON. Excludes root-severity stamps per spec Section 4.2.
func computeStampGeometry(stamps []StampRecord) map[string]GeometryBar {
	geometry := make(map[string]GeometryBar)
	dims := []string{"quality", "reliability", "creativity"}
	sums := make(map[string]float64)
	counts := make(map[string]int)

	for _, s := range stamps {
		if s.Severity == "root" {
			continue
		}
		vals := parseValenceJSON(s.Valence)
		for _, dim := range dims {
			if v, ok := vals[dim]; ok {
				sums[dim] += v
				counts[dim]++
			}
		}
	}

	for _, dim := range dims {
		if counts[dim] > 0 {
			geometry[dim] = GeometryBar{
				Avg:   math.Round(sums[dim]/float64(counts[dim])*10) / 10,
				Count: counts[dim],
			}
		}
	}

	return geometry
}

// computeSkillCoverage aggregates skill tags from stamps, splitting peer stamps
// from boot blocks per spec Section 4.3.
func computeSkillCoverage(stamps []StampRecord) []SkillEntry {
	type skillCounts struct {
		peer int
		boot int
	}
	skills := make(map[string]*skillCounts)

	for _, s := range stamps {
		tags := parseSkillTagsJSON(s.SkillTags)
		for _, tag := range tags {
			tag = strings.TrimSpace(strings.ToLower(tag))
			if tag == "" {
				continue
			}
			sc, ok := skills[tag]
			if !ok {
				sc = &skillCounts{}
				skills[tag] = sc
			}
			if s.ContextType == "boot_block" {
				sc.boot++
			} else {
				sc.peer++
			}
		}
	}

	entries := make([]SkillEntry, 0, len(skills))
	for skill, sc := range skills {
		entries = append(entries, SkillEntry{
			Skill:      skill,
			PeerStamps: sc.peer,
			BootBlocks: sc.boot,
		})
	}

	// Sort by total stamps descending, then alphabetically
	sort.Slice(entries, func(i, j int) bool {
		ti := entries[i].PeerStamps + entries[i].BootBlocks
		tj := entries[j].PeerStamps + entries[j].BootBlocks
		if ti != tj {
			return ti > tj
		}
		return entries[i].Skill < entries[j].Skill
	})

	return entries
}

// computeTopStamps finds the highest effective-weight stamps per spec Section 4.7.
// Only includes completion and endorsement stamps with quality >= 4.
func computeTopStamps(stamps []StampRecord, limit int) []StampHighlight {
	type scored struct {
		stamp  StampRecord
		weight float64
		vals   map[string]float64
	}

	var candidates []scored
	for _, s := range stamps {
		if s.ContextType != "completion" && s.ContextType != "endorsement" {
			continue
		}
		vals := parseValenceJSON(s.Valence)
		q, ok := vals["quality"]
		if !ok || q < 4 {
			continue
		}
		candidates = append(candidates, scored{
			stamp:  s,
			weight: q * s.Confidence,
			vals:   vals,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].weight > candidates[j].weight
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	highlights := make([]StampHighlight, len(candidates))
	for i, c := range candidates {
		highlights[i] = StampHighlight{
			Author:      c.stamp.Author,
			Message:     c.stamp.Message,
			Valence:     c.vals,
			Confidence:  c.stamp.Confidence,
			ContextType: c.stamp.ContextType,
		}
	}
	return highlights
}

// computeWarnings extracts root-severity stamps for the WARNINGS section.
func computeWarnings(stamps []StampRecord) []Warning {
	var warnings []Warning
	for _, s := range stamps {
		if s.Severity == "root" {
			warnings = append(warnings, Warning{
				Severity: "root",
				Author:   s.Author,
				Message:  s.Message,
				Date:     s.CreatedAt,
			})
		}
	}
	return warnings
}

// computeTier determines the rig's tier from stamp count and average effective weight.
// Phase 1: cluster breadth is set to 0 (clusters not yet computed).
func computeTier(sheet *CharacterSheet) string {
	if sheet.StampCount == 0 {
		return "newcomer"
	}

	// Compute average effective weight (quality * confidence) from geometry
	avgEffWeight := 0.0
	if g, ok := sheet.StampGeometry["quality"]; ok && g.Count > 0 {
		avgEffWeight = g.Avg // Simplified: use avg quality as proxy for pilot
	}

	for _, t := range TierThresholds {
		if sheet.StampCount >= t.MinStamps &&
			avgEffWeight >= t.MinAvgEffWeight &&
			sheet.ClusterBreadth >= t.MinClusterBreadth {
			return t.Name
		}
	}
	return "newcomer"
}

// parseValenceJSON parses a valence JSON string like {"quality":4,"reliability":3}
// into a map of dimension names to float64 values.
func parseValenceJSON(valence string) map[string]float64 {
	if valence == "" {
		return nil
	}
	var vals map[string]float64
	if err := json.Unmarshal([]byte(valence), &vals); err != nil {
		return nil
	}
	return vals
}

// parseSkillTagsJSON parses a skill_tags JSON array string like ["go","federation"]
// into a string slice.
func parseSkillTagsJSON(skillTags string) []string {
	if skillTags == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(skillTags), &tags); err != nil {
		return nil
	}
	return tags
}

// QueryStampsForSubject fetches all stamps where the given handle is the subject.
func QueryStampsForSubject(townRoot, subject string) ([]StampRecord, error) {
	query := fmt.Sprintf(`USE %s; SELECT id, author, subject, valence, confidence, severity, COALESCE(context_id,'') as context_id, COALESCE(context_type,'') as context_type, COALESCE(stamp_type,'') as stamp_type, COALESCE(skill_tags,'') as skill_tags, COALESCE(message,'') as message, COALESCE(prev_stamp_hash,'') as prev_stamp_hash, COALESCE(stamp_index,-1) as stamp_index, COALESCE(created_at,'') as created_at FROM stamps WHERE subject='%s' ORDER BY created_at DESC;`,
		WLCommonsDB, EscapeSQL(subject))

	output, err := doltSQLQuery(townRoot, query)
	if err != nil {
		return nil, err
	}

	rows := parseSimpleCSV(output)
	if len(rows) == 0 {
		return nil, nil
	}

	stamps := make([]StampRecord, 0, len(rows))
	for _, row := range rows {
		confidence := 0.0
		fmt.Sscanf(row["confidence"], "%f", &confidence)
		idx := -1
		fmt.Sscanf(row["stamp_index"], "%d", &idx)

		stamps = append(stamps, StampRecord{
			ID:            row["id"],
			Author:        row["author"],
			Subject:       row["subject"],
			Valence:       row["valence"],
			Confidence:    confidence,
			Severity:      row["severity"],
			ContextID:     row["context_id"],
			ContextType:   row["context_type"],
			StampType:     row["stamp_type"],
			SkillTags:     row["skill_tags"],
			Message:       row["message"],
			PrevStampHash: row["prev_stamp_hash"],
			StampIndex:    idx,
			CreatedAt:     row["created_at"],
		})
	}
	return stamps, nil
}

// QueryBadges fetches all badges for a rig handle.
func QueryBadges(townRoot, handle string) ([]BadgeRecord, error) {
	query := fmt.Sprintf(`USE %s; SELECT id, badge_type, COALESCE(awarded_at,'') as awarded_at, COALESCE(evidence,'') as evidence FROM badges WHERE rig_handle='%s' ORDER BY awarded_at ASC;`,
		WLCommonsDB, EscapeSQL(handle))

	output, err := doltSQLQuery(townRoot, query)
	if err != nil {
		return nil, err
	}

	rows := parseSimpleCSV(output)
	if len(rows) == 0 {
		return nil, nil
	}

	badges := make([]BadgeRecord, 0, len(rows))
	for _, row := range rows {
		badges = append(badges, BadgeRecord{
			ID:        row["id"],
			Type:      row["badge_type"],
			AwardedAt: row["awarded_at"],
			Evidence:  row["evidence"],
		})
	}
	return badges, nil
}

// QueryAllSubjects returns all distinct subject handles from the stamps table.
func QueryAllSubjects(townRoot string) ([]string, error) {
	query := fmt.Sprintf(`USE %s; SELECT DISTINCT subject FROM stamps ORDER BY subject;`, WLCommonsDB)
	output, err := doltSQLQuery(townRoot, query)
	if err != nil {
		return nil, err
	}
	rows := parseSimpleCSV(output)
	subjects := make([]string, 0, len(rows))
	for _, row := range rows {
		if h := row["subject"]; h != "" {
			subjects = append(subjects, h)
		}
	}
	return subjects, nil
}

// UpsertLeaderboard inserts or updates a leaderboard entry.
func UpsertLeaderboard(townRoot string, entry *LeaderboardEntry) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	displayName := "NULL"
	if entry.DisplayName != "" {
		displayName = fmt.Sprintf("'%s'", EscapeSQL(entry.DisplayName))
	}
	topSkills := "NULL"
	if entry.TopSkills != "" {
		topSkills = fmt.Sprintf("'%s'", EscapeSQL(entry.TopSkills))
	}
	badges := "NULL"
	if entry.Badges != "" {
		badges = fmt.Sprintf("'%s'", EscapeSQL(entry.Badges))
	}

	script := fmt.Sprintf(`USE %s;
REPLACE INTO leaderboard (handle, display_name, tier, stamp_count, avg_quality, cluster_breadth, top_skills, badges, computed_at)
VALUES ('%s', %s, '%s', %d, %f, %d, %s, %s, '%s');
`,
		WLCommonsDB,
		EscapeSQL(entry.Handle), displayName, EscapeSQL(entry.Tier),
		entry.StampCount, entry.AvgQuality, entry.ClusterBreadth,
		topSkills, badges, now)

	return doltSQLScriptWithRetry(townRoot, script)
}

// RunScorekeeper computes tier standings for all subjects and materializes
// them into the leaderboard table. Returns the entries and any errors.
func RunScorekeeper(store WLCommonsStore) ([]*LeaderboardEntry, error) {
	subjects, err := store.QueryAllSubjects()
	if err != nil {
		return nil, fmt.Errorf("querying subjects: %w", err)
	}

	var entries []*LeaderboardEntry
	for _, handle := range subjects {
		sheet, err := AssembleCharacterSheet(store, handle)
		if err != nil {
			continue // skip rigs with query errors
		}

		// Build top skills JSON
		topSkillsJSON := ""
		if len(sheet.SkillCoverage) > 0 {
			skillMap := make(map[string]int)
			for _, s := range sheet.SkillCoverage {
				skillMap[s.Skill] = s.PeerStamps + s.BootBlocks
			}
			if b, err := json.Marshal(skillMap); err == nil {
				topSkillsJSON = string(b)
			}
		}

		// Build badges JSON
		badgesJSON := ""
		if len(sheet.Badges) > 0 {
			badgeTypes := make([]string, len(sheet.Badges))
			for i, b := range sheet.Badges {
				badgeTypes[i] = b.Type
			}
			if b, err := json.Marshal(badgeTypes); err == nil {
				badgesJSON = string(b)
			}
		}

		avgQuality := 0.0
		if g, ok := sheet.StampGeometry["quality"]; ok {
			avgQuality = g.Avg
		}

		entry := &LeaderboardEntry{
			Handle:         handle,
			Tier:           sheet.Tier,
			StampCount:     sheet.StampCount,
			AvgQuality:     avgQuality,
			ClusterBreadth: sheet.ClusterBreadth,
			TopSkills:      topSkillsJSON,
			Badges:         badgesJSON,
		}

		if err := store.UpsertLeaderboard(entry); err != nil {
			continue // skip upsert failures
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
