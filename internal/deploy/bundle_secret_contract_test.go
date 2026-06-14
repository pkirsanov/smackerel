// Spec 052 SCN-052-S05 — Bundle secret injection contract test (Go).
//
// This file is the long-lived adversarial contract harness for spec 052
// (placeholder mode + 3-mirror SST). It enforces five invariants:
//
//   Happy path (TestBundleSecretContract_NoLiteralSecretsInHomeLab):
//     A live `./smackerel.sh config generate --env home-lab --bundle` produces
//     a deterministic tarball whose `app.env` (a) emits every key declared by
//     `config.SecretKeys()` as `__SECRET_PLACEHOLDER__<KEY>__`, (b) contains
//     no literal value from `internal/config/secrets.go::DevDBPasswords`,
//     and (c) ships a sibling `secret-keys.yaml` whose `secretKeys` field
//     equals `config.SecretKeys()` byte-for-byte.
//
//   A1 drift detector (TestBundleSecretContract_AdversarialA1_DriftDetector):
//     A TEMP COPY of `scripts/commands/config.sh` whose `SHELL_SECRET_KEYS`
//     array drops one key, when used as the loader, produces a bundle whose
//     sibling `secret-keys.yaml` no longer matches `config.SecretKeys()`.
//     This proves the contract harness has bite — any future regression that
//     desyncs the shell mirror from the Go mirror is caught.
//
//   A2 leakage detector (TestBundleSecretContract_AdversarialA2_LeakageDetector):
//     A TEMP COPY of `config/smackerel.yaml` whose `infrastructure.secret_keys`
//     omits POSTGRES_PASSWORD AND whose `infrastructure.postgres.password`
//     is changed to a non-DevDBPassword sentinel ("test-leak-sentinel-…"),
//     when consumed by the live loader, produces a bundle whose `app.env`
//     contains the sentinel literal — proving placeholder-mode shielding is
//     gated by the secret_keys manifest, not implicit.
//
//   A3 determinism detector (TestBundleSecretContract_AdversarialA3_DeterminismDetector):
//     Two consecutive invocations of the live loader (with identical
//     --source-sha) into separate output dirs produce byte-identical
//     `config-bundle-home-lab-<sha>.tar.gz` files (sha256 match). Any future
//     regression that reintroduces a timestamp/nonce into the bundle is
//     caught.
//
//   A4 opt-out detector (TestBundleSecretContract_AdversarialA4_OptOutDetector):
//     A TEMP COPY of `config/smackerel.yaml` whose `infrastructure.production_class_targets`
//     no longer lists `home-lab` AND whose `infrastructure.postgres.password`
//     is changed to a non-DevDBPassword sentinel ("test-optout-sentinel-…"),
//     when consumed by the live loader with --env home-lab, produces a bundle
//     whose `app.env` contains the sentinel literal (NOT a placeholder marker).
//     This proves placeholder mode is a real opt-in keyed on production-class
//     targets — flipping the opt-in off reverts to literal-emission behavior.
//
// Live repo files (`scripts/commands/config.sh`, `config/smackerel.yaml`)
// are NEVER mutated. All adversarial sub-tests assemble a temporary
// REPO_ROOT under `t.TempDir()` whose `scripts/commands/config.sh` and/or
// `config/smackerel.yaml` are tampered byte copies, and whose other paths
// (`config/prometheus/`, `config/prompt_contracts/`, `config/assistant/`,
// `config/searxng/`, `config/nats_contract.json`, `deploy/`) are symlinked to
// the live repo so the loader's other inputs remain unchanged.

package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
	"gopkg.in/yaml.v3"
)

// bundleSecretSourceSha is the deterministic --source-sha used by every
// invocation in this file. Forty zeros (no git lookup, fully reproducible).
const bundleSecretSourceSha = "0000000000000000000000000000000000000000"

// Spec 045 BUG-045-001 Scope 2 collateral — cached pre-compiled
// cmd/config-validate binary path used by every loader invocation in this
// file so the loader's pre-emit gate has a stable binary to call without
// needing the cmd/ + go.mod tree inside the sandboxed REPO_ROOT.
var (
	bundleSecretBinOnce sync.Once
	bundleSecretBinPath string
	bundleSecretBinErr  error
)

// bundleSecretRepoRoot resolves the repo root by walking up from this file's
// location (mirrors the pattern in internal/config/secret_keys_test.go).
func bundleSecretRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) returned !ok")
	}
	// internal/deploy/bundle_secret_contract_test.go → repo root is two parents up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// secretKeysManifest is the parse target for the bundle's sibling
// secret-keys.yaml file.
type secretKeysManifest struct {
	SecretKeys []string `yaml:"secretKeys"`
}

