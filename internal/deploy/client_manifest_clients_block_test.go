// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy — spec 085 client-manifest emitter test.
//
// Exercises scripts/deploy/client-manifest-clients-block.sh (the CI helper that
// publish-build-manifest appends to build-manifest-<sourceSha>.yaml). The
// emitter is the fail-closed boundary for SCN-085-B04: a contracted-platform
// artifact with an empty or malformed digest is REFUSED before it can enter the
// manifest (mirroring the knb gate's check (c) /
// E025-CLIENT-MANIFEST-NO-DIGEST). The happy path proves the emitted block is
// well-formed YAML AND satisfies the same contract the live deploy/contract.yaml
// android entry does (assertClientsContract), so the manifest the knb adapter
// consumes is gate-compatible by construction.
//
// This test runs the REAL bash emitter via os/exec — no reimplementation — so
// it is an authentic functional test of the CI artifact, runnable natively
// (reliable even when the Docker test container surface is contended).
//
// Cross-reference:
//   - specs/085-client-binary-release/ (FR-CBR-006, SCN-085-B04)
//   - scripts/deploy/client-manifest-clients-block.sh
//   - internal/deploy/clients_contract_test.go (assertClientsContract reused)
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

// emitterScriptPath returns the absolute path to the emitter under test.
func emitterScriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "scripts", "deploy", "client-manifest-clients-block.sh")
}

