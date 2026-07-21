package intent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPTransportCompileUsesAuthenticatedSidecarRoute(t *testing.T) {
	const authToken = "test-intent-compiler-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != compileRoute {
			t.Fatalf("path = %s, want %s", request.URL.Path, compileRoute)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer "+authToken {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"schema_version":"v1","compiled_intent":{"version":"v1","language":"en","user_goal":"weather","action_class":"external_lookup","side_effect_class":"external_read","scenario_hint":"weather_query","tool_hints":[],"normalized_request":{},"slots":{},"missing_slots":[],"confidence":0.9,"clarification_prompt":null,"safety_flags":[],"source_policy":{"requires_citations":true,"allowed_source_kinds":["tool"]}},"provider":"deterministic-e2e","model":"intent-fixture-v1","latency_ms":1}`)
	}))
	defer server.Close()

	transport, err := NewHTTPTransport(server.URL, authToken, time.Second, 4096)
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	response, err := transport.Compile(context.Background(), CompileRequest{SchemaVersion: "v1"})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if response.Provider != "deterministic-e2e" {
		t.Fatalf("provider = %q, want deterministic-e2e", response.Provider)
	}
	if len(response.CompiledIntent) == 0 {
		t.Fatal("compiled_intent is empty")
	}
}

func TestNewHTTPTransportFailsLoudOnMissingConfig(t *testing.T) {
	testCases := []struct {
		name            string
		baseURL         string
		authToken       string
		timeout         time.Duration
		responseBodyMax int
		want            string
	}{
		{name: "base URL", authToken: "token", timeout: time.Second, responseBodyMax: 1, want: "baseURL"},
		{name: "auth token", baseURL: "http://ml", timeout: time.Second, responseBodyMax: 1, want: "authToken"},
		{name: "timeout", baseURL: "http://ml", authToken: "token", responseBodyMax: 1, want: "timeout"},
		{name: "response max", baseURL: "http://ml", authToken: "token", timeout: time.Second, want: "responseBodyMax"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewHTTPTransport(testCase.baseURL, testCase.authToken, testCase.timeout, testCase.responseBodyMax)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want named %s error", err, testCase.want)
			}
		})
	}
}

func TestHTTPTransportRejectsOversizedOrMismatchedResponse(t *testing.T) {
	testCases := []struct {
		name            string
		body            string
		responseBodyMax int
		want            string
	}{
		{name: "oversized", body: strings.Repeat("x", 65), responseBodyMax: 64, want: "exceeds configured max"},
		{name: "schema mismatch", body: `{"schema_version":"v2","compiled_intent":{}}`, responseBodyMax: 256, want: "does not match request"},
		{name: "trailing JSON", body: `{"schema_version":"v1","compiled_intent":{}} {}`, responseBodyMax: 256, want: "trailing JSON"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(writer, testCase.body)
			}))
			defer server.Close()
			transport, err := NewHTTPTransport(server.URL, "token", time.Second, testCase.responseBodyMax)
			if err != nil {
				t.Fatalf("NewHTTPTransport: %v", err)
			}
			_, err = transport.Compile(context.Background(), CompileRequest{SchemaVersion: "v1"})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
		})
	}
}
