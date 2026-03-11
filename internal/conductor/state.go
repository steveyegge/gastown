package conductor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FeatureStatus represents the overall status of a feature being orchestrated.
type FeatureStatus string

const (
	// StatusPlanning means the conductor is building the execution plan.
	StatusPlanning FeatureStatus = "planning"

	// StatusExamining means waiting for the Architect's Phase 1 report.
	StatusExamining FeatureStatus = "examining"

	// StatusInProgress means phases are being executed.
	StatusInProgress FeatureStatus = "in_progress"

	// StatusAwaitingApproval means Phase 4 (specify) needs user sign-off.
	StatusAwaitingApproval FeatureStatus = "awaiting_approval"

	// StatusEscalated means something failed and needs human intervention.
	StatusEscalated FeatureStatus = "escalated"

	// StatusComplete means all phases are done.
	StatusComplete FeatureStatus = "complete"
)

// SubBeadStatus tracks the progress of an individual sub-bead.
type SubBeadStatus string

const (
	SubBeadPending    SubBeadStatus = "pending"     // Not yet dispatched
	SubBeadDispatched SubBeadStatus = "dispatched"  // Assigned to artisan
	SubBeadInProgress SubBeadStatus = "in_progress" // Artisan working
	SubBeadDone       SubBeadStatus = "done"        // PR merged / phase complete
	SubBeadFailed     SubBeadStatus = "failed"      // Failed, needs escalation
	SubBeadBlocked    SubBeadStatus = "blocked"     // Waiting on dependencies
)

// FeatureState is the persistent state of a feature being orchestrated.
// Stored at <rig>/conductor/features/<feature-name>.json.
type FeatureState struct {
	// FeatureName is the slugified feature name.
	FeatureName string `json:"feature_name"`

	// ParentBeadID is the original bead that triggered this feature.
	ParentBeadID string `json:"parent_bead_id"`

	// RigName is the rig being orchestrated.
	RigName string `json:"rig_name"`

	// Status is the overall feature status.
	Status FeatureStatus `json:"status"`

	// CurrentPhase is the active phase number.
	CurrentPhase Phase `json:"current_phase"`

	// Plan is the generated execution plan.
	Plan *Plan `json:"plan"`

	// SubBeadStates tracks progress of each sub-bead.
	SubBeadStates []SubBeadState `json:"sub_bead_states"`

	// EscalationReason explains why the feature was escalated (if applicable).
	EscalationReason string `json:"escalation_reason,omitempty"`

	// specifyApproved is an in-memory flag set by ApproveSpecification
	// to allow AdvancePhase to pass the user gate exactly once.
	specifyApproved bool `json:"-"`

	// CreatedAt is when orchestration started.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the last state change.
	UpdatedAt time.Time `json:"updated_at"`
}

