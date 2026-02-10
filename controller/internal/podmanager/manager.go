// Package podmanager handles K8s pod CRUD for Gas Town agents.
// It translates beads lifecycle decisions into pod create/delete operations.
// The pod manager never makes lifecycle decisions — it executes them.
package podmanager

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	VolumeWorkspace    = "workspace"
	VolumeTmp          = "tmp"
	VolumeBeadsConfig  = "beads-config"
	VolumeClaudeCreds  = "claude-creds"

	// Mount paths.
	MountWorkspace   = "/home/agent/gt"
	MountTmp         = "/tmp"
	MountBeadsConfig = "/etc/agent-pod"
	MountClaudeCreds = "/tmp/claude-credentials"

	// Session persistence: state dir on the workspace PVC.
	MountStateDir = "/home/agent/gt/.state"

	// Container constants.
	ContainerName = "agent"
	AgentUID      = int64(1000)
	AgentGID      = int64(1000)

	// Init container constants.
	InitCloneName    = "init-clone"
	InitCloneImage   = "alpine/git:latest"
	GitDaemonPort    = 9418

	// Coop sidecar constants.
	CoopContainerName       = "coop"
	CoopDefaultPort         = 8080
	CoopDefaultHealthPort   = 9090
	CoopDefaultCPURequest   = "50m"
	CoopDefaultCPULimit     = "200m"
	CoopDefaultMemRequest   = "32Mi"
	CoopDefaultMemLimit     = "64Mi"
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

	// CredentialsSecret is the K8s Secret name containing Claude OAuth credentials.
	// The "credentials.json" key is mounted at ~/.claude/.credentials.json.
	// Used for Claude Max/Corp accounts (no API key needed).
	CredentialsSecret string

	// DaemonTokenSecret is the K8s Secret name containing BD_DAEMON_TOKEN.
	// The "token" key is injected as the BD_DAEMON_TOKEN env var.
	DaemonTokenSecret string

	// CoopBuiltin indicates the agent image has coop built into its entrypoint.
	// When true, the agent container exposes coop HTTP ports (8080/9090) and
	// uses HTTP probes. No sidecar is added. Mutually exclusive with CoopSidecar.
	CoopBuiltin bool

	// CoopSidecar configures a Coop sidecar container for PTY-based agent
	// management. When set, the pod gets a coop container with health probes,
	// shareProcessNamespace is enabled, and backend metadata is set to "coop".
	CoopSidecar *CoopSidecarSpec

	// GitMirrorService is the in-cluster git mirror service name for this rig
	// (e.g., "git-mirror-beads"). When set and the role needs code access
	// (polecat, crew, refinery), an init container is added that clones from
	// git://<service>:9418/<rig>.git into the workspace.
	GitMirrorService string

	// GitURL is the actual upstream repository URL (e.g., "https://github.com/...").
	// Used to set the git remote origin after cloning from the mirror.
	GitURL string

	// GitDefaultBranch is the branch to checkout after cloning (default: "main").
	GitDefaultBranch string
}

// CoopSidecarSpec configures the Coop sidecar container.
type CoopSidecarSpec struct {
	// Image is the Coop container image (e.g., "ghcr.io/groblegark/coop:latest").
	Image string

	// Port is the Coop HTTP API port (default: 8080).
	Port int32

	// HealthPort is the Coop health probe port (default: 9090).
	HealthPort int32

	// AuthTokenSecret is the K8s Secret name containing the Coop auth token.
	// The "token" key within the Secret is used. Optional.
	AuthTokenSecret string

	// NatsURL is the NATS server URL for event bus integration.
	// Typically "nats://<daemon-service>:4222".
	NatsURL string

	// NatsTokenSecret is the K8s Secret name for NATS auth token.
	// The "token" key within the Secret is used. Optional.
	NatsTokenSecret string

	// Resources sets compute requests/limits for the sidecar.
	// If nil, defaults (50m/32Mi → 200m/64Mi) are used.
	Resources *corev1.ResourceRequirements
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
// If the spec includes WorkspaceStorage, a PVC is created first (idempotent).
func (m *K8sManager) CreateAgentPod(ctx context.Context, spec AgentPodSpec) error {
	// Ensure PVC exists before creating the pod.
	if spec.WorkspaceStorage != nil {
		if err := m.ensurePVC(ctx, spec); err != nil {
			return fmt.Errorf("ensuring workspace PVC: %w", err)
		}
	}

	pod := m.buildPod(spec)
	m.logger.Info("creating agent pod",
		"pod", pod.Name, "rig", spec.Rig, "role", spec.Role, "agent", spec.AgentName)

	_, err := m.client.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("creating pod %s: %w", pod.Name, err)
	}
	return nil
}

