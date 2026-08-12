// Package config — TP-05-02 contract test for the spec 108 SST key
// `auth.corpus_grant_enforcement`.
//
// The key selects the OBSERVE/ENFORCE stage of a security gate. Under
// .github/instructions/smackerel-no-defaults.instructions.md a security-stage
// selector must never be silently supplied by a fallback: if the SST key is
// missing, generation must abort loudly rather than resolve to some assumed
// value. A `${VAR:-false}` anywhere on the path would turn a deleted key into
// a silent, permanent OBSERVE — the gate would look configured and enforce
// nothing.
//
// Two distinct obligations, asserted separately so a failure names the right
// one:
//
//  1. DECLARED — `auth.corpus_grant_enforcement` exists in
//     `config/smackerel.yaml` with an explicit boolean literal. A key that is
//     absent, null, or empty is not a declaration.
//  2. NO DEFAULT ON THE RESOLUTION PATH — the generator reads it via
//     `required_value` (which aborts on a missing key) and emits it with the
//     fail-loud `${VAR:?...}` form. No `${VAR:-…}`, `${VAR-…}`, `${VAR:=…}`,
//     getenv-with-default, or `yaml_get … || VAR=""` shape may exist for this
//     key.
//
// The invariant is a pure function over text so adversarial fixtures are
// literals and the real generator is never mutated. Forbidden shapes are
// scanned over the FULL resolution-path text with no comment-skipping: the key
// appears exactly twice in `scripts/commands/config.sh`, neither in a comment,
// so the strictest scan is also the correct one and cannot false-negative on a
// violation parked after a `#`.
//
// Scope note: the Go-side resolver `cmd/core/wiring_corpus_grant.go` has its
// own fail-loud coverage under Scope 02 (TP-02-01/TP-02-02) and is not
// re-asserted here. The getenv-with-default shape is still refused by this
// contract because the resolution path is a text surface and a Go-side default
// would be exactly as fatal as a shell one.
package config

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	corpusGrantSSTKeyPath = "auth.corpus_grant_enforcement"
	corpusGrantSSTEnvVar  = "SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT"

	corpusGrantSSTRequiredValueCall = "required_value " + corpusGrantSSTKeyPath
	corpusGrantSSTFailLoudEmission  = "${" + corpusGrantSSTEnvVar + ":?"

	corpusGrantSSTYAMLRelpath      = "config/smackerel.yaml"
	corpusGrantSSTGeneratorRelpath = "scripts/commands/config.sh"
)

// corpusGrantSSTForbiddenShellDefaults are the shell expansions that would
// supply a value the SST never declared.
var corpusGrantSSTForbiddenShellDefaults = []string{
	"${" + corpusGrantSSTEnvVar + ":-",
	"${" + corpusGrantSSTEnvVar + "-",
	"${" + corpusGrantSSTEnvVar + ":=",
}

// corpusGrantSSTGetenvWithDefault matches a getenv-style read of the variable
// that carries a second (default) argument — `os.Getenv(k, "false")`,
// `getEnvOr("…", "false")`, and friends.
var corpusGrantSSTGetenvWithDefault = regexp.MustCompile(
	`(?i)getenv(?:or)?\(\s*"?` + regexp.QuoteMeta(corpusGrantSSTEnvVar) + `"?\s*,`)

// corpusGrantSSTFallbackRead matches a non-aborting read of the SST key that
// falls back on failure — the `yaml_get … || VAR=""` shape used elsewhere in
// the generator for genuinely optional keys. This key is not optional.
var corpusGrantSSTFallbackRead = regexp.MustCompile(
	regexp.QuoteMeta(corpusGrantSSTKeyPath) + `[^\n]*\|\|`)

// corpusGrantSSTDoc is the minimal SST shape this contract needs. The value is
// decoded as a yaml.Node so an absent key, a null key, and a non-boolean key
// are three distinguishable failures rather than one opaque type error. It must
// be the value form: yaml.v3 refuses to unmarshal a scalar into *yaml.Node, so
// the pointer form fails the whole document instead of the one field.
type corpusGrantSSTDoc struct {
	Auth struct {
		CorpusGrantEnforcement yaml.Node `yaml:"corpus_grant_enforcement"`
	} `yaml:"auth"`
}

