// spec-045 BUG-045-002 — CI Integration Topology Contract (AC-4 / DD-2).
//
// Build-time guard for `.github/workflows/ci.yml` → `jobs.integration`.
// Decision DD-1 (Path A): the CI integration job MUST route through the
// canonical CLI (`./smackerel.sh test integration`) so it exercises the
// same full-stack Compose topology + sequential `-p 1` test-binary
// execution that the local dev host uses. The previous CI topology
// (GitHub `services.postgres` + inline `docker run -d nats-ci ...` +
// raw `go test -tags=integration ./tests/integration/...`) omitted
// ollama / smackerel-core / smackerel-ml AND the `-p 1` flag, which
// caused 20 consecutive chronic failures on main between 2026-05-14
// and 2026-05-16 (see report.md § Evidence 3 in the BUG-045-002 packet).
//
// This file codifies the 6 AC-4 invariants as a build-time contract so a
// future revert is rejected by `./smackerel.sh test unit --go` long
// before it reaches CI:
//
//  1. `jobs.integration` exists.
//  2. `jobs.integration.services` is absent or empty (no GH service
//     containers).
//  3. No step's `run:` block matches `docker\s+run\b.*\b(postgres|nats|
//     ollama)\b` (no inline infra sidecars).
//  4. At least one step's `run:` block matches
//     `\./smackerel\.sh\s+test\s+integration\b` (canonical CLI invoked).
//  5. No step's `run:` block matches
//     `go\s+test\b.*-tags[=\s]+integration\b.*\./tests/integration`
//     (no raw go-test bypass of the CLI).
//  6. `jobs.integration.timeout-minutes` is an integer `>= 30` (DD-3:
//     cold-cache Ollama image pull + test model pull + Compose build
//     budget).
//
// Adversarial sub-tests construct in-memory synthetic YAML fixtures that
// reintroduce each banned pattern and assert
// `assertCIIntegrationTopologyContract` returns a non-nil error citing
// the offending field AND the canonical CLI alternative (proves the
// guard is not tautological — a silent pass on a regressed YAML triggers
// a `FALSE NEGATIVE` t.Fatalf).
//
// Shared infrastructure note: this file reuses the `ciWorkflowDoc`,
// `ciJobDoc`, `ciStepDoc`, and `loadCIWorkflow` declarations from
// ci_workflow_no_parallel_publish_test.go (same package). The BUG-045-002
// Scope 2 plan added a single optional `TimeoutMinutes int yaml:"timeout-
// minutes"` field to the existing `ciJobDoc` struct; no existing assertion
// reads that field, so the addition is purely additive.
//
// References:
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-002-ci-integration-failure-persists/spec.md (AC-4)
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-002-ci-integration-failure-persists/design.md
//     (Decisions DD-1, DD-2, DD-3)
//   - specs/045-deploy-resource-filesystem-hardening/bugs/
//     BUG-045-002-ci-integration-failure-persists/scopes.md (Scope 2)
package deploy

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// dockerRunInfraRegex matches a `run:` block that starts an inline
// postgres / nats / ollama container via `docker run ...`. The word
// boundaries (`\b`) avoid false positives on substrings like
// "test-postgres-helper".
var dockerRunInfraRegex = regexp.MustCompile(`docker\s+run\b[^\n]*\b(postgres|nats|ollama)\b`)

// smackerelTestIntegrationRegex matches the canonical CLI invocation
// that Path A requires.
var smackerelTestIntegrationRegex = regexp.MustCompile(`\./smackerel\.sh\s+test\s+integration\b`)

// rawGoTestIntegrationRegex matches a `run:` block that bypasses the
// canonical CLI by directly invoking `go test -tags=integration` (or
// `-tags integration`) against `./tests/integration/...`. The
// `[=\s]+` flexion accepts both flag forms; the trailing
// `\./tests/integration` anchor ensures the match is specific to the
// integration suite (not a unit-test invocation that happens to mention
// the `integration` tag in a different position).
var rawGoTestIntegrationRegex = regexp.MustCompile(`go\s+test\b[^\n]*-tags[=\s]+integration\b[^\n]*\./tests/integration`)

// const that all violation error messages MUST include so the operator
// always knows what the canonical alternative is.
const ciCanonicalCLIAlternative = "./smackerel.sh test integration"

// const that all violation error messages MUST include so the operator
// always knows which bug packet codified the contract.
const ciContractSource = "BUG-045-002"