// SubBeadState tracks an individual sub-bead's progress.
type SubBeadState struct {
	// Branch identifies which sub-bead this tracks (matches SubBead.Branch).
	Branch string `json:"branch"`

	// BeadID is the created bead ID (populated after dispatch).
	BeadID string `json:"bead_id,omitempty"`

	// ArtisanName is the assigned artisan (populated after routing).
	ArtisanName string `json:"artisan_name,omitempty"`

	// Status is this sub-bead's progress.
	Status SubBeadStatus `json:"status"`

	// Phase is which development phase this belongs to.
	Phase Phase `json:"phase"`

	// FailureReason explains why this sub-bead failed (if applicable).
	FailureReason string `json:"failure_reason,omitempty"`

	// DispatchedAt is when the sub-bead was sent to an artisan.
	DispatchedAt *time.Time `json:"dispatched_at,omitempty"`

	// CompletedAt is when the sub-bead finished.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// NewFeatureState creates initial state for a new feature orchestration.
func NewFeatureState(plan *Plan) *FeatureState {
	now := time.Now()
	states := make([]SubBeadState, len(plan.SubBeads))
	for i, sb := range plan.SubBeads {
		states[i] = SubBeadState{
			Branch: sb.Branch,
			Phase:  sb.Phase,
			Status: SubBeadPending,
		}
	}

	return &FeatureState{
		FeatureName:   plan.FeatureName,
		ParentBeadID:  plan.ParentBeadID,
		RigName:       plan.RigName,
		Status:        StatusPlanning,
		CurrentPhase:  PhaseExamine,
		Plan:          plan,
		SubBeadStates: states,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// GetSubBeadState returns the state for a given branch, or nil if not found.
func (fs *FeatureState) GetSubBeadState(branch string) *SubBeadState {
	for i := range fs.SubBeadStates {
		if fs.SubBeadStates[i].Branch == branch {
			return &fs.SubBeadStates[i]
		}
	}
	return nil
}

// PhaseComplete returns true if all sub-beads for the given phase are done.
func (fs *FeatureState) PhaseComplete(phase Phase) bool {
	hasPhase := false
	for _, sbs := range fs.SubBeadStates {
		if sbs.Phase == phase {
			hasPhase = true
			if sbs.Status != SubBeadDone {
				return false
			}
		}
	}
	return hasPhase
}

// PhaseFailed returns true if any sub-bead in the phase has failed.
func (fs *FeatureState) PhaseFailed(phase Phase) bool {
	for _, sbs := range fs.SubBeadStates {
		if sbs.Phase == phase && sbs.Status == SubBeadFailed {
			return true
		}
	}
	return false
}

// ReadyToDispatch returns sub-beads that are pending and have all dependencies met.
func (fs *FeatureState) ReadyToDispatch() []SubBead {
	// Build set of completed branches
	doneBranches := make(map[string]bool)
	for _, sbs := range fs.SubBeadStates {
		if sbs.Status == SubBeadDone {
			doneBranches[sbs.Branch] = true
		}
	}

	var ready []SubBead
	for i, sb := range fs.Plan.SubBeads {
		if fs.SubBeadStates[i].Status != SubBeadPending {
			continue
		}

		// Check all dependencies are done
		allDepsMet := true
		for _, dep := range sb.DependsOn {
			if !doneBranches[dep] {
				allDepsMet = false
				break
			}
		}

		if allDepsMet {
			ready = append(ready, sb)
		}
	}

	return ready
}

// AdvancePhase transitions to the next phase if current phase is complete.
// Returns the new phase, or 0 if no transition occurred.
func (fs *FeatureState) AdvancePhase() Phase {
	if fs.Status == StatusEscalated || fs.Status == StatusComplete {
		return 0
	}

	// Check if current phase failed
	if fs.PhaseFailed(fs.CurrentPhase) {
		return 0
	}

	// Phase 1 (examine) has no sub-beads — advance when status moves to in_progress
	if fs.CurrentPhase == PhaseExamine && fs.Status == StatusInProgress {
		fs.CurrentPhase = PhaseHarden
		fs.UpdatedAt = time.Now()
		return fs.CurrentPhase
	}

	// For other phases, check completion
	if !fs.PhaseComplete(fs.CurrentPhase) {
		return 0
	}

	// Phase 4 (specify) requires user approval before advancing.
	// ApproveSpecification() sets approvalGranted by moving status back to InProgress.
	// We detect the "already approved" case by checking a marker: if the user just
	// approved, we allow advance. Otherwise we gate.
	if fs.CurrentPhase == PhaseSpecify {
		if fs.Status == StatusAwaitingApproval {
			// Still waiting for user to call ApproveSpecification()
			return 0
		}
		if !fs.specifyApproved {
			// First time seeing specify complete — trigger the gate
			fs.Status = StatusAwaitingApproval
			fs.UpdatedAt = time.Now()
			return 0
		}
		// User approved — clear flag and continue to next phase
		fs.specifyApproved = false
	}

	nextPhase := fs.CurrentPhase + 1
	if nextPhase > PhaseDocument {
		fs.Status = StatusComplete
		fs.UpdatedAt = time.Now()
		return 0
	}

	fs.CurrentPhase = nextPhase
	fs.UpdatedAt = time.Now()
	return nextPhase
}

// Escalate marks the feature as requiring human intervention.
func (fs *FeatureState) Escalate(reason string) {
	fs.Status = StatusEscalated
	fs.EscalationReason = reason
	fs.UpdatedAt = time.Now()
}

// ApproveSpecification marks the user approval gate as passed (Phase 4).
func (fs *FeatureState) ApproveSpecification() error {
	if fs.Status != StatusAwaitingApproval {
		return fmt.Errorf("feature is not awaiting approval (status: %s)", fs.Status)
	}
	if fs.CurrentPhase != PhaseSpecify {
		return fmt.Errorf("approval only applies to specify phase (current: %s)", fs.CurrentPhase)
	}

	fs.Status = StatusInProgress
	fs.specifyApproved = true
	fs.UpdatedAt = time.Now()
	return nil
}

// MarkSubBeadDispatched records that a sub-bead has been sent to an artisan.
func (fs *FeatureState) MarkSubBeadDispatched(branch, beadID, artisanName string) error {
	sbs := fs.GetSubBeadState(branch)
	if sbs == nil {
		return fmt.Errorf("sub-bead not found for branch %q", branch)
	}
	now := time.Now()
	sbs.Status = SubBeadDispatched
	sbs.BeadID = beadID
	sbs.ArtisanName = artisanName
	sbs.DispatchedAt = &now
	fs.UpdatedAt = now
	return nil
}

// MarkSubBeadDone records that a sub-bead has completed successfully.
func (fs *FeatureState) MarkSubBeadDone(branch string) error {
	sbs := fs.GetSubBeadState(branch)
	if sbs == nil {
		return fmt.Errorf("sub-bead not found for branch %q", branch)
	}
	now := time.Now()
	sbs.Status = SubBeadDone
	sbs.CompletedAt = &now
	fs.UpdatedAt = now
	return nil
}

// MarkSubBeadFailed records that a sub-bead has failed.
func (fs *FeatureState) MarkSubBeadFailed(branch, reason string) error {
	sbs := fs.GetSubBeadState(branch)
	if sbs == nil {
		return fmt.Errorf("sub-bead not found for branch %q", branch)
	}
	now := time.Now()
	sbs.Status = SubBeadFailed
	sbs.FailureReason = reason
	sbs.CompletedAt = &now
	fs.UpdatedAt = now
	return nil
}

// StateStore handles persistence of feature state to disk.
type StateStore struct {
	rigPath string
}

// NewStateStore creates a store for the given rig.
func NewStateStore(rigPath string) *StateStore {
	return &StateStore{rigPath: rigPath}
}

func (s *StateStore) dir() string {
	return filepath.Join(s.rigPath, "conductor", "features")
}

func (s *StateStore) path(featureName string) string {
	return filepath.Join(s.dir(), featureName+".json")
}

// Save persists feature state to disk.
func (s *StateStore) Save(state *FeatureState) error {
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		return fmt.Errorf("creating features dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := os.WriteFile(s.path(state.FeatureName), data, 0o644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	return nil
}

// Load reads feature state from disk.
func (s *StateStore) Load(featureName string) (*FeatureState, error) {
	data, err := os.ReadFile(s.path(featureName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("feature %q not found", featureName)
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var state FeatureState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	return &state, nil
}

// List returns all tracked feature names.
func (s *StateStore) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading features dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".json" {
			names = append(names, name[:len(name)-5])
		}
	}
	return names, nil
}