// setupTestRepoRoot constructs a temp directory that mirrors the live repo's
// REPO_ROOT layout via symlinks, optionally overriding `scripts/commands/config.sh`
// and/or `config/smackerel.yaml` with tampered byte copies.
//
// The returned path is suitable as the working dir for invoking
// `bash <path>/scripts/commands/config.sh ...` — the script's own
// `REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"` will resolve to <path>,
// and dependent paths (`config/prometheus/`, `deploy/`, etc.) are reachable
// via the symlinks.
func setupTestRepoRoot(t *testing.T, configShOverride []byte, smackerelYamlOverride []byte) string {
	t.Helper()
	repoRoot := bundleSecretRepoRoot(t)
	tmpRoot := t.TempDir()

	mkdir := func(p string) {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	symlink := func(src, dst string) {
		mkdir(filepath.Dir(dst))
		if err := os.Symlink(src, dst); err != nil {
			t.Fatalf("symlink %s -> %s: %v", dst, src, err)
		}
	}
	writeFile := func(path string, data []byte, mode os.FileMode) {
		mkdir(filepath.Dir(path))
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// Symlink top-level deploy/ (loader reads deploy/compose.deploy.yml).
	symlink(filepath.Join(repoRoot, "deploy"), filepath.Join(tmpRoot, "deploy"))

	// Build config/ — mkdir so loader can later create config/generated/.
	mkdir(filepath.Join(tmpRoot, "config"))
	symlink(filepath.Join(repoRoot, "config", "prometheus"),
		filepath.Join(tmpRoot, "config", "prometheus"))
	symlink(filepath.Join(repoRoot, "config", "prompt_contracts"),
		filepath.Join(tmpRoot, "config", "prompt_contracts"))
	symlink(filepath.Join(repoRoot, "config", "assistant"),
		filepath.Join(tmpRoot, "config", "assistant"))
	// config/searxng/settings.yml is required by config.sh --bundle (the bundle
	// stage copies it into <stage>/config/searxng/settings.yml; its absence makes
	// the loader exit 1 with "searxng settings file not found"). Symlink it like
	// the other config dirs so the sandbox bundle generation succeeds.
	symlink(filepath.Join(repoRoot, "config", "searxng"),
		filepath.Join(tmpRoot, "config", "searxng"))
	symlink(filepath.Join(repoRoot, "config", "nats_contract.json"),
		filepath.Join(tmpRoot, "config", "nats_contract.json"))

	// Place yaml: copy override OR copy live as a REAL file (not a symlink).
	// Why a real copy and not a symlink: bash `cd "$(dirname "${BASH_SOURCE[0]}")" && pwd`
	// in config.sh resolves symlinks, so a symlinked yaml is not strictly
	// required, but for consistency we always materialize the file the loader
	// will read so adversarial mutations always land in tmpRoot.
	yamlPath := filepath.Join(tmpRoot, "config", "smackerel.yaml")
	if smackerelYamlOverride != nil {
		writeFile(yamlPath, smackerelYamlOverride, 0o644)
	} else {
		raw, err := os.ReadFile(filepath.Join(repoRoot, "config", "smackerel.yaml"))
		if err != nil {
			t.Fatalf("read live smackerel.yaml: %v", err)
		}
		writeFile(yamlPath, raw, 0o644)
	}

	// Place config.sh: ALWAYS as a real file copy (NOT a symlink). bash
	// `cd "$(dirname "${BASH_SOURCE[0]}")" && pwd` resolves symlinks, so a
	// symlinked config.sh would set SCRIPT_DIR to the live repo path and
	// REPO_ROOT to the live repo, defeating the temp-root isolation that
	// the adversarial sub-tests rely on. Real-file copy keeps SCRIPT_DIR
	// inside tmpRoot so the loader reads tmpRoot/config/smackerel.yaml.
	configShPath := filepath.Join(tmpRoot, "scripts", "commands", "config.sh")
	if configShOverride != nil {
		writeFile(configShPath, configShOverride, 0o755)
	} else {
		raw, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "commands", "config.sh"))
		if err != nil {
			t.Fatalf("read live config.sh: %v", err)
		}
		writeFile(configShPath, raw, 0o755)
	}

	return tmpRoot
}