// assertCorpusGrantSSTHasNoDefault is the pure invariant.
//
//	sstYAML        — text of config/smackerel.yaml
//	resolutionPath — concatenated text of every file that resolves the key
func assertCorpusGrantSSTHasNoDefault(sstYAML, resolutionPath string) error {
	var doc corpusGrantSSTDoc
	if err := yaml.Unmarshal([]byte(sstYAML), &doc); err != nil {
		return fmt.Errorf("contract violation: %s is not valid YAML: %w", corpusGrantSSTYAMLRelpath, err)
	}
	// An absent key leaves the field at its zero value (Kind 0); an explicitly
	// null one decodes as a scalar tagged !!null. Both are refused, but only
	// this check can tell them apart.
	node := doc.Auth.CorpusGrantEnforcement
	if node.Kind == 0 {
		return fmt.Errorf("contract violation: %s does not declare %s; with the key absent there is no SST value at all and `required_value` would abort generation", corpusGrantSSTYAMLRelpath, corpusGrantSSTKeyPath)
	}
	if node.Tag != "!!bool" {
		return fmt.Errorf("contract violation: %s declares %s as %s (%q); the enforcement stage is a closed two-value boolean vocabulary, and an empty or null declaration is not a declaration", corpusGrantSSTYAMLRelpath, corpusGrantSSTKeyPath, node.Tag, node.Value)
	}

	if !strings.Contains(resolutionPath, corpusGrantSSTRequiredValueCall) {
		return fmt.Errorf("contract violation: the resolution path never calls %q; without required_value a missing SST key would resolve to empty instead of aborting generation", corpusGrantSSTRequiredValueCall)
	}
	if !strings.Contains(resolutionPath, corpusGrantSSTFailLoudEmission) {
		return fmt.Errorf("contract violation: the resolution path never emits %s with the fail-loud %s…} form; an empty resolution could then reach a generated env file", corpusGrantSSTEnvVar, corpusGrantSSTFailLoudEmission)
	}
	for _, shape := range corpusGrantSSTForbiddenShellDefaults {
		if strings.Contains(resolutionPath, shape) {
			return fmt.Errorf("contract violation (smackerel-no-defaults): the resolution path contains the fallback shape %q; a deleted SST key would then resolve silently to an assumed enforcement stage instead of failing loudly", shape)
		}
	}
	if m := corpusGrantSSTGetenvWithDefault.FindString(resolutionPath); m != "" {
		return fmt.Errorf("contract violation (smackerel-no-defaults): the resolution path reads %s via a getenv-with-default shape (%q); the enforcement stage must have no default anywhere", corpusGrantSSTEnvVar, m)
	}
	if m := corpusGrantSSTFallbackRead.FindString(resolutionPath); m != "" {
		return fmt.Errorf("contract violation (smackerel-no-defaults): the resolution path reads %s with a `||` fallback (%q); this key is REQUIRED and must abort, not degrade", corpusGrantSSTKeyPath, m)
	}
	return nil
}

// corpusGrantSSTBaselineYAML / corpusGrantSSTBaselineResolution are the
// minimal conformant shapes. Adversarial cases mutate them.
const corpusGrantSSTBaselineYAML = `auth:
  enabled: true
  corpus_grant_enforcement: false
`

const corpusGrantSSTBaselineResolution = `SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT="$(required_value auth.corpus_grant_enforcement)"
SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=${SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT:?auth.corpus_grant_enforcement resolved empty}
`

// mutateCorpusGrantSSTFixture applies exactly one mutation and refuses a
// no-op, so a stale literal cannot silently yield an unmutated fixture.
func mutateCorpusGrantSSTFixture(t *testing.T, source, old, replacement string) string {
	t.Helper()
	if !strings.Contains(source, old) {
		t.Fatalf("adversarial fixture is stale: %q is not present in the source under mutation", old)
	}
	mutated := strings.Replace(source, old, replacement, 1)
	if mutated == source {
		t.Fatalf("adversarial mutation of %q was a no-op; the fixture would be identical to the baseline", old)
	}
	return mutated
}

