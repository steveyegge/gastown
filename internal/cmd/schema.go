package cmd

import (
	"github.com/spf13/cobra"
)

// Schema command flags
var (
	schemaName        string
	schemaIntent      string
	schemaRequires    string
	schemaOptional    string
	schemaExample     string
	schemaCategory    string
	schemaJSON        bool
	schemaListJSON    bool
	schemaSearchJSON  bool
	schemaSearchLimit int
)

var schemaCmd = &cobra.Command{
	Use:     "schema",
	GroupID: GroupComm,
	Short:   "Discover and create decision schemas",
	Long: `Manage decision schemas for structured decision-making.

Schemas define the required context structure for different decision types.
Before creating a decision, search for a matching schema. If none fits
your intent, venture to create a new one.

DISCOVERY-FIRST WORKFLOW:
  1. Search for schemas matching your intent: gt schema search "cache strategy"
  2. If match found, use it: gt decision request --type=tradeoff ...
  3. If no match, create new schema: gt schema create --name="my-schema" ...

Schemas are stored as beads with the 'gt:schema' label. Popular schemas
(high usage) surface in searches; unused schemas fade over time.

Examples:
  gt schema search "choosing between options"   # Find matching schemas
  gt schema list                                # Show all schemas
  gt schema show tradeoff                       # Show schema details
  gt schema create --name="my-schema" ...       # Create new schema`,
}

var schemaSearchCmd = &cobra.Command{
	Use:   "search <intent>",
	Short: "Search for schemas matching intent",
	Long: `Search for decision schemas that match your intent.

Pass a description of what kind of decision you're making. Returns
schemas that might be a good fit, ranked by relevance and usage.

Examples:
  gt schema search "choosing between caching options"
  gt schema search "confirming destructive action"
  gt schema search "tradeoff" --json`,
	Args: cobra.ExactArgs(1),
	RunE: runSchemaSearch,
}

var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available schemas",
	Long: `List all available decision schemas.

Shows schemas grouped by category with usage counts.

Examples:
  gt schema list
  gt schema list --json`,
	RunE: runSchemaList,
}

var schemaShowCmd = &cobra.Command{
	Use:   "show <schema-name>",
	Short: "Show schema details",
	Long: `Display detailed information about a schema.

Shows intent, required/optional context fields, examples, and usage stats.

Examples:
  gt schema show tradeoff
  gt schema show confirmation --json`,
	Args: cobra.ExactArgs(1),
	RunE: runSchemaShow,
}

var schemaCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new decision schema",
	Long: `Create a new decision schema (the "venture" action).

Use this when no existing schema fits your decision pattern. Creating
a schema signals: "I think this pattern is common enough to deserve
its own structure."

FLAGS:
  --name        Schema identifier in kebab-case (required)
  --intent      What this schema is for (searchable description)
  --requires    Comma-separated required context fields
  --optional    Comma-separated optional context fields
  --example     Example context JSON
  --category    Category: choice, confirmation, checkpoint, custom

Examples:
  gt schema create \
    --name="cache-strategy" \
    --intent="Choosing between caching approaches for performance" \
    --requires="options,recommendation,latency_target" \
    --example='{"options": ["Redis", "In-memory"], "recommendation": "Redis"}'`,
	RunE: runSchemaCreate,
}

func init() {
	// Search flags
	schemaSearchCmd.Flags().BoolVar(&schemaSearchJSON, "json", false, "Output as JSON")
	schemaSearchCmd.Flags().IntVar(&schemaSearchLimit, "limit", 5, "Maximum results to show")

	// List flags
	schemaListCmd.Flags().BoolVar(&schemaListJSON, "json", false, "Output as JSON")

	// Show flags
	schemaShowCmd.Flags().BoolVar(&schemaJSON, "json", false, "Output as JSON")

	// Create flags
	schemaCreateCmd.Flags().StringVar(&schemaName, "name", "", "Schema identifier (kebab-case, required)")
	schemaCreateCmd.Flags().StringVar(&schemaIntent, "intent", "", "What this schema is for")
	schemaCreateCmd.Flags().StringVar(&schemaRequires, "requires", "", "Comma-separated required fields")
	schemaCreateCmd.Flags().StringVar(&schemaOptional, "optional", "", "Comma-separated optional fields")
	schemaCreateCmd.Flags().StringVar(&schemaExample, "example", "", "Example context JSON")
	schemaCreateCmd.Flags().StringVar(&schemaCategory, "category", "custom", "Category: choice, confirmation, checkpoint, custom")
	schemaCreateCmd.Flags().BoolVar(&schemaJSON, "json", false, "Output as JSON")
	_ = schemaCreateCmd.MarkFlagRequired("name")

	// Add subcommands
	schemaCmd.AddCommand(schemaSearchCmd)
	schemaCmd.AddCommand(schemaListCmd)
	schemaCmd.AddCommand(schemaShowCmd)
	schemaCmd.AddCommand(schemaCreateCmd)

	rootCmd.AddCommand(schemaCmd)
}
