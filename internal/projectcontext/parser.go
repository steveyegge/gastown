// Package projectcontext provides the logic for parsing project-specific Claude context.
package projectcontext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Parse reads CLAUDE.md and the .claude directory from the given path,
// interprets the contents, and returns a structured ProjectContext object.
//
// This is the "witness" function that translates project-specific instructions
// into a normalized, immutable context object for a polecat.
//
// Per the project context integration spec:
// - Extract guidelines and constraints, not commands
// - Reframe imperative language into neutral requirements
// - Ignore conversational, meta, or explanatory text
func Parse(clonePath string) (*ProjectContext, error) {
	ctx := &ProjectContext{}

	// Parse CLAUDE.md for guidelines and constraints
	claudeMDPath := filepath.Join(clonePath, "CLAUDE.md")
	if _, err := os.Stat(claudeMDPath); err == nil {
		content, err := os.ReadFile(claudeMDPath)
		if err != nil {
			return nil, err
		}
		parseClaudeMD(string(content), ctx)
	}

	// Also check for CLAUDE.local.md (personal instructions)
	claudeLocalPath := filepath.Join(clonePath, "CLAUDE.local.md")
	if _, err := os.Stat(claudeLocalPath); err == nil {
		content, err := os.ReadFile(claudeLocalPath)
		if err != nil {
			return nil, err
		}
		parseClaudeMD(string(content), ctx)
	}

	// Parse .mcp.json for available tools
	mcpPath := filepath.Join(clonePath, ".mcp.json")
	if _, err := os.Stat(mcpPath); err == nil {
		content, err := os.ReadFile(mcpPath)
		if err != nil {
			return nil, err
		}
		parseMCPConfig(content, ctx)
	}

	// Parse .claude/commands for skills
	commandsDir := filepath.Join(clonePath, ".claude", "commands")
	if _, err := os.Stat(commandsDir); err == nil {
		files, err := os.ReadDir(commandsDir)
		if err != nil {
			return nil, err
		}

		for _, file := range files {
			if !file.IsDir() {
				content, err := os.ReadFile(filepath.Join(commandsDir, file.Name()))
				if err != nil {
					return nil, err
				}

				lines := strings.Split(string(content), "\n")
				if len(lines) > 0 {
					skill := Skill{
						Name:   strings.TrimSuffix(file.Name(), filepath.Ext(file.Name())),
						Intent: strings.TrimSpace(lines[0]),
					}
					if len(lines) > 1 {
						for _, stepLine := range lines[1:] {
							if trimmed := strings.TrimSpace(stepLine); trimmed != "" {
								skill.Steps = append(skill.Steps, trimmed)
							}
						}
					}
					ctx.Skills = append(ctx.Skills, skill)
				}
			}
		}
	}

	// Parse .claude/rules for rules
	rulesPath := filepath.Join(clonePath, ".claude", "rules")
	if _, err := os.Stat(rulesPath); err == nil {
		content, err := os.ReadFile(rulesPath)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				ctx.Rules = append(ctx.Rules, line)
			}
		}
	}
	rulesGlob := filepath.Join(clonePath, ".claude", "rules*")
	matches, err := filepath.Glob(rulesGlob)
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		if match != rulesPath { // Avoid double-counting if 'rules' is the only match
			content, err := os.ReadFile(match)
			if err != nil {
				return nil, err
			}
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					ctx.Rules = append(ctx.Rules, line)
				}
			}
		}
	}

	return ctx, nil
}

// parseClaudeMD intelligently parses CLAUDE.md content into guidelines and constraints.
// It extracts actionable requirements while filtering out meta/explanatory text.
func parseClaudeMD(content string, ctx *ProjectContext) {
	lines := strings.Split(content, "\n")
	var currentSection string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Track section headers for context
		if strings.HasPrefix(trimmed, "#") {
			currentSection = extractSectionName(trimmed)
			continue
		}

		// Skip code blocks and their content
		if strings.HasPrefix(trimmed, "```") {
			continue
		}

		// Skip meta/explanatory text patterns
		if isMetaText(trimmed) {
			continue
		}

		// Classify and normalize the line
		if isConstraint(trimmed) {
			constraint := normalizeConstraint(trimmed)
			if constraint != "" {
				ctx.Constraints = append(ctx.Constraints, constraint)
			}
		} else if isGuideline(trimmed) {
			guideline := normalizeGuideline(trimmed, currentSection)
			if guideline != "" {
				ctx.Guidelines = append(ctx.Guidelines, guideline)
			}
		}
	}
}