// requireCorpusGrantSSTBaselinePasses is the anti-tautology precondition every
// adversarial case runs first.
func requireCorpusGrantSSTBaselinePasses(t *testing.T, sstYAML, resolutionPath string) {
	t.Helper()
	if err := assertCorpusGrantSSTHasNoDefault(sstYAML, resolutionPath); err != nil {
		t.Fatalf("adversarial precondition failed: the unmutated baseline must satisfy the contract, got: %v", err)
	}
}

// TestCorpusGrantSST_LiveFiles reads the real committed SST and generator. No
// fixture, no mock: this is the assertion about what the repo actually ships.
func TestCorpusGrantSST_LiveFiles(t *testing.T) {
	sstYAML := readRepoFile(t, corpusGrantSSTYAMLRelpath)
	generator := readRepoFile(t, corpusGrantSSTGeneratorRelpath)
	if err := assertCorpusGrantSSTHasNoDefault(sstYAML, generator); err != nil {
		t.Fatal(err)
	}
}

// TestCorpusGrantSST_AcceptsConformantFixture proves the contract is
// satisfiable rather than unconditionally refusing, which would make every
// adversarial case below meaningless.
func TestCorpusGrantSST_AcceptsConformantFixture(t *testing.T) {
	if err := assertCorpusGrantSSTHasNoDefault(corpusGrantSSTBaselineYAML, corpusGrantSSTBaselineResolution); err != nil {
		t.Fatalf("contract refused a conformant fixture: %v", err)
	}
}

// TestCorpusGrantSST_AdversarialRejectsShellDefaultShapes is the primary
// adversarial case: a `${VAR:-default}` (or `-` / `:=` variant) must be
// refused. This is the shape that turns a deleted SST key into a silent,
// permanent OBSERVE.
func TestCorpusGrantSST_AdversarialRejectsShellDefaultShapes(t *testing.T) {
	cases := []struct {
		name        string
		replacement string
	}{
		{"colon_dash", "${" + corpusGrantSSTEnvVar + ":-false}"},
		{"bare_dash", "${" + corpusGrantSSTEnvVar + "-false}"},
		{"colon_equals", "${" + corpusGrantSSTEnvVar + ":=false}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireCorpusGrantSSTBaselinePasses(t, corpusGrantSSTBaselineYAML, corpusGrantSSTBaselineResolution)

			broken := mutateCorpusGrantSSTFixture(t, corpusGrantSSTBaselineResolution,
				corpusGrantSSTFailLoudEmission+"auth.corpus_grant_enforcement resolved empty}",
				tc.replacement)
			err := assertCorpusGrantSSTHasNoDefault(corpusGrantSSTBaselineYAML, broken)
			if err == nil {
				t.Fatalf("contract accepted the fallback shape %q; smackerel-no-defaults forbids any default for this key", tc.replacement)
			}
			if !strings.Contains(err.Error(), "smackerel-no-defaults") && !strings.Contains(err.Error(), "fail-loud") {
				t.Fatalf("rejected for the wrong reason, want a no-defaults or fail-loud failure, got: %v", err)
			}
		})
	}
}

// TestCorpusGrantSST_AdversarialRejectsGetenvWithDefault covers the Go-side
// shape. A default supplied in Go is exactly as fatal as one supplied in
// shell, so the contract must refuse it wherever it appears on the path.
func TestCorpusGrantSST_AdversarialRejectsGetenvWithDefault(t *testing.T) {
	requireCorpusGrantSSTBaselinePasses(t, corpusGrantSSTBaselineYAML, corpusGrantSSTBaselineResolution)

	broken := corpusGrantSSTBaselineResolution +
		`enforce := os.Getenv("` + corpusGrantSSTEnvVar + `", "false")` + "\n"
	err := assertCorpusGrantSSTHasNoDefault(corpusGrantSSTBaselineYAML, broken)
	if err == nil {
		t.Fatalf("contract accepted a getenv-with-default read of %s; the enforcement stage must have no default anywhere on the resolution path", corpusGrantSSTEnvVar)
	}
	if !strings.Contains(err.Error(), "getenv-with-default") {
		t.Fatalf("rejected for the wrong reason, want a getenv-with-default failure, got: %v", err)
	}
}