// ensurePVC creates the workspace PVC if it does not already exist.
func (m *K8sManager) ensurePVC(ctx context.Context, spec AgentPodSpec) error {
	ws := spec.WorkspaceStorage
	claimName := ws.ClaimName
	if claimName == "" {
		claimName = spec.PodName() + "-workspace"
	}

	size := ws.Size
	if size == "" {
		size = "10Gi"
	}
	storageClass := ws.StorageClassName
	if storageClass == "" {
		storageClass = "gp2"
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claimName,
			Namespace: spec.Namespace,
			Labels:    spec.Labels(),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}

	_, err := m.client.CoreV1().PersistentVolumeClaims(spec.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		m.logger.Info("workspace PVC already exists", "pvc", claimName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("creating PVC %s: %w", claimName, err)
	}
	m.logger.Info("created workspace PVC", "pvc", claimName, "size", size, "storageClass", storageClass)
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

	containers := []corev1.Container{container}

	// Add Coop sidecar if configured.
	if spec.CoopSidecar != nil {
		containers = append(containers, m.buildCoopSidecar(spec))
	}

	var initContainers []corev1.Container
	if ic := m.buildInitCloneContainer(spec); ic != nil {
		initContainers = append(initContainers, *ic)
	}

	podSpec := corev1.PodSpec{
		InitContainers: initContainers,
		Containers:     containers,
		Volumes:        volumes,
		RestartPolicy:  restartPolicyForRole(spec.Role),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser:    intPtr(AgentUID),
			RunAsGroup:   intPtr(AgentGID),
			RunAsNonRoot: boolPtr(true),
			FSGroup:      intPtr(AgentGID),
		},
	}

	// Coop needs shareProcessNamespace to observe the agent process.
	if spec.CoopSidecar != nil {
		podSpec.ShareProcessNamespace = boolPtr(true)
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

	c := corev1.Container{
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

	if spec.CoopBuiltin {
		// Agent image has coop built-in (entrypoint starts coop as PID 1).
		// Use HTTP probes against coop's health endpoint and expose ports.
		c.Ports = []corev1.ContainerPort{
			{Name: "api", ContainerPort: CoopDefaultPort},
			{Name: "health", ContainerPort: CoopDefaultHealthPort},
		}
		c.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/v1/health",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       15,
			FailureThreshold:    3,
		}
		c.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/v1/health",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		}
		c.StartupProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/v1/health",
					Port: intstr.FromString("health"),
				},
			},
			FailureThreshold: 60,
			PeriodSeconds:    5,
		}
	} else {
		// Fallback exec probes for images without coop.
		c.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "pgrep -f 'claude|node' > /dev/null"},
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       15,
			FailureThreshold:    3,
		}
		c.StartupProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "pgrep -f 'claude|node' > /dev/null"},
				},
			},
			FailureThreshold: 60,
			PeriodSeconds:    5,
		}
	}

	return c
}

