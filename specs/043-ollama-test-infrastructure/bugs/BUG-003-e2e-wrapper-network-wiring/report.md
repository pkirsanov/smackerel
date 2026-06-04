# BUG-003 — e2e wrapper network wiring (Scope-3 Ollama)

> Filed by: stochastic-quality-sweep round 8 (chaos pass against spec 043 Scope 3).
> Parent spec: `043-ollama-test-infrastructure` (status: `done`).
> This is a lightweight evidence-only bug doc capturing five findings
> against the live `./smackerel.sh test e2e` Scope-3 wrapper + adjacent
> Ollama test plumbing. The fixes shipped in the same commit as this
> report; no separate scopes were spun up.
>
> Numbering note: user requested slug `BUG-001-e2e-wrapper-network-wiring`,
> but BUG-001 and BUG-002 already exist under this spec, so the next
> available sequential number BUG-003 was used with the user's slug.

## Root Cause Summary

All five findings collapse to one underlying defect: the Scope-3 Ollama
opt-in wrapper in `smackerel.sh` and its host-side pull helper were
written as if "Ollama" was reachable at a single URL, but the runtime
actually exposes **two** distinct surfaces:

1. **Host-side surface** — `scripts/commands/ollama-test-pull.sh`
   runs OUTSIDE the compose network and must reach Ollama via the
   loopback host-published port (`http://127.0.0.1:${OLLAMA_HOST_PORT}`).
2. **Compose-network surface** — the in-network `golang:1.25.10-bookworm`
   test container runs INSIDE the compose network and must reach
   Ollama via the compose service DNS name + container port
   (`http://ollama:${OLLAMA_CONTAINER_PORT}`).

Collapsing both into a single `ollama_pull_url` variable meant
whichever surface wasn't loopback was silently broken — and because
the Scope-3 lane is opt-in (`SMACKEREL_TEST_OLLAMA=1`), no green CI
run ever caught it.

The remaining findings (CORE_URL missing, lossy reachability error
class, substring tag check) are adjacent shape defects in the same
wrapper block / its test surface that the chaos pass surfaced once
the URL split was being audited.

## Findings

### F-001 (P1) + F-005 (P3) — Single Ollama URL for two distinct network surfaces

**Surface:** `smackerel.sh` Scope-3 wrapper (lines ~1545–1578).

**Pre-fix evidence:**

```bash
ollama_host_port="$(smackerel_env_value "$env_file" "OLLAMA_HOST_PORT")"
ollama_pull_url="http://127.0.0.1:${ollama_host_port}"
...
e2e_run_child docker run --rm \
  --network "$compose_network" \
  ...
  -e "OLLAMA_URL=$ollama_pull_url" \
  ...
```

The in-network test container was being told to dial
`http://127.0.0.1:${OLLAMA_HOST_PORT}` — that loopback resolves inside
the container itself, not to the Ollama service. The test would
collapse to "ollama unreachable" inside the container even when the
host-side pull succeeded.

**Fix:** Split into two variables — `ollama_host_url` (loopback, host
port) for the pull script; `ollama_network_url` (compose DNS name,
container port) for the in-network test container. SST key
`OLLAMA_CONTAINER_PORT` was verified present in `config/smackerel.yaml`
(via `scripts/commands/config.sh:560`) and in
`config/generated/{dev,test}.env` (value `11434`).

**Files changed:** `smackerel.sh` (Scope-3 Ollama wrapper block).

**Claim Source:** executed (`grep` against `config/smackerel.yaml` +
`config/generated/test.env`; visual diff of the wrapper block).

### F-002 (P1) — Test reads CORE_URL but wrapper sets only CORE_EXTERNAL_URL

**Surface:** `tests/e2e/agent/happy_path_test.go` lines 214, 302, 370
call `mustEnv(t, "CORE_URL")`. The Scope-3 wrapper in `smackerel.sh`
exported only `CORE_EXTERNAL_URL` to the in-network container.

**Pre-fix consequence:** `mustEnv` fail-loud aborts the test before it
can even probe Ollama. The CORE_URL/CORE_EXTERNAL_URL split is real
elsewhere in the codebase (CORE_EXTERNAL_URL is the operator-visible
URL; CORE_URL is the in-network base URL the test agent posts to). The
canonical non-Ollama go-e2e block (`smackerel.sh` ~lines 1498–1509)
also exports CORE_EXTERNAL_URL, but the test code in the Ollama lane
specifically reads CORE_URL.

**Fix:** Added `-e "CORE_URL=http://smackerel-core:${core_container_port}"`
to the in-network docker run for Scope-3, mirroring how
`CORE_EXTERNAL_URL` is built. Did NOT touch the canonical go-e2e block
because its tests don't read CORE_URL.

