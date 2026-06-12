// Package deploy — BUG-073-003 (cross-language renderer canary CI toolchain gating).
//
// Static-file contract for `.github/workflows/ci.yml`. The spec-073
// cross-language renderer canary
// (tests/unit/clients/render_descriptor_canary_test.go, TP-073-03) skips in the
// shared Go-only `lint-and-test` unit container (no node/dart). To keep
// cross-language drift detection ACTIVE in CI, ci.yml MUST carry a dedicated
// `cross-language-canary` job that provisions node (actions/setup-node) +
// Flutter (subosito/flutter-action; the dart package declares a flutter SDK
// dependency) and runs the canary via `go test`.
//
// This contract is the keystone that satisfies the BUG-073-003 hard
// requirement: "the cross-language canary must never silently stop running in
// CI." If the job is deleted, renamed, or stops provisioning either toolchain,
// or stops running the canary, this test FAILS the `lint-and-test` lane (it only
// parses YAML, so it runs in the Go-only container alongside the rest of the
// unit suite).
//
// Adversarial in-memory mutation tests prove the validator catches regressions
// (mirrors the sibling pattern in ci_workflow_no_parallel_publish_test.go).
//
// References:
//   - specs/073-web-mobile-assistant-frontend/bugs/BUG-073-003-canary-ci-toolchain-gating/design.md
//   - tests/unit/clients/render_descriptor_canary_test.go (the canary + decideRenderToolchain)
package deploy

import (
	"fmt"
	"strings"
	"testing"
)

// assertCrossLanguageCanaryCIJob verifies ci.yml wires the BUG-073-003
// dedicated cross-language canary lane: a job named `cross-language-canary`
// that provisions node + Flutter and runs the canary go test.
func assertCrossLanguageCanaryCIJob(doc *ciWorkflowDoc) error {
	job, ok := doc.Jobs["cross-language-canary"]
	if !ok {
		return fmt.Errorf("BUG-073-003 contract violation: ci.yml MUST define the `cross-language-canary` job so the spec-073 cross-language renderer canary runs in CI with node+dart (the shared Go-only unit lane skips it). The canary must never silently stop running in CI")
	}
	hasNode := false
	hasFlutter := false
	hasCanaryRun := false
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/setup-node@") {
			hasNode = true
		}
		if strings.HasPrefix(step.Uses, "subosito/flutter-action@") {
			hasFlutter = true
		}
		if strings.Contains(step.Run, "go test") &&
			strings.Contains(step.Run, "TestRenderDescriptorV1_CrossLanguageCanary") &&
			strings.Contains(step.Run, "tests/unit/clients") {
			hasCanaryRun = true
		}
	}
	if !hasNode {
		return fmt.Errorf("BUG-073-003 contract violation: `cross-language-canary` job MUST provision node via actions/setup-node (the JS renderer half of the canary)")
	}
	if !hasFlutter {
		return fmt.Errorf("BUG-073-003 contract violation: `cross-language-canary` job MUST provision Flutter via subosito/flutter-action (the Flutter-bundled dart renderer half; the clients/mobile/assistant package declares a flutter SDK dependency)")
	}
	if !hasCanaryRun {
		return fmt.Errorf("BUG-073-003 contract violation: `cross-language-canary` job MUST run the canary (a step whose run contains `go test` + `TestRenderDescriptorV1_CrossLanguageCanary` + `tests/unit/clients`)")
	}
	return nil
}

// TestCrossLanguageCanaryCIJob_LiveFile verifies the live ci.yml satisfies the
// BUG-073-003 contract.
func TestCrossLanguageCanaryCIJob_LiveFile(t *testing.T) {
	doc := loadCIWorkflow(t)
	if err := assertCrossLanguageCanaryCIJob(doc); err != nil {
		t.Fatal(err)
	}
}

// TestCrossLanguageCanaryCIJob_AdversarialMissingJob proves the validator
// rejects a ci.yml that dropped the canary job (the silent-disable regression).
func TestCrossLanguageCanaryCIJob_AdversarialMissingJob(t *testing.T) {
	doc := &ciWorkflowDoc{Jobs: map[string]ciJobDoc{"lint-and-test": {}}}
	if err := assertCrossLanguageCanaryCIJob(doc); err == nil {
		t.Fatal("validator must reject a ci.yml missing the cross-language-canary job")
	}
}

// TestCrossLanguageCanaryCIJob_AdversarialMissingFlutter proves the validator
// rejects a canary job that runs the canary but does not provision Flutter
// (so the dart renderer half would be absent and the canary would skip).
func TestCrossLanguageCanaryCIJob_AdversarialMissingFlutter(t *testing.T) {
	doc := &ciWorkflowDoc{Jobs: map[string]ciJobDoc{
		"cross-language-canary": {Steps: []ciStepDoc{
			{Uses: "actions/setup-node@deadbeef"},
			{Run: "go test -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/..."},
		}},
	}}
	if err := assertCrossLanguageCanaryCIJob(doc); err == nil {
		t.Fatal("validator must reject a cross-language-canary job that does not provision Flutter")
	}
}

// TestCrossLanguageCanaryCIJob_AdversarialMissingCanaryRun proves the validator
// rejects a job that provisions the toolchains but never runs the canary.
func TestCrossLanguageCanaryCIJob_AdversarialMissingCanaryRun(t *testing.T) {
	doc := &ciWorkflowDoc{Jobs: map[string]ciJobDoc{
		"cross-language-canary": {Steps: []ciStepDoc{
			{Uses: "actions/setup-node@deadbeef"},
			{Uses: "subosito/flutter-action@deadbeef"},
			{Run: "echo no canary here"},
		}},
	}}
	if err := assertCrossLanguageCanaryCIJob(doc); err == nil {
		t.Fatal("validator must reject a cross-language-canary job that does not run the canary go test")
	}
}
