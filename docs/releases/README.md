# Release Packets — Phase Index

Owner: `bubbles.releases`. Gate: **G101** (`release-delivery-reconciliation-guard.sh`).

This directory holds one **release phase packet** per phase. Each packet is the
canonical 8-doc set — `vision.md`, `features.md`, `actions.md`, `business-plan.md`,
`deployment.md`, `marketing.md`, `monetization.md`, `ops-scalability.md`. A packet
carries no `state.json`: packets are managed docs, not workflow artifacts, so the
spec state-transition guard does not apply to them.

This index file is **not** a packet doc. It sits one level above every packet
directory and is the phase register plus the binding vocabulary contract.

Last reconciled: **2026-08-27** against `HEAD` = `8a599234`.

<details>
<summary>What that stamp asserts, and what it does not</summary>

Verified by execution at this reconciliation:

- **Rule 1 (exhaustive home binding)** — `118` spec directories carry `state.json`
  (`112` numbered + `6` under `_ops`); `118` distinct specs are bound. **Zero
  unbound, zero dangling.** Three holes were closed here: `110-retrieval-quality-foundation`,
  `111-corpus-portability-sensitivity`, `112-capability-registry` were bound for
  the first time (the baseline bound `115`). They had been carried as
  `spec=none` "reserved slots" while the specs actually existed.
- **Rule 2 (home phase derived, never chosen)** — zero violations across every
  singly-bound spec.
- **Rule 3 (`deferred-to:` names a real phase)** — the only token in use is
  `deferred-to:v1`, and `v1/` exists. No `deferred-to:release-v1` survives.
- **Gate G101** — `mvp` (107 annotations), `next` (14), `v1` (2) each exit `0`.
  Packet-completeness, packet-location and ladder-schema guards each exit `0`.

Open, recorded rather than silently fixed:

- **`specs/110` train assignment vs this register's homing rationale** — routed to
  `bubbles.plan` as [`next/actions.md`](next/actions.md) **RTE-N7**. `bubbles.releases`
  owns no `specs/**` artifact, so the contradiction is recorded, not resolved.

The previous stamp read `2026-08-04` / `8d971420`. **1461** commits landed between
that SHA and this one, **995** of them touching `specs/`, so the register asserted
a reconciliation it had long outlived.

</details>

---

## Phase Register

| Phase | Packet | Backing train | Train slot | Status | What it covers |
|---|---|---|---|---|---|
| `mvp` | [`mvp/`](mvp/) | `mvp` | `<deploy-slot>` | active | The founding-promise gate **plus the entire pre-train legacy estate** (Phase 1–5 and every spec that predates the release-train policy). |
| `next` | [`next/`](next/) | `next` | `staging` | active | The promotion candidate: open-knowledge reasoning + synthesis, runtime-switchable models, retrieval-strategy routing, knowledge-graph public API, and the hardened-but-undelivered supervisor / corpus-grant / MCP plans. |
| `v1` | [`v1/`](v1/) | *(none yet)* | — | planning | The forward-looking personal-productivity + outbound-action product gate. No train is cut for it; it owns planned capability and the deferrals it inherited from `mvp`. |

### Axis vocabulary (this is the F3 unification)

Two axes were previously described with three vocabularies. There are exactly two:

| Axis | Source of truth | Legal values today | Owner |
|---|---|---|---|
| **Release train** | [`config/release-trains.yaml`](../../config/release-trains.yaml) | `mvp`, `next` | `bubbles.train` |
| **Release phase** | `docs/releases/<phase>/` | `mvp`, `next`, `v1` | `bubbles.releases` |

Binding rules between the axes:

1. **Every train MUST have an identically-named phase packet.** A train with no
   packet has no G101 surface, which is how train `next` shipped eleven specs
   with zero delivery enforcement.
2. **A phase MAY exist without a train.** `v1` is that case: it is a planning
   phase whose train has not been cut. This is legal and must be stated, not
   implied.
3. **`deferred-to:<phase>` MUST name a directory that exists under `docs/releases/`.**
   The token `release-v1` is **retired**: it never named a phase directory or a
   train, so every `deferred-to:release-v1` annotation was a dangling reference
   that G101 silently wildcarded. It is replaced by `deferred-to:v1`.

> Historical note, do not "fix": `specs/025-knowledge-synthesis-layer/scopes.md`
> and its `state.json` record the portfolio deferral `DI-025-05` against the
> literal string "release-v1 train". That is spec-owned text belonging to
> `bubbles.plan`, not to `bubbles.releases`, and it is not edited here. Read it
> as phase `v1`.

---

## Census rule (BINDING)

The packets used to be a partial record. At the 2026-08-02 schema review, 75 of
109 specs were bound to no packet and 67 of those were `done` — delivered
capability with no release-schema record at all. Silence is what let that happen,
so the scope of a packet is now stated explicitly rather than left to inference.

**Rule 1 — Home binding is exhaustive.**
Every spec directory under `specs/` that carries a `state.json` — every numbered
`specs/NNN-*` directory and every `specs/_ops/*` operational spec — has exactly
one **home phase**, and its home packet MUST carry a `bubbles:feature` annotation
binding it. Directories under `specs/_ops/` with no `state.json` (scratch sweep
rounds, harness folders) are not specs and are out of scope.

