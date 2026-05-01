//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
)

// apiPutJSON performs an authenticated JSON PUT against the live stack.
// Used by spec 039 Scope 4 watch tests.
func apiPutJSON(cfg e2eConfig, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPut, cfg.CoreURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * 1_000_000_000}
	return client.Do(req)
}

// httpDelete performs an authenticated DELETE against the live stack.
// Used by spec 039 Scope 4 watch tests for cleanup.
func httpDelete(cfg e2eConfig, path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, cfg.CoreURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	client := &http.Client{Timeout: 15 * 1_000_000_000}
	return client.Do(req)
}

// jsonInt is a small helper to inject int64 values into name suffixes without
// pulling in fmt for the watch consent tests.
func jsonInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
