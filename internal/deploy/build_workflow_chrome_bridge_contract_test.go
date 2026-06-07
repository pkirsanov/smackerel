// Spec 058 Scope 4 — Build-Manifest chromeBridge Contract.
//
// Static-file contract for `.github/workflows/build.yml` that closes the
// residual spec-058 DoD row "Persistent regression row
// Regression_BuildManifestRecordsZipSHA256 proves build-manifest contract".
//
// Spec 058 Scope 4 wired the Chrome Extension Bridge into the Build-Once
// Deploy-Many release surface: the `build-chrome-bridge` job builds the MV3
// zip via `./smackerel.sh build --extension chrome-bridge`, cosign-keyless
// signs it (Rekor transparency log), and uploads a `chrome-bridge-sha-<sha>`
// artifact; the `publish-build-manifest` job downloads that artifact and emits
// a `chromeBridge:` block into `build-manifest-<sourceSha>.yaml` carrying the
// zip's sha256 + signature provenance.
//
// Without a contract test, a future workflow edit could silently drop the
// zipSha256 (leaving the operator with no verifiable hash for the sideload
// artifact) or the signing provenance (downgrading the supply-chain guarantee)
// and no test would catch it until a release shipped an unverifiable artifact.
//
// The contract:
//
//  1. The `build-chrome-bridge` job MUST build the extension via the repo CLI,
//     cosign-keyless sign the zip, and upload a `chrome-bridge-sha-<sourceSha>`
//     artifact for the manifest job to consume.
//  2. The `publish-build-manifest` job MUST download that artifact before
//     emitting the manifest.
//  3. The build-manifest heredoc MUST emit a `chromeBridge:` block whose
//     `zipSha256:` is sourced from the per-build artifact and whose
//     `signatureScheme:`/`transparencyLog:` record the cosign-keyless + Rekor
//     provenance; the publish step's `env:` block MUST expose the
//     CHROME_BRIDGE_ZIP_SHA / CHROME_BRIDGE_ZIP_NAME vars the heredoc references.
//
// Adversarial sub-tests prove the contract function would FAIL if any
// invariant regressed (proves the test is not tautological).
//
// References:
//   - specs/058-chrome-extension-bridge/ (Scope 4 build/release wiring)
//   - .github/copilot-instructions.md "Build-Once Deploy-Many" forbidden patterns
//   - internal/deploy/build_workflow_bundle_hash_contract_test.go (sibling pattern)
package deploy

import (
	"fmt"
	"strings"
	"testing"
)

