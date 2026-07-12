// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy — spec 086 local-client-build orchestrator test.
//
// Exercises scripts/commands/local-client-build.sh end-to-end via os/exec with
// a SCRIPTED-FIXTURE build (SMACKEREL_FLUTTER_BUILD_CMD) and a RECORDING cosign
// shim (SMACKEREL_COSIGN_CMD). Per FC-4 (no fabrication), this node does NOT run a
// real `flutter build` or a real operator-sign — the REAL build/sign/placement
// run ON <deploy-host> (node <deploy-node>). This test proves the surrounding logic is correct:
//
//	SCN-086-A01 — `./smackerel.sh local-client-build` dispatches to the orchestrator
//	SCN-086-A02 — a missing/unsupported --target fails loud ([F086-LCB-01])
//	SCN-086-C01 — build→sign→emit produces a trustModel:local-operator manifest
//	              with provenance:local-operator and the REAL sha256 of the built
//	              (fixture) bytes; cosign sign-blob is invoked for AAB, APK, manifest
//	SCN-086-C02 — an EMPTY built artifact aborts [F086-LCB-03], NO manifest
//	SCN-086-C03 — a sign failure aborts [F086-LCB-05], NO manifest
//	SCN-086-C04 — COSIGN_PASSWORD's value never appears in output (presence only)
//
// Native, no Docker (reliable under a contended container surface).
//
// Cross-reference:
//   - specs/086-local-client-build/ (FR-086-02..07; FC-3, FC-4, FC-5)
//   - scripts/commands/local-client-build.sh
//   - scripts/commands/build-self-hosted.sh (the operator-sign precedent it mirrors)
package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// lcbSecretSentinel is a recognizable fake COSIGN_PASSWORD; the secret-discipline
// test asserts this string never appears in the orchestrator's output.
const lcbSecretSentinel = "s3cr3t-lcb-do-not-leak-DEADBEEF"

const lcbFlutterStub = `#!/usr/bin/env bash
set -euo pipefail
# Scripted fixture flutter: args are "build aab" | "build apk". Writes fixture bytes to the
# standard Flutter release output paths (relative to cwd = project dir). FC-4:
# this is a fixture writer; the real flutter build runs on <deploy-host> (node <deploy-node>).
case "${2:-}" in
  aab)
    mkdir -p build/app/outputs/bundle/release
    printf '%s' "${LCB_FIXTURE_AAB_BYTES:-}" >build/app/outputs/bundle/release/app-release.aab
    ;;
  apk)
    mkdir -p build/app/outputs/flutter-apk
    printf '%s' "${LCB_FIXTURE_APK_BYTES:-}" >build/app/outputs/flutter-apk/app-release.apk
    ;;
  *)
    echo "stub flutter: unexpected args: $*" >&2
    exit 2
    ;;
esac
`

const lcbCosignShim = `#!/usr/bin/env bash
set -euo pipefail
# Recording cosign shim: append argv to LCB_COSIGN_LOG and materialize the
# requested --output-signature file. Failure-injection seams:
#   LCB_COSIGN_EXIT=<n>          fail EVERY call with exit n (global).
#   LCB_COSIGN_FAIL_ON_CALL=<k>  fail ONLY the k-th sign-blob call (1-based,
#                                exit 1); all other calls succeed. The
#                                orchestrator signs in order AAB(1), APK(2),
#                                manifest(3), so k=3 fails ONLY the manifest sign
#                                while AAB+APK succeed — the manifest-orphan path
#                                a single global LCB_COSIGN_EXIT cannot reach.
printf '%s\n' "$*" >>"$LCB_COSIGN_LOG"
call_no="$(wc -l <"$LCB_COSIGN_LOG" | tr -d ' ')"
sig=""
prev=""
for a in "$@"; do
  [[ "$prev" == "--output-signature" ]] && sig="$a"
  prev="$a"
done
ec="${LCB_COSIGN_EXIT:-0}"
if [[ "$ec" != "0" ]]; then
  echo "stub cosign: forced failure (LCB_COSIGN_EXIT=$ec)" >&2
  exit "$ec"
fi
if [[ -n "${LCB_COSIGN_FAIL_ON_CALL:-}" && "$call_no" == "${LCB_COSIGN_FAIL_ON_CALL}" ]]; then
  echo "stub cosign: forced failure on call $call_no (LCB_COSIGN_FAIL_ON_CALL)" >&2
  exit 1
fi
[[ -n "$sig" ]] && printf 'FIXTURE-SIG' >"$sig"
exit 0
`