// runEmitter executes the emitter with exactly the given env (plus PATH so bash
// resolves) and returns stdout, combined stderr, and the process exit code.
func runEmitter(t *testing.T, env map[string]string) (string, string, int) {
	t.Helper()
	script := emitterScriptPath(t)
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

const (
	fixtureAABSha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 64 hex
	fixtureAPKSha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" // 64 hex
	fixtureReg    = "ghcr.io/pkirsanov/smackerel-clients"
)

// manifestClientsDoc reads the manifest-specific fields (sha256, apkSha256) the
// base clientArtifact (contract shape) does not model.
type manifestClientsDoc struct {
	Clients struct {
		None      bool `yaml:"none"`
		Artifacts []struct {
			Platform   string   `yaml:"platform"`
			Variant    string   `yaml:"variant"`
			Kind       []string `yaml:"kind"`
			Ref        string   `yaml:"ref"`
			Sha256     string   `yaml:"sha256"`
			Provenance string   `yaml:"provenance"`
			LaneB      bool     `yaml:"laneB"`
			ApkSha256  string   `yaml:"apkSha256"`
		} `yaml:"artifacts"`
	} `yaml:"clients"`
}

// TestClientManifestEmitter_ValidDigests proves the happy path: the emitter
// exits 0, emits well-formed YAML pinning the android AAB + APK digests with
// cosign-keyless provenance and laneB:false, and the emitted block satisfies the
// SAME contract the live deploy/contract.yaml android entry does.
func TestClientManifestEmitter_ValidDigests(t *testing.T) {
	stdout, stderr, code := runEmitter(t, map[string]string{
		"CLIENTS_REGISTRY":   fixtureReg,
		"ANDROID_AAB_SHA256": fixtureAABSha,
		"ANDROID_APK_SHA256": fixtureAPKSha,
	})
	if code != 0 {
		t.Fatalf("emitter exited %d on valid digests; stderr=%s", code, stderr)
	}

	// (1) The manifest-specific shape pins both digests with the right provenance.
	var mdoc manifestClientsDoc
	if err := yaml.Unmarshal([]byte(stdout), &mdoc); err != nil {
		t.Fatalf("emitted block is not valid YAML: %v\n%s", err, stdout)
	}
	if len(mdoc.Clients.Artifacts) != 1 {
		t.Fatalf("expected exactly 1 emitted artifact, got %d", len(mdoc.Clients.Artifacts))
	}
	a := mdoc.Clients.Artifacts[0]
	if a.Platform != "android" {
		t.Fatalf("emitted artifact platform=%q, expected android", a.Platform)
	}
	if a.Sha256 != fixtureAABSha {
		t.Fatalf("emitted sha256=%q, expected the AAB digest %q (the gate check-c key)", a.Sha256, fixtureAABSha)
	}
	if a.ApkSha256 != fixtureAPKSha {
		t.Fatalf("emitted apkSha256=%q, expected the APK digest %q", a.ApkSha256, fixtureAPKSha)
	}
	if a.Provenance != "cosign-keyless" {
		t.Fatalf("emitted provenance=%q, expected cosign-keyless (always-on)", a.Provenance)
	}
	if a.LaneB {
		t.Fatalf("emitted laneB=true, expected false (Lane B default-OFF)")
	}
	if !strings.Contains(a.Ref, "@sha256:"+fixtureAABSha) {
		t.Fatalf("emitted ref=%q does not pin the AAB digest by sha256", a.Ref)
	}

	// (2) The emitted block is contract-compatible (same invariants as the live
	// deploy/contract.yaml android entry — proves manifest↔contract coherence).
	var cdoc clientsContractDoc
	if err := yaml.Unmarshal([]byte(stdout), &cdoc); err != nil {
		t.Fatalf("emitted block does not parse into the contract shape: %v", err)
	}
	if err := assertClientsContract(cdoc); err != nil {
		t.Fatalf("emitted manifest block violates the clients contract: %v", err)
	}
}

// TestClientManifestEmitter_FailClosedEmptyAAB proves the SCN-085-B04 fail-closed
// guard: an empty AAB digest is REFUSED (non-zero) and emits nothing.
func TestClientManifestEmitter_FailClosedEmptyAAB(t *testing.T) {
	stdout, stderr, code := runEmitter(t, map[string]string{
		"CLIENTS_REGISTRY":   fixtureReg,
		"ANDROID_AAB_SHA256": "",
		"ANDROID_APK_SHA256": fixtureAPKSha,
	})
	if code == 0 {
		t.Fatalf("emitter ACCEPTED an empty AAB digest (fail-closed broken); stdout=%s", stdout)
	}
	if strings.Contains(stdout, "platform: android") {
		t.Fatalf("emitter wrote a clients block despite an empty digest (fail-open): %s", stdout)
	}
	if !strings.Contains(stderr, "ANDROID_AAB_SHA256") {
		t.Fatalf("fail-closed error did not name ANDROID_AAB_SHA256: %s", stderr)
	}
}

// TestClientManifestEmitter_FailClosedMalformed proves a non-64-hex digest is
// REFUSED (a truncated/garbage digest must never be pinned in the manifest).
func TestClientManifestEmitter_FailClosedMalformed(t *testing.T) {
	stdout, stderr, code := runEmitter(t, map[string]string{
		"CLIENTS_REGISTRY":   fixtureReg,
		"ANDROID_AAB_SHA256": fixtureAABSha,
		"ANDROID_APK_SHA256": "deadbeef", // too short / not 64-hex
	})
	if code == 0 {
		t.Fatalf("emitter ACCEPTED a malformed APK digest (fail-closed broken); stdout=%s", stdout)
	}
	if !strings.Contains(stderr, "ANDROID_APK_SHA256") {
		t.Fatalf("fail-closed error did not name ANDROID_APK_SHA256: %s", stderr)
	}
}

// TestClientManifestEmitter_FailClosedMissingRegistry proves the NO-DEFAULTS
// fail-loud contract: an absent CLIENTS_REGISTRY input refuses the build.
func TestClientManifestEmitter_FailClosedMissingRegistry(t *testing.T) {
	stdout, _, code := runEmitter(t, map[string]string{
		// CLIENTS_REGISTRY intentionally omitted.
		"ANDROID_AAB_SHA256": fixtureAABSha,
		"ANDROID_APK_SHA256": fixtureAPKSha,
	})
	if code == 0 {
		t.Fatalf("emitter ACCEPTED a missing CLIENTS_REGISTRY (NO-DEFAULTS violated); stdout=%s", stdout)
	}
}
