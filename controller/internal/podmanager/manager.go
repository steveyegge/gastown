// Package podmanager handles K8s pod CRUD for Gas Town agents.
// It translates beads lifecycle decisions into pod create/delete operations.
// The pod manager never makes lifecycle decisions — it executes them.
package podmanager

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	// Label keys for agent pods.
	LabelApp   = "app.kubernetes.io/name"
	LabelRig   = "gastown.io/rig"
	LabelRole  = "gastown.io/role"
	LabelAgent = "gastown.io/agent"

	// LabelAppValue is the app label value for all gastown pods.
	LabelAppValue = "gastown"

	// Default resource values.
	DefaultCPURequest    = "500m"
	DefaultCPULimit      = "2"
	DefaultMemoryRequest = "1Gi"
	DefaultMemoryLimit   = "4Gi"

	// Volume names.
	VolumeWorkspace  = "workspace"
	VolumeTmp        = "tmp"
	VolumeBeadsConfig = "beads-config"

	// Mount paths.
	MountWorkspace   = "/home/agent/gt"
	MountTmp         = "/tmp"
	MountBeadsConfig = "/etc/agent-pod"

	// Container constants.
	ContainerName = "agent"
	AgentUID      = int64(1000)
	AgentGID      = int64(1000)
)

// SecretEnvSource maps a K8s Secret key to a pod environment variable.
type SecretEnvSource struct {
	EnvName    string // env var name in the pod
	SecretName string // K8s Secret name
	SecretKey  string // key within the Secret
}

// AgentPodSpec describes the desired pod for an agent.
type AgentPodSpec struct {
	Rig       string
	Role      string // polecat, crew, witness, refinery, mayor, deacon
	AgentName string
	Image     string
	Namespace string
	Env       map[string]string

	// Resources sets compute requests/limits. If nil, defaults are used.
	Resources *corev1.ResourceRequirements

	// SecretEnv injects environment variables from K8s Secrets.
	// Used for API keys (ANTHROPIC_API_KEY) and git credentials.
	SecretEnv []SecretEnvSource

	// ConfigMapName is the name of a ConfigMap to mount at MountBeadsConfig.
	// Contains agent configuration (role, rig, daemon connection, etc.).
	ConfigMapName string

	// ServiceAccountName for the pod. Empty uses the namespace default.
	ServiceAccountName string

	// NodeSelector constrains pod scheduling.
	NodeSelector map[string]string

	// Tolerations for the pod.
	Tolerations []corev1.Toleration

	// WorkspaceStorage configures a PVC for persistent workspace.
	// Used by crew pods. If nil, an EmptyDir is used for polecat pods.
	WorkspaceStorage *WorkspaceStorageSpec
}

// WorkspaceStorageSpec configures a PVC-backed workspace volume.
type WorkspaceStorageSpec struct {
	// ClaimName is the PVC name. If empty, derived from pod name.
	ClaimName string

	// Size is the requested storage (e.g., "10Gi").
	Size string

	// StorageClassName is the storage class (e.g., "gp3").
	StorageClassName string
}

// PodName returns the canonical pod name: gt-{rig}-{role}-{name}.
func (s *AgentPodSpec) PodName() string {
	return fmt.Sprintf("gt-%s-%s-%s", s.Rig, s.Role, s.AgentName)
}

// Labels returns the standard label set for this agent pod.
func (s *AgentPodSpec) Labels() map[string]string {
	return map[string]string{
		LabelApp:   LabelAppValue,
		LabelRig:   s.Rig,
		LabelRole:  s.Role,
		LabelAgent: s.AgentName,
	}
}

// Manager creates, deletes, and lists agent pods in K8s.
type Manager interface {
	CreateAgentPod(ctx context.Context, spec AgentPodSpec) error
	DeleteAgentPod(ctx context.Context, name, namespace string) error
	ListAgentPods(ctx context.Context, namespace string, labelSelector map[string]string) ([]corev1.Pod, error)
	GetAgentPod(ctx context.Context, name, namespace string) (*corev1.Pod, error)
}

// K8sManager implements Manager using client-go.
type K8sManager struct {
	client kubernetes.Interface
	logger *slog.Logger
}

// New creates a pod manager backed by a K8s client.
func New(client kubernetes.Interface, logger *slog.Logger) *K8sManager {
	return &K8sManager{client: client, logger: logger}
}

// CreateAgentPod creates a pod for the given agent spec.
func (m *K8sManager) CreateAgentPod(ctx context.Context, spec AgentPodSpec) error {
	pod := m.buildPod(spec)
	m.logger.Info("creating agent pod",
		"pod", pod.Name, "rig", spec.Rig, "role", spec.Role, "agent", spec.AgentName)

	_, err := m.client.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating pod %s: %w", pod.Name, err)
	}
	return nil
}

// DeleteAgentPod deletes a pod by name and namespace.
func (m *K8sManager) DeleteAgentPod(ctx context.Context, name, namespace string) error {
	m.logger.Info("deleting agent pod", "pod", name, "namespace", namespace)
	return m.client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ListAgentPods lists pods matching the given labels.
func (m *K8sManager) ListAgentPods(ctx context.Context, namespace string, labelSelector map[string]string) ([]corev1.Pod, error) {
	sel := labels.Set(labelSelector).String()
	list, err := m.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: sel,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pods with selector %s: %w", sel, err)
	}
	return list.Items, nil
}

