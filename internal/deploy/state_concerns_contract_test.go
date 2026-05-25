// Package deploy contains static-file invariant tests for the deployment
// compose contract and the artifact-integrity contracts enforced by Bubbles
// completion-governance.
//
// This file enforces the done_with_concerns concerns-array schema for the
// spec 055 state.json files. See completion-governance.md (under
// .github/agents/bubbles_shared/) for the full schema definition. The
// contract:
//
//  1. When a state.json declares `status` OR `certification.status` equal to
//     "done_with_concerns", the file MUST carry a non-empty
//     `certification.concerns` array.
//  2. Every entry in `certification.concerns` MUST be a JSON object (NOT a
//     flat string).
//  3. Every entry MUST include the keys `id`, `severity`, `summary`,
//     `followUpOwner`, `followUpAction`.
//  4. `severity` MUST be exactly "low" or "medium". The values "high",
//     "critical", "" (empty), or any other string are rejected — anything
//     that would warrant "high" is a real gate failure and must use
//     `blocked`, not `done_with_concerns`.
//  5. `followUpAction` MUST be exactly one of "new-spec", "issue-doc",
//     "next-sprint-todo", or "accept".
//  6. `followUpOwner` MUST be a non-empty string. The literals "tbd" and
//     "everyone" are rejected (the schema requires a concrete owner).
//  7. `id` MUST be unique within its own state.json file's concerns array.
//
// This contract is enforced on two files that the round-6 devops sweep
// probe surfaced as schema-non-compliant:
//
//   - specs/055-notification-source-ntfy-adapter/state.json (parent)
//   - specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json (child bug)
//
// Other state.json files in the repo are deliberately out of scope here —
// broader enforcement belongs upstream in the framework-managed
// state-transition-guard.sh.
//
// References:
//   - .github/agents/bubbles_shared/completion-governance.md (canonical schema)
//   - specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001/
package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRootForConcernsTest walks up from this file's directory to the repo
// root (the directory containing go.mod) so the test can resolve
// repo-relative paths regardless of where `go test` is invoked from.
func repoRootForConcernsTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("repoRootForConcernsTest: runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("repoRootForConcernsTest: could not locate repo root from %s", file)
	return ""
}

// concernsContractCase describes one state.json file the contract test
// validates.
type concernsContractCase struct {
	label        string
	relativePath string
}

// validateConcernsContract loads a single state.json file and asserts the
// concerns-array schema. It returns a list of human-readable error messages;
// an empty slice means the file passes.
func validateConcernsContract(absPath string) []string {
	var errors []string

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return []string{fmt.Sprintf("read %s: %v", absPath, err)}
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return []string{fmt.Sprintf("unmarshal %s: %v", absPath, err)}
	}

	topStatus, _ := doc["status"].(string)
	cert, _ := doc["certification"].(map[string]any)
	certStatus, _ := cert["status"].(string)

	// Schema only applies when either status field is "done_with_concerns".
	if topStatus != "done_with_concerns" && certStatus != "done_with_concerns" {
		return nil
	}

	concernsRaw, ok := cert["concerns"]
	if !ok {
		errors = append(errors, fmt.Sprintf("%s: status is done_with_concerns but certification.concerns is missing entirely (completion-governance.md requires a non-empty structured array)", absPath))
		return errors
	}

	concerns, ok := concernsRaw.([]any)
	if !ok {
		errors = append(errors, fmt.Sprintf("%s: certification.concerns is not a JSON array (got %T)", absPath, concernsRaw))
		return errors
	}
	if len(concerns) == 0 {
		errors = append(errors, fmt.Sprintf("%s: certification.concerns is an empty array but status is done_with_concerns (completion-governance.md requires a non-empty array)", absPath))
		return errors
	}

	allowedSeverity := map[string]struct{}{
		"low":    {},
		"medium": {},
	}
	allowedAction := map[string]struct{}{
		"new-spec":         {},
		"issue-doc":        {},
		"next-sprint-todo": {},
		"accept":           {},
	}
	rejectedOwner := map[string]struct{}{
		"tbd":      {},
		"everyone": {},
	}
	requiredKeys := []string{"id", "severity", "summary", "followUpOwner", "followUpAction"}

	seenIDs := map[string]int{}

	for i, entry := range concerns {
		obj, ok := entry.(map[string]any)
		if !ok {
			errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] is not a JSON object (got %T — flat strings are forbidden by the structured-shape rule)", absPath, i, entry))
			continue
		}
		for _, key := range requiredKeys {
			if _, present := obj[key]; !present {
				errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] is missing required key %q", absPath, i, key))
			}
		}
		id, _ := obj["id"].(string)
		if strings.TrimSpace(id) == "" {
			errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] has empty or non-string id", absPath, i))
		} else {
			if prev, dup := seenIDs[id]; dup {
				errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] id %q duplicates entry %d (ids must be unique within the array)", absPath, i, id, prev))
			} else {
				seenIDs[id] = i
			}
		}
		sev, _ := obj["severity"].(string)
		if _, ok := allowedSeverity[sev]; !ok {
			errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] severity %q is not in {low, medium} (anything warranting high MUST use status=blocked instead)", absPath, i, sev))
		}
		summary, _ := obj["summary"].(string)
		if strings.TrimSpace(summary) == "" {
			errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] summary is empty", absPath, i))
		}
		owner, _ := obj["followUpOwner"].(string)
		ownerTrim := strings.TrimSpace(owner)
		if ownerTrim == "" {
			errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] followUpOwner is empty", absPath, i))
		} else if _, rejected := rejectedOwner[strings.ToLower(ownerTrim)]; rejected {
			errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] followUpOwner %q is forbidden (concrete agent name or the literal \"human\" required)", absPath, i, owner))
		}
		action, _ := obj["followUpAction"].(string)
		if _, ok := allowedAction[action]; !ok {
			errors = append(errors, fmt.Sprintf("%s: certification.concerns[%d] followUpAction %q is not in {new-spec, issue-doc, next-sprint-todo, accept}", absPath, i, action))
		}
	}

	return errors
}

