// Spec 047 — CI Image Vulnerability Gate.
//
// Static-file contract for `.github/workflows/build.yml`. The contract:
//
//  1. Every image built by the workflow MUST have a Trivy scan step that
//     references that image by digest. (FR-047-001, FR-047-005)
//  2. The Trivy scan step MUST appear BEFORE the first cosign sign step in
//     the same job. Signing/publishing a vulnerable artifact is the
//     deployability bypass spec 047 closes. (FR-047-002)
//  3. The Trivy scan step MUST set `severity: CRITICAL,HIGH` and
//     `exit-code: '1'` so CRITICAL/HIGH findings fail the workflow.
//     (FR-047-001)
//  4. The build manifest MUST include a vulnerabilityScan attestation
//     block referencing the scanner, threshold, and evidence artifact.
//     (FR-047-003)
//
// These invariants live in `.github/workflows/build.yml`. This test parses
// the workflow with gopkg.in/yaml.v3 and asserts the four conditions hold.
// Adversarial sub-tests prove the contract would FAIL if any invariant
// regressed (proves the test is not tautological).
//
// References:
//   - specs/047-ci-image-vulnerability-gate/spec.md
//   - specs/047-ci-image-vulnerability-gate/design.md
//   - specs/047-ci-image-vulnerability-gate/scopes.md
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// workflowStep is the minimal YAML shape of a single workflow step this
// test needs. It intentionally does NOT model every field of a step so
// unrelated workflow edits stay a non-event. ContinueOnError and If are
// modeled so the contract can reject the two most common gate-neutering
// mechanisms: a step that cannot fail the build (`continue-on-error: true`)
// and a step the runner skips because of a statically-false `if:`.
// ContinueOnError is a *bool so an unset field (nil) is distinguishable from
// an explicit `continue-on-error: false`.
type workflowStep struct {
	Name            string                 `yaml:"name"`
	ID              string                 `yaml:"id"`
	Uses            string                 `yaml:"uses"`
	With            map[string]interface{} `yaml:"with"`
	Run             string                 `yaml:"run"`
	ContinueOnError *bool                  `yaml:"continue-on-error"`
	If              string                 `yaml:"if"`
}

// workflowJob is the minimal YAML shape of a job: just its ordered steps.
type workflowJob struct {
	Steps []workflowStep `yaml:"steps"`
}

// workflowDoc is the minimal YAML shape this test needs to assert the
// vulnerability-gate contract. It intentionally does NOT model every field
// of build.yml so unrelated workflow edits stay a non-event.
type workflowDoc struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

// imageMatrix declares every image the workflow builds. This list is the
// authoritative drift gate: when a new image is added to build.yml, this
// list MUST grow OR the matrix-coverage assertion fails.
var imageMatrix = []struct {
	stepName       string // exact `name:` of the build-and-push step
	expectedDigest string // env-var name used in the digest reference
}{
	{stepName: "Build and push smackerel-core", expectedDigest: "IMAGE_CORE"},
	{stepName: "Build and push smackerel-ml", expectedDigest: "IMAGE_ML"},
}

func loadBuildWorkflow(t *testing.T) (*workflowDoc, []byte) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, ".github", "workflows", "build.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc workflowDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return &doc, raw
}

