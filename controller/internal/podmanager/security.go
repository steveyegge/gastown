package podmanager

import (
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ResourceCaps holds maximum resource limits for sidecars.
// Parsed from controller config (SidecarMaxCPU, SidecarMaxMemory).
type ResourceCaps struct {
	MaxCPU    resource.Quantity
	MaxMemory resource.Quantity
}

// ParseResourceCaps parses string-based resource caps into quantities.
// Empty strings result in zero-value quantities (no cap enforced).
func ParseResourceCaps(maxCPU, maxMemory string) ResourceCaps {
	caps := ResourceCaps{}
	if maxCPU != "" {
		caps.MaxCPU = resource.MustParse(maxCPU)
	}
	if maxMemory != "" {
		caps.MaxMemory = resource.MustParse(maxMemory)
	}
	return caps
}

// ClampResources enforces resource caps on the given requirements.
// If a limit exceeds the cap, it is clamped down and a warning is logged.
// Requests are also clamped if they exceed the cap (requests should never exceed limits).
// Zero-value caps are treated as "no cap".
func ClampResources(reqs corev1.ResourceRequirements, caps ResourceCaps, logger *slog.Logger) corev1.ResourceRequirements {
	if caps.MaxCPU.IsZero() && caps.MaxMemory.IsZero() {
		return reqs
	}

	result := reqs.DeepCopy()

	if !caps.MaxCPU.IsZero() {
		clampQuantity(result.Limits, corev1.ResourceCPU, caps.MaxCPU, "cpu limit", logger)
		clampQuantity(result.Requests, corev1.ResourceCPU, caps.MaxCPU, "cpu request", logger)
	}

	if !caps.MaxMemory.IsZero() {
		clampQuantity(result.Limits, corev1.ResourceMemory, caps.MaxMemory, "memory limit", logger)
		clampQuantity(result.Requests, corev1.ResourceMemory, caps.MaxMemory, "memory request", logger)
	}

	return *result
}

// SidecarPullPolicy returns the appropriate image pull policy for a sidecar.
// Custom images (no profile) always pull to pick up agent-pushed tags.
// Profile images with :latest or no tag always pull. Pinned tags use IfNotPresent.
func SidecarPullPolicy(tc *ToolchainSidecarSpec) corev1.PullPolicy {
	if tc.Profile == "" {
		return corev1.PullAlways
	}
	if hasLatestOrNoTag(tc.Image) {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}

// hasLatestOrNoTag returns true if the image ref has no tag, has :latest,
// or uses a digest (sha256:...) — all cases where we want PullAlways
// except digests which are immutable and use IfNotPresent.
func hasLatestOrNoTag(image string) bool {
	// Strip registry prefix to find tag portion.
	// Image refs: "registry/repo:tag", "registry/repo@sha256:...", "registry/repo"
	if strings.Contains(image, "@") {
		// Digest reference — immutable, no need to always pull.
		return false
	}
	// Find tag after last colon that's not part of a port (port is before /).
	lastSlash := strings.LastIndex(image, "/")
	tagPart := image
	if lastSlash >= 0 {
		tagPart = image[lastSlash:]
	}
	colonIdx := strings.LastIndex(tagPart, ":")
	if colonIdx < 0 {
		// No tag at all — treated as :latest by container runtime.
		return true
	}
	tag := tagPart[colonIdx+1:]
	return tag == "latest"
}

// clampQuantity clamps a single resource in a resource list to the given max.
func clampQuantity(list corev1.ResourceList, name corev1.ResourceName, max resource.Quantity, label string, logger *slog.Logger) {
	if list == nil {
		return
	}
	val, ok := list[name]
	if !ok {
		return
	}
	if val.Cmp(max) > 0 {
		logger.Warn("clamping sidecar resource to cap",
			"resource", label,
			"requested", val.String(),
			"cap", max.String(),
		)
		list[name] = max.DeepCopy()
	}
}