// runConfigGenerate invokes the loader script at <repoRoot>/scripts/commands/config.sh
// with the given env, output dir, and source sha. Returns combined stdout+stderr
// and the exit code (non-zero on failure).
func runConfigGenerate(t *testing.T, repoRoot, env, outputDir string, extraEnv ...string) (string, int) {
	t.Helper()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outputDir, err)
	}
	cmd := exec.Command("bash",
		filepath.Join(repoRoot, "scripts", "commands", "config.sh"),
		"--env", env,
		"--bundle",
		"--output-dir", outputDir,
		"--source-sha", bundleSecretSourceSha,
	)
	// Spec 045 BUG-045-001 Scope 2 collateral — pre-build cmd/config-validate
	// once per test invocation and pass its path via SMACKEREL_CONFIG_VALIDATE_BIN
	// so the loader's new pre-emit gate (when fired for non-placeholder targets,
	// e.g. the A4 opt-out test) can run without needing cmd/ + go.mod + internal/
	// in the sandbox repo root.
	binEnv := []string{"SMACKEREL_CONFIG_VALIDATE_BIN=" + bundleSecretConfigValidateBin(t)}
	// Spec 061 SCOPE-06c — config.sh requires SMACKEREL_HARDWARE_TIER (fail-loud
	// per smackerel-no-defaults). Pin to cpu for deterministic bundle generation.
	binEnv = append(binEnv, "SMACKEREL_HARDWARE_TIER=cpu")
	cmd.Env = append(append(os.Environ(), binEnv...), extraEnv...)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("exec config.sh: %v\n--- output ---\n%s\n--- end ---", err, out)
		}
	}
	return string(out), exitCode
}

// bundleSecretConfigValidateBin pre-builds cmd/config-validate against the
// LIVE repo root once per `go test` process and returns the absolute path to
// the compiled binary. Subsequent calls return the cached path. The build
// runs in the live module so all internal/ imports resolve correctly; the
// path is then passed to the sandboxed loader via SMACKEREL_CONFIG_VALIDATE_BIN
// so the loader skips its `go run` fallback and uses the precompiled binary.
func bundleSecretConfigValidateBin(t *testing.T) string {
	t.Helper()
	bundleSecretBinOnce.Do(func() {
		repoRoot := bundleSecretRepoRoot(t)
		tmpDir, err := os.MkdirTemp("", "bug045-config-validate-bin-")
		if err != nil {
			bundleSecretBinErr = fmt.Errorf("mkdir tmp: %w", err)
			return
		}
		binPath := filepath.Join(tmpDir, "config-validate")
		// -buildvcs=false because `./smackerel.sh test unit` may run from a
		// cwd where VCS stamping fails (exit 128). The binary's behavior
		// is identical with or without VCS metadata for this test purpose.
		buildCmd := exec.Command("go", "build", "-buildvcs=false", "-o", binPath, "./cmd/config-validate")
		buildCmd.Dir = repoRoot
		buildCmd.Env = os.Environ()
		out, buildErr := buildCmd.CombinedOutput()
		if buildErr != nil {
			bundleSecretBinErr = fmt.Errorf("go build cmd/config-validate: %w\n--- output ---\n%s\n--- end ---", buildErr, out)
			return
		}
		bundleSecretBinPath = binPath
	})
	if bundleSecretBinErr != nil {
		t.Fatalf("bundleSecretConfigValidateBin: %v", bundleSecretBinErr)
	}
	return bundleSecretBinPath
}

// extractTarGz reads a .tar.gz from disk and returns a map of relative path
// → file body. Top-level files only (no recursion into prompt_contracts/).
func extractTarGz(t *testing.T, path string) map[string][]byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip.NewReader %s: %v", path, err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next %s: %v", path, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar entry %s: %v", hdr.Name, err)
		}
		files[hdr.Name] = body
	}
	return files
}

// sha256Hex returns the lowercase hex sha256 digest of the file at path.
func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// bundlePath returns the canonical bundle filename for env home-lab + the
// fixed source sha used in this file.
func bundlePath(outputDir string) string {
	return filepath.Join(outputDir,
		fmt.Sprintf("config-bundle-home-lab-%s.tar.gz", bundleSecretSourceSha))
}

// shellSecretKeysRegex captures the body of the `SHELL_SECRET_KEYS=( ... )`
// array literal inside config.sh for byte-level mutation.
var shellSecretKeysRegex = regexp.MustCompile(
	`(?ms)^SHELL_SECRET_KEYS=\(\n([^)]*)\n\)`,
)

// =============================================================================
// Happy path — main contract assertions on the live bundle.
// =============================================================================