// assertVulnGateContract parses the build workflow and verifies the four
// invariants. It returns nil if the contract holds, or an error describing
// the first violation found.
func assertVulnGateContract(doc *workflowDoc, raw []byte) error {
	job, ok := doc.Jobs["build-images"]
	if !ok {
		return fmt.Errorf("contract violation: jobs.build-images missing")
	}

	// Condition 1: every image in the matrix has a Trivy scan step.
	// Condition 3: Trivy scan severity + exit-code are correct.
	scanStepIndexByImage := map[string]int{}
	firstSignStepIndex := -1
	for i, step := range job.Steps {
		// Detect cosign sign steps. Spec 047 defines the gate as "before
		// the first cosign sign of any image"; the workflow's signing
		// step names start with "Cosign keyless sign".
		if strings.HasPrefix(step.Name, "Cosign keyless sign") && firstSignStepIndex == -1 {
			firstSignStepIndex = i
		}
		// Detect Trivy scan steps by `uses:` prefix and image-ref in `with:`.
		if strings.HasPrefix(step.Uses, "aquasecurity/trivy-action@") {
			// Fail-open guard 1 (spec 047 FR-047-002): a trivy gate step with
			// `continue-on-error: true` REPORTS findings but does NOT fail the
			// workflow, so a non-zero (CRITICAL/HIGH) scan would let the
			// vulnerable image proceed to cosign-sign + manifest — the exact
			// deployability bypass spec 047 closes. This is the single most
			// common CI-gate-neutering mechanism, so the drift contract MUST
			// reject it. ContinueOnError is *bool: nil/unset and explicit
			// `false` are both fine; only an explicit `true` neuters the gate.
			if step.ContinueOnError != nil && *step.ContinueOnError {
				return fmt.Errorf("contract violation: trivy gate step %q has continue-on-error:true — a non-zero scan would not fail the build (fail-open)",
					step.Name)
			}
			// Fail-open guard 2 (spec 047 FR-047-002): a trivy gate step guarded
			// by a statically-false `if:` is silently skipped by the runner, so
			// the gate never runs even though the step still APPEARS present in
			// the workflow. Reject the unambiguous literal-false forms.
			if ifNeutersGate(step.If) {
				return fmt.Errorf("contract violation: trivy gate step %q is guarded by a gate-neutering if: %q",
					step.Name, step.If)
			}
			imageRef, _ := step.With["image-ref"].(string)
			severity, _ := step.With["severity"].(string)
			exitCode, _ := step.With["exit-code"].(string)
			if severity != "CRITICAL,HIGH" {
				return fmt.Errorf("contract violation: trivy step %q has severity=%q (expected CRITICAL,HIGH)",
					step.Name, severity)
			}
			if exitCode != "1" {
				return fmt.Errorf("contract violation: trivy step %q has exit-code=%q (expected '1' so CRITICAL/HIGH fails the workflow)",
					step.Name, exitCode)
			}
			// Threshold-tuning enforcement (spec 047 design.md §Threshold Tuning):
			// every trivy scan step MUST set ignore-unfixed: true. This blocks
			// regressions where an operator silently flips the gate back to
			// blocking on advisory CVEs in base images that have no upstream
			// fix — which is non-actionable and makes the deploy non-runnable.
			ignoreUnfixedRaw, present := step.With["ignore-unfixed"]
			if !present {
				return fmt.Errorf("contract violation: trivy step %q missing required `ignore-unfixed: true` (spec 047 design.md §Threshold Tuning)",
					step.Name)
			}
			ignoreUnfixed, ok := ignoreUnfixedRaw.(bool)
			if !ok || !ignoreUnfixed {
				return fmt.Errorf("contract violation: trivy step %q has ignore-unfixed=%v (expected true per spec 047 design.md §Threshold Tuning — block on FIXABLE CRITICAL/HIGH only)",
					step.Name, ignoreUnfixedRaw)
			}
			// Spec 047 R13: limit-severities-for-sarif MUST be true.
			// Without this flag, trivy-action v0.36.0 entrypoint.sh unsets
			// TRIVY_SEVERITY when format=sarif ("Building SARIF report with all
			// severities"). That makes --exit-code=1 trigger on MEDIUM/LOW
			// findings even though `severity` is set to CRITICAL,HIGH —
			// which is the silent-false-positive root cause we hit in CI run
			// 25876101377 on commit ded2fe5d. Setting limit-severities-for-sarif: true
			// preserves the severity filter for SARIF output AND for exit-code
			// computation, restoring the contracted gate behavior.
			limitSeveritiesRaw, present := step.With["limit-severities-for-sarif"]
			if !present {
				return fmt.Errorf("contract violation: trivy step %q missing required `limit-severities-for-sarif: true` (spec 047 R13 — without this, trivy-action exits 1 on MEDIUM/LOW)",
					step.Name)
			}
			limitSeverities, ok := limitSeveritiesRaw.(bool)
			if !ok || !limitSeverities {
				return fmt.Errorf("contract violation: trivy step %q has limit-severities-for-sarif=%v (expected true per spec 047 R13 — preserves severity filter for exit-code)",
					step.Name, limitSeveritiesRaw)
			}
			// Map the trivy step to its image by matching the image-ref env var.
			for _, img := range imageMatrix {
				if strings.Contains(imageRef, "${{ env."+img.expectedDigest+" }}") {
					scanStepIndexByImage[img.stepName] = i
				}
			}
		}
	}

	if firstSignStepIndex == -1 {
		return fmt.Errorf("contract violation: no cosign sign step found in build-images job")
	}

	// Condition 1 + 5: every matrix image MUST have a corresponding Trivy
	// scan step. Drift prevention: if a new image is added to imageMatrix
	// without a scan step, this fails.
	for _, img := range imageMatrix {
		idx, ok := scanStepIndexByImage[img.stepName]
		if !ok {
			return fmt.Errorf("contract violation: no trivy scan step found for image %q (FR-047-005 matrix coverage)",
				img.stepName)
		}
		// Condition 2: scan step MUST appear before first sign step.
		if idx >= firstSignStepIndex {
			return fmt.Errorf("contract violation: trivy scan for %q (step #%d) appears at or after first cosign sign (step #%d) — vulnerability gate must run BEFORE signing (FR-047-002)",
				img.stepName, idx, firstSignStepIndex)
		}
	}

	// Condition 4: build manifest MUST include vulnerabilityScan attestation.
	// Asserted on raw bytes because the publish-build-manifest job uses a
	// heredoc inside a `run:` block — yaml.v3 parses that as one big string,
	// not as nested keys. Substring match is sufficient because the heredoc
	// is reviewed-once content, not arbitrary user input.
	rawStr := string(raw)
	requiredManifestKeys := []string{
		"vulnerabilityScan:",
		"scanner: trivy",
		"severityThreshold: CRITICAL,HIGH",
		"gateBlocksOn: CRITICAL,HIGH-with-upstream-fix",
		"ignoreUnfixed: true",
		// FR-047-003 requires the attestation to record ALL THREE
		// deployability policy fields. The human-readable rationale anchors
		// WHY unfixed advisory CVEs do not block deploy; dropping it would
		// leave the manifest unable to prove the policy intent. Enforced so
		// the third policy field cannot drift away from the other two.
		"ignoreUnfixedRationale:",
		"evidenceArtifact: trivy-scan-reports-",
		"specReference: specs/047-ci-image-vulnerability-gate/spec.md",
	}
	for _, key := range requiredManifestKeys {
		if !strings.Contains(rawStr, key) {
			return fmt.Errorf("contract violation: build-manifest heredoc missing required vulnerabilityScan field %q (FR-047-003 + spec 047 design.md §Threshold Tuning)",
				key)
		}
	}

	return nil
}

