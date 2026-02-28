package quota

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// UsageInfo holds quota utilization data from the Claude usage API.
type UsageInfo struct {
	FiveHour *UsageWindow `json:"five_hour,omitempty"`
	SevenDay *UsageWindow `json:"seven_day,omitempty"`
}

// UsageWindow represents a single rate-limit window (5h or 7d).
type UsageWindow struct {
	Utilization float64 `json:"utilization"`          // 0-100 percentage
	ResetsAt    string  `json:"resets_at,omitempty"`  // ISO 8601 timestamp
}

// MaxUtilization returns the highest utilization across all windows.
func (u *UsageInfo) MaxUtilization() float64 {
	max := 0.0
	if u.FiveHour != nil && u.FiveHour.Utilization > max {
		max = u.FiveHour.Utilization
	}
	if u.SevenDay != nil && u.SevenDay.Utilization > max {
		max = u.SevenDay.Utilization
	}
	return max
}

// UsageChecker fetches quota utilization for an account.
type UsageChecker interface {
	FetchUsage(orgID, sessionCookie string) (*UsageInfo, error)
}

// HTTPUsageClient fetches usage data from the Claude web API.
type HTTPUsageClient struct {
	client *http.Client
}

// NewHTTPUsageClient creates a usage client with sensible defaults.
func NewHTTPUsageClient() *HTTPUsageClient {
	return &HTTPUsageClient{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchUsage queries the Claude usage API for the given organization.
// Returns nil with an error if the API is unreachable or returns non-200.
func (c *HTTPUsageClient) FetchUsage(orgID, sessionCookie string) (*UsageInfo, error) {
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/usage", orgID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Cookie", fmt.Sprintf("sessionKey=%s", sessionCookie))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching usage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage API returned %d", resp.StatusCode)
	}

	var usage UsageInfo
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("decoding usage response: %w", err)
	}
	return &usage, nil
}

// ReadOrgID attempts to extract the Claude organization UUID from the
// oauthAccount field in <configDir>/.claude.json. Returns empty string
// if the file doesn't exist or the field isn't present.
func ReadOrgID(configDir string) string {
	path := filepath.Join(configDir, ".claude.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return ""
	}

	raw, ok := config["oauthAccount"]
	if !ok {
		return ""
	}

	var acct map[string]interface{}
	if json.Unmarshal(raw, &acct) != nil {
		return ""
	}

	// Try common field names for org ID
	for _, key := range []string{"organizationUuid", "orgId", "organization_id", "orgUuid"} {
		if v, ok := acct[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}

	return ""
}
