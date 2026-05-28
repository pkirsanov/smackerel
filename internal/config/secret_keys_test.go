package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

// Spec 052 SCOPE-1 unit tests — SST-loader static manifest surface.
//
// These tests pin the canonical secret-key list and the deterministic
// placeholder format. The contract test in
// internal/deploy/bundle_secret_contract_test.go (added in SCOPE-3)
// extends coverage to the shell mirror in scripts/commands/config.sh.

// secretKeysRepoRoot resolves the repository root relative to this
// test file. internal/config is two levels deep, so ../.. lands at
// the root. Named distinctly to avoid colliding with the package's
// existing repoRoot helper in docker_security_test.go.
func secretKeysRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// yamlInfrastructure mirrors only the subset of config/smackerel.yaml
// the spec-052 manifest needs. Other infrastructure keys are decoded
// generically so they do not affect this test if/when they evolve.
type yamlInfrastructure struct {
	SecretKeys             []string `yaml:"secret_keys"`
	ProductionClassTargets []string `yaml:"production_class_targets"`
}

type yamlRoot struct {
	Infrastructure yamlInfrastructure `yaml:"infrastructure"`
}

// TestSecretKeys_MirrorsYAMLManifest (DoD T-052-001 / FR-052-001) —
// parses the live config/smackerel.yaml, extracts
// infrastructure.secret_keys, and asserts byte-for-byte (entries AND
// order) parity with config.SecretKeys(). This is the Go-side half of
// the three-way drift gate; the shell mirror is checked separately by
// the contract test in SCOPE-3.
func TestSecretKeys_MirrorsYAMLManifest(t *testing.T) {
	yamlPath := filepath.Join(secretKeysRepoRoot(t), "config", "smackerel.yaml")
	raw, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read %s: %v", yamlPath, err)
	}

	var root yamlRoot
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("unmarshal %s: %v", yamlPath, err)
	}

	yamlKeys := root.Infrastructure.SecretKeys
	goKeys := SecretKeys()

	if len(yamlKeys) == 0 {
		t.Fatalf("yaml infrastructure.secret_keys is empty — manifest missing or malformed")
	}
	if len(goKeys) == 0 {
		t.Fatalf("Go SecretKeys() returned empty slice — secret_keys.go regressed")
	}

	if !reflect.DeepEqual(yamlKeys, goKeys) {
		t.Fatalf("secret-key manifest drift detected\n  yaml (%d): %v\n  go   (%d): %v\nfix: update both config/smackerel.yaml infrastructure.secret_keys AND internal/config/secret_keys.go::secretKeys",
			len(yamlKeys), yamlKeys, len(goKeys), goKeys)
	}

	// Also verify the documented production_class_targets entry is
	// present so the rest of the design.md §3 manifest schema is
	// honored. SCOPE-2 will consume this; failing loud here catches a
	// future yaml-edit that drops it.
	if len(root.Infrastructure.ProductionClassTargets) == 0 {
		t.Fatalf("yaml infrastructure.production_class_targets is empty — required by FR-052-002")
	}
	wantTarget := "home-lab"
	found := false
	for _, target := range root.Infrastructure.ProductionClassTargets {
		if target == wantTarget {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("production_class_targets %v missing %q", root.Infrastructure.ProductionClassTargets, wantTarget)
	}
}

// TestSecretKeysMirror — supplementary in-memory check that the
// canonical list contains exactly the five documented keys in the
// documented order. This is the in-memory analogue of the yaml-driven
// drift test above; both must hold.
func TestSecretKeysMirror(t *testing.T) {
	want := []string{
		"POSTGRES_PASSWORD",
		"AUTH_SIGNING_ACTIVE_PRIVATE_KEY",
		"AUTH_AT_REST_HASHING_KEY",
		"AUTH_BOOTSTRAP_TOKEN",
		"TELEGRAM_BOT_TOKEN",
		"KEEP_GOOGLE_APP_PASSWORD",
	}
	got := SecretKeys()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SecretKeys() = %v, want %v", got, want)
	}

	// Defensive-copy contract: mutating the returned slice must NOT
	// affect subsequent calls.
	got[0] = "MUTATED"
	again := SecretKeys()
	if again[0] != "POSTGRES_PASSWORD" {
		t.Fatalf("SecretKeys() did not return a defensive copy: caller mutation leaked, got[0]=%q", again[0])
	}
}

