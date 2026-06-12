---
name: bubbles-supply-chain-source-locking
description: Lock build-time dependency resolution to an explicit allowlist of trusted registries/sources so dependencies can never be pulled from an arbitrary or implicit upstream. Use when adding or editing a dependency manifest or lockfile (Cargo.toml/Cargo.lock, package.json/package-lock.json, go.mod/go.sum, requirements.txt/constraints); when configuring a package-source file (cargo-deny config, .npmrc, GOPROXY/GOFLAGS env, pip index config); when wiring or reviewing a lint/pre-push gate that touches dependency installation; when reviewing a PR that adds a new external dependency, a new registry, or a git/path dependency; or when investigating dependency-confusion, typosquat, or malicious-mirror risk. Distinct from deploy-time artifact provenance (cosign/SLSA/SBOM/Trivy) — see "Two Axes" below.
---

# Bubbles Supply-Chain Source Locking

## Goal

Make every build resolve its dependencies from an **explicit, reviewed allowlist of
trusted sources** — never from an arbitrary, implicit, or fall-through upstream. The
build inputs are as governed as the build outputs.

The single sentence: **a dependency can only enter the build from a source you
explicitly allowed.**

## Use This Skill When

- Adding or editing a dependency manifest or lockfile (`Cargo.toml`/`Cargo.lock`,
  `package.json`/`package-lock.json`, `go.mod`/`go.sum`, `requirements.txt`/constraints).
- Configuring a package-source file (`cargo-deny` config, `.npmrc`, `GOPROXY`/`GOFLAGS`,
  pip index configuration).
- Wiring or reviewing a lint / pre-push gate that installs or resolves dependencies.
- Reviewing a PR that adds a new external dependency, a new registry, or a `git`/path
  dependency.
- Investigating dependency-confusion, typosquatting, or malicious-mirror exposure.

## Core Rule (NON-NEGOTIABLE)

> Dependency resolution MUST be locked to an explicit allowlist of trusted
> registries/sources. Arbitrary or implicit upstream sources are forbidden.
> The enforcement is BLOCKING — there is no `--skip` / `--force` / `--allow-once`.

A manifest that can silently resolve a package from an un-allowlisted index, a second
("extra") index, a `git`/`path` source, or an implicit fall-through proxy is a blocking
finding — even if today's lockfile happens to point at a trusted source. The control is
the *source allowlist*, not the current pin.

## Two Axes — Source Locking Is NOT Deploy-Time Provenance

These are **complementary controls on different axes**. A repo needs BOTH; one never
substitutes for the other.

| Axis | This skill (build-time SOURCE locking) | Deploy-time artifact PROVENANCE |
|------|----------------------------------------|---------------------------------|
| Question answered | *Where did the code enter the build from?* | *Is the built artifact authentic, signed, and scanned before it runs?* |
| Failure it stops | Dependency-confusion, typosquat, malicious mirror, unexpected upstream, second-index fall-through | Tampered/unsigned image, unknown provenance, unscanned CVEs at deploy |
| Mechanism | Source allowlist in manifest/lockfile config + blocking resolve-source check | Signature + attestation verification at the deploy boundary |
| Typical tools | cargo-deny `[sources]`, pinned `.npmrc` registry, pinned `GOPROXY` + `go.sum`, pip `--index-url` + hashes | cosign keyless signing, SLSA build-provenance, SBOM attestation, image CVE scan |
| Owned in Bubbles by | this skill + its instruction; wired into each repo's lint/pre-push gate | `bubbles-deployment-target-adapter` (see "See Also") |

Source locking stops bad inputs from entering the build. Provenance proves the output is
the expected, signed, scanned artifact. Do not collapse them: a perfectly signed image
built from a typosquatted dependency still ships the compromised code.

## Per-Ecosystem Guidance

The rule is toolchain-agnostic; the wiring is ecosystem-specific. In every case: pin the
source allowlist, commit the lockfile, deny implicit fall-through, and wire a **blocking**
check into the repo's existing lint/pre-push gate.

### Rust / Cargo

Use `cargo-deny`'s `[sources]` section as the source allowlist:

```toml
# deny.toml
[sources]
unknown-registry = "deny"        # any registry not in allow-registry => error
unknown-git = "deny"             # any git source not in allow-git => error
allow-registry = ["sparse+https://index.crates.io/"]   # exactly one trusted index
allow-git = []                   # no git deps unless explicitly allowlisted here
```

