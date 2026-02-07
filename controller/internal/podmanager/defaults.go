package podmanager

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PodDefaults holds default pod template values that can be overridden
// at each level of the merge hierarchy:
//   GasTown defaults < Rig overrides < Role overrides < AgentPool template
type PodDefaults struct {
	Image              string
	Resources          *corev1.ResourceRequirements
	ServiceAccountName string
	NodeSelector       map[string]string
	Tolerations        []corev1.Toleration
	Env                map[string]string
	SecretEnv          []SecretEnvSource
	ConfigMapName      string
	WorkspaceStorage   *WorkspaceStorageSpec
}

// MergePodDefaults merges an override layer onto a base, returning a new PodDefaults.
// Non-zero override values replace base values. Maps and slices are merged (override wins).
func MergePodDefaults(base, override *PodDefaults) *PodDefaults {
	if base == nil && override == nil {
		return &PodDefaults{}
	}
	if base == nil {
		cp := *override
		return &cp
	}
	if override == nil {
		cp := *base
		return &cp
	}

	result := *base

	if override.Image != "" {
		result.Image = override.Image
	}
	if override.Resources != nil {
		result.Resources = mergeResources(result.Resources, override.Resources)
	}
	if override.ServiceAccountName != "" {
		result.ServiceAccountName = override.ServiceAccountName
	}
	if len(override.NodeSelector) > 0 {
		result.NodeSelector = mergeMaps(result.NodeSelector, override.NodeSelector)
	}
	if len(override.Tolerations) > 0 {
		result.Tolerations = override.Tolerations
	}
	if len(override.Env) > 0 {
		result.Env = mergeMaps(result.Env, override.Env)
	}
	if len(override.SecretEnv) > 0 {
		result.SecretEnv = override.SecretEnv
	}
	if override.ConfigMapName != "" {
		result.ConfigMapName = override.ConfigMapName
	}
	if override.WorkspaceStorage != nil {
		result.WorkspaceStorage = override.WorkspaceStorage
	}

	return &result
}

// ApplyDefaults applies PodDefaults to an AgentPodSpec, filling in
// any fields that aren't already set on the spec.
func ApplyDefaults(spec *AgentPodSpec, defaults *PodDefaults) {
	if defaults == nil {
		return
	}

	if spec.Image == "" && defaults.Image != "" {
		spec.Image = defaults.Image
	}
	if spec.Resources == nil && defaults.Resources != nil {
		spec.Resources = defaults.Resources
	}
	if spec.ServiceAccountName == "" && defaults.ServiceAccountName != "" {
		spec.ServiceAccountName = defaults.ServiceAccountName
	}
	if len(spec.NodeSelector) == 0 && len(defaults.NodeSelector) > 0 {
		spec.NodeSelector = defaults.NodeSelector
	}
	if len(spec.Tolerations) == 0 && len(defaults.Tolerations) > 0 {
		spec.Tolerations = defaults.Tolerations
	}
	if spec.ConfigMapName == "" && defaults.ConfigMapName != "" {
		spec.ConfigMapName = defaults.ConfigMapName
	}
	if spec.WorkspaceStorage == nil && defaults.WorkspaceStorage != nil {
		spec.WorkspaceStorage = defaults.WorkspaceStorage
	}

	// Merge env maps (spec values take precedence over defaults).
	if len(defaults.Env) > 0 {
		if spec.Env == nil {
			spec.Env = make(map[string]string)
		}
		for k, v := range defaults.Env {
			if _, exists := spec.Env[k]; !exists {
				spec.Env[k] = v
			}
		}
	}

	// Append default secret env sources that aren't already in the spec.
	if len(defaults.SecretEnv) > 0 {
		existing := make(map[string]bool)
		for _, se := range spec.SecretEnv {
			existing[se.EnvName] = true
		}
		for _, se := range defaults.SecretEnv {
			if !existing[se.EnvName] {
				spec.SecretEnv = append(spec.SecretEnv, se)
			}
		}
	}
}

// mergeResources merges resource requirements, with override values taking precedence.
func mergeResources(base, override *corev1.ResourceRequirements) *corev1.ResourceRequirements {
	if base == nil {
		cp := *override
		return &cp
	}

	result := &corev1.ResourceRequirements{
		Requests: mergeResourceList(base.Requests, override.Requests),
		Limits:   mergeResourceList(base.Limits, override.Limits),
	}
	return result
}

func mergeResourceList(base, override corev1.ResourceList) corev1.ResourceList {
	if base == nil && override == nil {
		return nil
	}
	result := make(corev1.ResourceList)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

func mergeMaps(base, override map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

// DefaultPodDefaultsForRole returns sensible defaults for a given role.
func DefaultPodDefaultsForRole(role string) *PodDefaults {
	defaults := &PodDefaults{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(DefaultCPURequest),
				corev1.ResourceMemory: resource.MustParse(DefaultMemoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(DefaultCPULimit),
				corev1.ResourceMemory: resource.MustParse(DefaultMemoryLimit),
			},
		},
	}

	switch role {
	case "crew":
		// Crew pods get persistent workspace storage.
		defaults.WorkspaceStorage = &WorkspaceStorageSpec{
			Size:             "10Gi",
			StorageClassName: "gp3",
		}
	case "polecat":
		// Polecats use EmptyDir (no WorkspaceStorage).
	case "witness", "refinery":
		// Singletons get persistent storage for state.
		defaults.WorkspaceStorage = &WorkspaceStorageSpec{
			Size:             "5Gi",
			StorageClassName: "gp3",
		}
	}

	return defaults
}