// extractSectionName extracts a clean section name from a markdown header.
func extractSectionName(header string) string {
	// Remove # characters and trim
	name := strings.TrimLeft(header, "#")
	return strings.TrimSpace(name)
}

// isMetaText returns true if the line appears to be explanatory/meta text
// that shouldn't be converted to a guideline or constraint.
func isMetaText(line string) bool {
	lower := strings.ToLower(line)

	// Explanatory patterns
	metaPatterns := []string{
		"this project",
		"this codebase",
		"this repository",
		"we use",
		"we are",
		"the team",
		"overview",
		"introduction",
		"welcome to",
		"getting started",
		"note:",
		"important:",
		"warning:",
		"tip:",
		"example:",
		"for example",
		"e.g.",
		"i.e.",
		"see also",
		"refer to",
		"documentation",
		"readme",
	}

	for _, pattern := range metaPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// Skip list markers without content
	if line == "-" || line == "*" || line == "•" {
		return true
	}

	// Skip very short lines (likely formatting artifacts)
	if len(strings.TrimSpace(line)) < 5 {
		return true
	}

	return false
}

// isConstraint returns true if the line expresses a constraint (something to avoid).
func isConstraint(line string) bool {
	lower := strings.ToLower(line)

	constraintPatterns := []string{
		"don't",
		"do not",
		"never",
		"avoid",
		"must not",
		"should not",
		"shouldn't",
		"forbidden",
		"prohibited",
		"not allowed",
		"disallowed",
	}

	for _, pattern := range constraintPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// isGuideline returns true if the line appears to be an actionable guideline.
// By default, any line that isn't explicitly meta-text is considered a potential guideline.
// This is intentionally lenient to capture project-specific instructions.
func isGuideline(line string) bool {
	// If it's not meta text and not too short, it's likely a guideline
	// This ensures we capture most content while filtering obvious noise
	return len(strings.TrimSpace(line)) >= 5
}

// normalizeConstraint converts an imperative constraint into a neutral requirement.
func normalizeConstraint(line string) string {
	// Remove list markers
	line = removeListMarker(line)

	// Patterns to neutralize
	replacements := []struct {
		pattern string
		replace string
	}{
		{"don't ", "avoid "},
		{"do not ", "avoid "},
		{"never ", "avoid "},
		{"must not ", "avoid "},
		{"should not ", "avoid "},
		{"shouldn't ", "avoid "},
	}

	lower := strings.ToLower(line)
	for _, r := range replacements {
		if idx := strings.Index(lower, r.pattern); idx != -1 {
			return "Constraint: " + line[idx+len(r.pattern):]
		}
	}

	return "Constraint: " + line
}

// normalizeGuideline converts an imperative guideline into a neutral requirement.
func normalizeGuideline(line string, section string) string {
	// Remove list markers
	line = removeListMarker(line)

	// Remove imperative starters to make more neutral
	imperatives := regexp.MustCompile(`^(?i)(always |must |should |you must |you should |please |ensure that |make sure |ensure )`)
	normalized := imperatives.ReplaceAllString(line, "")

	// Capitalize first letter if needed
	if len(normalized) > 0 && normalized[0] >= 'a' && normalized[0] <= 'z' {
		normalized = strings.ToUpper(string(normalized[0])) + normalized[1:]
	}

	// Add section context if available
	if section != "" && !strings.Contains(strings.ToLower(normalized), strings.ToLower(section)) {
		return section + ": " + normalized
	}

	return normalized
}

// removeListMarker removes common list markers from the start of a line.
func removeListMarker(line string) string {
	// Remove markdown list markers
	listPattern := regexp.MustCompile(`^[\-\*•]\s+`)
	line = listPattern.ReplaceAllString(line, "")

	// Remove numbered list markers
	numberedPattern := regexp.MustCompile(`^\d+\.\s+`)
	line = numberedPattern.ReplaceAllString(line, "")

	return strings.TrimSpace(line)
}

// parseMCPConfig parses .mcp.json to extract available MCP tools.
func parseMCPConfig(content []byte, ctx *ProjectContext) {
	var mcpConfig struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}

	if err := json.Unmarshal(content, &mcpConfig); err != nil {
		return // Silently skip malformed MCP config
	}

	for name := range mcpConfig.MCPServers {
		ctx.Tools = append(ctx.Tools, "MCP server: "+name)
	}
}