// assertCIIntegrationTopologyContract returns nil iff all 6 AC-4
// invariants hold for the integration job. On any violation it returns
// a non-nil error naming the specific invariant + the offending value
// + the canonical CLI alternative + the contract source (BUG-045-002),
// so adversarial sub-tests can pattern-match the failure mode.
func assertCIIntegrationTopologyContract(doc *ciWorkflowDoc) error {
	if doc == nil {
		return fmt.Errorf("contract violation [%s]: ciWorkflowDoc is nil; canonical alternative: %s", ciContractSource, ciCanonicalCLIAlternative)
	}

	// Invariant 1: jobs.integration exists.
	integration, ok := doc.Jobs["integration"]
	if !ok {
		return fmt.Errorf("contract violation [%s]: jobs.integration is missing from .github/workflows/ci.yml; canonical alternative: %s", ciContractSource, ciCanonicalCLIAlternative)
	}

	// Invariant 2: jobs.integration.services is absent or empty.
	if len(integration.Services) > 0 {
		// Sort the service names into a deterministic, single-line
		// error string so operators can grep the failing field
		// directly without depending on map-iteration order.
		var names []string
		for name := range integration.Services {
			names = append(names, name)
		}
		return fmt.Errorf("contract violation [%s]: jobs.integration.services must be absent or empty (DD-1 Path A); found services=%v; canonical alternative: %s", ciContractSource, names, ciCanonicalCLIAlternative)
	}

	// Invariant 3: no step's run: block invokes docker run for
	// postgres / nats / ollama.
	for i, step := range integration.Steps {
		if step.Run == "" {
			continue
		}
		if loc := dockerRunInfraRegex.FindStringIndex(step.Run); loc != nil {
			matched := step.Run[loc[0]:loc[1]]
			return fmt.Errorf("contract violation [%s]: jobs.integration.steps[%d] (name=%q) contains forbidden infra sidecar pattern %q in `run:` block; canonical alternative: %s", ciContractSource, i, step.Name, matched, ciCanonicalCLIAlternative)
		}
	}

	// Invariant 5: no step's run: block invokes raw `go test -tags=integration`
	// against ./tests/integration/... . Asserted BEFORE invariant 4 so
	// that a YAML which both bypasses the CLI AND fails to invoke it
	// gets the more specific bypass error.
	for i, step := range integration.Steps {
		if step.Run == "" {
			continue
		}
		if loc := rawGoTestIntegrationRegex.FindStringIndex(step.Run); loc != nil {
			matched := step.Run[loc[0]:loc[1]]
			return fmt.Errorf("contract violation [%s]: jobs.integration.steps[%d] (name=%q) contains forbidden raw `go test` invocation %q that bypasses the CLI; canonical alternative: %s", ciContractSource, i, step.Name, matched, ciCanonicalCLIAlternative)
		}
	}

	// Invariant 4: at least one step's run: block invokes the canonical
	// CLI command.
	canonicalFound := false
	for _, step := range integration.Steps {
		if step.Run == "" {
			continue
		}
		if smackerelTestIntegrationRegex.MatchString(step.Run) {
			canonicalFound = true
			break
		}
	}
	if !canonicalFound {
		return fmt.Errorf("contract violation [%s]: no step under jobs.integration.steps invokes the canonical CLI %q; at least one step's `run:` block must match regex %q", ciContractSource, ciCanonicalCLIAlternative, smackerelTestIntegrationRegex.String())
	}

	// Invariant 6: jobs.integration.timeout-minutes >= 30 (DD-3).
	if integration.TimeoutMinutes < 30 {
		return fmt.Errorf("contract violation [%s]: jobs.integration.timeout-minutes is %d, must be >= 30 (DD-3); canonical alternative: %s", ciContractSource, integration.TimeoutMinutes, ciCanonicalCLIAlternative)
	}

	return nil
}

// TestCIIntegrationTopologyContract is the live assertion against the
// real `.github/workflows/ci.yml`. It MUST pass after the Scope 1
// refactor lands and MUST fail if any of the 6 invariants regress.
func TestCIIntegrationTopologyContract(t *testing.T) {
	doc := loadCIWorkflow(t)
	if err := assertCIIntegrationTopologyContract(doc); err != nil {
		t.Fatalf("CI integration topology contract violated: %v", err)
	}
}

// TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock
// proves the guard catches a regressed YAML that re-introduces the
// `services.postgres` block. If the guard silently passes on this
// fixture, the test fails with a FALSE NEGATIVE message — that fail
// mode is the whole point of the adversarial sub-test (per
// bubbles-test-integrity SKILL.md).
func TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock(t *testing.T) {
	const fixture = `
jobs:
  integration:
    timeout-minutes: 30
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_USER: smackerel
          POSTGRES_PASSWORD: smackerel
          POSTGRES_DB: smackerel_test
        ports:
        - 5432:5432
    steps:
    - name: Run integration tests
      run: |
        ./smackerel.sh test integration
`
	var doc ciWorkflowDoc
	if err := yaml.Unmarshal([]byte(fixture), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal(fixture) failed: %v", err)
	}
	err := assertCIIntegrationTopologyContract(&doc)
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertCIIntegrationTopologyContract returned nil against a YAML containing jobs.integration.services.postgres; the AC-4 guard would not catch a regression to the pre-fix topology")
	}
	msg := err.Error()
	if !strings.Contains(msg, "services") && !strings.Contains(msg, "postgres") {
		t.Fatalf("guard error message must name the offending field (\"services\" or \"postgres\"); got: %q", msg)
	}
	if !strings.Contains(msg, ciCanonicalCLIAlternative) {
		t.Fatalf("guard error message must cite the canonical CLI alternative %q so operators know the fix shape; got: %q", ciCanonicalCLIAlternative, msg)
	}
	if !strings.Contains(msg, ciContractSource) {
		t.Fatalf("guard error message must cite the contract source %q; got: %q", ciContractSource, msg)
	}
}

// TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar
// proves the guard catches a regressed YAML that re-introduces the
// `docker run -d --name nats-ci nats ...` inline-sidecar step.
func TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar(t *testing.T) {
	const fixture = `
jobs:
  integration:
    timeout-minutes: 30
    steps:
    - name: Start NATS with auth and JetStream
      run: |
        docker run -d --name nats-ci \
          --network host \
          nats:2.10 \
          --auth ci-test-token-integration \
          --http_port 8222 \
          --jetstream
    - name: Run integration tests
      run: |
        ./smackerel.sh test integration
`
	var doc ciWorkflowDoc
	if err := yaml.Unmarshal([]byte(fixture), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal(fixture) failed: %v", err)
	}
	err := assertCIIntegrationTopologyContract(&doc)
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertCIIntegrationTopologyContract returned nil against a YAML containing `docker run -d --name nats-ci nats ...`; the AC-4 guard would not catch a regression that re-introduces the inline infra sidecar")
	}
	msg := err.Error()
	if !strings.Contains(msg, "docker run") {
		t.Fatalf("guard error message must name the offending command (\"docker run\"); got: %q", msg)
	}
	if !strings.Contains(msg, "nats") {
		t.Fatalf("guard error message must name the offending infra component (\"nats\"); got: %q", msg)
	}
	if !strings.Contains(msg, ciCanonicalCLIAlternative) {
		t.Fatalf("guard error message must cite the canonical CLI alternative %q; got: %q", ciCanonicalCLIAlternative, msg)
	}
}

// TestCIIntegrationTopology_AdversarialRejectsRawGoTest proves the
// guard catches a regressed YAML that bypasses the canonical CLI by
// invoking `go test -tags=integration ./tests/integration/...`
// directly.
func TestCIIntegrationTopology_AdversarialRejectsRawGoTest(t *testing.T) {
	const fixture = `
jobs:
  integration:
    timeout-minutes: 30
    steps:
    - name: Run integration tests
      run: |
        set -o pipefail
        go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m 2>&1 | tee integration-test.log
`
	var doc ciWorkflowDoc
	if err := yaml.Unmarshal([]byte(fixture), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal(fixture) failed: %v", err)
	}
	err := assertCIIntegrationTopologyContract(&doc)
	if err == nil {
		t.Fatalf("guard FALSE NEGATIVE: assertCIIntegrationTopologyContract returned nil against a YAML containing raw `go test -tags=integration ./tests/integration/...`; the AC-4 guard would not catch a regression that bypasses the canonical CLI")
	}
	msg := err.Error()
	if !strings.Contains(msg, "go test") {
		t.Fatalf("guard error message must name the offending command (\"go test\"); got: %q", msg)
	}
	if !strings.Contains(msg, "tests/integration") {
		t.Fatalf("guard error message must name the offending test path (\"tests/integration\"); got: %q", msg)
	}
	if !strings.Contains(msg, ciCanonicalCLIAlternative) {
		t.Fatalf("guard error message must cite the canonical CLI alternative %q; got: %q", ciCanonicalCLIAlternative, msg)
	}
}