// TestSpec055StateConcernsContract enforces the done_with_concerns
// concerns-array schema on the spec 055 parent state.json and the
// BUG-CHAOS-20260524-001 child bug state.json. See file-level doc above
// for the full contract.
func TestSpec055StateConcernsContract(t *testing.T) {
	root := repoRootForConcernsTest(t)

	cases := []concernsContractCase{
		{
			label:        "spec055ParentState",
			relativePath: filepath.Join("specs", "055-notification-source-ntfy-adapter", "state.json"),
		},
		{
			label:        "spec055BugChaos20260524_001State",
			relativePath: filepath.Join("specs", "055-notification-source-ntfy-adapter", "bugs", "BUG-CHAOS-20260524-001", "state.json"),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			absPath := filepath.Join(root, tc.relativePath)
			if _, err := os.Stat(absPath); err != nil {
				t.Fatalf("state.json missing at %s: %v", tc.relativePath, err)
			}
			errs := validateConcernsContract(absPath)
			if len(errs) > 0 {
				t.Fatalf("state_concerns contract failed for %s:\n  - %s", tc.relativePath, strings.Join(errs, "\n  - "))
			}
		})
	}
}

// TestSpec055StateConcernsContractAdversarial proves the validator is not
// tautological by feeding it three crafted-broken state.json envelopes and
// asserting it rejects each one with a precise message.
func TestSpec055StateConcernsContractAdversarial(t *testing.T) {
	tempDir := t.TempDir()

	cases := []struct {
		name             string
		raw              string
		wantErrSubstring string
	}{
		{
			name: "missing-concerns-when-done-with-concerns",
			raw: `{
                "status": "done_with_concerns",
                "certification": {"status": "done_with_concerns"}
            }`,
			wantErrSubstring: "certification.concerns is missing",
		},
		{
			name: "string-entry-instead-of-object",
			raw: `{
                "status": "done_with_concerns",
                "certification": {
                    "status": "done_with_concerns",
                    "concerns": ["just a string note"]
                }
            }`,
			wantErrSubstring: "is not a JSON object",
		},
		{
			name: "invalid-severity-high",
			raw: `{
                "status": "done_with_concerns",
                "certification": {
                    "status": "done_with_concerns",
                    "concerns": [{
                        "id": "C-1",
                        "severity": "high",
                        "summary": "x",
                        "followUpOwner": "human",
                        "followUpAction": "accept"
                    }]
                }
            }`,
			wantErrSubstring: "severity \"high\" is not in {low, medium}",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tempDir, tc.name+".json")
			if err := os.WriteFile(path, []byte(tc.raw), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			errs := validateConcernsContract(path)
			if len(errs) == 0 {
				t.Fatalf("expected validator to reject %s but it accepted", tc.name)
			}
			joined := strings.Join(errs, "\n")
			if !strings.Contains(joined, tc.wantErrSubstring) {
				t.Fatalf("expected error containing %q, got:\n%s", tc.wantErrSubstring, joined)
			}
		})
	}
}