// TestBundleSecretContract_NoLiteralSecretsInHomeLab — SCN-052-S05 main path.
//
// Cite design.md §8 happy path + §10 Test Plan row 3 + spec.md FR-052-001..005.
func TestBundleSecretContract_NoLiteralSecretsInHomeLab(t *testing.T) {
	tmpRoot := setupTestRepoRoot(t, nil, nil)
	outputDir := filepath.Join(t.TempDir(), "bundle-out")
	out, exit := runConfigGenerate(t, tmpRoot, "home-lab", outputDir)
	if exit != 0 {
		t.Fatalf("loader exited %d (expected 0)\n--- output ---\n%s\n--- end ---", exit, out)
	}

	bundle := bundlePath(outputDir)
	if _, err := os.Stat(bundle); err != nil {
		t.Fatalf("bundle missing at %s: %v", bundle, err)
	}

	files := extractTarGz(t, bundle)

	// (a) Every key in config.SecretKeys() appears as
	//     <KEY>=__SECRET_PLACEHOLDER__<KEY>__ in app.env.
	appEnv, ok := files["app.env"]
	if !ok {
		t.Fatal("bundle missing app.env")
	}
	for _, key := range config.SecretKeys() {
		want := key + "=" + config.Placeholder(key)
		if !bytes.Contains(appEnv, []byte(want)) {
			t.Fatalf("contract violation: app.env missing line %q (key %s placeholder regressed)\n--- app.env ---\n%s\n--- end ---", want, key, appEnv)
		}
	}

	// (b) No DevDBPassword literal value appears as the value of any managed
	// secret key (POSTGRES_USER=smackerel and POSTGRES_DB=smackerel are NOT
	// credential leaks — only the keys in config.SecretKeys() are managed
	// credential bearers, per FR-052-005). Scan app.env line-by-line.
	for _, key := range config.SecretKeys() {
		for _, devPw := range config.DevDBPasswords {
			line := []byte(key + "=" + devPw + "\n")
			if bytes.Contains(appEnv, line) {
				t.Fatalf("contract violation: app.env contains line %q — managed secret key %s holds dev-default value %q (placeholder shielding regressed)",
					line, key, devPw)
			}
			// Also catch trailing-EOF case (no newline after final entry).
			eofLine := []byte(key + "=" + devPw)
			if bytes.HasSuffix(appEnv, eofLine) {
				t.Fatalf("contract violation: app.env trailing line %q — managed secret key %s holds dev-default value %q (placeholder shielding regressed)",
					eofLine, key, devPw)
			}
		}
	}

	// (c) Sibling secret-keys.yaml exists, parses, and equals config.SecretKeys().
	siblingRaw, ok := files["secret-keys.yaml"]
	if !ok {
		t.Fatal("bundle missing sibling secret-keys.yaml (FR-052-003)")
	}
	var sibling secretKeysManifest
	if err := yaml.Unmarshal(siblingRaw, &sibling); err != nil {
		t.Fatalf("sibling secret-keys.yaml does not parse: %v\n--- raw ---\n%s\n--- end ---", err, siblingRaw)
	}
	want := config.SecretKeys()
	if !equalStringSlices(sibling.SecretKeys, want) {
		t.Fatalf("contract violation: sibling secret-keys.yaml.secretKeys = %v, want %v (mirror drift)", sibling.SecretKeys, want)
	}
}

// =============================================================================
// A1 drift detector — TEMP COPY of config.sh with SHELL_SECRET_KEYS shrunk.
// =============================================================================