// lcbFixture holds the temp dirs, env, and expected digests for one run.
type lcbFixture struct {
	env       []string
	outDir    string
	cosignLog string
	aabBytes  string
	apkBytes  string
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// newLCBFixture builds an isolated environment: a temp Flutter project dir, a
// temp out-dir, dummy operator key + pubkey files, the stub flutter, and the
// recording cosign shim (which exits cosignExit). It returns the env to run the
// orchestrator with.
func newLCBFixture(t *testing.T, aabBytes, apkBytes string, cosignExit int) lcbFixture {
	t.Helper()
	tmp := t.TempDir()

	projectDir := filepath.Join(tmp, "project")
	outDir := filepath.Join(tmp, "out")
	keyDir := filepath.Join(tmp, "keys")
	binDir := filepath.Join(tmp, "bin")
	for _, d := range []string{projectDir, outDir, keyDir, binDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	keyPath := filepath.Join(keyDir, "cosign-operator.key")
	pubPath := filepath.Join(keyDir, "cosign-operator.pub")
	if err := os.WriteFile(keyPath, []byte("DUMMY-OPERATOR-PRIVATE-KEY\n"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(pubPath, []byte("DUMMY-OPERATOR-PUBLIC-KEY\n"), 0o644); err != nil {
		t.Fatalf("write pub: %v", err)
	}

	flutterStub := filepath.Join(binDir, "flutter-stub.sh")
	cosignShim := filepath.Join(binDir, "cosign-shim.sh")
	writeExecutable(t, flutterStub, lcbFlutterStub)
	writeExecutable(t, cosignShim, lcbCosignShim)

	cosignLog := filepath.Join(tmp, "cosign.log")

	env := append(os.Environ(),
		"SMACKEREL_FLUTTER_BUILD_CMD="+flutterStub,
		"SMACKEREL_COSIGN_CMD="+cosignShim,
		"SMACKEREL_LCB_PROJECT_DIR="+projectDir,
		"OPERATOR_COSIGN_KEY="+keyPath,
		"OPERATOR_COSIGN_PUBKEY="+pubPath,
		"COSIGN_PASSWORD="+lcbSecretSentinel,
		"LCB_FIXTURE_AAB_BYTES="+aabBytes,
		"LCB_FIXTURE_APK_BYTES="+apkBytes,
		"LCB_COSIGN_LOG="+cosignLog,
		"LCB_COSIGN_EXIT="+itoa(cosignExit),
		// Make the orchestrator's `git rev-parse HEAD` work under the
		// Docker test surface (golang container runs as root; the host-owned
		// /workspace mount otherwise trips git's "dubious ownership" guard).
		// Test-harness only — the real <deploy-host> run is by the repo owner, so the
		// orchestrator script itself never needs this.
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=safe.directory",
		"GIT_CONFIG_VALUE_0=*",
	)
	return lcbFixture{env: env, outDir: outDir, cosignLog: cosignLog, aabBytes: aabBytes, apkBytes: apkBytes}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// lcbEnvWithout returns a copy of env with EVERY entry for key removed, so the
// orchestrator child sees the variable as genuinely absent and its fail-loud
// guard (lcb_require_env / ${VAR:?}) must fire. Removing all matches is robust
// even if the parent shell already exported key.
func lcbEnvWithout(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// lcbEnvReplace returns a copy of env with key set to exactly val (any prior
// entries removed first, so the child sees one unambiguous value).
func lcbEnvReplace(env []string, key, val string) []string {
	return append(lcbEnvWithout(env, key), key+"="+val)
}

// runOrchestrator execs scripts/commands/local-client-build.sh directly.
func runOrchestrator(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	script := filepath.Join(repoRoot(t), "scripts", "commands", "local-client-build.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("orchestrator script not found: %v", err)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("orchestrator exec error: %v", err)
		}
	}
	return stdout.String(), stderr.String(), code
}

func lcbSha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// lcbManifestDoc is the minimal local build-manifest shape this test asserts.
type lcbManifestDoc struct {
	TrustModel string `yaml:"trustModel"`
	Product    string `yaml:"product"`
	SourceSha  string `yaml:"sourceSha"`
	Clients    struct {
		None      bool `yaml:"none"`
		Artifacts []struct {
			Platform   string `yaml:"platform"`
			Sha256     string `yaml:"sha256"`
			Provenance string `yaml:"provenance"`
			ApkSha256  string `yaml:"apkSha256"`
		} `yaml:"artifacts"`
	} `yaml:"clients"`
}

// TestLocalClientBuild_Dispatch proves SCN-086-A01: `./smackerel.sh
// local-client-build` routes to the orchestrator (a no-target invocation surfaces
// the orchestrator's own [F086-LCB-01], proving the dispatch arm is wired).
func TestLocalClientBuild_Dispatch(t *testing.T) {
	cli := filepath.Join(repoRoot(t), "smackerel.sh")
	cmd := exec.Command("bash", cli, "local-client-build")
	cmd.Env = os.Environ()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	if code == 0 {
		t.Fatalf("`smackerel.sh local-client-build` with no --target exited 0 (dispatch/validation broken); stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "F086-LCB-01") {
		t.Fatalf("dispatch did not route to the orchestrator (no [F086-LCB-01] surfaced); stderr=%s", stderr.String())
	}
}

// TestLocalClientBuild_HelpListsCommand proves the SECOND clause of SCN-086-A01
// ("And `./smackerel.sh --help` lists local-client-build") — the usage() entry,
// the half of DoD a01 the dispatch test does NOT cover. The dispatch test only
// proves routing (a no-target invocation surfaces [F086-LCB-01]); it never asserts
// the command is DISCOVERABLE in --help. Adversarial: it fails if the usage()
// line is ever dropped, independent of whether the dispatch arm still works.
func TestLocalClientBuild_HelpListsCommand(t *testing.T) {
	cli := filepath.Join(repoRoot(t), "smackerel.sh")
	cmd := exec.Command("bash", cli, "--help")
	cmd.Env = os.Environ()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("`smackerel.sh --help` exec error: %v", err)
	}
	if code != 0 {
		t.Fatalf("`smackerel.sh --help` exited %d, expected 0; stderr=%s", code, stderr.String())
	}
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "local-client-build") {
		t.Fatalf("`./smackerel.sh --help` does not list `local-client-build` (usage() entry missing — SCN-086-A01 clause 2); help=%s", help)
	}
}

// TestLocalClientBuild_TargetRequired proves SCN-086-A02: a missing or
// unsupported --target fails loud with [F086-LCB-01] and builds nothing.
func TestLocalClientBuild_TargetRequired(t *testing.T) {
	// Missing --target.
	_, stderr, code := runOrchestrator(t, os.Environ())
	if code == 0 || !strings.Contains(stderr, "F086-LCB-01") {
		t.Fatalf("missing --target was not rejected with [F086-LCB-01]; code=%d stderr=%s", code, stderr)
	}
	// Unsupported --target.
	_, stderr2, code2 := runOrchestrator(t, os.Environ(), "--target", "<deploy-host>")
	if code2 == 0 || !strings.Contains(stderr2, "F086-LCB-01") {
		t.Fatalf("unsupported --target was not rejected with [F086-LCB-01]; code=%d stderr=%s", code2, stderr2)
	}
}

// TestLocalClientBuild_HappyPath proves SCN-086-C01: a stubbed build + recording
// cosign shim yields a trustModel:local-operator manifest carrying
// provenance:local-operator and the REAL sha256 of the built (fixture) bytes,
// with sign-blob invoked for the AAB, the APK, and the manifest.
func TestLocalClientBuild_HappyPath(t *testing.T) {
	const aab = "FIXTURE-AAB-BYTES-v086-not-a-real-bundle"
	const apk = "FIXTURE-APK-BYTES-v086-not-a-real-package"
	fx := newLCBFixture(t, aab, apk, 0)

	stdout, stderr, code := runOrchestrator(t, fx.env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code != 0 {
		t.Fatalf("orchestrator exited %d on happy path; stdout=%s stderr=%s", code, stdout, stderr)
	}

	// Locate the manifest.
	entries, _ := os.ReadDir(fx.outDir)
	var manifestPath string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "local-client-manifest-") && strings.HasSuffix(e.Name(), ".yaml") {
			manifestPath = filepath.Join(fx.outDir, e.Name())
		}
	}
	if manifestPath == "" {
		t.Fatalf("no local-client-manifest-*.yaml written; out-dir=%v", entries)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var doc lcbManifestDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("manifest is not valid YAML: %v\n%s", err, raw)
	}
	if doc.TrustModel != "local-operator" {
		t.Fatalf("manifest trustModel=%q, expected local-operator", doc.TrustModel)
	}
	if doc.Product != "smackerel" {
		t.Fatalf("manifest product=%q, expected smackerel", doc.Product)
	}
	if len(doc.Clients.Artifacts) != 1 {
		t.Fatalf("expected 1 clients artifact, got %d", len(doc.Clients.Artifacts))
	}
	a := doc.Clients.Artifacts[0]
	if a.Provenance != "local-operator" {
		t.Fatalf("manifest provenance=%q, expected local-operator (trust-model-aware)", a.Provenance)
	}
	if want := lcbSha256Hex(aab); a.Sha256 != want {
		t.Fatalf("manifest sha256=%q, expected the REAL sha256 of the built AAB bytes %q", a.Sha256, want)
	}
	if want := lcbSha256Hex(apk); a.ApkSha256 != want {
		t.Fatalf("manifest apkSha256=%q, expected the REAL sha256 of the built APK bytes %q", a.ApkSha256, want)
	}

	// The cosign shim recorded sign-blob for the AAB, the APK, and the manifest.
	logRaw, err := os.ReadFile(fx.cosignLog)
	if err != nil {
		t.Fatalf("read cosign log: %v", err)
	}
	log := string(logRaw)
	if n := strings.Count(log, "sign-blob"); n != 3 {
		t.Fatalf("expected 3 sign-blob invocations (aab, apk, manifest), got %d:\n%s", n, log)
	}
	if !strings.Contains(log, ".aab.sig") || !strings.Contains(log, ".apk.sig") {
		t.Fatalf("cosign log missing artifact signature targets:\n%s", log)
	}
	if !strings.Contains(log, "--key") {
		t.Fatalf("cosign was not invoked with --key (operator-key signing):\n%s", log)
	}
	// Adjacent .sig files exist (what the knb local-operator adapter consumes).
	for _, suf := range []string{".aab", ".apk"} {
		sig := filepath.Join(fx.outDir, "smackerel-assistant-"+manifestShortSha(t, doc.SourceSha)+suf+".sig")
		if _, err := os.Stat(sig); err != nil {
			t.Fatalf("expected adjacent signature missing: %s (%v)", sig, err)
		}
	}
	if _, err := os.Stat(manifestPath + ".sig"); err != nil {
		t.Fatalf("manifest signature missing: %s (%v)", manifestPath+".sig", err)
	}
}

func manifestShortSha(t *testing.T, sourceSha string) string {
	t.Helper()
	if len(sourceSha) < 12 {
		t.Fatalf("sourceSha too short: %q", sourceSha)
	}
	return sourceSha[:12]
}

// TestLocalClientBuild_FailClosedEmptyArtifact proves SCN-086-C02: an EMPTY built
// AAB aborts [F086-LCB-03] and writes NO manifest.
func TestLocalClientBuild_FailClosedEmptyArtifact(t *testing.T) {
	fx := newLCBFixture(t, "", "FIXTURE-APK", 0) // empty AAB
	_, stderr, code := runOrchestrator(t, fx.env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code == 0 || !strings.Contains(stderr, "F086-LCB-03") {
		t.Fatalf("empty AAB not rejected with [F086-LCB-03]; code=%d stderr=%s", code, stderr)
	}
	assertNoManifest(t, fx.outDir)
}

// TestLocalClientBuild_FailClosedEmptyAPK proves the APK half of SCN-086-C02: a
// non-empty AAB but an EMPTY built APK still aborts [F086-LCB-03] with NO
// manifest. It is distinct from the empty-AAB case above because it exercises the
// SECOND, asymmetric `[[ -s "$APK_SRC" ]]` guard — an accidental drop of the APK
// non-empty check would be caught here even while the empty-AAB test stays green
// (i.e. this is not a duplicate; it closes a real uncovered branch).
func TestLocalClientBuild_FailClosedEmptyAPK(t *testing.T) {
	fx := newLCBFixture(t, "FIXTURE-AAB", "", 0) // non-empty AAB, EMPTY APK
	_, stderr, code := runOrchestrator(t, fx.env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code == 0 || !strings.Contains(stderr, "F086-LCB-03") {
		t.Fatalf("empty APK not rejected with [F086-LCB-03]; code=%d stderr=%s", code, stderr)
	}
	// Fail-closed: the APK guard fires before sign/emit, so NO manifest exists.
	assertNoManifest(t, fx.outDir)
}

// TestLocalClientBuild_FailClosedSignFailure proves SCN-086-C03: a cosign sign
// failure aborts [F086-LCB-05] and writes NO manifest.
func TestLocalClientBuild_FailClosedSignFailure(t *testing.T) {
	fx := newLCBFixture(t, "FIXTURE-AAB", "FIXTURE-APK", 1) // cosign exits 1
	_, stderr, code := runOrchestrator(t, fx.env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code == 0 || !strings.Contains(stderr, "F086-LCB-05") {
		t.Fatalf("sign failure not rejected with [F086-LCB-05]; code=%d stderr=%s", code, stderr)
	}
	assertNoManifest(t, fx.outDir)
}

// TestLocalClientBuild_FailClosedManifestSignOrphan proves the MANIFEST stage of
// the fail-closed "NO partial manifest" invariant — the same invariant
// assertNoManifest enforces for the empty-artifact and first-sign-failure paths.
// It targets the asymmetric sub-case a single global LCB_COSIGN_EXIT cannot
// reach: the AAB(call 1) and APK(call 2) signs SUCCEED, but the manifest sign
// (call 3) FAILS. By that point the orchestrator has ALREADY written
// local-client-manifest-<sha>.yaml to the out-dir; a fail-closed orchestrator
// MUST remove that just-written unsigned manifest rather than leave an orphan
// .yaml with no valid .sig (the manifest is the consumable entry point the knb
// adapter reads). Adversarial: if the manifest-sign failure branch ever stops
// removing the partial manifest, the orphan survives and this test goes RED via
// assertNoManifest — exactly the regression being guarded. Distinct from
// TestLocalClientBuild_FailClosedSignFailure, whose global exit fails the FIRST
// (AAB) sign and therefore aborts BEFORE any manifest is ever written.
func TestLocalClientBuild_FailClosedManifestSignOrphan(t *testing.T) {
	fx := newLCBFixture(t, "FIXTURE-AAB", "FIXTURE-APK", 0)
	// AAB(1) + APK(2) sign OK; ONLY the manifest sign (call 3) fails.
	env := lcbEnvReplace(fx.env, "LCB_COSIGN_FAIL_ON_CALL", "3")
	_, stderr, code := runOrchestrator(t, env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code == 0 {
		t.Fatalf("orchestrator exited 0 when the manifest sign failed (fail-OPEN on a partial manifest); stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "F086-LCB-05") {
		t.Fatalf("manifest sign failure not reported with [F086-LCB-05]; code=%d stderr=%s", code, stderr)
	}
	// The AAB + APK signs succeeded (so only the manifest sign failed), but the
	// out-dir MUST NOT retain an unsigned orphan manifest. Fail-closed: no
	// local-client-manifest-*.yaml may remain after the manifest sign fails.
	assertNoManifest(t, fx.outDir)
}

// TestLocalClientBuild_FailClosedMissingCosignPassword proves the trust-input
// fail-closed contract for the local-operator model's core SECRET. With a valid
// target, present build/cosign tool seams, and present operator key+pubkey files,
// but COSIGN_PASSWORD ABSENT, the orchestrator MUST abort [F086-LCB-01] BEFORE any
// build/sign and write NO manifest. Without the operator passphrase the
// local-operator trust chain cannot be honored, so the only safe outcome is to
// fail CLOSED. Adversarial: removing `lcb_require_env COSIGN_PASSWORD` (or giving
// it a default) would let the build proceed past a missing secret — this test
// fails in exactly that case. NOTE: this is the secret's ABSENCE guard; the
// existing TestLocalClientBuild_SecretNotLeaked covers the no-leak guard for a
// PRESENT secret — the two are complementary, not duplicates.
func TestLocalClientBuild_FailClosedMissingCosignPassword(t *testing.T) {
	fx := newLCBFixture(t, "FIXTURE-AAB", "FIXTURE-APK", 0)
	env := lcbEnvWithout(fx.env, "COSIGN_PASSWORD") // trust secret genuinely absent
	_, stderr, code := runOrchestrator(t, env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code == 0 {
		t.Fatalf("orchestrator exited 0 with COSIGN_PASSWORD absent (fail-OPEN on a missing trust secret); stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "F086-LCB-01") {
		t.Fatalf("missing COSIGN_PASSWORD not rejected with [F086-LCB-01]; code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "COSIGN_PASSWORD") {
		t.Fatalf("fail-closed error did not name COSIGN_PASSWORD (the absent trust secret); stderr=%s", stderr)
	}
	assertNoManifest(t, fx.outDir)
}

// TestLocalClientBuild_FailClosedMissingOperatorKey proves the trust-input
// fail-closed contract for the local-operator model's signing KEY. With everything
// else valid but OPERATOR_COSIGN_KEY pointing at a NON-EXISTENT file, the
// orchestrator MUST abort [F086-LCB-01] naming OPERATOR_COSIGN_KEY, before any
// build/sign, and write NO manifest. A local-operator build with no operator key
// cannot be operator-signed, so it MUST fail CLOSED rather than emit an unsigned
// or partially-built manifest. Adversarial: dropping the `[[ -f "$OPERATOR_COSIGN_KEY" ]]`
// guard would let signing be attempted against a missing key — this test fails then.
func TestLocalClientBuild_FailClosedMissingOperatorKey(t *testing.T) {
	fx := newLCBFixture(t, "FIXTURE-AAB", "FIXTURE-APK", 0)
	missingKey := filepath.Join(t.TempDir(), "absent", "cosign-operator.key")
	env := lcbEnvReplace(fx.env, "OPERATOR_COSIGN_KEY", missingKey)
	_, stderr, code := runOrchestrator(t, env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code == 0 {
		t.Fatalf("orchestrator exited 0 with OPERATOR_COSIGN_KEY missing (fail-OPEN on an absent signing key); stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "F086-LCB-01") {
		t.Fatalf("missing OPERATOR_COSIGN_KEY not rejected with [F086-LCB-01]; code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "OPERATOR_COSIGN_KEY") {
		t.Fatalf("fail-closed error did not name OPERATOR_COSIGN_KEY (the absent signing key); stderr=%s", stderr)
	}
	assertNoManifest(t, fx.outDir)
}

// TestLocalClientBuild_SecretNotLeaked proves SCN-086-C04: COSIGN_PASSWORD's
// value never appears in the orchestrator's stdout/stderr (presence only).
func TestLocalClientBuild_SecretNotLeaked(t *testing.T) {
	fx := newLCBFixture(t, "FIXTURE-AAB", "FIXTURE-APK", 0)
	stdout, stderr, code := runOrchestrator(t, fx.env, "--target", "self-hosted", "--allow-dirty", "--out-dir", fx.outDir)
	if code != 0 {
		t.Fatalf("orchestrator failed unexpectedly: code=%d stderr=%s", code, stderr)
	}
	if strings.Contains(stdout, lcbSecretSentinel) || strings.Contains(stderr, lcbSecretSentinel) {
		t.Fatalf("COSIGN_PASSWORD value LEAKED into output (terminal-discipline violation)")
	}
	if !strings.Contains(stdout, "COSIGN_PASSWORD: present") {
		t.Fatalf("expected presence-only report `COSIGN_PASSWORD: present`; stdout=%s", stdout)
	}
}

func assertNoManifest(t *testing.T, outDir string) {
	t.Helper()
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "local-client-manifest-") && strings.HasSuffix(e.Name(), ".yaml") {
			t.Fatalf("fail-closed broken: a partial manifest was written: %s", e.Name())
		}
	}
}
