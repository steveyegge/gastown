package migrate

import (
	"fmt"
	"sync"

	"github.com/steveyegge/gastown/internal/upgrade"
)

// Migration defines a version-to-version migration.
// Each migration consists of ordered steps that transform a workspace
// from one version layout to another.
type Migration struct {
	// ID is a unique identifier for this migration (e.g., "v0_1_to_v0_2").
	ID string

	// FromPattern is a version pattern this migration applies from (e.g., "0.1.x").
	FromPattern string

	// ToVersion is the target version after migration (e.g., "0.2.0").
	ToVersion string

	// Description is a human-readable description of the migration.
	Description string

	// Steps are the ordered migration steps to execute.
	Steps []Step
}

// Matches returns true if this migration can handle the given source version.
func (m *Migration) Matches(from *upgrade.Version) bool {
	return from.MatchesPattern(m.FromPattern)
}

// Global registry of migrations.
// Add new migrations here to register them.
var (
	migrationsMu sync.RWMutex
	migrations   = []Migration{}
)

// RegisterMigration adds a migration to the global registry.
// Call this from init() in migration definition files.
// Thread-safe for concurrent registration.
func RegisterMigration(m Migration) {
	migrationsMu.Lock()
	defer migrationsMu.Unlock()
	migrations = append(migrations, m)
}

// GetMigrations returns all registered migrations.
func GetMigrations() []Migration {
	migrationsMu.RLock()
	defer migrationsMu.RUnlock()
	// Return a copy to prevent external modification
	result := make([]Migration, len(migrations))
	copy(result, migrations)
	return result
}

// GetMigration returns a specific migration by ID.
func GetMigration(id string) (*Migration, error) {
	migrationsMu.RLock()
	defer migrationsMu.RUnlock()
	for i := range migrations {
		if migrations[i].ID == id {
			return &migrations[i], nil
		}
	}
	return nil, fmt.Errorf("migration not found: %s", id)
}

// FindMigration finds a migration that can handle the given source version.
func FindMigration(from *upgrade.Version) (*Migration, error) {
	migrationsMu.RLock()
	defer migrationsMu.RUnlock()
	for i := range migrations {
		if migrations[i].Matches(from) {
			return &migrations[i], nil
		}
	}
	return nil, fmt.Errorf("no migration found for version %s", from.String())
}

// GetMigrationPath finds the sequence of migrations needed to go from
// one version to another. For example, 0.1.5 -> 0.3.0 might return
// [v0_1_to_v0_2, v0_2_to_v0_3].
func GetMigrationPath(from, to *upgrade.Version) ([]Migration, error) {
	var path []Migration
	current := from

	// Prevent infinite loops
	maxIterations := 10
	iterations := 0

	for iterations < maxIterations {
		iterations++

		// Check if we've reached the target
		toVer, err := upgrade.ParseVersion(to.String())
		if err != nil {
			return nil, fmt.Errorf("parsing target version: %w", err)
		}
		if !current.LessThan(toVer) {
			break
		}

		// Find the next migration in the chain
		migration, err := FindMigration(current)
		if err != nil {
			if len(path) == 0 {
				return nil, fmt.Errorf("no migration path from %s to %s", from.String(), to.String())
			}
			// No more migrations available - we're done
			break
		}

		path = append(path, *migration)

		// Move to the next version
		nextVer, err := upgrade.ParseVersion(migration.ToVersion)
		if err != nil {
			return nil, fmt.Errorf("parsing migration target version: %w", err)
		}
		current = nextVer
	}

	if iterations >= maxIterations {
		return nil, fmt.Errorf("migration path too long (possible cycle)")
	}

	return path, nil
}

// ListMigrations returns a summary of all available migrations.
func ListMigrations() []MigrationInfo {
	migrationsMu.RLock()
	defer migrationsMu.RUnlock()
	var infos []MigrationInfo
	for _, m := range migrations {
		infos = append(infos, MigrationInfo{
			ID:          m.ID,
			FromPattern: m.FromPattern,
			ToVersion:   m.ToVersion,
			Description: m.Description,
			StepCount:   len(m.Steps),
		})
	}
	return infos
}

// MigrationInfo provides summary information about a migration.
type MigrationInfo struct {
	ID          string `json:"id"`
	FromPattern string `json:"from_pattern"`
	ToVersion   string `json:"to_version"`
	Description string `json:"description"`
	StepCount   int    `json:"step_count"`
}