// TestBundleSecretContract_AdversarialA1_DriftDetector — SCN-052-S05 A1.
//
// Cite design.md §8 A1 drift detector. Loader is run against a tampered
// config.sh whose SHELL_SECRET_KEYS array drops the LAST key. The contract
// is that the resulting sibling secret-keys.yaml MUST then differ from
// config.SecretKeys() — proving the harness catches the drift.
func TestBundleSecretContract_AdversarialA1_DriftDetector(t *testing.T) {
	repoRoot := bundleSecretRepoRoot(t)

	liveConfigSh, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "commands", "config.sh"))
	if err != nil {
		t.Fatalf("read live config.sh: %v", err)
	}

	// Sanity: live config.sh contains the canonical 4-key array.
	canonicalKeys := config.SecretKeys()
	if len(canonicalKeys) < 2 {
		t.Fatalf("config.SecretKeys() has %d entries; A1 needs ≥2 to drop one", len(canonicalKeys))
	}
	droppedKey := canonicalKeys[len(canonicalKeys)-1]
	wantBody := strings.Join(prefixLines("  ", canonicalKeys), "\n")
	matches := shellSecretKeysRegex.FindSubmatch(liveConfigSh)
	if matches == nil {
		t.Fatal("live config.sh missing SHELL_SECRET_KEYS=( ... ) block")
	}
	if strings.TrimSpace(string(matches[1])) != strings.TrimSpace(wantBody) {
		t.Fatalf("live SHELL_SECRET_KEYS body mismatch.\nwant:\n%s\ngot:\n%s",
			wantBody, string(matches[1]))
	}

	// Tamper: drop the last key.
	tamperedKeys := canonicalKeys[:len(canonicalKeys)-1]
	tamperedBody := strings.Join(prefixLines("  ", tamperedKeys), "\n")
	tamperedConfigSh := shellSecretKeysRegex.ReplaceAll(liveConfigSh,
		[]byte("SHELL_SECRET_KEYS=(\n"+tamperedBody+"\n)"))
	if bytes.Equal(tamperedConfigSh, liveConfigSh) {
		t.Fatal("byte-replace of SHELL_SECRET_KEYS array produced no diff (regex drift)")
	}

	tmpRoot := setupTestRepoRoot(t, tamperedConfigSh, nil)
	outputDir := filepath.Join(t.TempDir(), "bundle-out")
	out, exit := runConfigGenerate(t, tmpRoot, "home-lab", outputDir)
	if exit != 0 {
		t.Fatalf("tampered loader exited %d (expected 0; tamper drops a key but does not break loader)\n--- output ---\n%s\n--- end ---", exit, out)
	}

	files := extractTarGz(t, bundlePath(outputDir))

	// Drift assertion: sibling secret-keys.yaml MUST list only the tampered set.
	siblingRaw, ok := files["secret-keys.yaml"]
	if !ok {
		t.Fatal("tampered bundle missing sibling secret-keys.yaml")
	}
	var sibling secretKeysManifest
	if err := yaml.Unmarshal(siblingRaw, &sibling); err != nil {
		t.Fatalf("sibling secret-keys.yaml does not parse: %v", err)
	}
	if equalStringSlices(sibling.SecretKeys, canonicalKeys) {
		t.Fatalf("A1 drift detector regression: tampered loader still lists canonical keys %v — drift was NOT detected\n--- sibling ---\n%s\n--- end ---", canonicalKeys, siblingRaw)
	}
	if !equalStringSlices(sibling.SecretKeys, tamperedKeys) {
		t.Fatalf("A1 drift detector unexpected: sibling lists %v, want tampered %v", sibling.SecretKeys, tamperedKeys)
	}

	// And the dropped key's placeholder line MUST be missing from app.env.
	appEnv, ok := files["app.env"]
	if !ok {
		t.Fatal("tampered bundle missing app.env")
	}
	wantMissing := droppedKey + "=" + config.Placeholder(droppedKey)
	if bytes.Contains(appEnv, []byte(wantMissing)) {
		t.Fatalf("A1 drift detector regression: dropped key %s STILL emitted in app.env (tamper had no effect)", droppedKey)
	}
}

// =============================================================================
// A2 leakage detector — TEMP COPY of yaml drops a key from secret_keys AND
// changes its dev-default value to a non-DevDBPassword sentinel.
// =============================================================================

