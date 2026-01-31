package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// Schema represents a decision schema bead
type Schema struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Intent      string   `json:"intent"`
	Category    string   `json:"category"`
	Requires    []string `json:"requires"`
	Optional    []string `json:"optional"`
	Example     string   `json:"example,omitempty"`
	UsageCount  int      `json:"usage_count"`
	CreatedBy   string   `json:"created_by,omitempty"`
	Description string   `json:"description,omitempty"`
}

// runSchemaSearch searches for schemas matching intent
func runSchemaSearch(cmd *cobra.Command, args []string) error {
	intent := args[0]

	schemas, err := listSchemas()
	if err != nil {
		return err
	}

	// Score and rank schemas by relevance to intent
	type scored struct {
		schema Schema
		score  int
	}
	var results []scored

	intentLower := strings.ToLower(intent)
	intentWords := strings.Fields(intentLower)

	for _, s := range schemas {
		score := 0

		// Exact name match
		if strings.Contains(intentLower, strings.ToLower(s.Name)) {
			score += 100
		}

		// Intent keyword matches
		intentTextLower := strings.ToLower(s.Intent)
		for _, word := range intentWords {
			if len(word) > 2 && strings.Contains(intentTextLower, word) {
				score += 20
			}
		}

		// Category match
		if strings.Contains(intentLower, strings.ToLower(s.Category)) {
			score += 30
		}

		// Boost by usage (popular schemas are likely relevant)
		score += s.UsageCount / 10

		if score > 0 {
			results = append(results, scored{s, score})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Limit results
	if len(results) > schemaSearchLimit {
		results = results[:schemaSearchLimit]
	}

	if schemaSearchJSON {
		out := make([]Schema, len(results))
		for i, r := range results {
			out[i] = r.schema
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(results) == 0 {
		fmt.Printf("🔍 No schemas matching \"%s\"\n\n", intent)
		fmt.Println("Consider creating a new schema:")
		fmt.Printf("  gt schema create --name=\"%s\" --intent=\"%s\"\n",
			slugify(intent), intent)
		return nil
	}

	fmt.Printf("🔍 Schemas matching \"%s\":\n\n", intent)
	for i, r := range results {
		fmt.Printf("  %d. %s (%d uses)\n", i+1, r.schema.Name, r.schema.UsageCount)
		fmt.Printf("     %s\n", r.schema.Intent)
		if len(r.schema.Requires) > 0 {
			fmt.Printf("     Required: %s\n", strings.Join(r.schema.Requires, ", "))
		}
		fmt.Println()
	}

	fmt.Println("None of these fit? Create a new schema:")
	fmt.Printf("  gt schema create --name=\"...\" --intent=\"%s\"\n", intent)

	return nil
}

// runSchemaList lists all schemas
func runSchemaList(cmd *cobra.Command, args []string) error {
	schemas, err := listSchemas()
	if err != nil {
		return err
	}

	if schemaListJSON {
		data, _ := json.MarshalIndent(schemas, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(schemas) == 0 {
		fmt.Println("📋 No schemas defined yet")
		fmt.Println("\nCreate your first schema:")
		fmt.Println("  gt schema create --name=\"tradeoff\" --intent=\"Choosing between alternatives\"")
		return nil
	}

	// Group by category
	byCategory := make(map[string][]Schema)
	for _, s := range schemas {
		cat := s.Category
		if cat == "" {
			cat = "uncategorized"
		}
		byCategory[cat] = append(byCategory[cat], s)
	}

	fmt.Printf("📋 Decision Schemas (%d total)\n\n", len(schemas))

	// Sort categories
	var cats []string
	for cat := range byCategory {
		cats = append(cats, cat)
	}
	sort.Strings(cats)

	for _, cat := range cats {
		fmt.Printf("Category: %s\n", cat)
		for _, s := range byCategory[cat] {
			fmt.Printf("  ● %s (%d uses) - %s\n", s.Name, s.UsageCount, s.Intent)
		}
		fmt.Println()
	}

	return nil
}

// runSchemaShow shows schema details
func runSchemaShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	schema, err := getSchema(name)
	if err != nil {
		return err
	}

	if schemaJSON {
		data, _ := json.MarshalIndent(schema, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("📋 Schema: %s (%s)\n\n", schema.Name, schema.ID)
	fmt.Printf("Intent: %s\n\n", schema.Intent)

	if len(schema.Requires) > 0 {
		fmt.Println("Required context:")
		for _, r := range schema.Requires {
			fmt.Printf("  - %s\n", r)
		}
		fmt.Println()
	}

	if len(schema.Optional) > 0 {
		fmt.Println("Optional context:")
		for _, o := range schema.Optional {
			fmt.Printf("  - %s\n", o)
		}
		fmt.Println()
	}

	if schema.Example != "" {
		fmt.Println("Example:")
		fmt.Printf("  gt decision request \\\n")
		fmt.Printf("    --type=%s \\\n", schema.Name)
		fmt.Printf("    --prompt \"Your question\" \\\n")
		fmt.Printf("    --context '%s'\n\n", schema.Example)
	}

	fmt.Printf("Usage: %d decisions have used this schema\n", schema.UsageCount)
	if schema.CreatedBy != "" {
		fmt.Printf("Created by: %s\n", schema.CreatedBy)
	}

	return nil
}

// runSchemaCreate creates a new schema
func runSchemaCreate(cmd *cobra.Command, args []string) error {
	if schemaName == "" {
		return fmt.Errorf("--name is required")
	}

	// Build description with structured content
	var desc strings.Builder
	desc.WriteString("## Intent\n")
	if schemaIntent != "" {
		desc.WriteString(schemaIntent)
	} else {
		desc.WriteString("(no intent specified)")
	}
	desc.WriteString("\n\n")

	if schemaRequires != "" {
		desc.WriteString("## Required Context\n")
		for _, field := range strings.Split(schemaRequires, ",") {
			desc.WriteString(fmt.Sprintf("- `%s`\n", strings.TrimSpace(field)))
		}
		desc.WriteString("\n")
	}

	if schemaOptional != "" {
		desc.WriteString("## Optional Context\n")
		for _, field := range strings.Split(schemaOptional, ",") {
			desc.WriteString(fmt.Sprintf("- `%s`\n", strings.TrimSpace(field)))
		}
		desc.WriteString("\n")
	}

	if schemaExample != "" {
		desc.WriteString("## Example\n")
		desc.WriteString("```json\n")
		desc.WriteString(schemaExample)
		desc.WriteString("\n```\n")
	}

	// Create bead with gt:schema label
	bdArgs := []string{
		"create",
		"-t", "task", // Use task type, identify by label
		"--title", fmt.Sprintf("schema:%s", schemaName),
		"-d", desc.String(),
		"-l", "gt:schema",
		"-l", fmt.Sprintf("schema:name:%s", schemaName),
		"-l", fmt.Sprintf("schema:category:%s", schemaCategory),
		"--silent",
	}

	// Add each required field as separate label
	if schemaRequires != "" {
		for _, req := range strings.Split(schemaRequires, ",") {
			req = strings.TrimSpace(req)
			if req != "" {
				bdArgs = append(bdArgs, "-l", fmt.Sprintf("schema:requires:%s", req))
			}
		}
	}
	// Add each optional field as separate label
	if schemaOptional != "" {
		for _, opt := range strings.Split(schemaOptional, ",") {
			opt = strings.TrimSpace(opt)
			if opt != "" {
				bdArgs = append(bdArgs, "-l", fmt.Sprintf("schema:optional:%s", opt))
			}
		}
	}
	if schemaIntent != "" {
		bdArgs = append(bdArgs, "-l", fmt.Sprintf("schema:intent:%s", truncateLabel(schemaIntent, 50)))
	}

	output, err := exec.Command("bd", bdArgs...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("bd create failed: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("bd create failed: %w", err)
	}

	beadID := strings.TrimSpace(string(output))

	if schemaJSON {
		result := map[string]string{
			"id":       beadID,
			"name":     schemaName,
			"category": schemaCategory,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("📋 Created schema: %s (%s)\n\n", schemaName, beadID)
	fmt.Printf("Intent: %s\n", schemaIntent)
	if schemaRequires != "" {
		fmt.Printf("Required: %s\n", schemaRequires)
	}
	fmt.Println()
	fmt.Println("To use:")
	fmt.Printf("  gt decision request --type=%s ...\n\n", schemaName)
	fmt.Println("💡 Tip: Your schema is now discoverable. If others find it useful,")
	fmt.Println("   it will accumulate usage and become part of the shared vocabulary.")

	return nil
}

// listSchemas returns all schemas from beads
func listSchemas() ([]Schema, error) {
	// Query beads with gt:schema label
	output, err := exec.Command("bd", "list", "-l", "gt:schema", "--json").Output()
	if err != nil {
		// If no schemas exist, return empty list
		return []Schema{}, nil
	}

	var beads []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Labels      []string `json:"labels"`
	}

	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parse beads: %w", err)
	}

	var schemas []Schema
	for _, b := range beads {
		s := Schema{
			ID:          b.ID,
			Description: b.Description,
		}

		// Extract from labels (labels are strings like "schema:name:tradeoff")
		for _, label := range b.Labels {
			switch {
			case strings.HasPrefix(label, "schema:name:"):
				s.Name = strings.TrimPrefix(label, "schema:name:")
			case strings.HasPrefix(label, "schema:category:"):
				s.Category = strings.TrimPrefix(label, "schema:category:")
			case strings.HasPrefix(label, "schema:intent:"):
				s.Intent = strings.TrimPrefix(label, "schema:intent:")
			case strings.HasPrefix(label, "schema:requires:"):
				reqStr := strings.TrimPrefix(label, "schema:requires:")
				if reqStr != "" {
					s.Requires = append(s.Requires, reqStr)
				}
			case strings.HasPrefix(label, "schema:optional:"):
				optStr := strings.TrimPrefix(label, "schema:optional:")
				if optStr != "" {
					s.Optional = append(s.Optional, optStr)
				}
			case strings.HasPrefix(label, "schema:usage-count:"):
				fmt.Sscanf(strings.TrimPrefix(label, "schema:usage-count:"), "%d", &s.UsageCount)
			}
		}

		// Fallback: extract name from title if not in labels
		if s.Name == "" && strings.HasPrefix(b.Title, "schema:") {
			s.Name = strings.TrimPrefix(b.Title, "schema:")
		}

		if s.Name != "" {
			schemas = append(schemas, s)
		}
	}

	return schemas, nil
}

// getSchema retrieves a specific schema by name
func getSchema(name string) (*Schema, error) {
	schemas, err := listSchemas()
	if err != nil {
		return nil, err
	}

	for _, s := range schemas {
		if s.Name == name {
			return &s, nil
		}
	}

	return nil, fmt.Errorf("schema not found: %s", name)
}

// slugify converts text to kebab-case slug
func slugify(text string) string {
	// Simple slugification
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, " ", "-")
	// Remove non-alphanumeric except hyphens
	var result strings.Builder
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// truncateLabel truncates a string for use in labels
func truncateLabel(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