- `allow-registry` SHOULD contain exactly one entry (the canonical first-party index, or
  the operator's pinned mirror) — not a list of "wherever it happens to be".
- `allow-git = []` by default; every git dependency is an explicit, reviewed exception.
- Commit `Cargo.lock`.
- Wire `cargo deny check sources` into the repo's blocking lint/pre-push gate.

### Node / npm

- Pin the registry in a committed `.npmrc`: `registry=<trusted-registry>` (the canonical
  first-party registry or the operator's pinned mirror).
- Forbid arbitrary per-package / per-scope registry overrides (`@scope:registry=...`)
  unless that scope is an explicitly reviewed allowlist entry.
- Commit `package-lock.json` and install lockfile-strict (`npm ci`), never an unpinned
  resolve.
- No implicit fall-through to a second registry; a dependency that resolves only from an
  un-pinned source is a finding.

### Go

- `GOFLAGS=-mod=readonly` so the build can never silently rewrite `go.mod`/`go.sum`.
- Pin `GOPROXY` to a single trusted proxy; do not append an open `,direct` fall-through
  unless that direct fetch path is an explicit, reviewed exception.
- Commit `go.sum` and verify it (`go mod verify`) in the gate.
- The checksum-disable knobs `GONOSUMCHECK`, `GONOSUMDB`, `GOINSECURE`, and a permissive
  `GOPRIVATE` covering public modules are FORBIDDEN — they defeat source verification.

### Python / pip

- Pass an explicit `--index-url` (the trusted index); never rely on the implicit default.
- FORBID `--extra-index-url` to an un-allowlisted host — it is the classic
  dependency-confusion vector (pip resolves the highest version across all indexes).
- Hash-pin every requirement (`--require-hashes` with pinned `==` versions) and commit
  the locked requirements/constraints file.

## Forbidden vs Required

| ❌ Forbidden | ✅ Required |
|--------------|-------------|
| Manifest/config that can resolve from any registry not on an allowlist | Explicit single-source (or reviewed multi-entry) allowlist |
| cargo-deny `unknown-registry`/`unknown-git` unset or `allow` | `unknown-registry = "deny"`, `unknown-git = "deny"`, populated `allow-registry` |
| Implicit `,direct` / second-index / extra-index fall-through | One pinned source; every extra source is a reviewed allowlist entry |
| pip `--extra-index-url` to an un-allowlisted host | Single `--index-url`; hash-pinned, version-pinned requirements |
| Uncommitted lockfile, or unpinned resolve in CI | Committed lockfile + lockfile-strict install (`npm ci`, `-mod=readonly`) |
| Checksum/verification disable knobs (`GONOSUMCHECK`, `GOINSECURE`, `--trusted-host`) | Verification ON; `go.sum`/hashes verified in the gate |
| Un-reviewed `git`/`path`/URL dependency | Each such source is an explicit allowlist entry with a recorded reason |
| A `--skip` / `--force` / `--allow-once` bypass on the source check | Blocking check, no bypass flag |

## Enforcement

- Each downstream repo wires the ecosystem-appropriate source check into its **existing**
  blocking lint / pre-push gate (the same gate that already runs format/lint). Source
  locking adds NO new framework gate — it is a policy that each repo's existing gate
  enforces with the ecosystem's native tool.
- The check MUST be blocking: an un-allowlisted source, a missing lockfile, a disabled
  verification knob, or an un-reviewed extra index fails the gate.
- There is NO bypass flag. A legitimately new trusted source is added by editing the
  allowlist (a reviewed change), never by skipping the check for one run.
- New trusted sources are a reviewed config change (allowlist edit), consistent with
  `bubbles-config-sst` single-source-of-truth governance for registry/lockfile config.

## Verification Commands

```bash
# Rust: source allowlist is enforced and blocking
grep -nE 'unknown-(registry|git)|allow-(registry|git)' deny.toml
# Expectation: unknown-registry = "deny", unknown-git = "deny", explicit allow-registry

# Node: registry pinned, lockfile committed, no stray per-scope registry
grep -nE '^registry=|:registry=' .npmrc
ls -la package-lock.json

# Go: read-only modules, pinned proxy, no checksum-disable knobs
grep -nE 'GOFLAGS|GOPROXY|GONOSUM|GOINSECURE|GOPRIVATE' .env* *.mk Makefile 2>/dev/null
ls -la go.sum

# Python: single index, hash-pinned, no extra index
grep -nE -- '--index-url|--extra-index-url|--require-hashes|--trusted-host' requirements*.txt pip.conf 2>/dev/null
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| "The lockfile already points at the trusted source, so we're fine" | The control is the *source allowlist*, not the current pin; the next resolve can drift | Lock the allowlist, not just today's pin |
| Adding `--extra-index-url` "to grab one internal package" | Dependency-confusion: a public typosquat at a higher version wins | Allowlist the internal index explicitly; pin/hash the package |
| Disabling `go.sum` verification to "unblock a build" | Defeats source verification entirely | Fix the source/pin; never disable verification |
| Treating cosign/SLSA/SBOM as "we already do supply chain" | Provenance is a different axis; it does not govern build inputs | Add source locking AND keep provenance |
| A `--skip-source-check` escape hatch in the gate | First skip becomes the permanent path | Blocking check; new sources via allowlist edit only |

## Integration With Bubbles Governance

| Bubbles Surface | Relevance |
|-----------------|-----------|
| `bubbles-config-sst` | Registry/lockfile/source config is single-source-of-truth; no scattered or hand-drifted source config |
| Repo lint / pre-push gate | Hosts the ecosystem source check; blocking, no bypass |
| `bubbles-deployment-target-adapter` | The complementary deploy-time provenance axis (cosign/SLSA/SBOM/Trivy) |
| Spec / design authoring | A spec that adds a new external dependency or registry declares the trusted source and any allowlist exception |

## See Also

- [bubbles-supply-chain-source-locking.instructions.md](../../instructions/bubbles-supply-chain-source-locking.instructions.md) — binding, agent-loaded counterpart
- [bubbles-deployment-target-adapter skill](../bubbles-deployment-target-adapter/SKILL.md) — deploy-time artifact provenance (cosign keyless signing, SLSA build-provenance, SBOM attestation, image CVE scan) — the complementary axis
- [bubbles-backup-bcdr-doctrine skill](../bubbles-backup-bcdr-doctrine/SKILL.md) — offsite/backup trust boundaries
- [bubbles-config-sst skill](../bubbles-config-sst/SKILL.md) — single-source-of-truth governance for registry/lockfile config
