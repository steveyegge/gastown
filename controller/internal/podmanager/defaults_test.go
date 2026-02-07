package podmanager

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMergePodDefaults_BothNil(t *testing.T) {
	result := MergePodDefaults(nil, nil)
	if result == nil {
		t.Fatal("MergePodDefaults(nil, nil) should return non-nil")
	}
}

func TestMergePodDefaults_BaseOnly(t *testing.T) {
	base := &PodDefaults{Image: "base:latest"}
	result := MergePodDefaults(base, nil)
	if result.Image != "base:latest" {
		t.Errorf("Image = %q, want %q", result.Image, "base:latest")
	}
}

func TestMergePodDefaults_OverrideOnly(t *testing.T) {
	override := &PodDefaults{Image: "override:latest"}
	result := MergePodDefaults(nil, override)
	if result.Image != "override:latest" {
		t.Errorf("Image = %q, want %q", result.Image, "override:latest")
	}
}

func TestMergePodDefaults_OverrideWins(t *testing.T) {
	base := &PodDefaults{
		Image:              "base:latest",
		ServiceAccountName: "base-sa",
		ConfigMapName:      "base-config",
	}
	override := &PodDefaults{
		Image: "override:v2",
	}

	result := MergePodDefaults(base, override)
	if result.Image != "override:v2" {
		t.Errorf("Image = %q, want %q", result.Image, "override:v2")
	}
	if result.ServiceAccountName != "base-sa" {
		t.Errorf("ServiceAccountName = %q, want %q (preserved from base)", result.ServiceAccountName, "base-sa")
	}
	if result.ConfigMapName != "base-config" {
		t.Errorf("ConfigMapName = %q, want %q (preserved from base)", result.ConfigMapName, "base-config")
	}
}

func TestMergePodDefaults_EnvMerge(t *testing.T) {
	base := &PodDefaults{
		Env: map[string]string{"A": "1", "B": "2"},
	}
	override := &PodDefaults{
		Env: map[string]string{"B": "override", "C": "3"},
	}

	result := MergePodDefaults(base, override)
	if result.Env["A"] != "1" {
		t.Errorf("Env[A] = %q, want %q", result.Env["A"], "1")
	}
	if result.Env["B"] != "override" {
		t.Errorf("Env[B] = %q, want %q (override wins)", result.Env["B"], "override")
	}
	if result.Env["C"] != "3" {
		t.Errorf("Env[C] = %q, want %q", result.Env["C"], "3")
	}
}

