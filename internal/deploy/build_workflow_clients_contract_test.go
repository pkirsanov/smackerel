// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy — spec 085 build-clients workflow contract test.
//
// Static-file contract for the `build-clients` job in
// `.github/workflows/build.yml`. The contract (FR-CBR-002/003/004/005/006/012):
//
//  1. A `build-clients` job exists and consumes the build-images sourceSha
//     (Build-Once Deploy-Many — same source SHA as the two images).
//  2. The job cosign-keyless provenance-signs each client artifact (no key
//     material) and pushes it to the OCI registry (oras) by digest.
//  3. The job STOPS at registry push: NO ssh/scp/rsync/apply — deploy is
//     downstream of CI (the build.yml trust boundary; bubbles G074).
//  4. The build is reproducible: it pins SOURCE_DATE_EPOCH = commit time and
//     derives the version from the commit (git rev-list --count / git show
//     --format=%ct), never the wall clock.
//  5. Distribution signing is env-ref only: the upload keystore + the three
//     passwords are sourced from GitHub secrets (operator-private; FR-CBR-007).
//  6. publish-build-manifest pins the android client by digest under
//     clients.artifacts[] (downloads the client-shas artifact + appends the
//     emitter's block).
//
// Adversarial sub-tests prove the step-level contract would FAIL if a deploy
// (ssh/apply) step, a wall-clock build input, key-bearing cosign, or a missing
// cosign/oras step were introduced (proves the test is not tautological).
//
// References:
//   - specs/085-client-binary-release/ (SCN-085-B01..B06, SCN-085-C01..C03)
//   - .github/workflows/build.yml (build-clients + publish-build-manifest)
//   - scripts/deploy/client-manifest-clients-block.sh
package deploy

import (
	"fmt"
	"strings"
	"testing"
)

// bannedDeployTokens are the CI-trust-boundary violations: the client build job
// MUST stop at registry push and never reach into a deploy target.
var bannedDeployTokens = []string{"ssh ", "scp ", "rsync ", "apply.sh"}

// assertClientsBuildJobSteps verifies the step-level invariants of the
// build-clients job (operates only on parsed steps so adversarial sub-tests can
// craft a job without a backing raw workflow).
func assertClientsBuildJobSteps(job workflowJob) error {
	hasCosignKeyless := false
	hasOrasPush := false
	pushIdx := -1
	signIdx := -1

	for i, s := range job.Steps {
		run := s.Run

		// Trust boundary (FR-CBR-012): no deploy reach-through.
		for _, banned := range bannedDeployTokens {
			if strings.Contains(run, banned) {
				return fmt.Errorf("contract violation: build-clients step %q contains forbidden deploy token %q — the client build MUST stop at registry push (no ssh/apply; bubbles G074)", s.Name, strings.TrimSpace(banned))
			}
		}

		// Reproducibility (NFR-CBR-005): no wall-clock in the client build.
		if strings.Contains(run, "$(date") || strings.Contains(run, "`date") {
			return fmt.Errorf("contract violation: build-clients step %q reads the wall clock ($(date ...)) — the build MUST be deterministic (SOURCE_DATE_EPOCH + commit-derived version; NFR-CBR-005)", s.Name)
		}

		if strings.Contains(run, "cosign sign") {
			// Keyless only (FR-CBR-004): no committed/ref'd key material.
			if strings.Contains(run, "--key ") || strings.Contains(run, "--key=") || strings.Contains(run, "COSIGN_PRIVATE_KEY") {
				return fmt.Errorf("contract violation: build-clients step %q signs with key material — provenance MUST be cosign-keyless (OIDC/id-token; FR-CBR-004)", s.Name)
			}
			hasCosignKeyless = true
			if signIdx == -1 {
				signIdx = i
			}
		}
		if strings.Contains(run, "oras push") {
			hasOrasPush = true
			if pushIdx == -1 {
				pushIdx = i
			}
		}
	}

	if !hasOrasPush {
		return fmt.Errorf("contract violation: build-clients does not push the client artifact to the OCI registry (oras push) — the binary MUST be an immutable digest-pinned OCI artifact (FR-CBR-005)")
	}
	if !hasCosignKeyless {
		return fmt.Errorf("contract violation: build-clients has no cosign keyless sign step — every client artifact MUST carry always-on cosign-keyless provenance (FR-CBR-004)")
	}
	return nil
}

// assertClientsBuildRawMarkers verifies the workflow-level markers that the
// parsed-step model cannot reach (heredoc + env wiring across jobs).
func assertClientsBuildRawMarkers(raw []byte) error {
	rawStr := string(raw)

	// Reproducibility markers (NFR-CBR-005 / FR-CBR-003).
	repro := []string{"SOURCE_DATE_EPOCH", "git show -s --format=%ct", "git rev-list --count"}
	for _, m := range repro {
		if !strings.Contains(rawStr, m) {
			return fmt.Errorf("contract violation: build workflow missing reproducibility marker %q (FR-CBR-003/NFR-CBR-005)", m)
		}
	}

	// Env-ref distribution signing (FR-CBR-007): every credential from secrets.
	for _, secret := range []string{"ANDROID_KEYSTORE_BASE64", "ANDROID_KEYSTORE_PASSWORD", "ANDROID_KEY_ALIAS", "ANDROID_KEY_PASSWORD"} {
		if !strings.Contains(rawStr, "secrets."+secret) {
			return fmt.Errorf("contract violation: build-clients does not source %q from GitHub secrets — distribution signing MUST be operator-private/env-ref (FR-CBR-007)", secret)
		}
	}

	// Manifest emission (FR-CBR-006): publish-build-manifest pins the android
	// client by digest via the fail-closed emitter + the client-shas artifact.
	for _, m := range []string{"client-manifest-clients-block.sh", "client-shas-${{ needs.build-images.outputs.sourceSha }}"} {
		if !strings.Contains(rawStr, m) {
			return fmt.Errorf("contract violation: build workflow does not wire the clients.artifacts manifest emission (%q missing; FR-CBR-006)", m)
		}
	}

	// Same sourceSha (Build-Once Deploy-Many) + publish needs build-clients.
	if !strings.Contains(rawStr, "needs.build-images.outputs.sourceSha") {
		return fmt.Errorf("contract violation: build-clients does not consume needs.build-images.outputs.sourceSha (it MUST build the SAME sourceSha as the images; FR-CBR-002)")
	}
	if !strings.Contains(rawStr, "build-clients ]") {
		return fmt.Errorf("contract violation: publish-build-manifest does not list build-clients in its needs (the manifest MUST include the client digests)")
	}
	return nil
}

