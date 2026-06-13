// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy — spec 086 local-operator client-manifest emitter test.
//
// Exercises scripts/deploy/local-client-manifest-clients-block.sh — the
// local-operator analogue of the spec-085 CI emitter. On evo-x2 the Flutter
// Android client is BUILT + operator-signed LOCALLY, so its build-manifest
// `clients:` entry carries `provenance: local-operator` (NOT cosign-keyless) and
// a LOCAL `file://` ref (no ghcr / no embedded @sha256:). This test proves:
//
//	(1) happy path: well-formed YAML, provenance local-operator (trust-model
//	    alignment with smackerel/home-lab/params.yaml::signing.trustModel), the
//	    real AAB/APK digests passed through, laneB false;
//	(2) fail-closed: a missing OR malformed digest is REFUSED before it can enter
//	    the manifest (mirrors the knb gate check (c) / E025-CLIENT-MANIFEST-NO-DIGEST,
//	    enforced at emit time);
//	(3) NO-DEFAULTS: an absent required ref/digest input refuses the build.
//
// It runs the REAL bash emitter via os/exec (no reimplementation), natively
// (reliable even when the Docker test container surface is contended). It does
// NOT reuse assertClientsContract (clients_contract_test.go) because that helper
// hard-requires provenance "cosign-keyless" (the CI-path contract default); the
// local manifest intentionally carries provenance "local-operator".
//
// Cross-reference:
//   - specs/086-local-client-build/ (FR-086-05, FR-086-07, FR-086-08; SCN-086-B01..B04)
//   - scripts/deploy/local-client-manifest-clients-block.sh
//   - knb/specs/028-client-binary-local-operator-trust-model/spec.md
//   - knb/scripts/lint/client-binary-conformance.sh (check c, local-operator)
package deploy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	lcbFixtureAABSha = "1111111111111111111111111111111111111111111111111111111111111111" // 64 hex
	lcbFixtureAPKSha = "2222222222222222222222222222222222222222222222222222222222222222" // 64 hex
	lcbFixtureAABRef = "file:///srv/smackerel/clients/smackerel-assistant-abc123def456.aab"
	lcbFixtureAPKRef = "file:///srv/smackerel/clients/smackerel-assistant-abc123def456.apk"
)

// localEmitterScriptPath returns the absolute path to the local-operator emitter.
func localEmitterScriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "scripts", "deploy", "local-client-manifest-clients-block.sh")
}

