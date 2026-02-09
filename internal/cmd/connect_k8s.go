package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// discoverK8sDaemon finds the daemon URL in a Kubernetes namespace using kubectl.
func discoverK8sDaemon(namespace, kubeContext string) (string, error) {
	// Find daemon service by label.
	svcArgs := []string{"get", "svc", "-n", namespace, "-l", "app.kubernetes.io/component=daemon", "-o", "json"}
	if kubeContext != "" {
		svcArgs = append(svcArgs, "--context", kubeContext)
	}
	svcOut, err := exec.Command("kubectl", svcArgs...).Output()
	if err != nil {
		return "", fmt.Errorf("kubectl get svc failed: %w", err)
	}

	var svcList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				ClusterIP string `json:"clusterIP"`
				Ports     []struct {
					Name string `json:"name"`
					Port int    `json:"port"`
				} `json:"ports"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(svcOut, &svcList); err != nil {
		return "", fmt.Errorf("parsing service JSON: %w", err)
	}
	if len(svcList.Items) == 0 {
		return "", fmt.Errorf("no daemon service found in namespace %s", namespace)
	}
	svc := svcList.Items[0]

	// Check for Traefik IngressRoute to get a public hostname.
	irArgs := []string{"get", "ingressroutes.traefik.io", "-n", namespace, "-l", "app.kubernetes.io/component=daemon", "-o", "json"}
	if kubeContext != "" {
		irArgs = append(irArgs, "--context", kubeContext)
	}
	irOut, err := exec.Command("kubectl", irArgs...).Output()
	if err == nil {
		var irList struct {
			Items []struct {
				Spec struct {
					Routes []struct {
						Match string `json:"match"`
					} `json:"routes"`
				} `json:"spec"`
			} `json:"items"`
		}
		if err := json.Unmarshal(irOut, &irList); err == nil && len(irList.Items) > 0 {
			for _, ir := range irList.Items {
				for _, route := range ir.Spec.Routes {
					host := extractHostFromMatch(route.Match)
					if host != "" {
						return "https://" + host, nil
					}
				}
			}
		}
	}

	// No IngressRoute found — fall back to ClusterIP with HTTP port.
	httpPort := 9080
	for _, p := range svc.Spec.Ports {
		if p.Name == "http" {
			httpPort = p.Port
			break
		}
	}

	return "", fmt.Errorf("no IngressRoute found, daemon %s is ClusterIP only (%s:%d) - use --port-forward or --url",
		svc.Metadata.Name, svc.Spec.ClusterIP, httpPort)
}

// extractHostFromMatch parses a Traefik Host(`...`) match rule and returns the hostname.
func extractHostFromMatch(match string) string {
	// Match rules look like: Host(`gastown-uat.app.e2e.dev.fics.ai`)
	const prefix = "Host(`"
	idx := strings.Index(match, prefix)
	if idx < 0 {
		return ""
	}
	rest := match[idx+len(prefix):]
	end := strings.Index(rest, "`)")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// extractK8sToken retrieves the daemon auth token from a Kubernetes secret.
func extractK8sToken(namespace, kubeContext string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace is required for K8s token extraction")
	}

	args := []string{"get", "secrets", "-n", namespace, "-l", "app.kubernetes.io/component=daemon", "-o", "json"}
	if kubeContext != "" {
		args = append(args, "--context", kubeContext)
	}
	out, err := exec.Command("kubectl", args...).Output()
	if err != nil {
		return "", fmt.Errorf("kubectl get secrets failed: %w", err)
	}

	var secretList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Data map[string]string `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &secretList); err != nil {
		return "", fmt.Errorf("parsing secrets JSON: %w", err)
	}

	// Find the secret whose name ends with "-daemon-token".
	for _, s := range secretList.Items {
		if !strings.HasSuffix(s.Metadata.Name, "-daemon-token") {
			continue
		}
		encoded, ok := s.Data["token"]
		if !ok {
			return "", fmt.Errorf("secret %s has no 'token' key", s.Metadata.Name)
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", fmt.Errorf("decoding token from secret %s: %w", s.Metadata.Name, err)
		}
		return string(decoded), nil
	}

	return "", fmt.Errorf("daemon token secret not found in namespace %s", namespace)
}