// buildEnvVars constructs environment variables from plain values and secret references.
func (m *K8sManager) buildEnvVars(spec AgentPodSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{Name: "GT_ROLE", Value: spec.Role},
		{Name: "GT_RIG", Value: spec.Rig},
		{Name: "GT_AGENT", Value: spec.AgentName},
		{Name: "HOME", Value: "/home/agent"},
		// Session persistence: point XDG_STATE_HOME to the PVC so Claude
		// session logs and coop session artifacts survive pod restarts.
		{Name: "XDG_STATE_HOME", Value: MountStateDir},
	}

	// Enable session resume for persistent roles (those with workspace PVC).
	if spec.WorkspaceStorage != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "GT_SESSION_RESUME", Value: "1"})
	}

	// Add role-specific env vars.
	// All roles get BD_ACTOR and GIT_AUTHOR_NAME for beads attribution.
	switch spec.Role {
	case "polecat":
		envVars = append(envVars,
			corev1.EnvVar{Name: "GT_POLECAT", Value: spec.AgentName},
			corev1.EnvVar{Name: "GT_SCOPE", Value: "rig"},
			corev1.EnvVar{Name: "BD_ACTOR", Value: spec.AgentName},
			corev1.EnvVar{Name: "GIT_AUTHOR_NAME", Value: spec.AgentName},
		)
	case "crew":
		envVars = append(envVars,
			corev1.EnvVar{Name: "GT_CREW", Value: spec.AgentName},
			corev1.EnvVar{Name: "GT_SCOPE", Value: "rig"},
			corev1.EnvVar{Name: "BD_ACTOR", Value: spec.AgentName},
			corev1.EnvVar{Name: "GIT_AUTHOR_NAME", Value: spec.AgentName},
		)
	case "witness", "refinery":
		envVars = append(envVars,
			corev1.EnvVar{Name: "GT_SCOPE", Value: "rig"},
			corev1.EnvVar{Name: "BD_ACTOR", Value: spec.Role},
			corev1.EnvVar{Name: "GIT_AUTHOR_NAME", Value: spec.Role},
		)
	case "mayor":
		envVars = append(envVars,
			corev1.EnvVar{Name: "GT_SCOPE", Value: "town"},
			corev1.EnvVar{Name: "BD_ACTOR", Value: "mayor"},
			corev1.EnvVar{Name: "GIT_AUTHOR_NAME", Value: "mayor"},
		)
	case "deacon":
		envVars = append(envVars,
			corev1.EnvVar{Name: "GT_SCOPE", Value: "town"},
			corev1.EnvVar{Name: "BD_ACTOR", Value: "deacon"},
			corev1.EnvVar{Name: "GIT_AUTHOR_NAME", Value: "deacon"},
		)
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

	// Daemon token from secret for agent→daemon authentication.
	if spec.DaemonTokenSecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "BD_DAEMON_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: spec.DaemonTokenSecret},
					Key:                  "token",
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

	// Claude credentials volume: Secret mount for OAuth token.
	if spec.CredentialsSecret != "" {
		volumes = append(volumes, corev1.Volume{
			Name: VolumeClaudeCreds,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: spec.CredentialsSecret,
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

	// Claude credentials: mount secret to staging dir; entrypoint copies to PVC.
	if spec.CredentialsSecret != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      VolumeClaudeCreds,
			MountPath: MountClaudeCreds,
			ReadOnly:  true,
		})
	}

	return mounts
}

// buildCoopSidecar constructs the Coop sidecar container with health probes,
// NATS env vars, and auth token injection.
func (m *K8sManager) buildCoopSidecar(spec AgentPodSpec) corev1.Container {
	coop := spec.CoopSidecar

	port := coop.Port
	if port == 0 {
		port = CoopDefaultPort
	}
	healthPort := coop.HealthPort
	if healthPort == 0 {
		healthPort = CoopDefaultHealthPort
	}

	args := []string{
		"--agent=claude",
		fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--port-health=%d", healthPort),
		"--cols=200",
		"--rows=50",
		"--", "claude", "--dangerously-skip-permissions",
	}

	envVars := []corev1.EnvVar{}

	// Auth token from secret.
	if coop.AuthTokenSecret != "" {
		args = append([]string{fmt.Sprintf("--auth-token=$(%s)", "COOP_AUTH_TOKEN")}, args...)
		envVars = append(envVars, corev1.EnvVar{
			Name: "COOP_AUTH_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: coop.AuthTokenSecret},
					Key:                  "token",
				},
			},
		})
	}

	// NATS event bus integration.
	if coop.NatsURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "COOP_NATS_URL",
			Value: coop.NatsURL,
		})
	}
	if coop.NatsTokenSecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "COOP_NATS_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: coop.NatsTokenSecret},
					Key:                  "token",
				},
			},
		})
	}

	resources := m.buildCoopResources(coop)

	// Coop env: set XDG_STATE_HOME so coop writes session artifacts to the PVC.
	envVars = append(envVars, corev1.EnvVar{
		Name:  "XDG_STATE_HOME",
		Value: MountStateDir,
	})

	coopMounts := []corev1.VolumeMount{
		{Name: VolumeTmp, MountPath: "/tmp"},
	}
	// Give coop access to the workspace PVC so it can discover Claude session
	// logs in .state/claude/ for resume and write its own artifacts to .state/coop/.
	if spec.WorkspaceStorage != nil {
		coopMounts = append(coopMounts, corev1.VolumeMount{
			Name:      VolumeWorkspace,
			MountPath: MountWorkspace,
		})
	}

	return corev1.Container{
		Name:  CoopContainerName,
		Image: coop.Image,
		Args:  args,
		Env:   envVars,
		Ports: []corev1.ContainerPort{
			{Name: "api", ContainerPort: port},
			{Name: "health", ContainerPort: healthPort},
		},
		VolumeMounts: coopMounts,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/v1/health",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/v1/agent/state",
					Port: intstr.FromString("health"),
				},
			},
			InitialDelaySeconds: 3,
			PeriodSeconds:       5,
		},
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/api/v1/health",
					Port: intstr.FromString("health"),
				},
			},
			FailureThreshold: 30,
			PeriodSeconds:    2,
		},
		Resources:       resources,
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