// runLocalEmitter executes the emitter with exactly the given env (plus PATH so
// bash resolves) and returns stdout, combined stderr, and the exit code.
func runLocalEmitter(t *testing.T, env map[string]string) (string, string, int) {
	t.Helper()
	script := localEmitterScriptPath(t)
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("emitter script not found: %v", err)
	}
	cmd := exec.Command("bash", script)
	// Start from a minimal env so an intentionally-omitted variable is TRULY
	// absent (the emitter's `${VAR:?}` fail-loud check must fire).
	cmdEnv := []string{"PATH=" + os.Getenv("PATH")}
	for k, v := range env {
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	cmd.Env = cmdEnv
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("emitter exec error (not an exit error): %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

// localManifestClientsDoc reads the local-operator manifest `clients:` shape.
type localManifestClientsDoc struct {
	Clients struct {
		None      bool `yaml:"none"`
		Artifacts []struct {
			Platform   string   `yaml:"platform"`
			Variant    string   `yaml:"variant"`
			Kind       []string `yaml:"kind"`
			Ref        string   `yaml:"ref"`
			Sha256     string   `yaml:"sha256"`
			Provenance string   `yaml:"provenance"`
			Embeds     []string `yaml:"embeds"`
			LaneB      bool     `yaml:"laneB"`
			AabRef     string   `yaml:"aabRef"`
			ApkRef     string   `yaml:"apkRef"`
			ApkSha256  string   `yaml:"apkSha256"`
		} `yaml:"artifacts"`
	} `yaml:"clients"`
}

func lcbKindContains(kinds []string, want string) bool {
	for _, k := range kinds {
		if strings.EqualFold(strings.TrimSpace(k), want) {
			return true
		}
	}
	return false
}

// TestLocalClientManifestEmitter_ValidDigests proves SCN-086-B01 + SCN-086-B04:
// the happy path emits a well-formed local-operator clients block pinning the
// real AAB+APK digests with provenance local-operator (NOT cosign-keyless),
// a LOCAL file:// ref, and laneB:false.
func TestLocalClientManifestEmitter_ValidDigests(t *testing.T) {
	stdout, stderr, code := runLocalEmitter(t, map[string]string{
		"ANDROID_AAB_REF":    lcbFixtureAABRef,
		"ANDROID_AAB_SHA256": lcbFixtureAABSha,
		"ANDROID_APK_REF":    lcbFixtureAPKRef,
		"ANDROID_APK_SHA256": lcbFixtureAPKSha,
	})
	if code != 0 {
		t.Fatalf("emitter exited %d on valid inputs; stderr=%s", code, stderr)
	}

	var doc localManifestClientsDoc
	if err := yaml.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("emitted block is not valid YAML: %v\n%s", err, stdout)
	}
	if doc.Clients.None {
		t.Fatalf("emitted clients.none=true, expected false (smackerel ships a client)")
	}
	if len(doc.Clients.Artifacts) != 1 {
		t.Fatalf("expected exactly 1 emitted artifact, got %d", len(doc.Clients.Artifacts))
	}
	a := doc.Clients.Artifacts[0]
	if a.Platform != "android" {
		t.Fatalf("emitted artifact platform=%q, expected android", a.Platform)
	}
	if a.Variant != "-" {
		t.Fatalf("emitted variant=%q, expected \"-\"", a.Variant)
	}
	if !lcbKindContains(a.Kind, "aab") || !lcbKindContains(a.Kind, "apk") {
		t.Fatalf("emitted kind=%v, expected to contain aab AND apk", a.Kind)
	}
	// SCN-086-B04 — trust-model alignment: provenance MUST be local-operator, the
	// ACTIVE evo-x2 mode, NEVER cosign-keyless (the parked CI mode).
	if a.Provenance != "local-operator" {
		t.Fatalf("emitted provenance=%q, expected local-operator (trust-model alignment); cosign-keyless is the PARKED CI path", a.Provenance)
	}
	if a.Sha256 != lcbFixtureAABSha {
		t.Fatalf("emitted sha256=%q, expected the AAB digest %q (the gate check-c key)", a.Sha256, lcbFixtureAABSha)
	}
	if a.ApkSha256 != lcbFixtureAPKSha {
		t.Fatalf("emitted apkSha256=%q, expected the APK digest %q", a.ApkSha256, lcbFixtureAPKSha)
	}
	if a.LaneB {
		t.Fatalf("emitted laneB=true, expected false (Lane B default-OFF)")
	}
	// The local ref is a file:// path WITHOUT an embedded @sha256: (local-operator
	// acquisition is a path copy) — so it cannot false-match the line-anchored
	// sha256 field the knb gate parses.
	if a.Ref != lcbFixtureAABRef {
		t.Fatalf("emitted ref=%q, expected the local AAB ref %q", a.Ref, lcbFixtureAABRef)
	}
	if strings.Contains(a.Ref, "@sha256:") {
		t.Fatalf("emitted ref=%q embeds @sha256: — a local-operator ref must be a bare local path", a.Ref)
	}
	if a.AabRef != lcbFixtureAABRef || a.ApkRef != lcbFixtureAPKRef {
		t.Fatalf("emitted aabRef/apkRef mismatch: aabRef=%q apkRef=%q", a.AabRef, a.ApkRef)
	}
}

// TestLocalClientManifestEmitter_FailClosedMissingAAB proves SCN-086-B02: a
// missing AAB digest is REFUSED (non-zero) and emits nothing.
func TestLocalClientManifestEmitter_FailClosedMissingAAB(t *testing.T) {
	stdout, stderr, code := runLocalEmitter(t, map[string]string{
		"ANDROID_AAB_REF": lcbFixtureAABRef,
		// ANDROID_AAB_SHA256 intentionally omitted.
		"ANDROID_APK_REF":    lcbFixtureAPKRef,
		"ANDROID_APK_SHA256": lcbFixtureAPKSha,
	})
	if code == 0 {
		t.Fatalf("emitter ACCEPTED a missing AAB digest (fail-closed broken); stdout=%s", stdout)
	}
	if strings.Contains(stdout, "platform: android") {
		t.Fatalf("emitter wrote a clients block despite a missing digest (fail-open): %s", stdout)
	}
	if !strings.Contains(stderr, "ANDROID_AAB_SHA256") {
		t.Fatalf("fail-closed error did not name ANDROID_AAB_SHA256: %s", stderr)
	}
}

// TestLocalClientManifestEmitter_FailClosedMalformed proves SCN-086-B03: a
// non-64-hex digest is REFUSED (a truncated/garbage digest must never be pinned).
func TestLocalClientManifestEmitter_FailClosedMalformed(t *testing.T) {
	stdout, stderr, code := runLocalEmitter(t, map[string]string{
		"ANDROID_AAB_REF":    lcbFixtureAABRef,
		"ANDROID_AAB_SHA256": lcbFixtureAABSha,
		"ANDROID_APK_REF":    lcbFixtureAPKRef,
		"ANDROID_APK_SHA256": "deadbeef", // too short / not 64-hex
	})
	if code == 0 {
		t.Fatalf("emitter ACCEPTED a malformed APK digest (fail-closed broken); stdout=%s", stdout)
	}
	if !strings.Contains(stderr, "ANDROID_APK_SHA256") {
		t.Fatalf("fail-closed error did not name ANDROID_APK_SHA256: %s", stderr)
	}
}

// TestLocalClientManifestEmitter_FailClosedMissingRef proves the NO-DEFAULTS
// fail-loud contract: an absent ANDROID_AAB_REF input refuses the build.
func TestLocalClientManifestEmitter_FailClosedMissingRef(t *testing.T) {
	stdout, _, code := runLocalEmitter(t, map[string]string{
		// ANDROID_AAB_REF intentionally omitted.
		"ANDROID_AAB_SHA256": lcbFixtureAABSha,
		"ANDROID_APK_REF":    lcbFixtureAPKRef,
		"ANDROID_APK_SHA256": lcbFixtureAPKSha,
	})
	if code == 0 {
		t.Fatalf("emitter ACCEPTED a missing ANDROID_AAB_REF (NO-DEFAULTS violated); stdout=%s", stdout)
	}
}
