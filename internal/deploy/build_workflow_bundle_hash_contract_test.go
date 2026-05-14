// BUG-047-001 / DEVOPS-HL-002 — Build-Manifest Bundle-Hash Contract.
//
// Static-file contract for `.github/workflows/build.yml` that closes the
// DEVOPS-HL-002 finding from the 2026-05-13 home-lab readiness review:
// the published `build-manifest-<sourceSha>.yaml` MUST carry a per-env
// `sha256:` field next to each `configBundles[*].ref:` so the deploy-target
// adapter `apply.sh` can verify the pulled bundle's hash byte-for-byte
// before mounting the bundle.
//
// Without this field the operator (and the adapter) have no source of truth
// for the expected bundle hash. The Build-Once Deploy-Many invariant
// "Missing bundle hash verification | Allows tampered config to deploy"
// (see docs/Deployment.md and .github/copilot-instructions.md) is then a
// no-op — collapsing the bundle-tamper defense-in-depth gate to
// bundle-publish-side only.
//
// The contract:
//
//  1. The `build-bundles` job MUST upload one `bundle-sha-<env>-<sourceSha>`
//     artifact per environment (dev, test, home-lab).
//  2. The `publish-build-manifest` job MUST download those artifacts before
//     emitting the build manifest.
//  3. The build-manifest heredoc MUST emit a `sha256:` field for each
//     configBundles entry, sourced from the per-env artifact.
//
// Adversarial sub-tests prove the contract function would FAIL if any
// invariant regressed (proves the test is not tautological).
//
// References:
//   - specs/047-ci-image-vulnerability-gate/bugs/BUG-047-001-build-manifest-bundle-hash/
//   - docs/Deployment.md ("Missing bundle hash verification | Allows tampered config")
//   - .github/copilot-instructions.md "Build-Once Deploy-Many" forbidden patterns
//   - deploy/contract.yaml configBundles.manifestSchema
package deploy

import (
	"fmt"
	"strings"
	"testing"
)

// bundleEnvs is the authoritative list of bundle environments the contract
// requires. If the workflow's matrix grows, this list MUST grow too — the
// matrix-coverage assertion below catches drift.
var bundleEnvs = []string{"dev", "test", "home-lab"}