func TestMergePodDefaults_ResourcesMerge(t *testing.T) {
	base := &PodDefaults{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	}
	override := &PodDefaults{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("1"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}

	result := MergePodDefaults(base, override)
	if result.Resources == nil {
		t.Fatal("Resources should not be nil")
	}

	// CPU request should be overridden to "1".
	cpuReq := result.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "1" {
		t.Errorf("CPU request = %s, want 1", cpuReq.String())
	}

	// Memory request preserved from base.
	memReq := result.Resources.Requests[corev1.ResourceMemory]
	if memReq.String() != "1Gi" {
		t.Errorf("Memory request = %s, want 1Gi (from base)", memReq.String())
	}

	// Limits added from override.
	cpuLimit := result.Resources.Limits[corev1.ResourceCPU]
	if cpuLimit.String() != "4" {
		t.Errorf("CPU limit = %s, want 4", cpuLimit.String())
	}
}

func TestMergePodDefaults_WorkspaceStorageOverride(t *testing.T) {
	base := &PodDefaults{
		WorkspaceStorage: &WorkspaceStorageSpec{Size: "5Gi", StorageClassName: "gp2"},
	}
	override := &PodDefaults{
		WorkspaceStorage: &WorkspaceStorageSpec{Size: "20Gi", StorageClassName: "gp3"},
	}

	result := MergePodDefaults(base, override)
	if result.WorkspaceStorage.Size != "20Gi" {
		t.Errorf("WorkspaceStorage.Size = %q, want %q", result.WorkspaceStorage.Size, "20Gi")
	}
	if result.WorkspaceStorage.StorageClassName != "gp3" {
		t.Errorf("WorkspaceStorage.StorageClassName = %q, want %q", result.WorkspaceStorage.StorageClassName, "gp3")
	}
}

func TestMergePodDefaults_TolerationsOverride(t *testing.T) {
	base := &PodDefaults{
		Tolerations: []corev1.Toleration{
			{Key: "old", Value: "v1"},
		},
	}
	override := &PodDefaults{
		Tolerations: []corev1.Toleration{
			{Key: "new", Value: "v2"},
		},
	}

	result := MergePodDefaults(base, override)
	if len(result.Tolerations) != 1 {
		t.Fatalf("got %d tolerations, want 1", len(result.Tolerations))
	}
	if result.Tolerations[0].Key != "new" {
		t.Errorf("toleration key = %q, want %q (override replaces)", result.Tolerations[0].Key, "new")
	}
}

func TestMergePodDefaults_SecretEnvOverride(t *testing.T) {
	base := &PodDefaults{
		SecretEnv: []SecretEnvSource{
			{EnvName: "OLD_SECRET", SecretName: "s1", SecretKey: "k1"},
		},
	}
	override := &PodDefaults{
		SecretEnv: []SecretEnvSource{
			{EnvName: "NEW_SECRET", SecretName: "s2", SecretKey: "k2"},
		},
	}

	result := MergePodDefaults(base, override)
	if len(result.SecretEnv) != 1 {
		t.Fatalf("got %d secret envs, want 1", len(result.SecretEnv))
	}
	if result.SecretEnv[0].EnvName != "NEW_SECRET" {
		t.Errorf("SecretEnv[0].EnvName = %q, want %q", result.SecretEnv[0].EnvName, "NEW_SECRET")
	}
}

func TestApplyDefaults_FillsMissing(t *testing.T) {
	spec := &AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "test",
		Namespace: "gastown",
	}
	defaults := &PodDefaults{
		Image:              "default-agent:latest",
		ServiceAccountName: "default-sa",
		ConfigMapName:      "default-config",
		Env:                map[string]string{"DEFAULT_VAR": "val"},
		SecretEnv: []SecretEnvSource{
			{EnvName: "API_KEY", SecretName: "keys", SecretKey: "api"},
		},
	}

	ApplyDefaults(spec, defaults)

	if spec.Image != "default-agent:latest" {
		t.Errorf("Image = %q, want %q", spec.Image, "default-agent:latest")
	}
	if spec.ServiceAccountName != "default-sa" {
		t.Errorf("ServiceAccountName = %q, want %q", spec.ServiceAccountName, "default-sa")
	}
	if spec.ConfigMapName != "default-config" {
		t.Errorf("ConfigMapName = %q, want %q", spec.ConfigMapName, "default-config")
	}
	if spec.Env["DEFAULT_VAR"] != "val" {
		t.Errorf("Env[DEFAULT_VAR] = %q, want %q", spec.Env["DEFAULT_VAR"], "val")
	}
	if len(spec.SecretEnv) != 1 {
		t.Fatalf("got %d secret envs, want 1", len(spec.SecretEnv))
	}
}