**Rule 2 — Home phase is derived, never chosen.**

| Spec's `state.json` | Home phase |
|---|---|
| `releaseTrain: mvp` | `mvp` |
| `releaseTrain: next` | `next` |
| no `releaseTrain` (the pre-078 legacy estate) | `mvp` |

The legacy estate homes to `mvp` because `mvp` is the founding gate that inherits
it. Those specs predate the release-train policy and are **not** being backfilled
with a `releaseTrain` field; this packet's annotations are their authoritative
train-membership record.

**Rule 3 — Delivery class is derived from `state.json`, never asserted.**

| Observed spec state | Class | Enforced by G101? |
|---|---|---|
| `done` (or terminal-for-mode) **and** `validate` present in completed phases | `required` | **yes** |
| `done` but `validate` not visible to the guard's parse | `carried` + stated reason | no |
| `specs_hardened` / `delivered_pending_activation` planning or handoff artifact | `optional` + stated flip condition | no |
| `in_progress` | `optional` + stated flip condition | no |
| `blocked` | `deferred-to:<phase>` if a later phase receives it, else `optional` + the recorded blocker quoted | no |
| capability with no owning spec | `spec=none delivery=optional` | no |

**Rule 4 — More than one annotation per spec is legal; zero is not.**
A spec that owns several distinct capabilities may carry one annotation per
capability (spec 025 owns two separately-deferred producers). A later phase may
re-bind an already-delivered spec under a distinct id when that phase's narrative
depends on it — that is what `v1`'s carry-forward set does. What is forbidden is a
spec with no annotation anywhere, and an annotation whose class is stronger than
the spec's real state.

**Rule 5 — Adding a spec without adding its binding is a release-schema defect.**
It is caught by re-running the census command below, not by review discipline.

### Verify the census

```bash
# Every spec that carries state.json — numbered specs (depth 2) AND _ops
# packets (depth 3). The `-not -path '*/bugs/*'` prune keeps bug packets
# (depth 4) out of a spec census even if maxdepth is later widened.
find specs -mindepth 2 -maxdepth 3 -name state.json -not -path '*/bugs/*' | wc -l

# Every distinct spec bound by an annotation across all packets:
grep -ho 'bubbles:feature[^>]*' docs/releases/*/features.md \
  | grep -o 'spec=[^ ]*' | sed 's/^spec=//' | grep -v '^none$' | sort -u | wc -l
```

**The two counts must be EQUAL when the census is clean.** They were `118` and
`118` at the 2026-08-27 reconciliation.

Read a mismatch as follows — the direction is the diagnosis:

| Observation | Meaning |
|---|---|
| second **<** first | Unbound specs. A spec carries `state.json` but no packet binds it. This is the Rule 5 defect. |
| second **>** first | Either an annotation names a `specs/` directory that does not exist (dangling binding), **or** the first command is undercounting. |
| equal | Clean. |

> **Do not read a double-binding as a count difference.** The second command ends
> in `sort -u`, so a spec deliberately bound in more than one packet still
> contributes exactly one entry. Double-binding has **zero** effect on either
> count, and an earlier version of this section said otherwise — it claimed the
> second count "is expected to be lower than the first only by the number of
> specs deliberately bound in more than one packet", which would have explained
> away a real hole as normal.
>
> **`-maxdepth 2` was the concrete bug.** It matched only `specs/NNN-*/state.json`
> and silently dropped all six `specs/_ops/*` packets, reporting `112` against a
> true `118`. Because the *other* count was correct at `118`, the pair rendered a
> perfectly clean census as `second > first` — the dangling-binding signature.
> The command under-reported the population it was the sole check on.

`spec=none` is excluded by design and is not a hole: it is the legal binding for
an owner-decision row that owns no spec. Exactly one exists today —
`m3-ratify-product-principles` in [`mvp/features.md`](mvp/features.md).

---

## Running Gate G101

```bash
bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh --repo-root "$(pwd)" --phase mvp  --require-coverage
bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh --repo-root "$(pwd)" --phase next --require-coverage
bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh --repo-root "$(pwd)" --phase v1   --require-coverage
```

Exit `0` = clean, `1` = violation, `2` = usage error. There is no `--skip`,
`--force`, or `--ignore` flag, and none will be added. A packet that binds
nothing while carrying a reconciled-packet header fails loud by design: a gate
that asserts nothing must never report green.

Packet placement is separately enforced by
`.github/bubbles/scripts/release-packet-location-guard.sh`.

---

## Ownership boundary

`bubbles.releases` owns everything under `docs/releases/` and the Phase Overview
table inside [`docs/INVESTOR_OVERVIEW.md`](../INVESTOR_OVERVIEW.md). It does
**not** own, and does not edit:

| Artifact | Owner |
|---|---|
| `config/release-trains.yaml`, `config/feature-flags.*.yaml` | `bubbles.train` |
| any spec's `state.json`, `spec.md`, `scopes.md` | `bubbles.plan` / the spec's workflow |
| `docs/Product-Principles.md` | `bubbles.analyst` |
| `.github/instructions/product-principles.instructions.md` | `bubbles.setup` |
| deployment-target adapters and per-target manifests | `bubbles.devops` (concrete bindings live in the knb overlay, never here) |

A defect found in one of those artifacts is routed to its owner, not corrected here.