// TestBundleSecretContract_AdversarialA2_LeakageDetector — SCN-052-S05 A2.
//
// Cite design.md §8 A2 leakage detector. Patches yaml so POSTGRES_PASSWORD
// is no longer a manifest secret AND the dev value is replaced with a
// non-DevDBPassword sentinel (so FR-051-005 does not block emission). The
// contract is that the sentinel literal MUST then leak into app.env —
// proving placeholder shielding is gated on the manifest, not implicit.
func TestBundleSecretContract_AdversarialA2_LeakageDetector(t *testing.T) {
	repoRoot := bundleSecretRepoRoot(t)
	const leakSentinel = "test-leak-sentinel-92837465"

	// Belt-and-suspenders: sentinel must not collide with any DevDBPassword.
	for _, devPw := range config.DevDBPasswords {
		if strings.EqualFold(leakSentinel, devPw) {
			t.Fatalf("A2 sentinel %q collides with DevDBPassword %q — pick a different sentinel", leakSentinel, devPw)
		}
	}

	liveYaml, err := os.ReadFile(filepath.Join(repoRoot, "config", "smackerel.yaml"))
	if err != nil {
		t.Fatalf("read live smackerel.yaml: %v", err)
	}
	liveConfigSh, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "commands", "config.sh"))
	if err != nil {
		t.Fatalf("read live config.sh: %v", err)
	}

	// yaml mutation 1: replace `password: smackerel` with the sentinel so the
	// dev-default check (FR-051-005) does not fire on the literal flow.
	yamlA2 := bytes.Replace(liveYaml,
		[]byte("password: smackerel"),
		[]byte("password: "+leakSentinel),
		1)
	if bytes.Equal(yamlA2, liveYaml) {
		t.Fatal("A2 yaml mutation 1 (password swap) had no effect — yaml shape changed")
	}
	// yaml mutation 2 (mirror-side documentation): drop POSTGRES_PASSWORD from
	// secret_keys so the yaml mirror reflects the leakage class. This alone is
	// not what causes leakage — the live loader uses SHELL_SECRET_KEYS — but it
	// keeps the tampered yaml internally consistent for the contract narrative.
	dropYamlTarget := []byte("\n  - POSTGRES_PASSWORD")
	if !bytes.Contains(yamlA2, dropYamlTarget) {
		t.Fatalf("A2 yaml mutation 2 precondition failed: live yaml does not contain %q (yaml indent shape changed?)", dropYamlTarget)
	}
	yamlA2 = bytes.Replace(yamlA2, dropYamlTarget, []byte(""), 1)
	if bytes.Contains(yamlA2, dropYamlTarget) {
		t.Fatalf("A2 yaml mutation 2 failed — entry still present\n--- yamlA2 ---\n%s\n--- end ---", yamlA2)
	}

	// config.sh mutation: remove POSTGRES_PASSWORD from SHELL_SECRET_KEYS so the
	// in_secret_keys gate fails and the placeholder branch is bypassed. The
	// loader then falls through to the yaml literal path — the actual leakage.
	shellTarget := []byte("SHELL_SECRET_KEYS=(\n  POSTGRES_PASSWORD\n  AUTH_SIGNING_ACTIVE_PRIVATE_KEY\n  AUTH_AT_REST_HASHING_KEY\n  AUTH_BOOTSTRAP_TOKEN\n  TELEGRAM_BOT_TOKEN\n  KEEP_GOOGLE_APP_PASSWORD\n  CARD_REWARDS_GCAL_CREDENTIALS\n  WEB_REGISTRATION_INVITE_TOKEN\n)")
	shellReplacement := []byte("SHELL_SECRET_KEYS=(\n  AUTH_SIGNING_ACTIVE_PRIVATE_KEY\n  AUTH_AT_REST_HASHING_KEY\n  AUTH_BOOTSTRAP_TOKEN\n  TELEGRAM_BOT_TOKEN\n  KEEP_GOOGLE_APP_PASSWORD\n  CARD_REWARDS_GCAL_CREDENTIALS\n  WEB_REGISTRATION_INVITE_TOKEN\n)")
	if !bytes.Contains(liveConfigSh, shellTarget) {
		t.Fatalf("A2 config.sh mutation precondition failed: live config.sh does not contain expected SHELL_SECRET_KEYS array shape")
	}
	configShA2 := bytes.Replace(liveConfigSh, shellTarget, shellReplacement, 1)
	if bytes.Equal(configShA2, liveConfigSh) {
		t.Fatal("A2 config.sh mutation had no effect")
	}

	tmpRoot := setupTestRepoRoot(t, configShA2, yamlA2)
	outputDir := filepath.Join(t.TempDir(), "bundle-out")
	out, exit := runConfigGenerate(t, tmpRoot, "home-lab", outputDir)
	if exit != 0 {
		t.Fatalf("tampered loader exited %d (expected 0; sentinel is not a dev-default and key is no longer in shell manifest)\n--- output ---\n%s\n--- end ---", exit, out)
	}

	files := extractTarGz(t, bundlePath(outputDir))
	appEnv, ok := files["app.env"]
	if !ok {
		t.Fatal("tampered bundle missing app.env")
	}

	// Leakage assertion: sentinel literal MUST appear in app.env, proving the
	// detector would catch this regression class.
	leakedLine := []byte("POSTGRES_PASSWORD=" + leakSentinel)
	if !bytes.Contains(appEnv, leakedLine) {
		t.Fatalf("A2 leakage detector regression: tampered loader did NOT emit literal sentinel %q into app.env — leakage path was unexpectedly shielded\n--- app.env ---\n%s\n--- end ---", leakedLine, appEnv)
	}

	// Strict line-anchored placeholder-absence: the placeholder marker for
	// POSTGRES_PASSWORD is exactly `POSTGRES_PASSWORD=__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__`
	// and MUST NOT appear after the leakage path is taken.
	placeholderLine := []byte("POSTGRES_PASSWORD=" + config.Placeholder("POSTGRES_PASSWORD"))
	if bytes.Contains(appEnv, placeholderLine) {
		t.Fatalf("A2 leakage detector regression: tampered app.env STILL contains placeholder line %q (key was supposed to be removed from shell manifest)", placeholderLine)
	}
}

// =============================================================================
// A3 determinism detector — two consecutive invocations produce identical
// sha256, proving the contract's determinism invariant.
// =============================================================================

