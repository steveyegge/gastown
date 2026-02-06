//go:build k8s

// Package v1alpha1 contains the CRD API types for Gas Town Kubernetes resources.
//
// These types define the schema for GasTown, Rig, and AgentPool custom resources.
// The CRDs provide K8s-native pod templates and status projection. Beads is the
// single source of truth for all state; the controller is a reactive bridge that
// watches beads events and translates them to pod operations.
//
// Architecture:
//   Beads (Dolt) = source of truth for lifecycle, config, state
//   CRD spec     = K8s-specific pod concerns (image, resources, scheduling)
//   CRD status   = projection of beads state into K8s-native format
//   Controller   = reactive bridge: beads events → pod create/delete
//
// See docs/design/k8s-crd-schema.md for the full design document.
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================================
// GasTown CRD
// ============================================================================

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Rigs",type=integer,JSONPath=`.status.rigCount`
// +kubebuilder:printcolumn:name="Agents",type=integer,JSONPath=`.status.agentCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GasTown is the top-level resource. It configures how the controller
// connects to the BD Daemon (to watch beads events) and provides default
// pod templates for agent creation. Status is projected from beads state.
type GasTown struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GasTownSpec   `json:"spec,omitempty"`
	Status GasTownStatus `json:"status,omitempty"`
}

// GasTownSpec defines the controller's daemon connection and pod defaults.
// This contains ONLY K8s-specific concerns. All beads configuration
// (merge strategy, naming, workflows, roles, etc.) lives in beads config
// files and is managed by gt/bd commands.
type GasTownSpec struct {
	// Name is the town identifier (e.g., "gt11"). Used for pod labeling.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// Daemon configures the BD Daemon connection. The controller watches
	// beads lifecycle events through this daemon via RPC.
	Daemon DaemonConnectionSpec `json:"daemon"`

	// Defaults provides default pod template values for all agents.
	// Rigs and AgentPools can override individual fields.
	// +optional
	Defaults *PodDefaultsSpec `json:"defaults,omitempty"`

	// RigSelector selects which Rig CRs belong to this town.
	// +optional
	RigSelector *metav1.LabelSelector `json:"rigSelector,omitempty"`
}

// DaemonConnectionSpec configures how the controller connects to BD Daemon
// to watch beads lifecycle events.
type DaemonConnectionSpec struct {
	// Host is the BD Daemon service hostname.
	Host string `json:"host"`

	// Port is the BD Daemon RPC port.
	// +kubebuilder:default=9876
	Port int32 `json:"port"`

	// TokenSecretRef references an optional auth token for the daemon.
	// +optional
	TokenSecretRef *SecretKeyRef `json:"tokenSecretRef,omitempty"`
}

