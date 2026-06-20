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

	// Same sourceSha (Build-Once Deploy-Many) + publish keeps build-clients in needs.
	if !strings.Contains(rawStr, "needs.build-images.outputs.sourceSha") {
		return fmt.Errorf("contract violation: build-clients does not consume needs.build-images.outputs.sourceSha (it MUST build the SAME sourceSha as the images; FR-CBR-002)")
	}
	// Spec 098 — build-clients stays in publish-build-manifest's `needs` for
	// RELEASE ordering (a tagged build must finish + upload its digests before the
	// clients block is appended). It is NO LONGER an unconditional manifest
	// dependency — publish-build-manifest tolerates a SKIPPED build-clients (see
	// assertConditionalClientsDecoupling). The marker is retained because dropping
	// the `needs` edge entirely would race the clients-block append against an
	// unfinished client build on a release.
	if !strings.Contains(rawStr, "build-clients ]") {
		return fmt.Errorf("contract violation: publish-build-manifest does not keep build-clients in its needs (spec 098 — build-clients stays in `needs` for release ORDERING so a tagged client build finishes before its digests are appended)")
	}
	return nil
}

// assertConditionalClientsDecoupling verifies the spec-098 conditional contract
// that decouples the CI mobile-client build from the SERVER deploy manifest:
//
//  1. build-clients is gated on RELEASE intent (a tag push OR an explicit
//     build_clients workflow_dispatch). Without this gate it always runs and
//     re-blocks the server manifest on a missing operator-private Android secret
//     — the exact defect spec 098 closes.
//  2. publish-build-manifest TOLERATES a skipped build-clients
//     (needs.build-clients.result == 'skipped'), so a non-release push still
//     publishes a server-only manifest.
//  3. The clients-block contribution steps are SUCCESS-gated
//     (if: needs.build-clients.result == 'success'), so a non-release manifest is
//     server-only (android NOT contracted) and a release manifest pins the
//     clients by digest.
//
// Checks 1+2 are job-level `if:`/`needs:` markers (asserted on the raw string,
// which the parsed-step model does not carry). Check 3 is asserted on the PARSED
// publish-build-manifest steps so removing a SINGLE step's success-gate is caught
// even though the job-level `if:` still mentions 'success'.
func assertConditionalClientsDecoupling(doc *workflowDoc, rawStr string) error {
	if !strings.Contains(rawStr, "startsWith(github.ref, 'refs/tags/')") {
		return fmt.Errorf("contract violation: build-clients is not gated on release intent (startsWith(github.ref, 'refs/tags/')) — it would always run and re-block the server manifest on a missing Android secret (spec 098 FR-098-01)")
	}
	if !strings.Contains(rawStr, "github.event.inputs.build_clients") {
		return fmt.Errorf("contract violation: build-clients release gate has no explicit workflow_dispatch override (github.event.inputs.build_clients) — an operator could not force a client build without a tag (spec 098 FR-098-01)")
	}
	if !strings.Contains(rawStr, "needs.build-clients.result == 'skipped'") {
		return fmt.Errorf("contract violation: publish-build-manifest does not tolerate a skipped build-clients (needs.build-clients.result == 'skipped') — a non-release push would skip the manifest and re-block the server deploy (spec 098 FR-098-02)")
	}
	publishJob, ok := doc.Jobs["publish-build-manifest"]
	if !ok {
		return fmt.Errorf("contract violation: jobs.publish-build-manifest missing")
	}
	const successGate = "needs.build-clients.result == 'success'"
	for _, want := range []string{
		"Download client-sha artifact",
		"Resolve android client digests",
		"Append clients block to build manifest (knb spec 025)",
	} {
		var step *workflowStep
		for i := range publishJob.Steps {
			if publishJob.Steps[i].Name == want {
				step = &publishJob.Steps[i]
				break
			}
		}
		if step == nil {
			return fmt.Errorf("contract violation: publish-build-manifest is missing the %q step that contributes the android clients block (spec 085/098)", want)
		}
		if !strings.Contains(step.If, successGate) {
			return fmt.Errorf("contract violation: publish-build-manifest step %q is not success-gated on build-clients (if: %q lacks %q) — a non-release server-only manifest would still contract the android platform with no digest, tripping the knb E025-CLIENT-MANIFEST-NO-DIGEST gate (spec 098 FR-098-04)", want, step.If, successGate)
		}
	}
	return nil
}