// TestBundleSecretContract_AdversarialA3_DeterminismDetector — SCN-052-S05 A3.
//
// Cite design.md §8 A3 determinism detector + spec.md NFR "Determinism".
// Two consecutive bundle generations against the live (un-tampered) tree
// MUST produce byte-identical .tar.gz files (sha256 match). Any future
// regression that introduces a timestamp/nonce into the bundle is caught
// here.
func TestBundleSecretContract_AdversarialA3_DeterminismDetector(t *testing.T) {
	tmpRoot := setupTestRepoRoot(t, nil, nil)

	outA := filepath.Join(t.TempDir(), "bundle-out-a")
	outB := filepath.Join(t.TempDir(), "bundle-out-b")

	if outOut, exit := runConfigGenerate(t, tmpRoot, "home-lab", outA); exit != 0 {
		t.Fatalf("first run exited %d\n--- output ---\n%s\n--- end ---", exit, outOut)
	}
	if outOut, exit := runConfigGenerate(t, tmpRoot, "home-lab", outB); exit != 0 {
		t.Fatalf("second run exited %d\n--- output ---\n%s\n--- end ---", exit, outOut)
	}

	shaA := sha256Hex(t, bundlePath(outA))
	shaB := sha256Hex(t, bundlePath(outB))
	if shaA != shaB {
		t.Fatalf("A3 determinism violation: two consecutive bundle generations produced different sha256\n  run A: %s\n  run B: %s", shaA, shaB)
	}

	// Adversarial cross-check: a synthetic byte-different blob MUST hash
	// differently. This proves the comparator itself is real (not a constant).
	syntheticA := sha256.Sum256([]byte("synthetic-a"))
	syntheticB := sha256.Sum256([]byte("synthetic-b"))
	if syntheticA == syntheticB {
		t.Fatal("A3 comparator self-check failed: sha256 of two distinct inputs collided")
	}
}

// =============================================================================
// A4 opt-out detector — TEMP COPY of yaml removes home-lab from
// production_class_targets AND swaps the dev-default password for a sentinel.
// =============================================================================