// assertChromeBridgeManifestContract verifies the spec-058 Scope-4 build
// manifest contract. It returns nil if the contract holds, or an error
// describing the first violation found. doc is the parsed workflow shape; raw
// is the file's bytes (the build-manifest heredoc lives inside a `run:` block
// that yaml.v3 parses as one big string).
func assertChromeBridgeManifestContract(doc *workflowDoc, raw []byte) error {
	rawStr := string(raw)

	// Condition 1: build-chrome-bridge job builds via the repo CLI, signs the
	// zip with cosign, and uploads the per-build sha artifact.
	bridgeJob, ok := doc.Jobs["build-chrome-bridge"]
	if !ok {
		return fmt.Errorf("contract violation: jobs.build-chrome-bridge missing (spec 058 Scope 4)")
	}
	foundBuild := false
	foundSign := false
	foundShaUpload := false
	for _, step := range bridgeJob.Steps {
		if strings.Contains(step.Run, "./smackerel.sh build --extension chrome-bridge") {
			foundBuild = true
		}
		// The cosign sign step signs the built zip blob (keyless).
		if strings.Contains(step.Run, "cosign sign-blob") &&
			strings.Contains(step.Run, "steps.artifact.outputs.zip_path") {
			foundSign = true
		}
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			name, _ := step.With["name"].(string)
			if strings.Contains(name, "chrome-bridge-sha-") &&
				strings.Contains(name, "${{ needs.build-images.outputs.sourceSha }}") {
				foundShaUpload = true
			}
		}
	}
	if !foundBuild {
		return fmt.Errorf("contract violation: build-chrome-bridge job has no step that builds via `./smackerel.sh build --extension chrome-bridge` (spec 058 Scope 4)")
	}
	if !foundSign {
		return fmt.Errorf("contract violation: build-chrome-bridge job does not cosign sign-blob the built zip (steps.artifact.outputs.zip_path) — the sideload artifact would ship unsigned (spec 058 Scope 4)")
	}
	if !foundShaUpload {
		return fmt.Errorf("contract violation: build-chrome-bridge job has no actions/upload-artifact step named chrome-bridge-sha-${{ needs.build-images.outputs.sourceSha }} (spec 058 Scope 4 — the publish job consumes it)")
	}

	// Condition 2: publish-build-manifest downloads the chrome-bridge-sha
	// artifact before emitting the manifest.
	publishJob, ok := doc.Jobs["publish-build-manifest"]
	if !ok {
		return fmt.Errorf("contract violation: jobs.publish-build-manifest missing")
	}
	foundDownload := false
	for _, step := range publishJob.Steps {
		if strings.HasPrefix(step.Uses, "actions/download-artifact@") {
			name, _ := step.With["name"].(string)
			if strings.Contains(name, "chrome-bridge-sha-") &&
				strings.Contains(name, "${{ needs.build-images.outputs.sourceSha }}") {
				foundDownload = true
			}
		}
	}
	if !foundDownload {
		return fmt.Errorf("contract violation: publish-build-manifest job has no actions/download-artifact step pulling chrome-bridge-sha-${{ needs.build-images.outputs.sourceSha }} (spec 058 Scope 4 — without it the chromeBridge.zipSha256 would be empty)")
	}

	// Condition 3: the build-manifest heredoc emits a chromeBridge: block with
	// the zip sha256 sourced from the per-build var and the cosign-keyless +
	// Rekor provenance fields. Asserted on the raw file because the heredoc
	// body is a string.
	idx := strings.Index(rawStr, "chromeBridge:")
	if idx == -1 {
		return fmt.Errorf("contract violation: build-manifest heredoc has no `chromeBridge:` block (spec 058 Scope 4 — the manifest would omit the sideload artifact entirely)")
	}
	end := idx + 400 // generous window — the block is short
	if end > len(rawStr) {
		end = len(rawStr)
	}
	window := rawStr[idx:end]
	if !strings.Contains(window, "zipSha256: ${CHROME_BRIDGE_ZIP_SHA}") {
		return fmt.Errorf("contract violation: chromeBridge: block has no `zipSha256: ${CHROME_BRIDGE_ZIP_SHA}` line — the sideload artifact would ship without a verifiable hash (spec 058 Scope 4; Regression_BuildManifestRecordsZipSHA256)")
	}
	if !strings.Contains(window, "signatureScheme: cosign-keyless") {
		return fmt.Errorf("contract violation: chromeBridge: block has no `signatureScheme: cosign-keyless` line — the manifest would not record the signing provenance (spec 058 Scope 4)")
	}
	if !strings.Contains(window, "transparencyLog: rekor") {
		return fmt.Errorf("contract violation: chromeBridge: block has no `transparencyLog: rekor` line — the manifest would not record the transparency-log provenance (spec 058 Scope 4)")
	}

	// Condition 3b: the publish step's env: block MUST expose the
	// CHROME_BRIDGE_ZIP_SHA var the heredoc references, sourced from the
	// resolve-chrome-bridge step. This catches the silent regression where the
	// download still runs but the env exposure is dropped, leaving zipSha256
	// as an empty literal.
	needle := "CHROME_BRIDGE_ZIP_SHA: ${{ steps.resolve-chrome-bridge.outputs.zipSha }}"
	if !strings.Contains(rawStr, needle) {
		return fmt.Errorf("contract violation: publish-build-manifest's env: block does not expose CHROME_BRIDGE_ZIP_SHA from steps.resolve-chrome-bridge.outputs.zipSha (spec 058 Scope 4 — the heredoc's ${CHROME_BRIDGE_ZIP_SHA} would expand to empty)")
	}

	return nil
}

// TestChromeBridgeManifestContract_LiveFile verifies the live
// `.github/workflows/build.yml` satisfies the spec-058 Scope-4 build-manifest
// chromeBridge contract.
func TestChromeBridgeManifestContract_LiveFile(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	if err := assertChromeBridgeManifestContract(doc, raw); err != nil {
		t.Fatalf("live build.yml violates spec 058 Scope 4 chromeBridge contract: %v", err)
	}
	t.Logf("contract OK: build.yml builds+signs the chrome-bridge zip and the build manifest records its zipSha256 + cosign-keyless/Rekor provenance (Regression_BuildManifestRecordsZipSHA256)")
}