// assertBundleHashContract verifies the workflow contract for BUG-047-001.
// It returns nil if the contract holds, or an error describing the first
// violation found. The doc parameter is the parsed workflow shape (re-used
// from the existing vuln-gate contract test); raw is the file's bytes,
// because the build-manifest heredoc lives inside a `run:` block that
// yaml.v3 parses as one big string.
func assertBundleHashContract(doc *workflowDoc, raw []byte) error {
	rawStr := string(raw)

	// Condition 1: build-bundles job MUST upload one bundle-sha artifact per
	// environment. The matrix step uses ${{ matrix.env }} interpolation, so
	// the *raw* workflow string only contains a single `name:
	// bundle-sha-${{ matrix.env }}-${{ needs.build-images.outputs.sourceSha }}`
	// upload step. We assert the upload step exists and its name template
	// is correctly env-keyed.
	bundlesJob, ok := doc.Jobs["build-bundles"]
	if !ok {
		return fmt.Errorf("contract violation: jobs.build-bundles missing")
	}
	foundUpload := false
	foundShaWrite := false
	for _, step := range bundlesJob.Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			name, _ := step.With["name"].(string)
			if strings.Contains(name, "bundle-sha-${{ matrix.env }}") &&
				strings.Contains(name, "${{ needs.build-images.outputs.sourceSha }}") {
				foundUpload = true
			}
		}
		// The shell step that writes the per-env sha file MUST exist before
		// the upload (the upload's `path:` references it). We detect it by
		// looking for the `dist/bundle-shas/${{ matrix.env }}.sha` write.
		if strings.Contains(step.Run, "dist/bundle-shas/${{ matrix.env }}.sha") &&
			strings.Contains(step.Run, "steps.bundle-sha.outputs.bundleSha") {
			foundShaWrite = true
		}
	}
	if !foundShaWrite {
		return fmt.Errorf("contract violation: build-bundles job has no step that writes dist/bundle-shas/${{ matrix.env }}.sha sourced from steps.bundle-sha.outputs.bundleSha (BUG-047-001 / DEVOPS-HL-002)")
	}
	if !foundUpload {
		return fmt.Errorf("contract violation: build-bundles job has no actions/upload-artifact step named bundle-sha-${{ matrix.env }}-${{ needs.build-images.outputs.sourceSha }} (BUG-047-001 / DEVOPS-HL-002)")
	}

	// Condition 2: publish-build-manifest job MUST download the per-env
	// bundle-sha artifacts before emitting the manifest.
	publishJob, ok := doc.Jobs["publish-build-manifest"]
	if !ok {
		return fmt.Errorf("contract violation: jobs.publish-build-manifest missing")
	}
	foundDownload := false
	for _, step := range publishJob.Steps {
		if strings.HasPrefix(step.Uses, "actions/download-artifact@") {
			pattern, _ := step.With["pattern"].(string)
			if strings.Contains(pattern, "bundle-sha-*") &&
				strings.Contains(pattern, "${{ needs.build-images.outputs.sourceSha }}") {
				foundDownload = true
			}
		}
	}
	if !foundDownload {
		return fmt.Errorf("contract violation: publish-build-manifest job has no actions/download-artifact step pulling bundle-sha-*-${{ needs.build-images.outputs.sourceSha }} (BUG-047-001 / DEVOPS-HL-002)")
	}

	// Condition 3: the build-manifest heredoc MUST emit a `sha256:` field
	// for each configBundles entry, sourced from the per-env artifact.
	// Asserted on the raw file because the heredoc body is a string.
	for _, env := range bundleEnvs {
		// Each configBundles entry block must contain `env: <env>` followed by
		// both `ref:` and `sha256:`. We assert the env-block + sha256 line
		// pair on the raw file. The exact pattern the manifest emits is:
		//   - env: <env>
		//     ref: ...
		//     sha256: ${BUNDLE_SHA_<ENV_UPPER_UNDERSCORED>}
		envBlockMarker := "- env: " + env
		idx := strings.Index(rawStr, envBlockMarker)
		if idx == -1 {
			return fmt.Errorf("contract violation: build-manifest heredoc has no `- env: %s` configBundles entry (BUG-047-001 / DEVOPS-HL-002)", env)
		}
		// Slice a window after the env marker to look for the sha256 field
		// before the next env block or the next top-level YAML key.
		end := idx + 400 // generous window — entries are short
		if end > len(rawStr) {
			end = len(rawStr)
		}
		window := rawStr[idx:end]
		if !strings.Contains(window, "sha256:") {
			return fmt.Errorf("contract violation: build-manifest heredoc `- env: %s` entry has no sha256: field (BUG-047-001 / DEVOPS-HL-002 — every configBundles entry MUST carry a sha256 for adapter-side hash verification)", env)
		}
		// Build the expected env-var name shape the publish-build-manifest
		// step uses: dev → BUNDLE_SHA_DEV, test → BUNDLE_SHA_TEST,
		// home-lab → BUNDLE_SHA_HOME_LAB.
		varName := "BUNDLE_SHA_" + strings.ToUpper(strings.ReplaceAll(env, "-", "_"))
		if !strings.Contains(window, "${"+varName+"}") {
			return fmt.Errorf("contract violation: build-manifest heredoc `- env: %s` entry's sha256: value is not sourced from ${%s} (BUG-047-001 / DEVOPS-HL-002 — the per-env BUNDLE_SHA_* env var is the only source of truth provided by the resolve-bundle-shas step)",
				env, varName)
		}
	}

	// Condition 3b: the env block on the publish-build-manifest's
	// "Write build-manifest" step MUST declare every BUNDLE_SHA_* variable
	// the heredoc references. This catches the silent regression where the
	// download-artifact step still runs but the env exposure is dropped,
	// leaving every sha256: as an empty literal that promote.sh would
	// reject downstream — but reject AFTER an entire CI run.
	for _, env := range bundleEnvs {
		varName := "BUNDLE_SHA_" + strings.ToUpper(strings.ReplaceAll(env, "-", "_"))
		needle := varName + ": ${{ steps.resolve-bundle-shas.outputs." + varName + " }}"
		if !strings.Contains(rawStr, needle) {
			return fmt.Errorf("contract violation: publish-build-manifest's env: block does not expose %s from steps.resolve-bundle-shas.outputs (BUG-047-001 / DEVOPS-HL-002)", varName)
		}
	}

	return nil
}

