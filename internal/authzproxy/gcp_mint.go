package authzproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2/google"
)

// MintGCPTokenFromProfile mints a short-lived GCP token using the given profile.
// For downscope mode: exchanges the caller's ADC token for a restricted token via STS.
// For impersonate mode: uses generateAccessToken on the target SA.
func MintGCPTokenFromProfile(profile GCPProfile) (token string, expiry time.Time, err error) {
	mode := profile.Mode
	if mode == "" {
		mode = "impersonate"
	}

	switch mode {
	case "downscope":
		return mintDownscope(profile)
	case "impersonate":
		return mintImpersonate(profile)
	default:
		return "", time.Time{}, fmt.Errorf("unknown GCP profile mode: %s", mode)
	}
}

func mintDownscope(profile GCPProfile) (string, time.Time, error) {
	// Get source ADC token
	ctx := context.Background()

	creds, err := google.FindDefaultCredentials(ctx, profile.Scopes...)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("finding default credentials: %w", err)
	}
	srcToken, err := creds.TokenSource.Token()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("getting ADC token: %w", err)
	}

	// Build CAB access boundary rules
	type accessBoundaryRule struct {
		AvailablePermissions []string `json:"availablePermissions"`
		AvailableResource    string   `json:"availableResource"`
	}
	type accessBoundary struct {
		AccessBoundaryRules []accessBoundaryRule `json:"accessBoundaryRules"`
	}

	scopeToRole := map[string]string{
		"https://www.googleapis.com/auth/compute.readonly":        "inRole:roles/compute.viewer",
		"https://www.googleapis.com/auth/devstorage.read_only":    "inRole:roles/storage.objectViewer",
		"https://www.googleapis.com/auth/cloud-platform.read-only": "inRole:roles/viewer",
	}

	var rules []accessBoundaryRule
	project := profile.Project
	for _, scope := range profile.Scopes {
		role, ok := scopeToRole[scope]
		if ok && project != "" {
			rules = append(rules, accessBoundaryRule{
				AvailablePermissions: []string{role},
				AvailableResource:    fmt.Sprintf("//cloudresourcemanager.googleapis.com/projects/%s", project),
			})
		}
	}

	if len(rules) == 0 {
		// No mappable rules — return source token
		return srcToken.AccessToken, srcToken.Expiry, nil
	}

	boundary := struct {
		AccessBoundary accessBoundary `json:"accessBoundary"`
	}{
		AccessBoundary: accessBoundary{AccessBoundaryRules: rules},
	}
	boundaryJSON, _ := json.Marshal(boundary)

	// Exchange via STS
	resp, err := http.PostForm("https://sts.googleapis.com/v1/token", url.Values{
		"grant_type":           {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token_type":   {"urn:ietf:params:oauth:token-type:access_token"},
		"requested_token_type": {"urn:ietf:params:oauth:token-type:access_token"},
		"subject_token":        {srcToken.AccessToken},
		"options":              {string(boundaryJSON)},
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("STS token exchange: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("STS error %d: %s", resp.StatusCode, string(body))
	}

	var stsResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stsResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parsing STS response: %w", err)
	}

	expiry := time.Now().Add(time.Duration(stsResp.ExpiresIn) * time.Second)
	return stsResp.AccessToken, expiry, nil
}

func mintImpersonate(profile GCPProfile) (string, time.Time, error) {
	if profile.TargetSA == "" {
		return "", time.Time{}, fmt.Errorf("target_sa required for impersonate mode")
	}

	creds, err := google.FindDefaultCredentials(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", time.Time{}, fmt.Errorf("finding default credentials: %w", err)
	}
	srcToken, err := creds.TokenSource.Token()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("getting ADC token: %w", err)
	}

	lifetime := profile.Lifetime
	if lifetime == "" {
		lifetime = "3600s"
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"scope":    profile.Scopes,
		"lifetime": lifetime,
	})

	url := fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken", profile.TargetSA)
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Set("Authorization", "Bearer "+srcToken.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(newBytesReader(reqBody))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("IAM API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("IAM API %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"accessToken"`
		ExpireTime  string `json:"expireTime"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parsing token response: %w", err)
	}

	expiry, err := time.Parse(time.RFC3339, tokenResp.ExpireTime)
	if err != nil {
		expiry = time.Now().Add(time.Hour)
	}

	return tokenResp.AccessToken, expiry, nil
}

type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader { return &bytesReader{data: data} }

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
