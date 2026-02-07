package podmanager

import (
	"context"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAgentPodSpec_PodName(t *testing.T) {
	spec := AgentPodSpec{Rig: "gastown", Role: "polecat", AgentName: "furiosa"}
	want := "gt-gastown-polecat-furiosa"
	if got := spec.PodName(); got != want {
		t.Errorf("PodName() = %q, want %q", got, want)
	}
}

func TestAgentPodSpec_Labels(t *testing.T) {
	spec := AgentPodSpec{Rig: "beads", Role: "crew", AgentName: "quartz"}
	labels := spec.Labels()

	checks := map[string]string{
		LabelApp:   LabelAppValue,
		LabelRig:   "beads",
		LabelRole:  "crew",
		LabelAgent: "quartz",
	}
	for k, want := range checks {
		if got := labels[k]; got != want {
			t.Errorf("Labels()[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestK8sManager_CreateAndGet(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())

	spec := AgentPodSpec{
		Rig:       "gastown",
		Role:      "polecat",
		AgentName: "furiosa",
		Image:     "gastown-agent:latest",
		Namespace: "gastown",
		Env:       map[string]string{"BD_DAEMON_HOST": "localhost"},
	}

	ctx := context.Background()

	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatalf("CreateAgentPod() error: %v", err)
	}

	pod, err := mgr.GetAgentPod(ctx, "gt-gastown-polecat-furiosa", "gastown")
	if err != nil {
		t.Fatalf("GetAgentPod() error: %v", err)
	}
	if pod.Name != "gt-gastown-polecat-furiosa" {
		t.Errorf("pod name = %q, want %q", pod.Name, "gt-gastown-polecat-furiosa")
	}
	if pod.Spec.RestartPolicy != "Never" {
		t.Errorf("polecat restart policy = %q, want Never", pod.Spec.RestartPolicy)
	}
}

func TestK8sManager_ListByLabels(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	for _, name := range []string{"a", "b"} {
		spec := AgentPodSpec{
			Rig: "gastown", Role: "polecat", AgentName: name,
			Image: "agent:latest", Namespace: "gastown",
		}
		if err := mgr.CreateAgentPod(ctx, spec); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	pods, err := mgr.ListAgentPods(ctx, "gastown", map[string]string{
		LabelRole: "polecat",
		LabelRig:  "gastown",
	})
	if err != nil {
		t.Fatalf("ListAgentPods() error: %v", err)
	}
	if len(pods) != 2 {
		t.Errorf("got %d pods, want 2", len(pods))
	}
}

func TestK8sManager_Delete(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "colonization",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	if err := mgr.DeleteAgentPod(ctx, "gt-gastown-crew-colonization", "gastown"); err != nil {
		t.Fatalf("DeleteAgentPod() error: %v", err)
	}

	_, err := mgr.GetAgentPod(ctx, "gt-gastown-crew-colonization", "gastown")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestRestartPolicyForRole(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"polecat", "Never"},
		{"crew", "Always"},
		{"witness", "Always"},
		{"refinery", "Always"},
		{"mayor", "Always"},
		{"deacon", "Always"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := restartPolicyForRole(tt.role)
			if string(got) != tt.want {
				t.Errorf("restartPolicyForRole(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}

func TestK8sManager_CreateSetsEnvVars(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "witness", AgentName: "w1",
		Image: "agent:latest", Namespace: "gastown",
		Env: map[string]string{"BD_DAEMON_HOST": "daemon.svc"},
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-witness-w1", metav1.GetOptions{})
	envMap := make(map[string]string)
	for _, e := range pod.Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}

	required := []string{"GT_ROLE", "GT_RIG", "GT_AGENT", "BD_DAEMON_HOST", "HOME"}
	for _, key := range required {
		if _, ok := envMap[key]; !ok {
			t.Errorf("missing env var %s", key)
		}
	}
}

func TestK8sManager_PolecatEnvVars(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "valkyrie",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-polecat-valkyrie", metav1.GetOptions{})
	envMap := make(map[string]string)
	for _, e := range pod.Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}

	if envMap["GT_POLECAT"] != "valkyrie" {
		t.Errorf("GT_POLECAT = %q, want %q", envMap["GT_POLECAT"], "valkyrie")
	}
}

func TestK8sManager_CrewEnvVars(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "colonization",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-crew-colonization", metav1.GetOptions{})
	envMap := make(map[string]string)
	for _, e := range pod.Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}

	if envMap["GT_CREW"] != "colonization" {
		t.Errorf("GT_CREW = %q, want %q", envMap["GT_CREW"], "colonization")
	}
}

func TestK8sManager_SecretEnvVars(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "furiosa",
		Image: "agent:latest", Namespace: "gastown",
		SecretEnv: []SecretEnvSource{
			{EnvName: "ANTHROPIC_API_KEY", SecretName: "api-keys", SecretKey: "anthropic"},
			{EnvName: "GIT_TOKEN", SecretName: "git-creds", SecretKey: "token"},
		},
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-polecat-furiosa", metav1.GetOptions{})

	// Find secret env vars.
	secretEnvs := make(map[string]*corev1.SecretKeySelector)
	for _, e := range pod.Spec.Containers[0].Env {
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			secretEnvs[e.Name] = e.ValueFrom.SecretKeyRef
		}
	}

	if ref, ok := secretEnvs["ANTHROPIC_API_KEY"]; !ok {
		t.Error("missing ANTHROPIC_API_KEY secret env var")
	} else {
		if ref.Name != "api-keys" {
			t.Errorf("ANTHROPIC_API_KEY secret name = %q, want %q", ref.Name, "api-keys")
		}
		if ref.Key != "anthropic" {
			t.Errorf("ANTHROPIC_API_KEY secret key = %q, want %q", ref.Key, "anthropic")
		}
	}

	if _, ok := secretEnvs["GIT_TOKEN"]; !ok {
		t.Error("missing GIT_TOKEN secret env var")
	}
}

func TestK8sManager_SecurityContext(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "test",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-crew-test", metav1.GetOptions{})

	// Pod security context.
	psc := pod.Spec.SecurityContext
	if psc == nil {
		t.Fatal("pod security context is nil")
	}
	if *psc.RunAsUser != AgentUID {
		t.Errorf("RunAsUser = %d, want %d", *psc.RunAsUser, AgentUID)
	}
	if *psc.RunAsGroup != AgentGID {
		t.Errorf("RunAsGroup = %d, want %d", *psc.RunAsGroup, AgentGID)
	}
	if !*psc.RunAsNonRoot {
		t.Error("RunAsNonRoot should be true")
	}

	// Container security context.
	csc := pod.Spec.Containers[0].SecurityContext
	if csc == nil {
		t.Fatal("container security context is nil")
	}
	if *csc.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation should be false")
	}
	if csc.Capabilities == nil || len(csc.Capabilities.Drop) == 0 {
		t.Error("should drop ALL capabilities")
	}
}

func TestK8sManager_DefaultResources(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "test",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-polecat-test", metav1.GetOptions{})
	resources := pod.Spec.Containers[0].Resources

	cpuReq := resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != DefaultCPURequest {
		t.Errorf("CPU request = %s, want %s", cpuReq.String(), DefaultCPURequest)
	}

	memLimit := resources.Limits[corev1.ResourceMemory]
	if memLimit.String() != DefaultMemoryLimit {
		t.Errorf("Memory limit = %s, want %s", memLimit.String(), DefaultMemoryLimit)
	}
}

func TestK8sManager_CustomResources(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	customResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "heavy",
		Image: "agent:latest", Namespace: "gastown",
		Resources: customResources,
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-crew-heavy", metav1.GetOptions{})
	resources := pod.Spec.Containers[0].Resources

	cpuReq := resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "1" {
		t.Errorf("CPU request = %s, want %s", cpuReq.String(), "1")
	}

	memLimit := resources.Limits[corev1.ResourceMemory]
	if memLimit.String() != "8Gi" {
		t.Errorf("Memory limit = %s, want %s", memLimit.String(), "8Gi")
	}
}

func TestK8sManager_VolumesEmptyDir(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "ephemeral",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-polecat-ephemeral", metav1.GetOptions{})

	volMap := make(map[string]corev1.Volume)
	for _, v := range pod.Spec.Volumes {
		volMap[v.Name] = v
	}

	// Workspace should be EmptyDir for polecats.
	ws, ok := volMap[VolumeWorkspace]
	if !ok {
		t.Fatal("missing workspace volume")
	}
	if ws.EmptyDir == nil {
		t.Error("workspace should be EmptyDir for polecat")
	}

	// Tmp should always be EmptyDir.
	tmp, ok := volMap[VolumeTmp]
	if !ok {
		t.Fatal("missing tmp volume")
	}
	if tmp.EmptyDir == nil {
		t.Error("tmp should be EmptyDir")
	}
}

func TestK8sManager_VolumesPVC(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "persistent",
		Image: "agent:latest", Namespace: "gastown",
		WorkspaceStorage: &WorkspaceStorageSpec{
			ClaimName:        "my-pvc",
			Size:             "20Gi",
			StorageClassName: "gp3",
		},
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-crew-persistent", metav1.GetOptions{})

	volMap := make(map[string]corev1.Volume)
	for _, v := range pod.Spec.Volumes {
		volMap[v.Name] = v
	}

	ws, ok := volMap[VolumeWorkspace]
	if !ok {
		t.Fatal("missing workspace volume")
	}
	if ws.PersistentVolumeClaim == nil {
		t.Fatal("workspace should be PVC for crew with WorkspaceStorage")
	}
	if ws.PersistentVolumeClaim.ClaimName != "my-pvc" {
		t.Errorf("PVC claim name = %q, want %q", ws.PersistentVolumeClaim.ClaimName, "my-pvc")
	}
}

func TestK8sManager_VolumesPVCDefaultClaimName(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "jane",
		Image: "agent:latest", Namespace: "gastown",
		WorkspaceStorage: &WorkspaceStorageSpec{Size: "10Gi"},
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-crew-jane", metav1.GetOptions{})

	for _, v := range pod.Spec.Volumes {
		if v.Name == VolumeWorkspace && v.PersistentVolumeClaim != nil {
			want := "gt-gastown-crew-jane-workspace"
			if v.PersistentVolumeClaim.ClaimName != want {
				t.Errorf("default PVC claim name = %q, want %q", v.PersistentVolumeClaim.ClaimName, want)
			}
			return
		}
	}
	t.Error("workspace PVC volume not found")
}

func TestK8sManager_ConfigMapMount(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "witness", AgentName: "w1",
		Image: "agent:latest", Namespace: "gastown",
		ConfigMapName: "agent-config",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-witness-w1", metav1.GetOptions{})

	// Check volume.
	found := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == VolumeBeadsConfig && v.ConfigMap != nil {
			if v.ConfigMap.Name != "agent-config" {
				t.Errorf("ConfigMap name = %q, want %q", v.ConfigMap.Name, "agent-config")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("beads-config volume not found")
	}

	// Check mount.
	mountFound := false
	for _, m := range pod.Spec.Containers[0].VolumeMounts {
		if m.Name == VolumeBeadsConfig {
			if m.MountPath != MountBeadsConfig {
				t.Errorf("ConfigMap mount path = %q, want %q", m.MountPath, MountBeadsConfig)
			}
			if !m.ReadOnly {
				t.Error("ConfigMap mount should be read-only")
			}
			mountFound = true
			break
		}
	}
	if !mountFound {
		t.Error("beads-config volume mount not found")
	}
}

func TestK8sManager_ServiceAccount(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "refinery", AgentName: "r1",
		Image: "agent:latest", Namespace: "gastown",
		ServiceAccountName: "gastown-agent",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-refinery-r1", metav1.GetOptions{})
	if pod.Spec.ServiceAccountName != "gastown-agent" {
		t.Errorf("ServiceAccountName = %q, want %q", pod.Spec.ServiceAccountName, "gastown-agent")
	}
}

func TestK8sManager_NodeSelectorAndTolerations(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "heavy",
		Image: "agent:latest", Namespace: "gastown",
		NodeSelector: map[string]string{"node-type": "agent"},
		Tolerations: []corev1.Toleration{
			{Key: "dedicated", Value: "agents", Effect: corev1.TaintEffectNoSchedule},
		},
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-crew-heavy", metav1.GetOptions{})

	if pod.Spec.NodeSelector["node-type"] != "agent" {
		t.Errorf("NodeSelector[node-type] = %q, want %q", pod.Spec.NodeSelector["node-type"], "agent")
	}

	if len(pod.Spec.Tolerations) != 1 {
		t.Fatalf("got %d tolerations, want 1", len(pod.Spec.Tolerations))
	}
	if pod.Spec.Tolerations[0].Key != "dedicated" {
		t.Errorf("toleration key = %q, want %q", pod.Spec.Tolerations[0].Key, "dedicated")
	}
}

func TestK8sManager_TerminationGracePeriod(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "test",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-polecat-test", metav1.GetOptions{})
	if pod.Spec.TerminationGracePeriodSeconds == nil {
		t.Fatal("TerminationGracePeriodSeconds is nil")
	}
	if *pod.Spec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("TerminationGracePeriodSeconds = %d, want 30", *pod.Spec.TerminationGracePeriodSeconds)
	}
}

func TestK8sManager_ImagePullPolicy(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "crew", AgentName: "test",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-crew-test", metav1.GetOptions{})
	if pod.Spec.Containers[0].ImagePullPolicy != corev1.PullAlways {
		t.Errorf("ImagePullPolicy = %q, want %q", pod.Spec.Containers[0].ImagePullPolicy, corev1.PullAlways)
	}
}

func TestK8sManager_VolumeMountPaths(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := New(client, slog.Default())
	ctx := context.Background()

	spec := AgentPodSpec{
		Rig: "gastown", Role: "polecat", AgentName: "test",
		Image: "agent:latest", Namespace: "gastown",
	}
	if err := mgr.CreateAgentPod(ctx, spec); err != nil {
		t.Fatal(err)
	}

	pod, _ := client.CoreV1().Pods("gastown").Get(ctx, "gt-gastown-polecat-test", metav1.GetOptions{})
	mountMap := make(map[string]string)
	for _, m := range pod.Spec.Containers[0].VolumeMounts {
		mountMap[m.Name] = m.MountPath
	}

	if mountMap[VolumeWorkspace] != MountWorkspace {
		t.Errorf("workspace mount = %q, want %q", mountMap[VolumeWorkspace], MountWorkspace)
	}
	if mountMap[VolumeTmp] != MountTmp {
		t.Errorf("tmp mount = %q, want %q", mountMap[VolumeTmp], MountTmp)
	}
}