func TestApplyDefaults_SpecOverridesDefaults(t *testing.T) {
	spec := &AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "test",
		Image:              "custom:v1",
		Namespace:          "gastown",
		ServiceAccountName: "custom-sa",
		Env:                map[string]string{"SHARED": "spec-value"},
		SecretEnv: []SecretEnvSource{
			{EnvName: "API_KEY", SecretName: "custom-keys", SecretKey: "api"},
		},
	}
	defaults := &PodDefaults{
		Image:              "default:latest",
		ServiceAccountName: "default-sa",
		Env:                map[string]string{"SHARED": "default-value", "DEFAULT_ONLY": "val"},
		SecretEnv: []SecretEnvSource{
			{EnvName: "API_KEY", SecretName: "default-keys", SecretKey: "api"},
			{EnvName: "GIT_TOKEN", SecretName: "git", SecretKey: "token"},
		},
	}

	ApplyDefaults(spec, defaults)

	if spec.Image != "custom:v1" {
		t.Errorf("Image = %q, want %q (spec should win)", spec.Image, "custom:v1")
	}
	if spec.ServiceAccountName != "custom-sa" {
		t.Errorf("ServiceAccountName = %q, want %q (spec should win)", spec.ServiceAccountName, "custom-sa")
	}
	if spec.Env["SHARED"] != "spec-value" {
		t.Errorf("Env[SHARED] = %q, want %q (spec should win)", spec.Env["SHARED"], "spec-value")
	}
	if spec.Env["DEFAULT_ONLY"] != "val" {
		t.Errorf("Env[DEFAULT_ONLY] = %q, want %q (from defaults)", spec.Env["DEFAULT_ONLY"], "val")
	}

	// API_KEY from spec should be kept; GIT_TOKEN from defaults appended.
	if len(spec.SecretEnv) != 2 {
		t.Fatalf("got %d secret envs, want 2", len(spec.SecretEnv))
	}
	// First should be spec's API_KEY with custom-keys.
	if spec.SecretEnv[0].SecretName != "custom-keys" {
		t.Errorf("SecretEnv[0].SecretName = %q, want %q", spec.SecretEnv[0].SecretName, "custom-keys")
	}
}

func TestApplyDefaults_NilDefaults(t *testing.T) {
	spec := &AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "test",
		Image: "original:latest", Namespace: "gastown",
	}
	ApplyDefaults(spec, nil)
	if spec.Image != "original:latest" {
		t.Errorf("Image changed from %q", "original:latest")
	}
}

func TestDefaultPodDefaultsForRole_Polecat(t *testing.T) {
	d := DefaultPodDefaultsForRole("polecat")
	if d.WorkspaceStorage != nil {
		t.Error("polecats should not have workspace storage")
	}
	if d.Resources == nil {
		t.Fatal("Resources should not be nil")
	}
}

func TestDefaultPodDefaultsForRole_Crew(t *testing.T) {
	d := DefaultPodDefaultsForRole("crew")
	if d.WorkspaceStorage == nil {
		t.Fatal("crew should have workspace storage")
	}
	if d.WorkspaceStorage.Size != "10Gi" {
		t.Errorf("workspace size = %q, want %q", d.WorkspaceStorage.Size, "10Gi")
	}
}

func TestDefaultPodDefaultsForRole_Witness(t *testing.T) {
	d := DefaultPodDefaultsForRole("witness")
	if d.WorkspaceStorage == nil {
		t.Fatal("witness should have workspace storage")
	}
	if d.WorkspaceStorage.Size != "5Gi" {
		t.Errorf("workspace size = %q, want %q", d.WorkspaceStorage.Size, "5Gi")
	}
}

func TestMergePodDefaults_ThreeLayerHierarchy(t *testing.T) {
	// Simulate: GasTown defaults < Rig overrides < Role overrides
	town := &PodDefaults{
		Image:              "town-agent:latest",
		ServiceAccountName: "town-sa",
		Env:                map[string]string{"TOWN": "yes", "SHARED": "town"},
	}
	rig := &PodDefaults{
		Env: map[string]string{"SHARED": "rig", "RIG": "gastown"},
	}
	role := &PodDefaults{
		Image: "role-agent:v2",
		Env:   map[string]string{"SHARED": "role"},
	}

	merged := MergePodDefaults(MergePodDefaults(town, rig), role)

	if merged.Image != "role-agent:v2" {
		t.Errorf("Image = %q, want %q (role wins)", merged.Image, "role-agent:v2")
	}
	if merged.ServiceAccountName != "town-sa" {
		t.Errorf("ServiceAccountName = %q, want %q (preserved from town)", merged.ServiceAccountName, "town-sa")
	}
	if merged.Env["SHARED"] != "role" {
		t.Errorf("Env[SHARED] = %q, want %q (role wins)", merged.Env["SHARED"], "role")
	}
	if merged.Env["TOWN"] != "yes" {
		t.Errorf("Env[TOWN] = %q, want %q (from town)", merged.Env["TOWN"], "yes")
	}
	if merged.Env["RIG"] != "gastown" {
		t.Errorf("Env[RIG] = %q, want %q (from rig)", merged.Env["RIG"], "gastown")
	}
}
