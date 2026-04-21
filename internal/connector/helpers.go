package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GetCredential returns the value for key from a credentials map, or "" if
// the map is nil or the key is absent. Shared across all connectors that
// look up access_token, api_key, etc.
func GetCredential(creds map[string]string, key string) string {
	if creds == nil {
		return ""
	}
	return creds[key]
}

// GetStr returns the string value for key from a generic map, or "" if the
// key is absent or the value is not a string. Used by connector parse helpers.
func GetStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// OAuthAPIGet makes an authenticated GET request to a Google-style OAuth2 API
// and returns the parsed JSON response body. Handles 401 detection and limits
// the response body to 10 MB to prevent resource exhaustion.
//
// This replaces the per-connector gmailAPICall and youtubeAPICall helpers that
// contained identical logic.
func OAuthAPIGet(ctx context.Context, client *http.Client, apiURL string, token string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("API call: token expired or invalid (401)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("API call: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Limit response body to 10MB to prevent resource exhaustion
	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&result); err != nil {
		return nil, fmt.Errorf("API call: decode response: %w", err)
	}
	return result, nil
}
