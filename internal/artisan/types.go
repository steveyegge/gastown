// Package artisan provides artisan workspace management for specialized long-lived workers.
package artisan

import "time"

// Worker represents a specialized long-lived worker in a rig.
type Worker struct {
	// Name is the artisan identifier (e.g., "frontend-1", "backend-2").
	Name string `json:"name"`

	// Rig is the rig this artisan belongs to.
	Rig string `json:"rig"`

	// Specialty is the artisan's domain (e.g., "frontend", "backend", "tests", "docs", "security").
	Specialty string `json:"specialty"`

	// ClonePath is the path to the artisan's clone of the rig.
	ClonePath string `json:"clone_path"`

	// Branch is the current git branch.
	Branch string `json:"branch"`

	// CreatedAt is when the artisan was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the artisan was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// Summary provides a concise view of artisan status.
type Summary struct {
	Name      string `json:"name"`
	Specialty string `json:"specialty"`
	Branch    string `json:"branch"`
}

// Summary returns a Summary for this artisan.
func (a *Worker) Summary() Summary {
	return Summary{
		Name:      a.Name,
		Specialty: a.Specialty,
		Branch:    a.Branch,
	}
}
