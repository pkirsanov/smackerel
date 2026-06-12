---
applyTo: "**"
---

# Supply-Chain Source Locking Policy (NON-NEGOTIABLE)

> This instructions file is the binding, agent-loaded counterpart to the
> [`bubbles-supply-chain-source-locking`](../skills/bubbles-supply-chain-source-locking/SKILL.md)
> skill. Load the skill for per-ecosystem wiring, verification commands, and the
> full rationale.

## The Rule

**Build-time dependency resolution MUST be locked to an explicit allowlist of trusted registries/sources. Arbitrary or implicit upstream sources are forbidden. Each downstream repo wires the ecosystem-appropriate source check into its existing blocking lint/pre-push gate. There is no `--skip` / `--force` / `--allow-once` bypass.**

The control is the *source allowlist*, not the current lockfile pin. A manifest that
*can* silently resolve from an un-allowlisted index, a second/extra index, an
un-reviewed `git`/`path` source, or an implicit fall-through proxy is a blocking finding
even when today's lockfile happens to point at a trusted source.

## Two Axes — Do Not Confuse With Deploy-Time Provenance

This policy governs **build-time dependency SOURCE locking** — *where code enters the
build from*. It is a different, complementary axis from **deploy-time artifact
PROVENANCE** — cosign keyless signing, SLSA build-provenance, SBOM attestation, and image
CVE scanning — which governs *whether a built artifact is authentic and safe before it
runs* (owned by [`bubbles-deployment-target-adapter`](../skills/bubbles-deployment-target-adapter/SKILL.md)).

A repo needs BOTH. Provenance on its own does not stop a typosquatted or
dependency-confused package from entering the build; source locking on its own does not
prove the shipped artifact is the signed, scanned one. Neither substitutes for the other.

## What Counts As A Violation

1. **Unlocked resolution source** — a dependency manifest/config that can resolve from a
   registry, index, proxy, or git/path source not on an explicit allowlist.
2. **Implicit fall-through** — `--extra-index-url` to an un-allowlisted host, a Go
   `,direct` fall-through that is not a reviewed exception, or an un-pinned second npm
   registry.
3. **Disabled verification** — `GONOSUMCHECK`, `GONOSUMDB`, `GOINSECURE`, pip
   `--trusted-host`, a permissive `GOPRIVATE` covering public modules, or any knob that
   turns off source/checksum verification.
4. **Uncommitted or unenforced lockfile** — missing `Cargo.lock` / `package-lock.json` /
   `go.sum` / hash-pinned requirements, or a CI install that is not lockfile-strict.
5. **A bypass flag** — any `--skip` / `--force` / `--allow-once` escape hatch on the
   source check.

## What Is Allowed

- Exactly one pinned trusted source per ecosystem (the canonical first-party index/registry
  or the operator's pinned mirror).
- Additional sources (an internal index, a specific `git` dependency) ONLY as explicit,
  reviewed allowlist entries with a recorded reason.
- Adding a new trusted source by editing the allowlist — a reviewed config change,
  governed like any other single-source-of-truth config (`bubbles-config-sst`).

## Enforcement

Each downstream repo wires the ecosystem-appropriate check into its EXISTING blocking
lint/pre-push gate (no new framework gate is introduced):

- Rust: `cargo deny check sources` with `unknown-registry = "deny"`, `unknown-git = "deny"`, and a populated `allow-registry`.
- Node: a committed pinned `.npmrc` registry, committed `package-lock.json`, lockfile-strict install (`npm ci`).
- Go: `GOFLAGS=-mod=readonly`, a single pinned `GOPROXY`, committed `go.sum` verified with `go mod verify`, and no checksum-disable knobs.
- Python: a single explicit `--index-url`, hash-pinned (`--require-hashes`) version-pinned requirements, and no `--extra-index-url` to an un-allowlisted host.

The check is blocking. A legitimately new trusted source is added by editing the
allowlist, never by skipping the check for one run. No `--skip` / `--force` /
`--allow-once` flag exists; none will be added.

## Forbidden Patterns

| ❌ Forbidden | ✅ Required |
|---|---|
| Manifest/config resolvable from any un-allowlisted registry/index/source | Explicit source allowlist (single pinned source per ecosystem) |
| cargo-deny `unknown-registry`/`unknown-git` unset or `allow` | `unknown-registry = "deny"`, `unknown-git = "deny"`, populated `allow-registry` |
| pip `--extra-index-url` to an un-allowlisted host | Single `--index-url`; hash-pinned, version-pinned requirements |
| Go `,direct` / open second source as silent fall-through | One pinned `GOPROXY`; any direct/extra source is a reviewed exception |
| Disabled verification (`GONOSUMCHECK`, `GOINSECURE`, `--trusted-host`) | Verification ON; `go.sum`/hashes verified in the gate |
| Uncommitted lockfile or unpinned CI resolve | Committed lockfile + lockfile-strict install |
| Un-reviewed `git`/`path`/URL dependency | Explicit allowlist entry with a recorded reason |
| `--skip-source-check` / `--force` bypass | Blocking check; new sources via reviewed allowlist edit only |

## See Also

- Skill: [`bubbles-supply-chain-source-locking`](../skills/bubbles-supply-chain-source-locking/SKILL.md) — per-ecosystem wiring + verification commands
- Skill: [`bubbles-deployment-target-adapter`](../skills/bubbles-deployment-target-adapter/SKILL.md) — deploy-time artifact provenance (the complementary axis)
- Instruction: [`bubbles-config-sst.instructions.md`](bubbles-config-sst.instructions.md) — single-source-of-truth governance for registry/lockfile config
