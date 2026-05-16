// Package deploy — BUG-029-004 / HL-RESCAN-011 (Build-Once Deploy-Many).
//
// Static-file contract for `.github/workflows/ci.yml`. The contract:
//
//  1. ci.yml MUST NOT contain any `docker push` shell-command lines
//     in any step's `run:` block. (Sub-test A)
//  2. ci.yml MUST NOT contain any `docker tag <local>:<tag> ghcr.io/...`
//     cross-registry tag-mint shell-command lines in any step's
//     `run:` block. (Sub-test B)
//  3. ci.yml MUST NOT contain any `uses: docker/login-action@<sha>`
//     step entries whose `with.registry` resolves to `ghcr.io` (literal
//     or via `${{ env.REGISTRY }}` indirection). (Sub-test C)
//
// These invariants enforce that `.github/workflows/build.yml` is the
// SOLE publish path under the binding Build-Once Deploy-Many policy
// in `.github/copilot-instructions.md`. The pre-fix parallel ci.yml
// publish path (lines 125-159 at HEAD 765adddb) bypassed cosign
// keyless signing, SBOM attestation, SLSA provenance, Trivy
// vulnerability scanning, and digest pinning — all of which build.yml
// enforces — producing artifacts that no compliant deploy adapter
// can deploy.
//
// Adversarial in-memory mutation tests prove the validator catches
// regressions (mirrors TestVulnGateContract_AdversarialMissingScan and
// TestVulnGateContract_AdversarialScanAfterSign in the build_workflow
// contract test).
//
// References:
//   - specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/spec.md
//   - specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/design.md
//   - .github/copilot-instructions.md → "Build-Once Deploy-Many (BLOCKING — bubbles G074)"
//   - .github/instructions/bubbles-deployment-target.instructions.md
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ciWorkflowDoc is a richer workflow shape than the package-shared
// `workflowDoc` (declared in build_workflow_vuln_gate_contract_test.go)
// because the BUG-029-004 contract needs the `services:` block of the
// integration job for structural-preservation assertions
// (DD-8 sub-test inside parent + DoD B / SCN-029-004-B).
type ciWorkflowDoc struct {
	Jobs map[string]ciJobDoc `yaml:"jobs"`
}

type ciJobDoc struct {
	Needs    interface{}            `yaml:"needs"`
	Services map[string]interface{} `yaml:"services"`
	Steps    []ciStepDoc            `yaml:"steps"`
}

type ciStepDoc struct {
	Name string                 `yaml:"name"`
	ID   string                 `yaml:"id"`
	Uses string                 `yaml:"uses"`
	With map[string]interface{} `yaml:"with"`
	Run  string                 `yaml:"run"`
}

// loadCIWorkflow reads and parses the live `.github/workflows/ci.yml`
// from the repo root resolved via runtime.Caller. Mirrors the
// loadBuildWorkflow pattern in build_workflow_vuln_gate_contract_test.go.
func loadCIWorkflow(t *testing.T) *ciWorkflowDoc {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc ciWorkflowDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return &doc
}

// dockerPushLineRE matches a shell-command line whose first non-whitespace
// content is `docker push` followed by a word boundary. Sub-test A.
var dockerPushLineRE = regexp.MustCompile(`^\s*docker\s+push\b`)

// dockerTagCrossRegistryLineRE matches a shell-command line whose first
// non-whitespace content is `docker tag <source> <destination>` where the
// destination begins with a known foreign-registry hostname (optionally
// wrapped in single or double quotes). Locally-named retags (no foreign-
// registry prefix in the destination) are exempt per design.md Q-2 — those
// are a CI-side smoke pattern, not a publish-mint. Sub-test B.
var dockerTagCrossRegistryLineRE = regexp.MustCompile(`^\s*docker\s+tag\s+\S+\s+["']?(ghcr\.io|gcr\.io|quay\.io|docker\.io)/`)

// isCommentLine reports whether the line's first non-whitespace character
// is `#` (i.e., the entire line is a YAML/shell comment). Inline comments
// after a command (e.g., `docker push X # comment`) are NOT exempt — those
// lines still represent live shell commands.
func isCommentLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "#")
}