// buildCoopResources returns resource requirements for the Coop sidecar.
func (m *K8sManager) buildCoopResources(coop *CoopSidecarSpec) corev1.ResourceRequirements {
	if coop.Resources != nil {
		return *coop.Resources
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(CoopDefaultCPURequest),
			corev1.ResourceMemory: resource.MustParse(CoopDefaultMemRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(CoopDefaultCPULimit),
			corev1.ResourceMemory: resource.MustParse(CoopDefaultMemLimit),
		},
	}
}

// restartPolicyForRole returns the appropriate restart policy.
// Polecats are one-shot (Never); all others restart on failure.
// roleNeedsCode returns true for roles that need a working copy of the rig's repo.
func roleNeedsCode(role string) bool {
	switch role {
	case "polecat", "crew", "refinery":
		return true
	default:
		return false
	}
}

// buildInitCloneContainer creates an init container that clones the rig's repo
// from the in-cluster git mirror. Returns nil if the role doesn't need code
// or no git mirror is configured.
func (m *K8sManager) buildInitCloneContainer(spec AgentPodSpec) *corev1.Container {
	if spec.GitMirrorService == "" || !roleNeedsCode(spec.Role) {
		return nil
	}

	branch := spec.GitDefaultBranch
	if branch == "" {
		branch = "main"
	}

	// Clone from git mirror into workspace/{rig}/work/, set origin to real URL.
	script := fmt.Sprintf(`set -e
WORK_DIR="%s/%s/work"
if [ -d "$WORK_DIR/.git" ]; then
  echo "Repo already cloned, fetching updates..."
  cd "$WORK_DIR"
  git fetch --all --prune
  git checkout %s
  git pull --ff-only || true
else
  echo "Cloning from mirror %s..."
  mkdir -p "$(dirname "$WORK_DIR")"
  git clone -b %s git://%s:%d/%s.git "$WORK_DIR"
  cd "$WORK_DIR"
fi
`, MountWorkspace, spec.Rig, branch, spec.GitMirrorService, branch, spec.GitMirrorService, GitDaemonPort, spec.Rig)

	// Set origin to actual GitHub URL for pushes.
	if spec.GitURL != "" {
		script += fmt.Sprintf(`git remote set-url origin %s
`, spec.GitURL)
	}

	// Configure git identity from agent env vars.
	script += fmt.Sprintf(`git config user.name "%s"
git config user.email "%s@gastown"
`, spec.AgentName, spec.AgentName)

	return &corev1.Container{
		Name:            InitCloneName,
		Image:           InitCloneImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c", script},
		VolumeMounts: []corev1.VolumeMount{
			{Name: VolumeWorkspace, MountPath: MountWorkspace},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	}
}

func restartPolicyForRole(role string) corev1.RestartPolicy {
	if role == "polecat" {
		return corev1.RestartPolicyNever
	}
	return corev1.RestartPolicyAlways
}

func intPtr(i int64) *int64   { return &i }
func boolPtr(b bool) *bool    { return &b }
