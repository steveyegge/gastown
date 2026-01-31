package validator

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// SchemaBeadValidator validates decisions against schema beads.
// This is the bead-based alternative to script-based type validators.
type SchemaBeadValidator struct{}

// SchemaBead represents a decision schema stored as a bead.
type SchemaBead struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Intent   string   `json:"intent"`
	Requires []string `json:"requires"`
	Optional []string `json:"optional"`
}

// ValidateAgainstSchema checks if decision context satisfies schema requirements.
// Returns a ValidationResult with errors if required fields are missing.
func ValidateAgainstSchema(schemaName string, context map[string]interface{}) ValidationResult {
	if schemaName == "" {
		return ValidationResult{Valid: true}
	}

	schema, err := loadSchemaBead(schemaName)
	if err != nil {
		// Schema not found - this is OK, might be using script-based validation
		// Return valid but with a warning suggesting schema creation
		return ValidationResult{
			Valid:    true,
			Warnings: []string{fmt.Sprintf("No schema bead found for type '%s'. Consider: gt schema create --name=%s", schemaName, schemaName)},
		}
	}

	// Check required fields
	var missing []string
	for _, req := range schema.Requires {
		if _, ok := context[req]; !ok {
			missing = append(missing, req)
		}
	}

	if len(missing) > 0 {
		return ValidationResult{
			Valid:    false,
			Blocking: true,
			Errors: []string{
				fmt.Sprintf("Schema '%s' requires context fields: %s", schemaName, strings.Join(missing, ", ")),
			},
			Warnings: []string{
				fmt.Sprintf("Add missing fields to --context or use a different schema: gt schema search \"your intent\""),
			},
		}
	}

	return ValidationResult{Valid: true}
}

// loadSchemaBead retrieves a schema bead by name.
func loadSchemaBead(name string) (*SchemaBead, error) {
	// Query beads with gt:schema and schema:name:<name> labels
	output, err := exec.Command("bd", "list", "-l", "gt:schema", "-l", fmt.Sprintf("schema:name:%s", name), "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("bd list failed: %w", err)
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

	if len(beads) == 0 {
		return nil, fmt.Errorf("schema not found: %s", name)
	}

	b := beads[0]
	schema := &SchemaBead{
		ID:   b.ID,
		Name: name,
	}

	// Extract from labels
	for _, label := range b.Labels {
		switch {
		case strings.HasPrefix(label, "schema:intent:"):
			schema.Intent = strings.TrimPrefix(label, "schema:intent:")
		case strings.HasPrefix(label, "schema:requires:"):
			req := strings.TrimPrefix(label, "schema:requires:")
			if req != "" {
				schema.Requires = append(schema.Requires, req)
			}
		case strings.HasPrefix(label, "schema:optional:"):
			opt := strings.TrimPrefix(label, "schema:optional:")
			if opt != "" {
				schema.Optional = append(schema.Optional, opt)
			}
		}
	}

	return schema, nil
}

// IncrementSchemaUsage updates the usage count for a schema.
// Called when a decision is successfully created with a schema type.
func IncrementSchemaUsage(schemaName string) error {
	schema, err := loadSchemaBead(schemaName)
	if err != nil {
		return err // Schema not found, nothing to increment
	}

	// Add/update usage count label
	// For simplicity, we'll just add a new label each time (could dedupe later)
	_, err = exec.Command("bd", "update", schema.ID, "-l", "schema:used").Output()
	return err
}