// ifNeutersGate reports whether a step-level `if:` expression statically
// disables the step, so a present-but-skipped trivy gate would never run.
// It rejects only the unambiguous literal-false forms — `false` and
// `${{ false }}` (with surrounding-whitespace tolerance and case-insensitive
// matching). It deliberately does NOT try to evaluate arbitrary expressions:
// legitimate conditional guards (event/matrix filters such as
// `if: github.event_name == 'push'`) stay allowed, so this guard cannot turn
// into a false-positive that blocks unrelated workflow edits.
func ifNeutersGate(ifExpr string) bool {
	core := strings.TrimSpace(ifExpr)
	if core == "" {
		return false
	}
	// Unwrap a single `${{ ... }}` expression wrapper, then re-trim, so
	// `${{ false }}` collapses to `false` for the literal comparison.
	if strings.HasPrefix(core, "${{") && strings.HasSuffix(core, "}}") {
		core = strings.TrimSpace(core[len("${{") : len(core)-len("}}")])
	}
	return strings.EqualFold(core, "false")
}

// TestVulnGateContract_LiveFile verifies the live `.github/workflows/build.yml`
// satisfies the spec 047 vulnerability gate contract.
func TestVulnGateContract_LiveFile(t *testing.T) {
	doc, raw := loadBuildWorkflow(t)
	if err := assertVulnGateContract(doc, raw); err != nil {
		t.Fatalf("live build.yml violates spec 047 contract: %v", err)
	}
	t.Logf("contract OK: build.yml satisfies spec 047 (every matrix image scanned with CRITICAL/HIGH gate before signing; manifest carries scan evidence)")
}