// assertManifestClientsPolicy encodes the spec-098 manifest CONTENT contract: a
// non-release build publishes a SERVER-ONLY manifest (android NOT contracted —
// no client digests — so the knb E025-CLIENT-MANIFEST-NO-DIGEST gate has nothing
// to fail-close on), while a RELEASE build (tag or explicit build_clients
// dispatch) MUST pin the android client by digest (Build-Once Deploy-Many client
// integrity). Returns nil if the policy holds.
func assertManifestClientsPolicy(isRelease, manifestHasClientDigests bool) error {
	if isRelease && !manifestHasClientDigests {
		return fmt.Errorf("contract violation: a release build published a manifest WITHOUT android client digests — a release MUST pin the clients by digest (Build-Once Deploy-Many client integrity; spec 098 FR-098-04)")
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

	// Spec 098 — the conditional decoupling contract (release-gated client build +
	// skip-tolerant server manifest + success-gated clients block).
	if err := assertConditionalClientsDecoupling(doc, string(raw)); err != nil {
		t.Fatalf("live-file spec-098 conditional decoupling contract: %v", err)
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

// ─────────────────────────────────────────────────────────────────
// Spec 098 — conditional server-manifest / client-build decoupling tests.
// ─────────────────────────────────────────────────────────────────

// TestClientsDecoupling_LiveFile asserts the live build.yml satisfies the
// spec-098 conditional decoupling contract (release-gated client build,
// skip-tolerant server manifest, success-gated clients block).
func TestClientsDecoupling_LiveFile(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	if err := assertConditionalClientsDecoupling(doc, string(raw)); err != nil {
		t.Fatalf("live build.yml violates the spec-098 conditional decoupling contract: %v", err)
	}
	t.Logf("contract OK: build-clients is release-gated, publish-build-manifest tolerates a skipped client build, and the clients block is success-gated (spec 098)")
}

// TestClientsDecoupling_NonReleaseAcceptedWithoutDigests proves a non-release
// build publishes a server-only manifest WITHOUT android client digests and that
// this is ACCEPTED — the exact server-deploy unblock spec 098 delivers.
func TestClientsDecoupling_NonReleaseAcceptedWithoutDigests(t *testing.T) {
	if err := assertManifestClientsPolicy(false /*isRelease*/, false /*hasDigests*/); err != nil {
		t.Fatalf("policy regression: a non-release server-only manifest (no android digests) was REJECTED, but it must be accepted (spec 098 FR-098-02/04): %v", err)
	}
	t.Logf("policy OK: a non-release manifest without android digests is accepted (server-only)")
}

// TestClientsDecoupling_ReleaseRequiresDigests proves a RELEASE build that
// published a manifest WITHOUT android digests is REJECTED — release client
// integrity (Build-Once Deploy-Many) cannot silently regress — while a release
// manifest WITH digests is accepted.
func TestClientsDecoupling_ReleaseRequiresDigests(t *testing.T) {
	err := assertManifestClientsPolicy(true /*isRelease*/, false /*hasDigests*/)
	if err == nil {
		t.Fatal("adversarial: a release manifest WITHOUT android client digests was ACCEPTED — release client integrity is unenforced (spec 098 FR-098-04)")
	}
	if !strings.Contains(err.Error(), "release MUST pin the clients") {
		t.Fatalf("adversarial: rejection message does not name the release pin requirement: %v", err)
	}
	if err := assertManifestClientsPolicy(true, true); err != nil {
		t.Fatalf("policy regression: a release manifest WITH android digests was rejected: %v", err)
	}
	t.Logf("adversarial OK: a release manifest without android digests is rejected; with digests it is accepted")
}

// TestClientsDecoupling_AdversarialUngatedClientBuild proves the contract
// rejects a workflow where build-clients is NOT release-gated — the regression
// that re-couples the server manifest to the operator-private Android secret.
func TestClientsDecoupling_AdversarialUngatedClientBuild(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	tampered := strings.Replace(string(raw),
		"    if: ${{ startsWith(github.ref, 'refs/tags/') || github.event.inputs.build_clients == 'true' }}\n",
		"",
		1,
	)
	if tampered == string(raw) {
		t.Fatal("adversarial setup failure: could not strip the build-clients release gate — the live form may have changed; refresh this test")
	}
	err := assertConditionalClientsDecoupling(doc, tampered)
	if err == nil {
		t.Fatal("adversarial regression: a build-clients job with NO release gate was ACCEPTED — it would always run and re-block the server manifest on a missing Android secret")
	}
	// A pre-existing tag-gated step elsewhere in build.yml also matches
	// startsWith(github.ref,'refs/tags/'), so the unique signal that the
	// build-clients gate is gone is the missing github.event.inputs.build_clients
	// dispatch override (check 1b). Both release-gate messages contain "release".
	if !strings.Contains(err.Error(), "release") {
		t.Fatalf("adversarial regression: rejection message does not mention the release gate: %v", err)
	}
	t.Logf("adversarial OK: an ungated build-clients is rejected with: %v", err)
}

// TestClientsDecoupling_AdversarialNoSkipTolerance proves the contract rejects a
// publish-build-manifest that does NOT tolerate a skipped client build — the
// original defect where a skipped build-clients skips the manifest and blocks
// the server deploy.
func TestClientsDecoupling_AdversarialNoSkipTolerance(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	tampered := strings.Replace(string(raw),
		"needs.build-clients.result == 'skipped'",
		"needs.build-clients.result == 'success'",
		1,
	)
	if tampered == string(raw) {
		t.Fatal("adversarial setup failure: could not rewrite the skip-tolerance marker — the live form may have changed; refresh this test")
	}
	err := assertConditionalClientsDecoupling(doc, tampered)
	if err == nil {
		t.Fatal("adversarial regression: a publish-build-manifest with no skipped-client tolerance was ACCEPTED — a non-release push would skip the manifest and block the server deploy")
	}
	if !strings.Contains(err.Error(), "skipped") {
		t.Fatalf("adversarial regression: rejection message does not mention skip tolerance: %v", err)
	}
	t.Logf("adversarial OK: a manifest with no skip-tolerance is rejected with: %v", err)
}

// TestClientsDecoupling_AdversarialUnconditionalClientsBlock proves the contract
// rejects an UNCONDITIONAL clients-block append (success-gate stripped from the
// step). Mutates the PARSED doc and leaves raw intact, proving check 3 is
// independent of the job-level `if:` (which still mentions 'success'). An
// unconditional append on a non-release run would contract the android platform
// with no digest, tripping the knb E025-CLIENT-MANIFEST-NO-DIGEST gate.
func TestClientsDecoupling_AdversarialUnconditionalClientsBlock(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	job := doc.Jobs["publish-build-manifest"]
	mutated := false
	for i := range job.Steps {
		if job.Steps[i].Name == "Append clients block to build manifest (knb spec 025)" {
			job.Steps[i].If = "" // strip the success-gate → unconditional append
			mutated = true
		}
	}
	if !mutated {
		t.Fatal("adversarial setup failure: could not find the Append clients block step to strip its if: — refresh this test")
	}
	doc.Jobs["publish-build-manifest"] = job
	err := assertConditionalClientsDecoupling(doc, string(raw))
	if err == nil {
		t.Fatal("adversarial regression: an unconditional clients-block append was ACCEPTED — a non-release server-only manifest would contract android with no digest, tripping the knb E025 gate")
	}
	if !strings.Contains(err.Error(), "success-gated") {
		t.Fatalf("adversarial regression: rejection message does not mention the success-gate: %v", err)
	}
	t.Logf("adversarial OK: an unconditional clients-block append is rejected with: %v", err)
}
