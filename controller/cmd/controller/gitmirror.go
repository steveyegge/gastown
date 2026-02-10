package main

import (
	"context"
	"fmt"
	"log/slog"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"github.com/steveyegge/gastown/controller/internal/config"
)

const (
	gitMirrorImage = "alpine/git:latest"
	gitDaemonPort  = 9418
)

// provisionGitMirrors ensures git-mirror Deployments exist for all rigs
// in the cache that have a GitURL. Creates Deployment + Service + PVC
// if they don't already exist.
func provisionGitMirrors(ctx context.Context, logger *slog.Logger, client kubernetes.Interface, cfg *config.Config) {
	for name, entry := range cfg.RigCache {
		if entry.GitURL == "" {
			continue
		}
		svcName := fmt.Sprintf("git-mirror-%s", name)

		// Check if deployment already exists.
		_, err := client.AppsV1().Deployments(cfg.Namespace).Get(ctx, svcName, metav1.GetOptions{})
		if err == nil {
			continue // already exists
		}
		if !errors.IsNotFound(err) {
			logger.Warn("failed to check git-mirror deployment", "rig", name, "error", err)
			continue
		}

		logger.Info("provisioning git-mirror for rig", "rig", name, "url", entry.GitURL)

		// Create PVC.
		if err := createGitMirrorPVC(ctx, client, cfg.Namespace, svcName); err != nil {
			logger.Warn("failed to create git-mirror PVC", "rig", name, "error", err)
			continue
		}

		// Create Deployment.
		if err := createGitMirrorDeployment(ctx, client, cfg.Namespace, svcName, name, entry.GitURL); err != nil {
			logger.Warn("failed to create git-mirror deployment", "rig", name, "error", err)
			continue
		}

		// Create Service.
		if err := createGitMirrorService(ctx, client, cfg.Namespace, svcName, name); err != nil {
			logger.Warn("failed to create git-mirror service", "rig", name, "error", err)
			continue
		}

		// Update cache with the service name.
		entry.GitMirrorSvc = svcName
		cfg.RigCache[name] = entry

		logger.Info("provisioned git-mirror", "rig", name, "service", svcName)
	}
}

func createGitMirrorPVC(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "git-mirror",
				"app.kubernetes.io/managed-by": "gastown-controller",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: strPtr("gp2"),
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("2Gi"),
				},
			},
		},
	}

	_, err := client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func createGitMirrorDeployment(ctx context.Context, client kubernetes.Interface, namespace, deployName, rigName, gitURL string) error {
	replicas := int32(1)
	uid := int64(65534)
	gid := int64(65534)

	cloneScript := fmt.Sprintf(`set -e
REPO_DIR="/data/%s.git"
if [ -d "$REPO_DIR/HEAD" ]; then
  echo "Repo already cloned, fetching updates..."
  cd "$REPO_DIR"
  git fetch --all --prune
else
  echo "Cloning %s ..."
  git clone --bare --mirror %s "$REPO_DIR"
fi
touch "$REPO_DIR/git-daemon-export-ok"
`, rigName, gitURL, gitURL)

	daemonScript := fmt.Sprintf(`git daemon \
  --base-path=/data \
  --export-all \
  --reuseaddr \
  --informative-errors \
  --verbose \
  --listen=0.0.0.0 \
  --port=%d &
INTERVAL=300
while true; do
  sleep "$INTERVAL"
  REPO_DIR="/data/%s.git"
  if [ -d "$REPO_DIR" ]; then
    echo "$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ) Fetching updates for %s..."
    cd "$REPO_DIR"
    git fetch --all --prune 2>&1 || echo "Fetch failed, will retry"
  fi
done
`, gitDaemonPort, rigName, rigName)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":  "git-mirror",
				"app.kubernetes.io/managed-by": "gastown-controller",
				"gastown.io/rig":               rigName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/component": "git-mirror",
					"gastown.io/rig":              rigName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/component": "git-mirror",
						"gastown.io/rig":              rigName,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtrHelper(true),
						RunAsUser:    &uid,
						FSGroup:      &gid,
					},
					InitContainers: []corev1.Container{
						{
							Name:            "clone",
							Image:           gitMirrorImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c", cloneScript},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "repo-data", MountPath: "/data"},
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
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "git-daemon",
							Image:           gitMirrorImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c", daemonScript},
							Ports: []corev1.ContainerPort{
								{Name: "git", ContainerPort: int32(gitDaemonPort), Protocol: corev1.ProtocolTCP},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromString("git")},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromString("git")},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       30,
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "repo-data", MountPath: "/data"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "repo-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: deployName,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := client.AppsV1().Deployments(namespace).Create(ctx, deploy, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func createGitMirrorService(ctx context.Context, client kubernetes.Interface, namespace, svcName, rigName string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":  "git-mirror",
				"app.kubernetes.io/managed-by": "gastown-controller",
				"gastown.io/rig":               rigName,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{Name: "git", Port: int32(gitDaemonPort), TargetPort: intstr.FromString("git"), Protocol: corev1.ProtocolTCP},
			},
			Selector: map[string]string{
				"app.kubernetes.io/component": "git-mirror",
				"gastown.io/rig":              rigName,
			},
		},
	}

	_, err := client.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func strPtr(s string) *string      { return &s }
func boolPtrHelper(b bool) *bool   { return &b }
