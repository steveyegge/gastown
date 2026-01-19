# Gas Town Migrations

This directory contains documentation for workspace migrations between Gas Town versions.

## How Migrations Work

Gas Town uses an extensible migration framework that handles version upgrades with:

- **Atomic transactions** - All-or-nothing with automatic rollback on failure
- **Idempotent steps** - Steps check if they're already done and skip if so
- **Backup first** - Always creates a timestamped backup before making changes
- **Verification** - Runs checks after migration to ensure success

## Available Migrations

| From | To | Description |
|------|-----|-------------|
| 0.1.x | 0.2.0 | [Two-level beads and directory reorganization](0.2.0.md) |

## Running Migrations

```bash
# Check if migration is needed
gt migrate --check

# Preview changes without executing
gt migrate --dry-run

# Run migration (with confirmation)
gt migrate --execute

# Run migration without confirmation
gt migrate --force

# Restore from backup if needed
gt migrate --rollback

# Show migration status
gt migrate --status
```

## For Developers: Adding New Migrations

To add a new migration (e.g., 0.2.x → 0.3.x):

### 1. Create the Migration File

Create `internal/migrate/v0_2_to_v0_3.go`:

```go
package migrate

func init() {
    RegisterMigration(V0_2_to_V0_3)
}

var V0_2_to_V0_3 = Migration{
    ID:          "v0_2_to_v0_3",
    FromPattern: "0.2.x",
    ToVersion:   "0.3.0",
    Description: "Add federation support",
    Steps: []Step{
        &CreateFederationConfigStep{},
        &MigrateIdentityBeadsStep{},
        // ... more steps
    },
}
```

### 2. Implement Migration Steps

Each step implements the `Step` interface:

```go
type Step interface {
    ID() string           // Unique identifier
    Description() string  // Human-readable description
    Check(ctx *Context) (needed bool, err error)  // Idempotency check
    Execute(ctx *Context) error                    // Do the work
    Rollback(ctx *Context) error                   // Undo the work
    Verify(ctx *Context) error                     // Confirm success
}
```

Example step:

```go
type CreateFederationConfigStep struct {
    BaseStep
}

func (s *CreateFederationConfigStep) ID() string {
    return "create-federation-config"
}

func (s *CreateFederationConfigStep) Description() string {
    return "Create federation.json"
}

func (s *CreateFederationConfigStep) Check(ctx *Context) (bool, error) {
    path := filepath.Join(ctx.TownRoot, "mayor", "federation.json")
    _, err := os.Stat(path)
    return os.IsNotExist(err), nil
}

func (s *CreateFederationConfigStep) Execute(ctx *Context) error {
    config := FederationConfig{Version: 1, Enabled: false}
    return writeJSON(filepath.Join(ctx.TownRoot, "mayor", "federation.json"), config)
}

func (s *CreateFederationConfigStep) Rollback(ctx *Context) error {
    return os.Remove(filepath.Join(ctx.TownRoot, "mayor", "federation.json"))
}

func (s *CreateFederationConfigStep) Verify(ctx *Context) error {
    _, err := os.Stat(filepath.Join(ctx.TownRoot, "mayor", "federation.json"))
    return err
}
```

### 3. Update compatibility.json

Add the new migration to `compatibility.json`:

```json
{
  "version": "0.3.0",
  "migrations": [
    {
      "from_pattern": "0.2.x",
      "id": "v0_2_to_v0_3",
      "description": "Add federation support"
    },
    {
      "from_pattern": "0.1.x",
      "id": "v0_1_to_v0_2",
      "description": "Two-level beads and directory reorganization"
    }
  ]
}
```

### 4. Create Documentation

Create `docs/migrations/0.3.0.md` documenting:
- What changed and why
- Step-by-step migration guide
- Rollback instructions
- Troubleshooting

That's it! The framework handles:
- Migration path chaining (0.1.x → 0.2.x → 0.3.x)
- Backup creation and restoration
- Progress reporting
- Error handling and rollback