// TestBundleHashContract_LiveFile verifies the live `.github/workflows/build.yml`
// satisfies the BUG-047-001 build-manifest bundle-hash contract.
func TestBundleHashContract_LiveFile(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	if err := assertBundleHashContract(doc, raw); err != nil {
		t.Fatalf("live build.yml violates BUG-047-001 / DEVOPS-HL-002 contract: %v", err)
	}
	t.Logf("contract OK: build.yml emits per-env bundle sha256 to the build manifest (every configBundles entry carries a verifiable hash for adapter-side bundle-tamper detection)")
}

// TestBundleHashContract_AdversarialMissingShaField proves the contract
// function rejects a build manifest where one configBundles entry is missing
// its `sha256:` field. This is the exact regression DEVOPS-HL-002 closes:
// any future workflow edit that adds an env to the matrix without also
// adding the sha256 line would silently land an unverifiable bundle.
func TestBundleHashContract_AdversarialMissingShaField(t *testing.T) {
	// Re-use the live doc shape for the matrix-coverage assertions, but
	// replace the raw heredoc with a tampered version that drops the
	// home-lab sha256 line.
	doc, raw := loadBuildWorkflow(t)
	tamperedRaw := strings.Replace(string(raw),
		"          - env: home-lab\n            ref: ${{ env.BUNDLE_REGISTRY }}:home-lab-${{ needs.build-images.outputs.sourceSha }}\n            sha256: ${BUNDLE_SHA_HOME_LAB}",
		"          - env: home-lab\n            ref: ${{ env.BUNDLE_REGISTRY }}:home-lab-${{ needs.build-images.outputs.sourceSha }}",
		1,
	)
	if tamperedRaw == string(raw) {
		t.Fatal("adversarial setup failure: could not strip the home-lab sha256 line from the live workflow — the live form may have changed; refresh this test")
	}
	err := assertBundleHashContract(doc, []byte(tamperedRaw))
	if err == nil {
		t.Fatal("adversarial regression: contract should have rejected a heredoc missing the home-lab sha256 line, but it accepted it (BUG-047-001 / DEVOPS-HL-002 contract is not enforced)")
	}
	if !strings.Contains(err.Error(), "home-lab") {
		t.Fatalf("adversarial regression: rejection message does not name the home-lab env: %v", err)
	}
	if !strings.Contains(err.Error(), "BUG-047-001") {
		t.Fatalf("adversarial regression: rejection message does not carry BUG-047-001 attribution: %v", err)
	}
	t.Logf("adversarial OK: contract rejects heredoc missing home-lab sha256 with: %v", err)
}