// TestCorpusGrantSST_AdversarialRejectsMissingFailLoudEmission proves the
// `${VAR:?…}` requirement is falsifiable. A bare `$VAR` emission would let an
// empty resolution reach a generated env file unnoticed.
func TestCorpusGrantSST_AdversarialRejectsMissingFailLoudEmission(t *testing.T) {
	requireCorpusGrantSSTBaselinePasses(t, corpusGrantSSTBaselineYAML, corpusGrantSSTBaselineResolution)

	broken := mutateCorpusGrantSSTFixture(t, corpusGrantSSTBaselineResolution,
		corpusGrantSSTFailLoudEmission+"auth.corpus_grant_enforcement resolved empty}",
		"$"+corpusGrantSSTEnvVar)
	if err := assertCorpusGrantSSTHasNoDefault(corpusGrantSSTBaselineYAML, broken); err == nil {
		t.Fatalf("contract accepted a bare $%s emission with no fail-loud guard", corpusGrantSSTEnvVar)
	}
}

// TestCorpusGrantSST_AdversarialRejectsFallbackRead proves that swapping the
// aborting `required_value` read for a degrading `yaml_get … || VAR=""` read
// is refused. That swap is silent: generation still succeeds, and the gate
// ships configured with a value the SST never declared.
func TestCorpusGrantSST_AdversarialRejectsFallbackRead(t *testing.T) {
	requireCorpusGrantSSTBaselinePasses(t, corpusGrantSSTBaselineYAML, corpusGrantSSTBaselineResolution)

	broken := mutateCorpusGrantSSTFixture(t, corpusGrantSSTBaselineResolution,
		`"$(`+corpusGrantSSTRequiredValueCall+`)"`,
		`"$(yaml_get `+corpusGrantSSTKeyPath+` 2>/dev/null)" || `+corpusGrantSSTEnvVar+`="false"`)
	err := assertCorpusGrantSSTHasNoDefault(corpusGrantSSTBaselineYAML, broken)
	if err == nil {
		t.Fatalf("contract accepted a `||`-fallback read of %s; this key is REQUIRED and must abort, not degrade", corpusGrantSSTKeyPath)
	}
	if !strings.Contains(err.Error(), "required_value") && !strings.Contains(err.Error(), "smackerel-no-defaults") {
		t.Fatalf("rejected for the wrong reason, want a required_value or no-defaults failure, got: %v", err)
	}
}

// TestCorpusGrantSST_AdversarialRejectsUndeclaredKey covers the DECLARED half.
// Both an absent key and a null/empty one must fail: an empty declaration is
// not a declaration, and it would make the emitted stage unresolvable.
func TestCorpusGrantSST_AdversarialRejectsUndeclaredKey(t *testing.T) {
	cases := []struct {
		name        string
		old         string
		replacement string
	}{
		{"key_deleted", "  corpus_grant_enforcement: false\n", ""},
		{"key_null", "corpus_grant_enforcement: false", "corpus_grant_enforcement:"},
		{"key_empty_string", "corpus_grant_enforcement: false", `corpus_grant_enforcement: ""`},
		{"key_non_boolean", "corpus_grant_enforcement: false", `corpus_grant_enforcement: "observe"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireCorpusGrantSSTBaselinePasses(t, corpusGrantSSTBaselineYAML, corpusGrantSSTBaselineResolution)

			broken := mutateCorpusGrantSSTFixture(t, corpusGrantSSTBaselineYAML, tc.old, tc.replacement)
			if err := assertCorpusGrantSSTHasNoDefault(broken, corpusGrantSSTBaselineResolution); err == nil {
				t.Fatalf("contract accepted %s with %s %s", corpusGrantSSTYAMLRelpath, corpusGrantSSTKeyPath, tc.name)
			}
		})
	}
}