**Files changed:** `smackerel.sh` (Scope-3 in-network docker run env list).

**Claim Source:** executed (grepped `mustEnv(t, "CORE_URL")` in
`tests/e2e/agent/happy_path_test.go`; counted 3 occurrences at the
referenced line numbers).

### F-003 (P2) — `ollamaReachable` collapses connect vs HTTP errors

**Surface:** `tests/e2e/agent/happy_path_test.go` lines ~102–124
(`ollamaReachable`).

**Pre-fix evidence:**

```go
resp, err := client.Get(probeURL)
if err != nil {
    var nErr *net.OpError
    _ = nErr // we accept any error class as "not reachable"
    return false
}
defer func() { _ = resp.Body.Close() }()
return resp.StatusCode == http.StatusOK
```

Both "daemon down / DNS wrong" and "daemon up but returning 5xx" map
to a single `false`. The downstream `t.Fatalf` then prints only the
URL, leaving the operator to guess which class of failure occurred.

**Fix:** Introduced `ollamaReachability(t, ollamaURL) error` which
returns:

- `nil` on HTTP 200.
- `fmt.Errorf("ollama unreachable at %s: %w", probeURL, err)` on
  connect/transport class errors.
- `fmt.Errorf("ollama responded HTTP %d at %s: %s", ...)` (with up to
  200 chars of body) on non-200 HTTP responses.

Kept the boolean `ollamaReachable` wrapper for the down-path branch
in `TestOllamaUnreachable_FailsLoudly` (which only needs to know
"healthy / not healthy"). Updated the two `t.Fatalf` callers in
`TestAgentHappyPath_PlanToolSynthesis` and
`TestAgentHappyPath_DeterministicOutput` to print the richer error.
Removed unused `net` import.

**Files changed:** `tests/e2e/agent/happy_path_test.go`.

**Claim Source:** executed (`go vet -tags e2e_ollama ./tests/e2e/agent/...`
returned exit 0 after the cleanup).

### F-004 (P3) — Substring grep against `/api/tags` JSON is too loose

**Surface:** `scripts/commands/ollama-test-pull.sh` lines ~98–105.

**Pre-fix evidence:**

```bash
if ! curl --silent --show-error --fail --max-time 30 "${ollama_url}/api/tags" \
    | grep -q "\"name\":\"${test_model}\""; then
```

The substring check would false-positive if another tag in the
catalog contained the test model name as a prefix (`qwen2.5:0.5b` vs
`qwen2.5:0.5b-instruct`), and would false-negative on minor JSON
formatting drift from the daemon (e.g. whitespace between key and
value).

**Fix:** Strict `jq` equality on `.models[].name`:

```bash
if ! command -v jq >/dev/null 2>&1; then
  echo "ollama-test-pull: jq is required to verify /api/tags strictly but was not found on PATH" >&2
  exit 4
fi

if ! curl ... "${ollama_url}/api/tags" \
    | jq -e --arg m "$test_model" '(.models // []) | map(.name) | index($m) != null' >/dev/null; then
  echo "ollama-test-pull: model $test_model missing from ${ollama_url}/api/tags after successful pull" >&2
  exit 4
fi
```

Both failure modes (jq missing, model absent) map to exit code 4 as
specified by the script's header contract. The wrapper continues to
treat "jq missing on test stack" as a fail-loud condition; the script
header is unchanged so the existing exit-code contract still holds.

**Files changed:** `scripts/commands/ollama-test-pull.sh`.

**Claim Source:** executed (`bash -n scripts/commands/ollama-test-pull.sh`
returned exit 0).

## Verification

```text
$ go vet -tags e2e_ollama ./tests/e2e/agent/...
GOVET_EXIT=0

$ bash -n smackerel.sh
SH1_EXIT=0

$ bash -n scripts/commands/ollama-test-pull.sh
SH2_EXIT=0
```

**Not run:** `./smackerel.sh test e2e` itself (requires a running test
stack; the Scope-3 lane is opt-in via `SMACKEREL_TEST_OLLAMA=1` and
the user explicitly scoped it out of this round). Operator validation
on a live stack is the natural next step.

**Claim Source:** executed (commands run in the workspace shell;
exit codes captured verbatim above).

## Files Touched

| File | Lines (approx) | Finding |
|------|----------------|---------|
| `smackerel.sh` | 1545–1582 | F-001, F-002, F-005 |
| `tests/e2e/agent/happy_path_test.go` | 33–36 (imports), 102–138 (reachability helpers), 218–227, 308–315 (callers) | F-003 |
| `scripts/commands/ollama-test-pull.sh` | 98–115 | F-004 |
| `specs/043-ollama-test-infrastructure/bugs/BUG-003-e2e-wrapper-network-wiring/report.md` | new | this evidence doc |