// TestClientsBuildWorkflow_LiveFile parses the live build.yml and asserts the
// build-clients contract (step-level + workflow-level markers).
func TestClientsBuildWorkflow_LiveFile(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)

	job, ok := doc.Jobs["build-clients"]
	if !ok {
		t.Fatal("contract violation: jobs.build-clients missing from .github/workflows/build.yml (FR-CBR-002)")
	}
	if err := assertClientsBuildJobSteps(job); err != nil {
		t.Fatalf("live-file build-clients step contract: %v", err)
	}
	if err := assertClientsBuildRawMarkers(raw); err != nil {
		t.Fatalf("live-file build-clients marker contract: %v", err)
	}

	// id-token: write must be present at workflow level for cosign keyless.
	if !strings.Contains(string(raw), "id-token: write") {
		t.Fatal("contract violation: build.yml lacks `id-token: write` permission required for cosign keyless (FR-CBR-004)")
	}
}

func TestClientsBuildWorkflow_AdversarialSshDeploy(t *testing.T) {
	job := workflowJob{Steps: []workflowStep{
		{Name: "Push", Run: "oras push reg:tag file"},
		{Name: "Sign", Run: "cosign sign --yes reg@sha256:x"},
		{Name: "Deploy", Run: "ssh deploy@evo-x2 'docker compose up -d'"},
	}}
	err := assertClientsBuildJobSteps(job)
	if err == nil {
		t.Fatal("adversarial: an ssh deploy step in build-clients was ACCEPTED (the trust boundary is unenforced)")
	}
	if !strings.Contains(err.Error(), "ssh") {
		t.Fatalf("adversarial: error did not mention 'ssh': %v", err)
	}
	t.Logf("adversarial OK: ssh deploy rejected with: %v", err)
}

func TestClientsBuildWorkflow_AdversarialWallClock(t *testing.T) {
	job := workflowJob{Steps: []workflowStep{
		{Name: "Push", Run: "oras push reg:tag file"},
		{Name: "Sign", Run: "cosign sign --yes reg@sha256:x"},
		{Name: "Version", Run: "build_name=\"0.1.0+$(date +%s)\""},
	}}
	err := assertClientsBuildJobSteps(job)
	if err == nil {
		t.Fatal("adversarial: a wall-clock $(date) build input was ACCEPTED (reproducibility unenforced)")
	}
	if !strings.Contains(err.Error(), "deterministic") {
		t.Fatalf("adversarial: error did not mention determinism: %v", err)
	}
	t.Logf("adversarial OK: wall-clock rejected with: %v", err)
}

func TestClientsBuildWorkflow_AdversarialCosignKey(t *testing.T) {
	job := workflowJob{Steps: []workflowStep{
		{Name: "Push", Run: "oras push reg:tag file"},
		{Name: "Sign", Run: "cosign sign --key cosign.key --yes reg@sha256:x"},
	}}
	err := assertClientsBuildJobSteps(job)
	if err == nil {
		t.Fatal("adversarial: key-bearing cosign was ACCEPTED (keyless provenance unenforced)")
	}
	if !strings.Contains(err.Error(), "keyless") {
		t.Fatalf("adversarial: error did not mention keyless: %v", err)
	}
	t.Logf("adversarial OK: key-bearing cosign rejected with: %v", err)
}

func TestClientsBuildWorkflow_AdversarialNoCosign(t *testing.T) {
	job := workflowJob{Steps: []workflowStep{
		{Name: "Push", Run: "oras push reg:tag file"},
		// no cosign sign step at all.
	}}
	err := assertClientsBuildJobSteps(job)
	if err == nil {
		t.Fatal("adversarial: a build-clients job with no cosign sign step was ACCEPTED (provenance unenforced)")
	}
	if !strings.Contains(err.Error(), "cosign") {
		t.Fatalf("adversarial: error did not mention cosign: %v", err)
	}
	t.Logf("adversarial OK: missing cosign rejected with: %v", err)
}

func TestClientsBuildWorkflow_AdversarialMissingReproMarker(t *testing.T) {
	// A raw workflow missing the SOURCE_DATE_EPOCH marker must fail.
	raw := []byte("jobs:\n  build-clients:\n    steps:\n      - run: oras push; cosign sign --yes\n")
	err := assertClientsBuildRawMarkers(raw)
	if err == nil {
		t.Fatal("adversarial: a workflow without reproducibility markers was ACCEPTED")
	}
	if !strings.Contains(err.Error(), "reproducibility") {
		t.Fatalf("adversarial: error did not mention reproducibility: %v", err)
	}
	t.Logf("adversarial OK: missing repro marker rejected with: %v", err)
}