// GetAgentPod gets a single pod by name.
func (m *K8sManager) GetAgentPod(ctx context.Context, name, namespace string) (*corev1.Pod, error) {
	return m.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (m *K8sManager) buildPod(spec AgentPodSpec) *corev1.Pod {
	container := m.buildContainer(spec)
	volumes := m.buildVolumes(spec)

	podSpec := corev1.PodSpec{
		Containers:    []corev1.Container{container},
		Volumes:       volumes,
		RestartPolicy: restartPolicyForRole(spec.Role),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser:    intPtr(AgentUID),
			RunAsGroup:   intPtr(AgentGID),
			RunAsNonRoot: boolPtr(true),
			FSGroup:      intPtr(AgentGID),
		},
	}

	if spec.ServiceAccountName != "" {
		podSpec.ServiceAccountName = spec.ServiceAccountName
	}
	if len(spec.NodeSelector) > 0 {
		podSpec.NodeSelector = spec.NodeSelector
	}
	if len(spec.Tolerations) > 0 {
		podSpec.Tolerations = spec.Tolerations
	}

	// Polecats are one-shot; use a 30s termination grace period.
	// Persistent roles get the default (30s is also reasonable).
	gracePeriod := int64(30)
	podSpec.TerminationGracePeriodSeconds = &gracePeriod

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.PodName(),
			Namespace: spec.Namespace,
			Labels:    spec.Labels(),
		},
		Spec: podSpec,
	}
}

// buildContainer constructs the agent container with env vars, resources,
// volume mounts, and security context.
func (m *K8sManager) buildContainer(spec AgentPodSpec) corev1.Container {
	envVars := m.buildEnvVars(spec)
	mounts := m.buildVolumeMounts(spec)
	resources := m.buildResources(spec)

	return corev1.Container{
		Name:            ContainerName,
		Image:           spec.Image,
		Env:             envVars,
		Resources:       resources,
		VolumeMounts:    mounts,
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			ReadOnlyRootFilesystem:   boolPtr(false),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}

// buildEnvVars constructs environment variables from plain values and secret references.
func (m *K8sManager) buildEnvVars(spec AgentPodSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{Name: "GT_ROLE", Value: spec.Role},
		{Name: "GT_RIG", Value: spec.Rig},
		{Name: "GT_AGENT", Value: spec.AgentName},
		{Name: "HOME", Value: "/home/agent"},
	}

	// Add role-specific env vars.
	switch spec.Role {
	case "polecat":
		envVars = append(envVars, corev1.EnvVar{Name: "GT_POLECAT", Value: spec.AgentName})
	case "crew":
		envVars = append(envVars, corev1.EnvVar{Name: "GT_CREW", Value: spec.AgentName})
	}

	// Add plain env vars from spec.
	for k, v := range spec.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	// Add secret-sourced env vars.
	for _, se := range spec.SecretEnv {
		envVars = append(envVars, corev1.EnvVar{
			Name: se.EnvName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: se.SecretName},
					Key:                  se.SecretKey,
				},
			},
		})
	}

	return envVars
}

// buildResources returns resource requirements. Uses spec.Resources if provided,
// otherwise falls back to defaults.
func (m *K8sManager) buildResources(spec AgentPodSpec) corev1.ResourceRequirements {
	if spec.Resources != nil {
		return *spec.Resources
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(DefaultCPURequest),
			corev1.ResourceMemory: resource.MustParse(DefaultMemoryRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(DefaultCPULimit),
			corev1.ResourceMemory: resource.MustParse(DefaultMemoryLimit),
		},
	}
}

// buildVolumes returns the volumes for the pod based on role.
func (m *K8sManager) buildVolumes(spec AgentPodSpec) []corev1.Volume {
	var volumes []corev1.Volume

	// Workspace volume: PVC for persistent roles, EmptyDir for ephemeral.
	if spec.WorkspaceStorage != nil {
		claimName := spec.WorkspaceStorage.ClaimName
		if claimName == "" {
			claimName = spec.PodName() + "-workspace"
		}
		volumes = append(volumes, corev1.Volume{
			Name: VolumeWorkspace,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: VolumeWorkspace,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Tmp volume: always EmptyDir.
	volumes = append(volumes, corev1.Volume{
		Name: VolumeTmp,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	// Beads config volume: ConfigMap mount if specified.
	if spec.ConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: VolumeBeadsConfig,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: spec.ConfigMapName},
				},
			},
		})
	}

	return volumes
}

// buildVolumeMounts returns the volume mounts for the agent container.
func (m *K8sManager) buildVolumeMounts(spec AgentPodSpec) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{Name: VolumeWorkspace, MountPath: MountWorkspace},
		{Name: VolumeTmp, MountPath: MountTmp},
	}

	if spec.ConfigMapName != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      VolumeBeadsConfig,
			MountPath: MountBeadsConfig,
			ReadOnly:  true,
		})
	}

	return mounts
}

// restartPolicyForRole returns the appropriate restart policy.
// Polecats are one-shot (Never); all others restart on failure.
func restartPolicyForRole(role string) corev1.RestartPolicy {
	if role == "polecat" {
		return corev1.RestartPolicyNever
	}
	return corev1.RestartPolicyAlways
}

func intPtr(i int64) *int64   { return &i }
func boolPtr(b bool) *bool    { return &b }
