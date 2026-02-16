package cmd

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	gov "github.com/steveyegge/gastown/internal/governance"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	governanceUnfreezeArtifactPath string
	governanceUnfreezeAttestation  string
	governanceNow                  = time.Now
)

var governanceCmd = &cobra.Command{
	Use:     "governance",
	GroupID: GroupDiag,
	Short:   "Manage governance control-plane operations",
	Long: `Manage governance controls for promotion lanes.

Governance operations are fail-closed and require explicit artifacts when
changing system mode.`,
	RunE: requireSubcommand,
}

var governanceUnfreezeCmd = &cobra.Command{
	Use:   "unfreeze",
	Short: "Transition governance mode from ANCHOR_FREEZE to NORMAL",
	Long: `Unfreeze promotion lanes after anchor-health recovery.

This command requires:
  - active ANCHOR_FREEZE mode
  - matching freeze artifact hash
  - valid signed attestation from an independent domain
  - anchor health revalidation at unfreeze time`,
	RunE: runGovernanceUnfreeze,
}

type governanceAttestation struct {
	Version      int    `json:"version,omitempty"`
	ArtifactHash string `json:"artifact_hash"`
	SystemMode   string `json:"system_mode,omitempty"`
	Domain       string `json:"domain"`
	Signer       string `json:"signer"`
	IssuedAt     string `json:"issued_at"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	SignatureAlg string `json:"signature_alg,omitempty"`
	Signature    string `json:"signature"`
}

func init() {
	governanceUnfreezeCmd.Flags().StringVar(&governanceUnfreezeArtifactPath, "artifact", "", "Freeze artifact hash from the active ANCHOR_FREEZE event")
	governanceUnfreezeCmd.Flags().StringVar(&governanceUnfreezeAttestation, "attestation", "", "Path to signed attestation JSON blob")
	_ = governanceUnfreezeCmd.MarkFlagRequired("artifact")
	_ = governanceUnfreezeCmd.MarkFlagRequired("attestation")

	governanceCmd.AddCommand(governanceUnfreezeCmd)
	rootCmd.AddCommand(governanceCmd)
}

func runGovernanceUnfreeze(cmd *cobra.Command, args []string) error {
	artifactHash := strings.TrimSpace(governanceUnfreezeArtifactPath)
	if artifactHash == "" {
		return fmt.Errorf("--artifact is required")
	}

	attestationPath := strings.TrimSpace(governanceUnfreezeAttestation)
	if attestationPath == "" {
		return fmt.Errorf("--attestation is required")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	controller := gov.NewController(townRoot, gov.ThresholdsFromEnv())
	state, err := controller.LoadState()
	if err != nil {
		return fmt.Errorf("loading governance state: %w", err)
	}
	if state.SystemMode != gov.SystemModeAnchorFreeze {
		return fmt.Errorf("system mode is %s, not ANCHOR_FREEZE", state.SystemMode)
	}
	if state.Freeze == nil {
		return fmt.Errorf("freeze state missing")
	}

	activeHash := strings.TrimSpace(state.Freeze.ArtifactHash)
	if activeHash == "" {
		return fmt.Errorf("freeze artifact hash linkage missing")
	}
	if !strings.EqualFold(artifactHash, activeHash) {
		return fmt.Errorf("artifact hash mismatch: expected %s", activeHash)
	}

	artifactID := strings.TrimSpace(state.Freeze.ArtifactID)
	if artifactID == "" {
		return fmt.Errorf("freeze artifact ID linkage missing")
	}

	attestationRef, err := validateGovernanceUnfreezeAttestation(attestationPath, activeHash, governanceNow().UTC())
	if err != nil {
		return err
	}

	if err := controller.Unfreeze(artifactID, attestationRef); err != nil {
		return err
	}

	fmt.Printf("%s Governance mode set to NORMAL (artifact=%s)\n", style.Success.Render("✓"), shortGovernanceHash(activeHash))
	return nil
}

func validateGovernanceUnfreezeAttestation(path, expectedArtifactHash string, now time.Time) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("attestation path is required")
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is user-provided CLI input
	if err != nil {
		return "", fmt.Errorf("reading attestation file: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return "", fmt.Errorf("attestation file is empty")
	}

	var att governanceAttestation
	if err := json.Unmarshal(data, &att); err != nil {
		return "", fmt.Errorf("parsing attestation JSON: %w", err)
	}

	domain := strings.ToLower(strings.TrimSpace(att.Domain))
	if domain == "" {
		return "", fmt.Errorf("attestation domain is required")
	}
	if localDomain := strings.ToLower(strings.TrimSpace(os.Getenv("GT_GOVERNANCE_DOMAIN"))); localDomain != "" && localDomain == domain {
		return "", fmt.Errorf("attestation domain %q must differ from governance domain %q", domain, localDomain)
	}

	signer := strings.TrimSpace(att.Signer)
	if signer == "" {
		return "", fmt.Errorf("attestation signer is required")
	}

	issuedAt := strings.TrimSpace(att.IssuedAt)
	if issuedAt == "" {
		return "", fmt.Errorf("attestation issued_at is required")
	}
	issuedAtTime, err := time.Parse(time.RFC3339, issuedAt)
	if err != nil {
		return "", fmt.Errorf("attestation issued_at must be RFC3339: %w", err)
	}

	if expiresAt := strings.TrimSpace(att.ExpiresAt); expiresAt != "" {
		expiresAtTime, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return "", fmt.Errorf("attestation expires_at must be RFC3339: %w", err)
		}
		if !now.Before(expiresAtTime) {
			return "", fmt.Errorf("attestation expired at %s", expiresAtTime.UTC().Format(time.RFC3339))
		}
	}

	if attHash := strings.TrimSpace(att.ArtifactHash); attHash == "" {
		return "", fmt.Errorf("attestation artifact_hash is required")
	} else if !strings.EqualFold(attHash, strings.TrimSpace(expectedArtifactHash)) {
		return "", fmt.Errorf("attestation artifact_hash mismatch: expected %s", strings.TrimSpace(expectedArtifactHash))
	}

	if mode := strings.TrimSpace(att.SystemMode); mode != "" && !strings.EqualFold(mode, string(gov.SystemModeAnchorFreeze)) {
		return "", fmt.Errorf("attestation system_mode must be %s", gov.SystemModeAnchorFreeze)
	}

	alg := strings.ToLower(strings.TrimSpace(att.SignatureAlg))
	if alg != "" && alg != "ed25519" {
		return "", fmt.Errorf("unsupported attestation signature_alg: %s", att.SignatureAlg)
	}
	if strings.TrimSpace(att.Signature) == "" {
		return "", fmt.Errorf("attestation signature is required")
	}

	keyring, err := loadGovernanceAttestationKeyring()
	if err != nil {
		return "", err
	}
	pub, ok := keyring[domain]
	if !ok {
		return "", fmt.Errorf("no attestation key configured for domain %q (configured: %s)", domain, strings.Join(sortedGovernanceDomains(keyring), ", "))
	}

	sig, err := decodeGovernanceBase64(strings.TrimSpace(att.Signature))
	if err != nil {
		return "", fmt.Errorf("decoding attestation signature: %w", err)
	}

	clone := att
	clone.Signature = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("marshaling attestation payload: %w", err)
	}

	if !ed25519.Verify(pub, payload, sig) {
		return "", fmt.Errorf("attestation signature verification failed")
	}

	sum := sha256.Sum256(data)
	return fmt.Sprintf("%s/%s/%s/%s",
		domain,
		signer,
		issuedAtTime.UTC().Format(time.RFC3339),
		hex.EncodeToString(sum[:8]),
	), nil
}

func loadGovernanceAttestationKeyring() (map[string]ed25519.PublicKey, error) {
	raw := strings.TrimSpace(os.Getenv("GT_GOVERNANCE_ATTESTATION_PUBKEYS"))
	if raw == "" {
		return nil, fmt.Errorf("GT_GOVERNANCE_ATTESTATION_PUBKEYS is required")
	}

	keyring := make(map[string]ed25519.PublicKey)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		domain, key := splitGovernanceKeyringEntry(entry)
		if domain == "" || key == "" {
			return nil, fmt.Errorf("invalid attestation key entry %q (expected domain=key)", entry)
		}
		pub, err := decodeGovernancePublicKey(key)
		if err != nil {
			return nil, fmt.Errorf("decoding attestation key for %q: %w", domain, err)
		}
		keyring[strings.ToLower(domain)] = pub
	}
	if len(keyring) == 0 {
		return nil, fmt.Errorf("GT_GOVERNANCE_ATTESTATION_PUBKEYS has no valid keys")
	}
	return keyring, nil
}

func splitGovernanceKeyringEntry(entry string) (string, string) {
	if idx := strings.Index(entry, "="); idx > 0 {
		return strings.TrimSpace(entry[:idx]), strings.TrimSpace(entry[idx+1:])
	}
	if idx := strings.Index(entry, ":"); idx > 0 {
		return strings.TrimSpace(entry[:idx]), strings.TrimSpace(entry[idx+1:])
	}
	return "", ""
}

func decodeGovernancePublicKey(raw string) (ed25519.PublicKey, error) {
	decoded, err := decodeGovernanceBase64(raw)
	if err != nil {
		decoded, err = hex.DecodeString(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("expected base64 or hex")
		}
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key length %d != %d", len(decoded), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(decoded), nil
}

func decodeGovernanceBase64(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("empty input")
	}

	candidates := []string{
		value,
		strings.TrimRight(value, "="),
	}
	var firstErr error
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if out, err := base64.StdEncoding.DecodeString(candidate); err == nil {
			return out, nil
		} else if firstErr == nil {
			firstErr = err
		}
		if out, err := base64.RawStdEncoding.DecodeString(candidate); err == nil {
			return out, nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("invalid base64")
}

func sortedGovernanceDomains(keyring map[string]ed25519.PublicKey) []string {
	domains := make([]string, 0, len(keyring))
	for domain := range keyring {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

func shortGovernanceHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}