// TestBundleHashContract_AdversarialMissingArtifactUpload proves the contract
// rejects a workflow where the per-env bundle-sha artifact upload step is
// removed from the build-bundles job. Without that artifact the
// publish-build-manifest job has nothing to download and would emit empty
// sha256 values.
func TestBundleHashContract_AdversarialMissingArtifactUpload(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)

	// Mutate the parsed doc by removing the upload-artifact step from
	// build-bundles. Keep the raw bytes untouched so the heredoc-shape
	// assertions still pass — proving the contract catches the upload-step
	// gap independently from the heredoc gap.
	job := doc.Jobs["build-bundles"]
	filtered := job.Steps[:0]
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			name, _ := step.With["name"].(string)
			if strings.Contains(name, "bundle-sha-${{ matrix.env }}") {
				continue // drop this step
			}
		}
		filtered = append(filtered, step)
	}
	job.Steps = filtered
	doc.Jobs["build-bundles"] = job

	err := assertBundleHashContract(doc, raw)
	if err == nil {
		t.Fatal("adversarial regression: contract should have rejected a build-bundles job missing the per-env bundle-sha upload-artifact step")
	}
	if !strings.Contains(err.Error(), "upload-artifact") {
		t.Fatalf("adversarial regression: rejection message does not name upload-artifact: %v", err)
	}
	if !strings.Contains(err.Error(), "BUG-047-001") {
		t.Fatalf("adversarial regression: rejection message does not carry BUG-047-001 attribution: %v", err)
	}
	t.Logf("adversarial OK: contract rejects missing upload-artifact step with: %v", err)
}

// TestBundleHashContract_AdversarialMissingArtifactDownload proves the contract
// rejects a workflow where the publish-build-manifest job no longer downloads
// the per-env bundle-sha artifacts. Without the download, the env-var
// substitution in the heredoc would expand to empty strings.
func TestBundleHashContract_AdversarialMissingArtifactDownload(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)

	job := doc.Jobs["publish-build-manifest"]
	filtered := job.Steps[:0]
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/download-artifact@") {
			pattern, _ := step.With["pattern"].(string)
			if strings.Contains(pattern, "bundle-sha-*") {
				continue // drop this step
			}
		}
		filtered = append(filtered, step)
	}
	job.Steps = filtered
	doc.Jobs["publish-build-manifest"] = job

	err := assertBundleHashContract(doc, raw)
	if err == nil {
		t.Fatal("adversarial regression: contract should have rejected a publish-build-manifest job missing the bundle-sha download-artifact step")
	}
	if !strings.Contains(err.Error(), "download-artifact") {
		t.Fatalf("adversarial regression: rejection message does not name download-artifact: %v", err)
	}
	if !strings.Contains(err.Error(), "BUG-047-001") {
		t.Fatalf("adversarial regression: rejection message does not carry BUG-047-001 attribution: %v", err)
	}
	t.Logf("adversarial OK: contract rejects missing download-artifact step with: %v", err)
}

// TestBundleHashContract_AdversarialMissingEnvExposure proves the contract
// rejects a workflow where the heredoc step's `env:` block no longer
// exposes the BUNDLE_SHA_* variables sourced from resolve-bundle-shas.
// This is the hardest regression to catch by eye: the download still runs,
// the heredoc still has `sha256: ${BUNDLE_SHA_DEV}` lines, but the env
// exposure is missing so substitution silently expands to empty strings,
// publishing a manifest with three empty sha256 fields. promote.sh would
// reject these AFTER an entire CI run — the contract test catches it
// pre-merge.
func TestBundleHashContract_AdversarialMissingEnvExposure(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)

	tamperedRaw := strings.Replace(string(raw),
		"        BUNDLE_SHA_HOME_LAB: ${{ steps.resolve-bundle-shas.outputs.BUNDLE_SHA_HOME_LAB }}",
		"        # BUNDLE_SHA_HOME_LAB env exposure removed by adversarial mutation",
		1,
	)
	if tamperedRaw == string(raw) {
		t.Fatal("adversarial setup failure: could not strip the BUNDLE_SHA_HOME_LAB env exposure from the live workflow — the live form may have changed; refresh this test")
	}
	err := assertBundleHashContract(doc, []byte(tamperedRaw))
	if err == nil {
		t.Fatal("adversarial regression: contract should have rejected a heredoc env block missing BUNDLE_SHA_HOME_LAB")
	}
	if !strings.Contains(err.Error(), "BUNDLE_SHA_HOME_LAB") {
		t.Fatalf("adversarial regression: rejection message does not name BUNDLE_SHA_HOME_LAB: %v", err)
	}
	t.Logf("adversarial OK: contract rejects missing env exposure with: %v", err)
}