// PodDefaultsSpec defines default values for agent pod creation.
// These are K8s-specific concerns that beads doesn't manage.
// Used at town, rig, role, and pool levels in a merge hierarchy:
// GasTown defaults < Rig overrides < Role overrides < AgentPool template.
type PodDefaultsSpec struct {
	// Image is the agent container image.
	// +optional
	Image string `json:"image,omitempty"`

	// Resources defines compute resources.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configures workspace PVC.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// ScreenScrollback sets the screen session scrollback size.
	// +kubebuilder:default=10000
	// +optional
	ScreenScrollback int32 `json:"screenScrollback,omitempty"`

	// Env provides environment variables for agent pods.
	// Typically includes secret references for API keys and git credentials.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// NodeSelector constrains scheduling.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for agent pods.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// ServiceAccountName for agent pods.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// StorageSpec configures a PVC for agent workspace.
type StorageSpec struct {
	// Size is the requested storage capacity.
	// +kubebuilder:default="10Gi"
	Size resource.Quantity `json:"size"`

	// StorageClassName is the storage class.
	// +kubebuilder:default="gp3"
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`
}

// SecretKeyRef references a key within a Secret.
type SecretKeyRef struct {
	// Name of the Secret.
	Name string `json:"name"`

	// Key within the Secret data.
	// +optional
	Key string `json:"key,omitempty"`
}

// GasTownStatus is projected from beads state. The controller reads beads
// via the daemon and writes this status — it never computes state itself.
type GasTownStatus struct {
	// Phase is the high-level town state, projected from beads.
	// +kubebuilder:validation:Enum=Pending;Running;Degraded;Failed
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the last spec generation the controller acted on.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent controller-level observations.
	// Standard types: DaemonConnected, Healthy.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// RigCount is projected from beads (number of configured rigs).
	RigCount int32 `json:"rigCount,omitempty"`

	// AgentCount is projected from beads (total running agents).
	AgentCount int32 `json:"agentCount,omitempty"`

	// Daemon reflects the controller's connection to BD Daemon.
	// +optional
	Daemon *DaemonConnectionStatus `json:"daemon,omitempty"`
}

// DaemonConnectionStatus reports the controller's daemon connection.
type DaemonConnectionStatus struct {
	// Connected indicates the controller can reach the daemon.
	Connected bool `json:"connected"`

	// Endpoint is the daemon address.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// +kubebuilder:object:root=true

// GasTownList contains a list of GasTown resources.
type GasTownList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GasTown `json:"items"`
}

// ============================================================================
// Rig CRD
// ============================================================================

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Polecats",type=integer,JSONPath=`.status.polecatCount`
// +kubebuilder:printcolumn:name="Crew",type=integer,JSONPath=`.status.crewCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Rig provides per-rig pod template overrides and projects rig status
// from beads. When beads emits a lifecycle event for an agent in this rig,
// the controller uses these overrides to build the pod spec.
type Rig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RigSpec   `json:"spec,omitempty"`
	Status RigStatus `json:"status,omitempty"`
}

// RigSpec defines pod template overrides for a rig. Contains ONLY
// K8s-specific concerns. All beads rig config (merge queue, naming,
// workflows, etc.) stays in beads config files.
type RigSpec struct {
	// Name must match the beads rig name exactly.
	// The controller uses this to correlate beads events to this Rig CR.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_-]*$`
	Name string `json:"name"`

	// PodOverrides provides rig-level pod template overrides.
	// Merged with GasTown.spec.defaults — rig values take precedence.
	// +optional
	PodOverrides *PodDefaultsSpec `json:"podOverrides,omitempty"`

	// RoleOverrides provides role-specific pod template overrides.
	// Merge order: GasTown defaults < Rig podOverrides < Role overrides.
	// Keys are role names: "polecat", "crew", "witness", "refinery".
	// +optional
	RoleOverrides map[string]PodDefaultsSpec `json:"roleOverrides,omitempty"`
}

// RigStatus is projected from beads state for this rig.
type RigStatus struct {
	// Phase is the high-level rig state, projected from beads.
	// +kubebuilder:validation:Enum=Pending;Running;Degraded;Failed
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the last spec generation the controller acted on.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent controller-level observations.
	// Standard types: WitnessReady, RefineryReady, Reconciled.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// PolecatCount is the number of active polecats (from beads).
	PolecatCount int32 `json:"polecatCount,omitempty"`

	// CrewCount is the number of active crew members (from beads).
	CrewCount int32 `json:"crewCount,omitempty"`

	// Witness reflects witness agent status (from beads).
	// +optional
	Witness *SystemAgentStatus `json:"witness,omitempty"`

	// Refinery reflects refinery agent status (from beads).
	// +optional
	Refinery *SystemAgentStatus `json:"refinery,omitempty"`

	// MergeQueue reflects merge queue metrics (from beads).
	// +optional
	MergeQueue *MergeQueueStatus `json:"mergeQueue,omitempty"`
}

// SystemAgentStatus reports a system agent's state (witness, refinery).
type SystemAgentStatus struct {
	// Ready indicates the agent is healthy (from beads health checks).
	Ready bool `json:"ready"`

	// PodName is the K8s pod running this agent (set by controller).
	// +optional
	PodName string `json:"podName,omitempty"`
}

// MergeQueueStatus reports merge queue metrics from beads.
type MergeQueueStatus struct {
	// Depth is the number of items in the queue.
	Depth int32 `json:"depth,omitempty"`

	// Processing is the number of items currently being merged.
	Processing int32 `json:"processing,omitempty"`
}

// +kubebuilder:object:root=true

// RigList contains a list of Rig resources.
type RigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rig `json:"items"`
}

// ============================================================================
// AgentPool CRD
// ============================================================================

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`
// +kubebuilder:printcolumn:name="Rig",type=string,JSONPath=`.spec.rig`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Active",type=integer,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentPool defines the pod template for a specific (rig, role) pair.
// Beads decides WHEN to spawn agents — AgentPool defines HOW the pod is built.
// The controller watches beads for lifecycle events and uses this template
// to create/delete agent pods reactively.
type AgentPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentPoolSpec   `json:"spec,omitempty"`
	Status AgentPoolStatus `json:"status,omitempty"`
}

// AgentPoolSpec defines the pod template for a role in a rig.
type AgentPoolSpec struct {
	// Rig must match the beads rig name.
	Rig string `json:"rig"`

	// Role must match the beads role name.
	// +kubebuilder:validation:Enum=polecat;crew;witness;refinery;mayor;deacon
	Role string `json:"role"`

	// Template defines the pod specification for agents in this pool.
	// Final layer in the merge hierarchy — overrides everything above.
	Template PodDefaultsSpec `json:"template"`
}

// AgentPoolStatus is projected from beads state for this (rig, role) pair.
type AgentPoolStatus struct {
	// Ready is the number of running agents (from beads).
	Ready int32 `json:"ready,omitempty"`

	// Active is the number of agents with hooked work (from beads).
	Active int32 `json:"active,omitempty"`

	// Pending is the number of agents starting up.
	Pending int32 `json:"pending,omitempty"`

	// Agents lists individual agent statuses (from beads).
	// +optional
	Agents []AgentInstanceStatus `json:"agents,omitempty"`
}

// AgentInstanceStatus reports an individual agent's state.
// All fields except PodName are projected from beads.
type AgentInstanceStatus struct {
	// Name is the agent name from beads (e.g., "furiosa").
	Name string `json:"name"`

	// Phase is the agent lifecycle phase from beads.
	// +kubebuilder:validation:Enum=Pending;Starting;Running;Completing;Terminated
	Phase string `json:"phase"`

	// HookBead is the bead ID the agent is working on (from beads).
	// +optional
	HookBead string `json:"hookBead,omitempty"`

	// PodName is the K8s pod running this agent (set by controller).
	// +optional
	PodName string `json:"podName,omitempty"`

	// StartedAt is when the agent started (from beads).
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
}

// +kubebuilder:object:root=true

// AgentPoolList contains a list of AgentPool resources.
type AgentPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&GasTown{}, &GasTownList{},
		&Rig{}, &RigList{},
		&AgentPool{}, &AgentPoolList{},
	)
}