// TestVulnGateContract_AdversarialMissingScan proves the contract function
// catches a regression where a new image is added to the matrix without a
// matching Trivy scan step.
func TestVulnGateContract_AdversarialMissingScan(t *testing.T) {
	// Build a doc where smackerel-ml has a build step but NO trivy scan,
	// while smackerel-core does. The contract MUST fail because matrix
	// coverage is incomplete.
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					// NO trivy scan for smackerel-ml — adversarial.
					{Name: "Cosign keyless sign — core"},
					{Name: "Cosign keyless sign — ml"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (missing trivy scan for smackerel-ml) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "no trivy scan step found for image \"Build and push smackerel-ml\"") {
		t.Fatalf("expected matrix-coverage violation, got: %v", err)
	}
	t.Logf("adversarial OK: missing trivy scan for an image is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialScanAfterSign proves the contract function
// catches a regression where the Trivy scan runs AFTER cosign sign.
func TestVulnGateContract_AdversarialScanAfterSign(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					// Sign FIRST — adversarial.
					{Name: "Cosign keyless sign — core"},
					{Name: "Cosign keyless sign — ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (scan after sign) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "appears at or after first cosign sign") {
		t.Fatalf("expected scan-ordering violation, got: %v", err)
	}
	t.Logf("adversarial OK: trivy scan running after cosign sign is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialWeakSeverity proves the contract function
// catches a regression where the Trivy scan severity threshold is weakened
// (e.g. to MEDIUM only, or HIGH only without CRITICAL), which would let
// CRITICAL findings through.
func TestVulnGateContract_AdversarialWeakSeverity(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref": "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":  "MEDIUM", // weakened — adversarial
							"exit-code": "1",
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (weak severity) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "severity=\"MEDIUM\"") {
		t.Fatalf("expected severity violation, got: %v", err)
	}
	t.Logf("adversarial OK: weakened scan severity is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialNonBlockingExitCode proves the contract
// function catches a regression where the Trivy scan does not fail the
// workflow on findings (exit-code: '0' or unset).
func TestVulnGateContract_AdversarialNonBlockingExitCode(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref": "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":  "CRITICAL,HIGH",
							"exit-code": "0", // non-blocking — adversarial
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (non-blocking exit-code) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "exit-code=\"0\"") {
		t.Fatalf("expected exit-code violation, got: %v", err)
	}
	t.Logf("adversarial OK: non-blocking trivy exit-code is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialMissingManifestEvidence proves the
// contract function catches a regression where the build manifest no
// longer carries vulnerabilityScan attestation evidence.
func TestVulnGateContract_AdversarialMissingManifestEvidence(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	// Adversarial raw: manifest heredoc DOES NOT include vulnerabilityScan block.
	raw := []byte("attestations:\n  scheme: cosign-keyless\n  sbom: spdx-json\n  provenance: slsa\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (missing manifest evidence) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "vulnerabilityScan field") {
		t.Fatalf("expected manifest-evidence violation, got: %v", err)
	}
	t.Logf("adversarial OK: missing manifest vulnerabilityScan evidence is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialIgnoreUnfixedFlipped proves that the
// contract test would catch a regression where an operator silently flips
// the threshold-tuning flag from `ignore-unfixed: true` back to `false`
// (i.e., reverts spec 047 design.md §Threshold Tuning) on either trivy step.
func TestVulnGateContract_AdversarialIgnoreUnfixedFlipped(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":      "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":       "CRITICAL,HIGH",
							"exit-code":      "1",
							"ignore-unfixed": false, // adversarial: flipped back to false
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":      "${{ env.IMAGE_ML }}@sha256:def",
							"severity":       "CRITICAL,HIGH",
							"exit-code":      "1",
							"ignore-unfixed": false, // adversarial: flipped back to false
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (ignore-unfixed: false) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "ignore-unfixed=false") {
		t.Fatalf("expected ignore-unfixed-flipped violation, got: %v", err)
	}
	t.Logf("adversarial OK: ignore-unfixed: false on a trivy step is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialMissingIgnoreUnfixedField proves that the
// contract test would catch a regression where an operator removes the
// ignore-unfixed key entirely from a trivy step's `with:` block (relying on
// the action's default behavior instead of explicit policy declaration).
func TestVulnGateContract_AdversarialMissingIgnoreUnfixedField(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref": "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":  "CRITICAL,HIGH",
							"exit-code": "1",
							// adversarial: ignore-unfixed key entirely absent
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref": "${{ env.IMAGE_ML }}@sha256:def",
							"severity":  "CRITICAL,HIGH",
							"exit-code": "1",
							// adversarial: ignore-unfixed key entirely absent
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (ignore-unfixed missing) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "missing required `ignore-unfixed: true`") {
		t.Fatalf("expected missing-ignore-unfixed-field violation, got: %v", err)
	}
	t.Logf("adversarial OK: trivy step with no ignore-unfixed field is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialMissingIgnoreUnfixedManifestKey proves
// that the contract test would catch a regression where the workflow steps
// keep `ignore-unfixed: true` but the build-manifest heredoc forgets the
// `ignoreUnfixed: true` declaration — leaving the attestation record
// inconsistent with the actual gate behavior.
func TestVulnGateContract_AdversarialMissingIgnoreUnfixedManifestKey(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	// Adversarial raw: manifest heredoc has gateBlocksOn updated but is
	// MISSING the `ignoreUnfixed: true` declaration that proves the
	// attestation record matches the workflow's actual gate behavior.
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (missing ignoreUnfixed manifest key) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "ignoreUnfixed: true") {
		t.Fatalf("expected missing-ignoreUnfixed-manifest-key violation, got: %v", err)
	}
	t.Logf("adversarial OK: build-manifest heredoc missing ignoreUnfixed: true is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialMissingLimitSeveritiesForSarif proves that
// the contract test catches the spec 047 R13 regression: a trivy step that
// uses `format: sarif` without `limit-severities-for-sarif: true`.
//
// trivy-action v0.36.0 entrypoint.sh contains:
//
//	if [ "${TRIVY_FORMAT:-}" = "sarif" ]; then
//	  if [ "${INPUT_LIMIT_SEVERITIES_FOR_SARIF:-false,,}" != "true" ]; then
//	    echo "Building SARIF report with all severities"
//	    unset TRIVY_SEVERITY    # ← the bug
//	  fi
//	fi
//
// Unsetting TRIVY_SEVERITY makes `--exit-code 1` trip on ANY vulnerability
// (MEDIUM/LOW included), even though the user passed
// `severity: CRITICAL,HIGH`. This caused CI run 25876101377 step 9 to fail
// on commit ded2fe5d (image `sha256:3756cbdb...`) with 22 MEDIUM + 4 LOW
// findings and 0 HIGH/CRITICAL. The fix is to set
// `limit-severities-for-sarif: true` on every trivy step.
//
// Without this regression test, an operator could silently revert the R13
// fix and reintroduce the false-positive failure mode.
func TestVulnGateContract_AdversarialMissingLimitSeveritiesForSarif(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":      "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":       "CRITICAL,HIGH",
							"exit-code":      "1",
							"ignore-unfixed": true,
							// limit-severities-for-sarif missing — adversarial.
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (missing limit-severities-for-sarif on core) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "limit-severities-for-sarif") {
		t.Fatalf("expected limit-severities-for-sarif violation, got: %v", err)
	}
	t.Logf("adversarial OK: trivy step with no limit-severities-for-sarif is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialLimitSeveritiesForSarifFalse proves that
// the contract test catches the case where an operator explicitly sets
// `limit-severities-for-sarif: false`. This is just as broken as omitting it
// (trivy-action v0.36.0 still unsets TRIVY_SEVERITY) — both must be rejected.
func TestVulnGateContract_AdversarialLimitSeveritiesForSarifFalse(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": false, // adversarial: explicit false
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (limit-severities-for-sarif: false) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "limit-severities-for-sarif=false") {
		t.Fatalf("expected limit-severities-for-sarif=false violation, got: %v", err)
	}
	t.Logf("adversarial OK: trivy step with limit-severities-for-sarif: false is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialMissingIgnoreUnfixedRationaleManifestKey
// proves the contract test would catch a regression where the workflow
// steps keep the correct flags but the build-manifest heredoc drops the
// `ignoreUnfixedRationale` field.
//
// FR-047-003 (spec.md) requires the vulnerabilityScan attestation to record
// ALL THREE deployability policy fields — `gateBlocksOn`, `ignoreUnfixed`,
// AND `ignoreUnfixedRationale` — "so the manifest proves which CVE class
// blocked or did not block deploy". Before this test the contract enforced
// only the first two; an operator could delete the rationale from the
// manifest heredoc (the human-readable justification for why unfixed
// advisory CVEs do NOT block deploy) and the drift gate would stay green,
// silently violating FR-047-003. This regression test anchors the third
// policy field to the same drift contract as the other two.
func TestVulnGateContract_AdversarialMissingIgnoreUnfixedRationaleManifestKey(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	// Adversarial raw: manifest heredoc carries gateBlocksOn + ignoreUnfixed
	// but is MISSING the `ignoreUnfixedRationale` field that FR-047-003
	// requires the attestation to record.
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (missing ignoreUnfixedRationale manifest key) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "ignoreUnfixedRationale") {
		t.Fatalf("expected missing-ignoreUnfixedRationale-manifest-key violation, got: %v", err)
	}
	t.Logf("adversarial OK: build-manifest heredoc missing ignoreUnfixedRationale is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialContinueOnError proves the contract test
// catches the single most common CI-gate-neutering mechanism: an operator
// adding `continue-on-error: true` to a trivy gate step. With that flag a
// non-zero scan (CRITICAL/HIGH findings) reports the failure but does NOT
// fail the workflow, so the vulnerable image proceeds to cosign-sign +
// manifest — exactly the deployability bypass spec 047 exists to close. A
// chaos probe proved this was fail-open: injecting the flag into the live
// build.yml left TestVulnGateContract_LiveFile GREEN. This regression anchors
// the flag to the gate. Every other field on the doc is valid, so the ONLY
// reason this doc may be rejected is the continue-on-error flag — making the
// test non-tautological (it passes ONLY if assertVulnGateContract grew the
// continue-on-error guard).
func TestVulnGateContract_AdversarialContinueOnError(t *testing.T) {
	neuter := true
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name:            "Trivy vulnerability scan — smackerel-core",
						Uses:            "aquasecurity/trivy-action@v0.29.0",
						ContinueOnError: &neuter, // adversarial: fail-open
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  ignoreUnfixedRationale: advisory CVEs with no upstream fix are non-actionable\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (continue-on-error: true on trivy gate step) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "continue-on-error:true") {
		t.Fatalf("expected fail-open continue-on-error violation, got: %v", err)
	}
	t.Logf("adversarial OK: trivy gate step with continue-on-error:true is rejected with: %v", err)
}

// TestVulnGateContract_AdversarialNeuteringIf proves the contract test
// catches the other common gate-neutering mechanism: guarding the trivy gate
// step with a statically-false `if:` so the runner skips it. A skipped gate
// never fails the build, so a CRITICAL/HIGH image proceeds to sign + manifest
// even though the workflow still APPEARS to contain a scan step. Every other
// field on the doc is valid, so the ONLY reason this doc may be rejected is
// the gate-neutering `if:` — making the test non-tautological (it passes ONLY
// if assertVulnGateContract grew the if-guard).
func TestVulnGateContract_AdversarialNeuteringIf(t *testing.T) {
	doc := &workflowDoc{
		Jobs: map[string]workflowJob{
			"build-images": {
				Steps: []workflowStep{
					{Name: "Build and push smackerel-core"},
					{Name: "Build and push smackerel-ml"},
					{
						Name: "Trivy vulnerability scan — smackerel-core",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						If:   "false", // adversarial: gate is statically skipped
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_CORE }}@sha256:abc",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{
						Name: "Trivy vulnerability scan — smackerel-ml",
						Uses: "aquasecurity/trivy-action@v0.29.0",
						With: map[string]interface{}{
							"image-ref":                  "${{ env.IMAGE_ML }}@sha256:def",
							"severity":                   "CRITICAL,HIGH",
							"exit-code":                  "1",
							"ignore-unfixed":             true,
							"limit-severities-for-sarif": true,
						},
					},
					{Name: "Cosign keyless sign — core"},
				},
			},
		},
	}
	raw := []byte("vulnerabilityScan:\n  scanner: trivy\n  severityThreshold: CRITICAL,HIGH\n  gateBlocksOn: CRITICAL,HIGH-with-upstream-fix\n  ignoreUnfixed: true\n  ignoreUnfixedRationale: advisory CVEs with no upstream fix are non-actionable\n  evidenceArtifact: trivy-scan-reports-abc\n  specReference: specs/047-ci-image-vulnerability-gate/spec.md\n")
	err := assertVulnGateContract(doc, raw)
	if err == nil {
		t.Fatal("expected adversarial doc (if: false on trivy gate step) to fail contract, but it passed")
	}
	if !strings.Contains(err.Error(), "gate-neutering if") {
		t.Fatalf("expected gate-neutering if violation, got: %v", err)
	}
	t.Logf("adversarial OK: trivy gate step guarded by if: false is rejected with: %v", err)
}