// TestChromeBridgeManifestContract_AdversarialMissingZipSha256 proves the
// contract function rejects a build manifest whose chromeBridge: block is
// missing its `zipSha256:` line. This is the exact regression the residual
// spec-058 DoD row closes: any future workflow edit that drops the hash would
// silently ship a sideload artifact with no verifiable digest.
func TestChromeBridgeManifestContract_AdversarialMissingZipSha256(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	tamperedRaw := strings.Replace(string(raw),
		"          zipSha256: ${CHROME_BRIDGE_ZIP_SHA}\n",
		"",
		1,
	)
	if tamperedRaw == string(raw) {
		t.Fatal("adversarial setup failure: could not strip the zipSha256 line from the live workflow — the live form may have changed; refresh this test")
	}
	err := assertChromeBridgeManifestContract(doc, []byte(tamperedRaw))
	if err == nil {
		t.Fatal("adversarial regression: contract should have rejected a chromeBridge: block missing zipSha256, but it accepted it (Regression_BuildManifestRecordsZipSHA256 is not enforced)")
	}
	if !strings.Contains(err.Error(), "zipSha256") {
		t.Fatalf("adversarial regression: rejection message does not name zipSha256: %v", err)
	}
	t.Logf("adversarial OK: contract rejects a chromeBridge: block missing zipSha256 with: %v", err)
}

// TestChromeBridgeManifestContract_AdversarialMissingSignatureScheme proves the
// contract asserts the SIGNING provenance, not only the hash. Dropping
// `signatureScheme: cosign-keyless` must be rejected — otherwise the manifest
// could record a hash with no record of how (or whether) the artifact was
// signed, downgrading the supply-chain guarantee.
func TestChromeBridgeManifestContract_AdversarialMissingSignatureScheme(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	tamperedRaw := strings.Replace(string(raw),
		"          signatureScheme: cosign-keyless\n",
		"",
		1,
	)
	if tamperedRaw == string(raw) {
		t.Fatal("adversarial setup failure: could not strip the signatureScheme line from the live workflow — the live form may have changed; refresh this test")
	}
	err := assertChromeBridgeManifestContract(doc, []byte(tamperedRaw))
	if err == nil {
		t.Fatal("adversarial regression: contract should have rejected a chromeBridge: block missing signatureScheme, but it accepted it")
	}
	if !strings.Contains(err.Error(), "signatureScheme") {
		t.Fatalf("adversarial regression: rejection message does not name signatureScheme: %v", err)
	}
	t.Logf("adversarial OK: contract rejects a chromeBridge: block missing signatureScheme with: %v", err)
}

// TestChromeBridgeManifestContract_AdversarialMissingShaArtifactDownload proves
// the contract rejects a workflow where the publish job no longer downloads the
// chrome-bridge-sha artifact. Without the download the heredoc's
// ${CHROME_BRIDGE_ZIP_SHA} would expand to empty even though every raw-string
// line is still present — so this mutates the parsed doc and leaves raw intact,
// proving the download-step check is independent of the heredoc-shape checks.
func TestChromeBridgeManifestContract_AdversarialMissingShaArtifactDownload(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)

	job := doc.Jobs["publish-build-manifest"]
	filtered := job.Steps[:0]
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/download-artifact@") {
			name, _ := step.With["name"].(string)
			if strings.Contains(name, "chrome-bridge-sha-") {
				continue // drop this step
			}
		}
		filtered = append(filtered, step)
	}
	job.Steps = filtered
	doc.Jobs["publish-build-manifest"] = job

	err := assertChromeBridgeManifestContract(doc, raw)
	if err == nil {
		t.Fatal("adversarial regression: contract should have rejected a publish job that does not download the chrome-bridge-sha artifact, but it accepted it")
	}
	if !strings.Contains(err.Error(), "download-artifact") {
		t.Fatalf("adversarial regression: rejection message does not name the missing download-artifact step: %v", err)
	}
	t.Logf("adversarial OK: contract rejects a publish job missing the chrome-bridge-sha download with: %v", err)
}