// TestPlaceholderFormat — for every key in SecretKeys(), asserts
// Placeholder(key) returns "__SECRET_PLACEHOLDER__<KEY>__". This is
// the format the SST loader is required to emit (FR-052-001) and the
// adapter substitution is required to recognize (FR-052-006).
func TestPlaceholderFormat(t *testing.T) {
	for _, key := range SecretKeys() {
		want := "__SECRET_PLACEHOLDER__" + key + "__"
		got := Placeholder(key)
		if got != want {
			t.Errorf("Placeholder(%q) = %q, want %q", key, got, want)
		}
	}
}

// TestIsPlaceholder_TrueFalseMatrix (DoD T-052-002 / FR-052-001) —
// table-driven assertions covering the positive set (every declared
// key) and the negative set (empty string, real-looking secret value,
// undeclared-key placeholder shape, partial-match without trailing
// suffix). Drives confidence that the adapter's "is this still a
// placeholder?" probe (used in SCOPE-4 runtime defense) cannot be
// fooled by lookalikes.
func TestIsPlaceholder_TrueFalseMatrix(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		// Positive cases — every declared key.
		{"declared/postgres", "__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__", true},
		{"declared/auth-signing", "__SECRET_PLACEHOLDER__AUTH_SIGNING_ACTIVE_PRIVATE_KEY__", true},
		{"declared/auth-at-rest", "__SECRET_PLACEHOLDER__AUTH_AT_REST_HASHING_KEY__", true},
		{"declared/auth-bootstrap", "__SECRET_PLACEHOLDER__AUTH_BOOTSTRAP_TOKEN__", true},

		// Negative cases.
		{"empty", "", false},
		{"real-secret-value", "smackerel", false},
		{"undeclared-key", "__SECRET_PLACEHOLDER__UNKNOWN_KEY__", false},
		{"missing-trailing-suffix", "__SECRET_PLACEHOLDER__POSTGRES_PASSWORD", false},
		{"missing-leading-prefix", "POSTGRES_PASSWORD__", false},
		{"prefix-only", "__SECRET_PLACEHOLDER__", false},
		{"trailing-extra", "__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__extra", false},
		{"lowercase-key-not-recognized", "__SECRET_PLACEHOLDER__postgres_password__", false},
		{"random-string", "totally-unrelated-value", false},
		{"placeholder-substring-inside", "prefix__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__suffix", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsPlaceholder(tc.value)
			if got != tc.want {
				t.Errorf("IsPlaceholder(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}

	// Round-trip invariant: every Placeholder(k) MUST be recognized.
	for _, key := range SecretKeys() {
		if !IsPlaceholder(Placeholder(key)) {
			t.Errorf("round-trip: IsPlaceholder(Placeholder(%q)) = false, want true", key)
		}
	}
}

// TestIsPlaceholder — supplementary positive/negative coverage
// matching the user-facing description. Overlaps with the table above
// intentionally so future refactors that relax either name still leave
// at least one explicit per-case assertion in place.
func TestIsPlaceholder(t *testing.T) {
	for _, key := range SecretKeys() {
		if !IsPlaceholder(Placeholder(key)) {
			t.Errorf("IsPlaceholder(Placeholder(%q)) = false, want true", key)
		}
	}
	if IsPlaceholder("") {
		t.Errorf("IsPlaceholder(\"\") = true, want false")
	}
	if IsPlaceholder("realsecretvalue") {
		t.Errorf("IsPlaceholder(\"realsecretvalue\") = true, want false")
	}
	if IsPlaceholder("__SECRET_PLACEHOLDER__POSTGRES_PASSWORD") {
		t.Errorf("IsPlaceholder partial-suffix accepted, want false")
	}
}

// TestPlaceholder_DeterministicKeyDerived (DoD T-052-003 / FR-052-001)
// — Placeholder(k) MUST be a pure function of k. Calling it twice with
// the same key must return byte-identical output. No nonce, no
// timestamp, no source-SHA mixing. Determinism is the property the
// bundle-determinism NFR depends on (identical inputs → identical
// bundle bytes).
func TestPlaceholder_DeterministicKeyDerived(t *testing.T) {
	for _, key := range SecretKeys() {
		first := Placeholder(key)
		second := Placeholder(key)
		if first != second {
			t.Fatalf("Placeholder(%q) not deterministic: first=%q second=%q", key, first, second)
		}
		// Independently verify the byte-shape: prefix + key + suffix,
		// nothing else. This catches a future regression that adds a
		// suffix (e.g., a nonce) but happens to repeat it across both
		// calls in the same process.
		want := "__SECRET_PLACEHOLDER__" + key + "__"
		if first != want {
			t.Fatalf("Placeholder(%q) = %q, want %q (no extra suffix permitted)", key, first, want)
		}
	}
}

// TestPlaceholderDeterminism — supplementary process-wide repetition
// check. Calls Placeholder for every key 100 times; every call must
// return the same string.
func TestPlaceholderDeterminism(t *testing.T) {
	const iters = 100
	for _, key := range SecretKeys() {
		base := Placeholder(key)
		for i := 0; i < iters; i++ {
			again := Placeholder(key)
			if again != base {
				t.Fatalf("Placeholder(%q) drift on iter %d: base=%q now=%q", key, i, base, again)
			}
		}
	}
}

// TestSecretKeys_KeepAppPasswordRegistered (spec 059 Scope 1, SCN-059-001) —
// pin KEEP_GOOGLE_APP_PASSWORD as a declared Bucket-2 secret. The mirror
// parity test (TestSecretKeys_MirrorsYAMLManifest) covers cross-mirror drift;
// this regression test fails loud if a future edit removes the Keep entry
// from the in-memory Go list without also touching the yaml/shell mirrors.
func TestSecretKeys_KeepAppPasswordRegistered(t *testing.T) {
	keys := SecretKeys()
	found := false
	for _, k := range keys {
		if k == "KEEP_GOOGLE_APP_PASSWORD" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("KEEP_GOOGLE_APP_PASSWORD missing from SecretKeys() = %v\nfix: spec 059 Scope 1 requires this Bucket-2 secret in all three mirrors", keys)
	}
}

// TestKeepAppPasswordReadOnlyFromSidecarNotCore (spec 059 Scope 1, SCN-059-002) —
// the App Password is delivered into the core container's environment via the
// shared app.env env_file (matching the TELEGRAM_BOT_TOKEN precedent), so the
// sidecar boundary cannot be enforced at the compose env-injection layer. It
// is instead enforced at the APPLICATION LAYER: zero Go source files under
// internal/ or cmd/ may read the KEEP_GOOGLE_APP_PASSWORD environment variable.
// Only ml/app/keep_bridge.py is permitted to read it.
//
// This test greps the Go source tree under internal/ and cmd/ for literal
// references to "KEEP_GOOGLE_APP_PASSWORD". Test files and this test file
// itself are excluded; any non-test Go source containing the literal counts
// as a sidecar-boundary violation.
func TestKeepAppPasswordReadOnlyFromSidecarNotCore(t *testing.T) {
	root := secretKeysRepoRoot(t)
	const literal = "KEEP_GOOGLE_APP_PASSWORD"
	roots := []string{
		filepath.Join(root, "internal"),
		filepath.Join(root, "cmd"),
	}
	var offenders []string
	for _, base := range roots {
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			// Only .go files; exclude _test.go files (which may legitimately
			// reference the literal for documentation / negative assertions).
			name := info.Name()
			if filepath.Ext(name) != ".go" {
				return nil
			}
			if len(name) > len("_test.go") && name[len(name)-len("_test.go"):] == "_test.go" {
				return nil
			}
			// Exclude the canonical manifest file: it lists every Bucket-2
			// secret key as a string literal in the secretKeys slice. That's
			// a declaration, not a consumer read.
			if path == filepath.Join(root, "internal", "config", "secret_keys.go") {
				return nil
			}
			body, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			if bytesContainsString(body, literal) {
				offenders = append(offenders, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("sidecar-boundary violation: %d Go source file(s) reference %q (only ml/app/keep_bridge.py may read this env var):\n  %v",
			len(offenders), literal, offenders)
	}
}

// bytesContainsString reports whether haystack contains needle as a literal
// byte substring. Inlined here to avoid importing bytes/strings package
// solely for one call in this file.
func bytesContainsString(haystack []byte, needle string) bool {
	n := len(needle)
	if n == 0 {
		return true
	}
	if len(haystack) < n {
		return false
	}
	for i := 0; i+n <= len(haystack); i++ {
		match := true
		for j := 0; j < n; j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