// TestBundleSecretContract_AdversarialA4_OptOutDetector — SCN-052-S05 A4.
//
// Cite design.md §8 A4 opt-out detector + spec.md FR-052-011. Patches yaml
// so home-lab is no longer in production_class_targets AND the postgres
// password is a non-DevDBPassword sentinel (so FR-051-005 does not block
// emission). The contract is that the sentinel literal MUST then appear in
// app.env (NOT a placeholder marker) — proving the placeholder mode is a
// real opt-in keyed on production_class_targets.
func TestBundleSecretContract_AdversarialA4_OptOutDetector(t *testing.T) {
	repoRoot := bundleSecretRepoRoot(t)
	const optOutSentinel = "test-optout-sentinel-49382716"

	for _, devPw := range config.DevDBPasswords {
		if strings.EqualFold(optOutSentinel, devPw) {
			t.Fatalf("A4 sentinel %q collides with DevDBPassword %q — pick a different sentinel", optOutSentinel, devPw)
		}
	}

	liveYaml, err := os.ReadFile(filepath.Join(repoRoot, "config", "smackerel.yaml"))
	if err != nil {
		t.Fatalf("read live smackerel.yaml: %v", err)
	}
	liveConfigSh, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "commands", "config.sh"))
	if err != nil {
		t.Fatalf("read live config.sh: %v", err)
	}

	// yaml mutation 1: swap dev password for sentinel so FR-051-005 does not
	// fire on the literal flow.
	yamlA4 := bytes.Replace(liveYaml,
		[]byte("password: smackerel"),
		[]byte("password: "+optOutSentinel),
		1)
	if bytes.Equal(yamlA4, liveYaml) {
		t.Fatal("A4 yaml mutation 1 (password swap) had no effect — yaml shape changed")
	}
	// yaml mutation 1b (BUG-045-001 Scope 2 collateral): provide a literal
	// runtime.auth_token sentinel so the new pre-emit Validate() gate does
	// not trip on "SMACKEREL_AUTH_TOKEN must be set when SMACKEREL_ENV=production".
	// In opt-out mode home-lab behaves like dev/test (literal values required);
	// the operator who opts out also accepts responsibility for providing all
	// required literals. The sentinel choice is non-DevDBPassword and
	// non-placeholder so the opt-out assertion logic below is unaffected.
	authTokenAnchor := []byte(`auth_token: ""`)
	if !bytes.Contains(yamlA4, authTokenAnchor) {
		t.Fatalf("A4 yaml mutation 1b precondition failed: live yaml does not contain %q (auth_token shape changed?)", authTokenAnchor)
	}
	yamlA4 = bytes.Replace(yamlA4,
		authTokenAnchor,
		[]byte(`auth_token: "test-optout-auth-token-sentinel-49382716"`),
		1)
	// yaml mutation 1c (BUG-045-001 Scope 2 collateral): flip the home-lab
	// per-env `auth_enabled` override from true to false so the pre-emit
	// Validate() gate does not require AUTH_SIGNING_ACTIVE_PRIVATE_KEY /
	// AUTH_SIGNING_ACTIVE_KEY_ID / AUTH_AT_REST_HASHING_KEY / AUTH_BOOTSTRAP_TOKEN
	// (those are gated on `production && auth.enabled`). A4 tests the opt-out
	// mechanism for placeholder mode, not the auth secret contract; disabling
	// auth in the same opt-out mutation keeps the test focused on its
	// hypothesis (sentinel literal flows through when opt-out is active).
	authEnabledAnchor := []byte("    auth_enabled: true")
	if !bytes.Contains(yamlA4, authEnabledAnchor) {
		t.Fatalf("A4 yaml mutation 1c precondition failed: live yaml does not contain %q (home-lab auth_enabled shape changed?)", authEnabledAnchor)
	}
	yamlA4 = bytes.Replace(yamlA4,
		authEnabledAnchor,
		[]byte("    auth_enabled: false"),
		1)
	// yaml mutation 2 (mirror-side documentation): drop home-lab from
	// production_class_targets in yaml so the yaml mirror reflects the opt-out.
	dropYamlTarget := []byte("\n  - home-lab")
	if !bytes.Contains(yamlA4, dropYamlTarget) {
		t.Fatalf("A4 yaml mutation 2 precondition failed: live yaml does not contain %q (yaml indent shape changed?)", dropYamlTarget)
	}
	yamlA4 = bytes.Replace(yamlA4, dropYamlTarget, []byte(""), 1)
	if bytes.Contains(yamlA4, []byte("production_class_targets:\n  - home-lab")) {
		t.Fatalf("A4 yaml mutation 2 failed — entry still present\n--- yamlA4 ---\n%s\n--- end ---", yamlA4)
	}

	// config.sh mutation: empty SHELL_PRODUCTION_CLASS_TARGETS so home-lab is
	// no longer a production-class target. The placeholder branch is then
	// skipped and the yaml literal flows through.
	shellTarget := []byte("SHELL_PRODUCTION_CLASS_TARGETS=(\n  home-lab\n)")
	shellReplacement := []byte("SHELL_PRODUCTION_CLASS_TARGETS=(\n)")
	if !bytes.Contains(liveConfigSh, shellTarget) {
		t.Fatalf("A4 config.sh mutation precondition failed: live config.sh does not contain expected SHELL_PRODUCTION_CLASS_TARGETS array shape")
	}
	configShA4 := bytes.Replace(liveConfigSh, shellTarget, shellReplacement, 1)
	if bytes.Equal(configShA4, liveConfigSh) {
		t.Fatal("A4 config.sh mutation had no effect")
	}

	tmpRoot := setupTestRepoRoot(t, configShA4, yamlA4)
	outputDir := filepath.Join(t.TempDir(), "bundle-out")
	out, exit := runConfigGenerate(t, tmpRoot, "home-lab", outputDir)
	if exit != 0 {
		t.Fatalf("tampered loader exited %d (expected 0; sentinel is not a dev-default and home-lab is no longer production-class)\n--- output ---\n%s\n--- end ---", exit, out)
	}

	files := extractTarGz(t, bundlePath(outputDir))
	appEnv, ok := files["app.env"]
	if !ok {
		t.Fatal("tampered bundle missing app.env")
	}

	// Opt-out assertion: sentinel literal MUST appear, proving opt-in is real.
	leakedLine := []byte("POSTGRES_PASSWORD=" + optOutSentinel)
	if !bytes.Contains(appEnv, leakedLine) {
		t.Fatalf("A4 opt-out detector regression: tampered loader did NOT emit literal sentinel %q into app.env — opt-in mechanism was unexpectedly bypassed\n--- app.env ---\n%s\n--- end ---", leakedLine, appEnv)
	}

	// And placeholder MUST be absent for POSTGRES_PASSWORD.
	placeholderLine := []byte("POSTGRES_PASSWORD=" + config.Placeholder("POSTGRES_PASSWORD"))
	if bytes.Contains(appEnv, placeholderLine) {
		t.Fatalf("A4 opt-out detector regression: tampered app.env STILL contains placeholder line %q (home-lab was supposed to be opted out)", placeholderLine)
	}
}

// =============================================================================
// helpers
// =============================================================================

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func prefixLines(prefix string, lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = prefix + line
	}
	return out
}
