// Package web — contract test for `config/searxng/settings.yml`.
//
// The settings.yml file is mounted READ-ONLY into the searxng container
// at /etc/searxng/settings.yml. Multiple invariants MUST hold or the Go
// open-knowledge web provider fails at runtime:
//
//  1. search.formats MUST include "json" — internal/assistant/openknowledge/
//     web/searxng.go expects format=json in its requests; the upstream
//     default `formats: [html]` would make every request fail with
//     "format=json not enabled" (explicitly cited in the settings.yml
//     file header comment).
//
//  2. server.secret_key MUST equal "ultrasecretkey" — the upstream
//     entrypoint script substitutes this literal token with the
//     SEARXNG_SECRET env var at container start. Changing it breaks the
//     substitution.
//
//  3. server.bind_address MUST be "0.0.0.0" — the container has to listen
//     on all interfaces for the compose network (smackerel-core ->
//     searxng) to reach it.
//
//  4. At least one engine MUST be enabled — searxng with zero engines
//     returns empty results for every query, silently breaking the
//     open-knowledge fallback.
//
//  5. use_default_settings MUST be true — without it, this minimal file
//     REPLACES upstream defaults instead of overlaying, breaking
//     engine baselines.
//
// A regression to any of these would slip past `./smackerel.sh test unit`
// today (no Go-side validation existed). Adversarial sub-tests prove
// each invariant catches its target failure mode.
package web

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type searxngSettingsDoc struct {
	UseDefaultSettings bool `yaml:"use_default_settings"`
	Search             struct {
		Formats []string `yaml:"formats"`
	} `yaml:"search"`
	Server struct {
		SecretKey   string `yaml:"secret_key"`
		BindAddress string `yaml:"bind_address"`
	} `yaml:"server"`
	Engines []struct {
		Name     string `yaml:"name"`
		Disabled bool   `yaml:"disabled"`
	} `yaml:"engines"`
}

func searxngRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	// internal/assistant/openknowledge/web/ -> repo root is 4 parents up
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
}

// assertSearxngSettings validates the searxng settings.yml structure.
func assertSearxngSettings(yamlBytes []byte) error {
	var doc searxngSettingsDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	if !doc.UseDefaultSettings {
		return fmt.Errorf("contract violation: use_default_settings must be true — without it this minimal file REPLACES upstream defaults instead of overlaying, breaking the engine baselines and search behavior")
	}

	jsonFormatPresent := false
	for _, f := range doc.Search.Formats {
		if f == "json" {
			jsonFormatPresent = true
			break
		}
	}
	if !jsonFormatPresent {
		return fmt.Errorf("contract violation: search.formats=%v MUST include 'json' — without it the Go provider (internal/assistant/openknowledge/web/searxng.go) fails every request with 'format=json not enabled'", doc.Search.Formats)
	}

	if doc.Server.SecretKey != "ultrasecretkey" {
		return fmt.Errorf("contract violation: server.secret_key=%q MUST equal 'ultrasecretkey' — the upstream entrypoint substitutes this literal token with SEARXNG_SECRET at container start; changing it breaks the substitution and the deployment silently runs with the hardcoded literal as the actual secret", doc.Server.SecretKey)
	}

	if doc.Server.BindAddress != "0.0.0.0" {
		return fmt.Errorf("contract violation: server.bind_address=%q MUST be '0.0.0.0' — the container must listen on all interfaces so the compose network (smackerel-core -> searxng) can reach it", doc.Server.BindAddress)
	}

	enabledCount := 0
	for _, e := range doc.Engines {
		if !e.Disabled {
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return fmt.Errorf("contract violation: zero engines have disabled=false — searxng with zero enabled engines returns empty results for every query, silently breaking the open-knowledge web fallback")
	}

	return nil
}

func TestSearxngSettingsContract_LiveFile(t *testing.T) {
	path := filepath.Join(searxngRepoRoot(t), "config", "searxng", "settings.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := assertSearxngSettings(b); err != nil {
		t.Fatalf("live settings.yml violates contract: %v", err)
	}
}

func TestSearxngSettingsContract_AdversarialMissingJSONFormat(t *testing.T) {
	bad := []byte(`use_default_settings: true
search:
  formats:
  - html
server:
  secret_key: "ultrasecretkey"
  bind_address: "0.0.0.0"
engines:
- name: duckduckgo
  disabled: false
`)
	err := assertSearxngSettings(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted formats without 'json' — would silently break the open-knowledge Go provider")
	}
	if !strings.Contains(err.Error(), "format=json not enabled") {
		t.Fatalf("expected json-format rejection, got: %v", err)
	}
}

func TestSearxngSettingsContract_AdversarialChangedSecretKey(t *testing.T) {
	bad := []byte(`use_default_settings: true
search:
  formats:
  - json
server:
  secret_key: "myrealsecret"
  bind_address: "0.0.0.0"
engines:
- name: duckduckgo
  disabled: false
`)
	err := assertSearxngSettings(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted hardcoded real secret in committed file — entrypoint substitution would silently leave the literal value in place as the deployment's actual secret")
	}
	if !strings.Contains(err.Error(), "ultrasecretkey") {
		t.Fatalf("expected secret_key rejection, got: %v", err)
	}
}

func TestSearxngSettingsContract_AdversarialBindAddressLoopback(t *testing.T) {
	bad := []byte(`use_default_settings: true
search:
  formats:
  - json
server:
  secret_key: "ultrasecretkey"
  bind_address: "127.0.0.1"
engines:
- name: duckduckgo
  disabled: false
`)
	err := assertSearxngSettings(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted bind_address=127.0.0.1 — container would not be reachable from smackerel-core over the compose network")
	}
	if !strings.Contains(err.Error(), "0.0.0.0") {
		t.Fatalf("expected bind_address rejection, got: %v", err)
	}
}

func TestSearxngSettingsContract_AdversarialAllEnginesDisabled(t *testing.T) {
	bad := []byte(`use_default_settings: true
search:
  formats:
  - json
server:
  secret_key: "ultrasecretkey"
  bind_address: "0.0.0.0"
engines:
- name: duckduckgo
  disabled: true
- name: wikipedia
  disabled: true
`)
	err := assertSearxngSettings(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted all-engines-disabled — every search would return empty, silently breaking open-knowledge fallback")
	}
	if !strings.Contains(err.Error(), "zero engines") {
		t.Fatalf("expected zero-engines rejection, got: %v", err)
	}
}

func TestSearxngSettingsContract_AdversarialMissingUseDefaults(t *testing.T) {
	bad := []byte(`use_default_settings: false
search:
  formats:
  - json
server:
  secret_key: "ultrasecretkey"
  bind_address: "0.0.0.0"
engines:
- name: duckduckgo
  disabled: false
`)
	err := assertSearxngSettings(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted use_default_settings=false — this minimal file would REPLACE upstream defaults instead of overlaying")
	}
	if !strings.Contains(err.Error(), "use_default_settings") {
		t.Fatalf("expected use_default_settings rejection, got: %v", err)
	}
}