// assertCIWorkflowStructure asserts the ci.yml workflow continues to
// contain the lint-and-test, build, and integration jobs with their
// expected child structure. Removing the parallel publish steps must
// NOT inadvertently damage adjacent surfaces (DoD B / SCN-029-004-B).
func assertCIWorkflowStructure(doc *ciWorkflowDoc) error {
	if _, ok := doc.Jobs["lint-and-test"]; !ok {
		return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: required job %q missing from ci.yml", "lint-and-test")
	}
	buildJob, ok := doc.Jobs["build"]
	if !ok {
		return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: required job %q missing from ci.yml", "build")
	}
	hasBuildDocker := false
	for _, step := range buildJob.Steps {
		if step.Name == "Build Docker images" {
			hasBuildDocker = true
			break
		}
	}
	if !hasBuildDocker {
		return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: required step %q missing from build job in ci.yml (the integration job's `needs: build` chain depends on this step's existence)", "Build Docker images")
	}
	intJob, ok := doc.Jobs["integration"]
	if !ok {
		return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: required job %q missing from ci.yml", "integration")
	}
	if _, ok := intJob.Services["postgres"]; !ok {
		return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: integration job's `services:` block must name a %q service", "postgres")
	}
	hasMigrate := false
	hasIntegrationTest := false
	for _, step := range intJob.Steps {
		if strings.Contains(step.Run, "cmd/dbmigrate") {
			hasMigrate = true
		}
		if strings.Contains(step.Run, "go test -tags=integration") || strings.Contains(step.Run, "smackerel.sh test integration") {
			hasIntegrationTest = true
		}
	}
	if !hasMigrate {
		return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: integration job must contain a step that runs db migrations (run: containing %q)", "cmd/dbmigrate")
	}
	if !hasIntegrationTest {
		return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: integration job must contain a step that executes the integration test command (run: containing %q or %q)", "go test -tags=integration", "smackerel.sh test integration")
	}
	return nil
}

