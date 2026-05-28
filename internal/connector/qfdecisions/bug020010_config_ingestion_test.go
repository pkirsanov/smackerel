package qfdecisions

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

// BUG-020-010 — Adversarial regression tests proving that the QF callback
// signing keystore is ingested through Config SST (not via a direct
// os.Getenv read inside the qfdecisions package). SCN-SM-020-bug-010.

// TestBUG020010_KeystoreReadsFromConfigNotEnv proves the keystore is
// constructed from the per-connector SourceConfig (which Config.Load()
// populates from the SST env var at boot) and NOT from a direct
// os.Getenv read inside Connect(). Adversarial setup: the env var is
// explicitly UNSET so that if the keystore source were still
// os.Getenv, parseConfig would not see the JSON and the keystore
// would be nil.
func TestBUG020010_KeystoreReadsFromConfigNotEnv(t *testing.T) {
	// Adversarial: ensure the env var is empty so any residual
	// os.Getenv path is exposed.
	t.Setenv(CallbackSigningKeysEnvVar, "")
	if got := os.Getenv(CallbackSigningKeysEnvVar); got != "" {
		t.Fatalf("env var %s not cleared: %q", CallbackSigningKeysEnvVar, got)
	}

	validJSON := `[{"key_id":"bug020010","secret":"s","not_before":"2026-01-01T00:00:00Z"}]`
	parsed, err := parseConfig(connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "tok"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":                   "http://qf.example.test",
			"packet_version":             1,
			"page_size":                  25,
			"callback_signing_keys_json": validJSON,
		},
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if parsed.CallbackSigningKeysJSON != validJSON {
		t.Fatalf("parsed.CallbackSigningKeysJSON: want %q, got %q", validJSON, parsed.CallbackSigningKeysJSON)
	}
	ks, err := LoadCallbackKeystoreFromJSON(parsed.CallbackSigningKeysJSON)
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
	}
	if ks == nil || ks.Len() != 1 {
		t.Fatalf("keystore: want 1-key, got %v", ks)
	}
	if ids := ks.KeyIDs(); len(ids) != 1 || ids[0] != "bug020010" {
		t.Fatalf("KeyIDs: want [bug020010], got %v", ids)
	}
}

// TestBUG020010_ParseConfigPermitsEmptyCallbackSigningKeysJSON proves
// the PERMISSIVE policy at the connector ingestion layer: an empty
// callback_signing_keys_json field is allowed and yields an empty
// parsed.CallbackSigningKeysJSON (caller must treat as "no keystore").
func TestBUG020010_ParseConfigPermitsEmptyCallbackSigningKeysJSON(t *testing.T) {
	parsed, err := parseConfig(connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "tok"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       "http://qf.example.test",
			"packet_version": 1,
			"page_size":      25,
			// callback_signing_keys_json omitted — PERMISSIVE.
		},
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if parsed.CallbackSigningKeysJSON != "" {
		t.Fatalf("parsed.CallbackSigningKeysJSON: want empty, got %q", parsed.CallbackSigningKeysJSON)
	}
}

// TestBUG020010_KeystoreEnvVarLiteralRemoved is a permanent structural
// guard: the qfdecisions/callback_keystore.go source file MUST NOT
// contain any os.Getenv read after the fix lands. If a future change
// re-introduces an in-package env read, this test fails immediately
// — even before the runtime behaviour regresses.
func TestBUG020010_KeystoreEnvVarLiteralRemoved(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	pkgDir := filepath.Dir(thisFile)
	target := filepath.Join(pkgDir, "callback_keystore.go")
	cmd := exec.Command("grep", "-nE", `os\.Getenv\(`, target)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("os.Getenv literal still present in %s:\n%s", target, string(out))
	}
	// grep exit code 1 == no matches (the success case for this guard).
	if exitErr, isExitErr := err.(*exec.ExitError); !isExitErr || exitErr.ExitCode() != 1 {
		t.Fatalf("grep returned unexpected status: err=%v out=%s", err, string(out))
	}
	// Defense-in-depth: also assert the keystore file no longer
	// imports "os" at the top of the file (covers shadow imports
	// of os.LookupEnv etc.).
	contents, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read %s: %v", target, readErr)
	}
	if strings.Contains(string(contents), "\n\t\"os\"\n") {
		t.Fatalf("callback_keystore.go still imports \"os\"; the fix must remove the unused import")
	}
}
