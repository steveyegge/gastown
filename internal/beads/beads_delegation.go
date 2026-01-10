// Package beads provides delegation tracking for work units.
package beads

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Delegation represents a work delegation relationship between work units.
// Delegation links a parent work unit to a child work unit, tracking who
// delegated the work and to whom, along with any terms of the delegation.
// This enables work distribution with credit cascade - work flows down,
// validation and credit flow up.
type Delegation struct {
	// Parent is the work unit ID that delegated the work
	Parent string `json:"parent"`

	// Child is the work unit ID that received the delegated work
	Child string `json:"child"`

	// DelegatedBy is the entity (hop:// URI or actor string) that delegated
	DelegatedBy string `json:"delegated_by"`

	// DelegatedTo is the entity (hop:// URI or actor string) receiving delegation
	DelegatedTo string `json:"delegated_to"`

	// Terms contains optional conditions of the delegation
	Terms *DelegationTerms `json:"terms,omitempty"`

	// CreatedAt is when the delegation was created
	CreatedAt string `json:"created_at,omitempty"`
}

// DelegationTerms holds optional terms/conditions for a delegation.
type DelegationTerms struct {
	// Portion describes what part of the parent work is delegated
	Portion string `json:"portion,omitempty"`

	// Deadline is the expected completion date
	Deadline string `json:"deadline,omitempty"`

	// AcceptanceCriteria describes what constitutes completion
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`

	// CreditShare is the percentage of credit that flows to the delegate (0-100)
	CreditShare int `json:"credit_share,omitempty"`
}

// AddDelegation creates a delegation relationship from parent to child work unit.
// The delegation tracks who delegated (delegatedBy) and who received (delegatedTo),
// along with optional terms. Delegations enable credit cascade - when child work
// is completed, credit flows up to the parent work unit and its delegator.
//
// Note: This is stored as metadata on the child issue until bd CLI has native
// delegation support. Once bd supports `bd delegate add`, this will be updated.
func (b *Beads) AddDelegation(d *Delegation) error {
	if d.Parent == "" || d.Child == "" {
		return fmt.Errorf("delegation requires both parent and child work unit IDs")
	}
	if d.DelegatedBy == "" || d.DelegatedTo == "" {
		return fmt.Errorf("delegation requires both delegated_by and delegated_to entities")
	}

	// Store delegation as JSON in the child issue's delegated_from slot
	delegationJSON, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshaling delegation: %w", err)
	}

	// Set the delegated_from slot on the child issue
	_, err = b.run("slot", "set", d.Child, "delegated_from", string(delegationJSON))
	if err != nil {
		return fmt.Errorf("setting delegation slot: %w", err)
	}

	// Also add a dependency so child blocks parent (work must complete before parent can close)
	if err := b.AddDependency(d.Parent, d.Child); err != nil {
		// Log but don't fail - the delegation is still recorded
		fmt.Printf("Warning: could not add blocking dependency for delegation: %v\n", err)
	}

	return nil
}

// RemoveDelegation removes a delegation relationship.
func (b *Beads) RemoveDelegation(parent, child string) error {
	// Clear the delegated_from slot on the child
	_, err := b.run("slot", "clear", child, "delegated_from")
	if err != nil {
		return fmt.Errorf("clearing delegation slot: %w", err)
	}

	// Also remove the blocking dependency
	if err := b.RemoveDependency(parent, child); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: could not remove blocking dependency: %v\n", err)
	}

	return nil
}

// GetDelegation retrieves the delegation information for a child work unit.
// Returns nil if the issue has no delegation.
func (b *Beads) GetDelegation(child string) (*Delegation, error) {
	// Verify the issue exists first
	if _, err := b.Show(child); err != nil {
		return nil, fmt.Errorf("getting issue: %w", err)
	}

	// Get delegation from the slot
	out, err := b.run("slot", "get", child, "delegated_from")
	if err != nil {
		// No delegation slot means no delegation
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no slot") {
			return nil, nil
		}
		return nil, fmt.Errorf("getting delegation slot: %w", err)
	}

	slotValue := strings.TrimSpace(string(out))
	if slotValue == "" || slotValue == "null" {
		return nil, nil
	}

	var delegation Delegation
	if err := json.Unmarshal([]byte(slotValue), &delegation); err != nil {
		return nil, fmt.Errorf("parsing delegation: %w", err)
	}

	return &delegation, nil
}

// ListDelegationsFrom returns all delegations from a parent work unit.
// This searches for issues that have delegated_from pointing to the parent.
func (b *Beads) ListDelegationsFrom(parent string) ([]*Delegation, error) {
	// List all issues that depend on this parent (delegated work blocks parent)
	issues, err := b.List(ListOptions{Status: "all"})
	if err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}

	var delegations []*Delegation
	for _, issue := range issues {
		d, err := b.GetDelegation(issue.ID)
		if err != nil {
			continue // Skip issues with errors
		}
		if d != nil && d.Parent == parent {
			delegations = append(delegations, d)
		}
	}

	return delegations, nil
}
