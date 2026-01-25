package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkillInfo holds parsed skill metadata from SKILL.md frontmatter
type SkillInfo struct {
	Name        string
	Description string
	Triggers    []string
}

// relevantSkills defines skills relevant for autonomous workers (polecats/crew).
// Package-level to avoid repeated allocation.
var relevantSkills = map[string]bool{
	// Core workflow
	"implement": true,
	"beads":     true,
	"status":    true,
	"molecules": true,
	// Communication
	"dispatch": true,
	"mail":     true,
	"handoff":  true,
	// Infrastructure
	"polecat-lifecycle": true,
	"crew":              true,
	"roles":             true,
	"bd-routing":        true,
	// Work patterns
	"crank":          true,
	"implement-wave": true,
	"retro":          true,
	// Validation
	"vibe":             true,
	"validation-chain": true,
}

// DiscoverSkills scans skill directories for SKILL.md files
func DiscoverSkills() ([]SkillInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var skills []SkillInfo

	// Scan primary skill directories
	skillDirs := []string{
		filepath.Join(homeDir, ".claude", "cowork-skills"),
		filepath.Join(homeDir, ".claude", "skills"),
	}

	for _, dir := range skillDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		dirSkills, err := scanSkillDirectory(dir)
		if err != nil {
			continue
		}
		skills = append(skills, dirSkills...)
	}

	// Scan plugin cache for marketplace skills
	pluginCacheDir := filepath.Join(homeDir, ".claude", "plugins", "cache", "agentops-marketplace")
	if _, err := os.Stat(pluginCacheDir); err == nil {
		pluginSkills, err := scanPluginSkills(pluginCacheDir)
		if err == nil {
			skills = append(skills, pluginSkills...)
		}
	}

	return skills, nil
}

// scanSkillDirectory scans a directory for SKILL.md files (one level deep)
func scanSkillDirectory(dir string) ([]SkillInfo, error) {
	var skills []SkillInfo

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		skill, err := parseSkillFile(skillPath)
		if err != nil {
			continue
		}
		if skill != nil {
			skills = append(skills, *skill)
		}
	}

	return skills, nil
}

// scanPluginSkills scans plugin cache for skills
// Structure: <cache>/<plugin>/<version>/skills/<skill>/SKILL.md
func scanPluginSkills(cacheDir string) ([]SkillInfo, error) {
	var skills []SkillInfo

	plugins, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, err
	}

	for _, plugin := range plugins {
		if !plugin.IsDir() {
			continue
		}

		pluginPath := filepath.Join(cacheDir, plugin.Name())
		versions, err := os.ReadDir(pluginPath)
		if err != nil || len(versions) == 0 {
			continue
		}

		// Sort versions descending to get latest first
		// Semantic versions sort correctly as strings (1.0.0 < 1.1.0 < 2.0.0)
		versionNames := make([]string, 0, len(versions))
		for _, v := range versions {
			if v.IsDir() {
				versionNames = append(versionNames, v.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(versionNames)))

		if len(versionNames) == 0 {
			continue
		}

		versionPath := filepath.Join(pluginPath, versionNames[0], "skills")
		if _, err := os.Stat(versionPath); os.IsNotExist(err) {
			continue
		}

		dirSkills, err := scanSkillDirectory(versionPath)
		if err != nil {
			continue
		}
		skills = append(skills, dirSkills...)
	}

	return skills, nil
}

// parseSkillFile reads YAML frontmatter from a SKILL.md file
func parseSkillFile(path string) (*SkillInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	return parseFrontmatter(scanner), nil
}

// parseFrontmatter parses YAML frontmatter from a scanner
func parseFrontmatter(scanner *bufio.Scanner) *SkillInfo {
	// Check for YAML frontmatter delimiter
	if !scanner.Scan() || scanner.Text() != "---" {
		return nil
	}

	skill := &SkillInfo{}
	state := &parserState{}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			break
		}
		state.parseLine(line, skill)
	}

	// Finalize any pending multi-line description
	state.finalize(skill)

	if skill.Name == "" {
		return nil
	}

	return skill
}

// parserState tracks YAML parsing state
type parserState struct {
	inTriggers       bool
	inDescription    bool
	descriptionLines []string
}

// parseLine processes a single line of YAML frontmatter
func (s *parserState) parseLine(line string, skill *SkillInfo) {
	// Handle continuation of multi-line description
	if s.inDescription {
		if strings.HasPrefix(line, "  ") {
			s.descriptionLines = append(s.descriptionLines, strings.TrimSpace(line))
			return
		}
		// End of description block
		skill.Description = strings.Join(s.descriptionLines, " ")
		s.inDescription = false
	}

	// Parse key-value pairs
	switch {
	case strings.HasPrefix(line, "name:"):
		skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))

	case strings.HasPrefix(line, "description:"):
		desc := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		if desc == ">" || desc == "|" {
			s.inDescription = true
			s.descriptionLines = nil
		} else {
			skill.Description = desc
		}

	case strings.HasPrefix(line, "triggers:"):
		s.inTriggers = true

	case s.inTriggers && strings.HasPrefix(line, "  - "):
		trigger := strings.TrimPrefix(line, "  - ")
		trigger = strings.Trim(trigger, "\"'")
		skill.Triggers = append(skill.Triggers, trigger)

	case !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "-"):
		s.inTriggers = false
	}
}

// finalize completes any pending multi-line values
func (s *parserState) finalize(skill *SkillInfo) {
	if s.inDescription && len(s.descriptionLines) > 0 {
		skill.Description = strings.Join(s.descriptionLines, " ")
	}
}

// OutputSkillContext writes available skills section to stdout
func OutputSkillContext(w io.Writer, role Role) error {
	skills, err := DiscoverSkills()
	if err != nil || len(skills) == 0 {
		return nil // Graceful degradation
	}

	// Filter and deduplicate skills
	unique := filterAndDedupe(skills, role)
	if len(unique) == 0 {
		return nil
	}

	// Use buffered writer for batched error handling
	bw := bufio.NewWriter(w)

	fmt.Fprintln(bw)
	fmt.Fprintln(bw, "## Available Skills")
	fmt.Fprintln(bw)
	fmt.Fprintln(bw, "Load skills with `/skill-name` for documented workflows:")
	fmt.Fprintln(bw)
	fmt.Fprintln(bw, "| Skill | Description |")
	fmt.Fprintln(bw, "|-------|-------------|")

	for _, s := range unique {
		desc := truncateDescription(s.Description, 60)
		fmt.Fprintf(bw, "| `/%s` | %s |\n", s.Name, desc)
	}

	fmt.Fprintln(bw)

	return bw.Flush()
}

// filterAndDedupe filters skills by role and removes duplicates
func filterAndDedupe(skills []SkillInfo, role Role) []SkillInfo {
	seen := make(map[string]bool)
	var unique []SkillInfo

	for _, s := range skills {
		if !isRelevantForRole(s, role) {
			continue
		}
		if seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		unique = append(unique, s)
	}

	return unique
}

// truncateDescription shortens and cleans description for table display
func truncateDescription(desc string, maxLen int) string {
	// Clean up for table display
	desc = strings.ReplaceAll(desc, "|", "-")
	desc = strings.ReplaceAll(desc, "\n", " ")

	if len(desc) > maxLen {
		return desc[:maxLen-3] + "..."
	}
	return desc
}

// isRelevantForRole determines if a skill is relevant for polecats/crew
func isRelevantForRole(skill SkillInfo, role Role) bool {
	// Only show for polecat and crew roles
	if role != RolePolecat && role != RoleCrew {
		return false
	}
	return relevantSkills[skill.Name]
}