// assertNoDockerPush asserts no step's run: block contains a `docker push`
// shell command (sub-test A / SCN-029-004-A primary dimension).
func assertNoDockerPush(doc *ciWorkflowDoc) error {
	for jobName, job := range doc.Jobs {
		for _, step := range job.Steps {
			for i, line := range strings.Split(step.Run, "\n") {
				if isCommentLine(line) {
					continue
				}
				if dockerPushLineRE.MatchString(line) {
					return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: step %q in job %q contains forbidden 'docker push' at run-block line %d (%q) — this is the parallel publish path that build.yml's signed/attested digest-pinned chain replaces",
						step.Name, jobName, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
	return nil
}

// assertNoGhcrTagging asserts no step's run: block contains a `docker tag`
// shell command whose destination begins with a known foreign-registry
// prefix (sub-test B / SCN-029-004-A cross-registry mint dimension).
// Locally-named retags (e.g., `docker tag smackerel-core:latest
// smackerel-core:test`) are exempt because they are a CI-side smoke
// pattern, not a publish-mint (per design.md Q-2 resolution).
func assertNoGhcrTagging(doc *ciWorkflowDoc) error {
	for jobName, job := range doc.Jobs {
		for _, step := range job.Steps {
			for i, line := range strings.Split(step.Run, "\n") {
				if isCommentLine(line) {
					continue
				}
				if dockerTagCrossRegistryLineRE.MatchString(line) {
					return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: step %q in job %q contains forbidden cross-registry 'docker tag <local> <foreign-registry>/...' at run-block line %d (%q) — local-only retags are exempt; only foreign-registry destinations are publish-mints",
						step.Name, jobName, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
	return nil
}

// assertNoGhcrLogin asserts no step uses docker/login-action against
// the ghcr.io registry, either literally or via the `${{ env.REGISTRY }}`
// indirection that build.yml uses (sub-test C / SCN-029-004-A login
// dimension).
func assertNoGhcrLogin(doc *ciWorkflowDoc) error {
	for jobName, job := range doc.Jobs {
		for _, step := range job.Steps {
			if !strings.HasPrefix(step.Uses, "docker/login-action@") {
				continue
			}
			registry, _ := step.With["registry"].(string)
			if registry == "ghcr.io" || registry == "${{ env.REGISTRY }}" {
				return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: step %q in job %q is a docker/login-action against ghcr.io (registry=%q) — only build.yml may log into ghcr.io for publishing",
					step.Name, jobName, registry)
			}
		}
	}
	return nil
}

// assertNoParallelPublishPath runs the structural pre-check first
// (DoD B / SCN-029-004-B) and then the three forbidden-construct
// invariants (DoD A / SCN-029-004-A) in order. It returns the first
// error found.
func assertNoParallelPublishPath(doc *ciWorkflowDoc) error {
	if err := assertCIWorkflowStructure(doc); err != nil {
		return err
	}
	if err := assertNoDockerPush(doc); err != nil {
		return err
	}
	if err := assertNoGhcrTagging(doc); err != nil {
		return err
	}
	if err := assertNoGhcrLogin(doc); err != nil {
		return err
	}
	return nil
}

// TestCIWorkflow_NoParallelPublishPath_PostBUG029004 verifies the live
// `.github/workflows/ci.yml` satisfies the BUG-029-004 contract:
//   - structural-preservation invariants (DoD B / SCN-029-004-B)
//   - sub-test A: no `docker push` shell commands (SCN-029-004-A)
//   - sub-test B: no cross-registry `docker tag` mints (SCN-029-004-A)
//   - sub-test C: no `docker/login-action` against ghcr.io (SCN-029-004-A)
func TestCIWorkflow_NoParallelPublishPath_PostBUG029004(t *testing.T) {
	doc := loadCIWorkflow(t)

	// Structural pre-check (DoD B / SCN-029-004-B): if any required
	// job/step is missing, fail before checking the forbidden-construct
	// invariants. This proves removal of the parallel publish steps
	// did not over-reach into adjacent surfaces.
	if err := assertCIWorkflowStructure(doc); err != nil {
		t.Fatalf("structural-preservation contract violation: %v", err)
	}

	t.Run("A_no_docker_push_in_ci_yml", func(t *testing.T) {
		if err := assertNoDockerPush(doc); err != nil {
			t.Fatalf("BUG-029-004 sub-test A: %v", err)
		}
		t.Logf("sub-test A OK: ci.yml contains zero `docker push` shell commands in any step's run: block")
	})

	t.Run("B_no_ghcr_tagging_in_ci_yml", func(t *testing.T) {
		if err := assertNoGhcrTagging(doc); err != nil {
			t.Fatalf("BUG-029-004 sub-test B: %v", err)
		}
		t.Logf("sub-test B OK: ci.yml contains zero cross-registry `docker tag <local> <foreign-registry>/...` mints in any step's run: block")
	})

	t.Run("C_no_ghcr_login_in_ci_yml", func(t *testing.T) {
		if err := assertNoGhcrLogin(doc); err != nil {
			t.Fatalf("BUG-029-004 sub-test C: %v", err)
		}
		t.Logf("sub-test C OK: ci.yml contains zero docker/login-action steps targeting the ghcr.io registry")
	})
}

// minimalValidWorkflowDoc constructs an in-memory ciWorkflowDoc that
// passes the structural pre-check (lint-and-test + build with `Build
// Docker images` + integration with services.postgres + dbmigrate +
// integration test). Adversarial mutation tests start from this base
// and inject ONE forbidden construct each, so the structural pre-check
// does not mask the forbidden-construct check.
func minimalValidWorkflowDoc() *ciWorkflowDoc {
	return &ciWorkflowDoc{
		Jobs: map[string]ciJobDoc{
			"lint-and-test": {
				Steps: []ciStepDoc{{Name: "Lint"}},
			},
			"build": {
				Steps: []ciStepDoc{
					{Name: "Build Docker images", Run: "./smackerel.sh build"},
				},
			},
			"integration": {
				Services: map[string]interface{}{"postgres": map[string]interface{}{}},
				Steps: []ciStepDoc{
					{Name: "Apply database migrations", Run: "go run ./cmd/dbmigrate"},
					{Name: "Run integration tests", Run: "go test -tags=integration ./tests/integration/..."},
				},
			},
		},
	}
}

// TestCIWorkflow_AdversarialDockerPushReintroduced proves the validator
// catches a regression that re-introduces a `docker push ghcr.io/...`
// line into any step's run: block (sub-test A regression vector).
func TestCIWorkflow_AdversarialDockerPushReintroduced(t *testing.T) {
	doc := minimalValidWorkflowDoc()
	job := doc.Jobs["build"]
	job.Steps = append(job.Steps, ciStepDoc{
		Name: "Adversarial: re-introduce parallel push to ghcr.io",
		Run: `VERSION="${GITHUB_REF#refs/tags/}"
docker push ghcr.io/${{ github.repository_owner }}/smackerel-core:${VERSION}`,
	})
	doc.Jobs["build"] = job

	err := assertNoParallelPublishPath(doc)
	if err == nil {
		t.Fatal("expected adversarial doc (re-introduced docker push) to FAIL contract, but it PASSED — validator is tautological")
	}
	if !strings.Contains(err.Error(), "BUG-029-004") {
		t.Fatalf("expected error message to name BUG-029-004, got: %v", err)
	}
	if !strings.Contains(err.Error(), "docker push") {
		t.Fatalf("expected error message to name 'docker push' as the offending construct, got: %v", err)
	}
	t.Logf("adversarial OK: re-introduced `docker push ghcr.io/...` is rejected with: %v", err)
}

// TestCIWorkflow_AdversarialGhcrTaggingReintroduced proves the validator
// catches a regression that re-introduces a cross-registry `docker tag
// <local> ghcr.io/...` line (sub-test B regression vector).
func TestCIWorkflow_AdversarialGhcrTaggingReintroduced(t *testing.T) {
	doc := minimalValidWorkflowDoc()
	job := doc.Jobs["build"]
	job.Steps = append(job.Steps, ciStepDoc{
		Name: "Adversarial: re-introduce cross-registry docker tag",
		Run: `VERSION="${GITHUB_REF#refs/tags/}"
docker tag smackerel-core:latest ghcr.io/${{ github.repository_owner }}/smackerel-core:${VERSION}`,
	})
	doc.Jobs["build"] = job

	err := assertNoParallelPublishPath(doc)
	if err == nil {
		t.Fatal("expected adversarial doc (re-introduced cross-registry docker tag) to FAIL contract, but it PASSED — validator is tautological")
	}
	if !strings.Contains(err.Error(), "BUG-029-004") {
		t.Fatalf("expected error message to name BUG-029-004, got: %v", err)
	}
	if !strings.Contains(err.Error(), "docker tag") {
		t.Fatalf("expected error message to name 'docker tag' as the offending construct, got: %v", err)
	}
	t.Logf("adversarial OK: re-introduced `docker tag <local> ghcr.io/...` is rejected with: %v", err)
}

// TestCIWorkflow_AdversarialGhcrLoginReintroduced proves the validator
// catches a regression that re-introduces a `docker/login-action`
// against the ghcr.io registry (sub-test C regression vector).
func TestCIWorkflow_AdversarialGhcrLoginReintroduced(t *testing.T) {
	doc := minimalValidWorkflowDoc()
	job := doc.Jobs["build"]
	job.Steps = append(job.Steps, ciStepDoc{
		Name: "Adversarial: re-introduce ghcr.io login",
		Uses: "docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9",
		With: map[string]interface{}{
			"registry": "ghcr.io",
			"username": "${{ github.actor }}",
			"password": "${{ secrets.GITHUB_TOKEN }}",
		},
	})
	doc.Jobs["build"] = job

	err := assertNoParallelPublishPath(doc)
	if err == nil {
		t.Fatal("expected adversarial doc (re-introduced ghcr.io login) to FAIL contract, but it PASSED — validator is tautological")
	}
	if !strings.Contains(err.Error(), "BUG-029-004") {
		t.Fatalf("expected error message to name BUG-029-004, got: %v", err)
	}
	if !strings.Contains(err.Error(), "docker/login-action") {
		t.Fatalf("expected error message to name 'docker/login-action' as the offending construct, got: %v", err)
	}
	t.Logf("adversarial OK: re-introduced docker/login-action against ghcr.io is rejected with: %v", err)
}
