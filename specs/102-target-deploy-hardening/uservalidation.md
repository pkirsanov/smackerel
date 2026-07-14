# User Validation — Spec 102 (<deploy-host> Deploy Hardening)

> **Convention (anti-fabrication).** This checklist was authored before
> implementation. The 2026-07-10 planning reconciliation preserves every
> *new-acceptance* item as UNCHECKED `[ ]`: it flips to `[x]` ONLY when the
> implement → test → validate phases prove it in-repo with real evidence, and the
> live-host items are confirmed by the operator against the operator-gated <deploy-host>
> apply (R-102-A/B/C/D). The **Regression Baseline** items are CHECKED `[x]`
> because they describe behavior that is genuinely true on the current live
> `639472f7` stack and MUST remain true after this hardening — the User
> Validation Gate flags any that become unchecked. Nothing here is claimed as
> already-passing that is not; nothing new is pre-checked.
>
> Each item is phrased as something the **operator can actually check**. Items are
> grouped by the four concerns / scenario families (`SCN-102-C1..C4`; see
> [spec.md](spec.md) §4). Repository evidence is indexed in
> [report.md](report.md), while [test-plan.json](test-plan.json) records the
> machine-readable verification handoff.

## Checklist

### Regression Baseline (must remain checked)

- [x] `smackerel-core` still boots with its full `app.env` (it legitimately owns
  the datastore/auth secrets) — the projection tightens ONLY `smackerel-ml`.
- [x] The `monitoring` compose profile is OFF by default — no always-up
  Alertmanager on a normal (non-monitoring) bring-up.
- [x] A healthy backup (postgres + NATS both captured) is still recorded as a
  clean `success` — the degrade is conditional, never always-on.
- [x] Existing gates stay intact: NO-DEFAULTS / fail-loud SST (G028),
  read-only-root posture, externalImages byte-lockstep, the spec-045 resource
  contract, and the isolated-ML-sidecar code-plane isolation (NATS-only transport).
- [x] The QF-companion boundary and QF packet metadata (Principle 10) are
  untouched.

### SCN-102-C1 — ML-sidecar compute-only secret isolation

Evidence linkage: [report.md § SCOPE-102-01](report.md#scope-102-01--ml-sidecar-compute-only-secret-isolation).

- [ ] After a config generate, the `smackerel-ml` env file (`ml.env`) contains
  NONE of the managed secrets (`POSTGRES_PASSWORD`, `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`,
  `AUTH_AT_REST_HASHING_KEY`, `AUTH_BOOTSTRAP_TOKEN`, `TELEGRAM_BOT_TOKEN`,
  `KEEP_GOOGLE_APP_PASSWORD`, `CARD_REWARDS_GCAL_CREDENTIALS`,
  `WEB_REGISTRATION_INVITE_TOKEN`, `LLM_PROVIDER_SECRET_MASTER_KEY`) and none of the
  `POSTGRES_*` connection parts.
- [ ] `smackerel-ml` still starts cleanly against `ml.env` — no missing-env
  fail-loud error (it has every compute key it actually reads).
- [ ] Re-adding `env_file: ./app.env` to `smackerel-ml` (or slipping a secret into
  its env) makes `./smackerel.sh test pre-push` FAIL and name the offending key —
  and there is no `--skip`/`--force`/`--ignore` flag that suppresses it.
- [ ] Adding a database driver (e.g. `psycopg`) or a `DATABASE_URL` read to the ML
  Python surface makes the guard FAIL and name the file+line.
- [ ] `smackerel-ml` has no network route to the `postgres` container (defense in
  depth), while `smackerel-core` still reaches postgres.

### SCN-102-C2 — Durable Prometheus → Alertmanager → ntfy routing

Evidence linkage: [report.md § SCOPE-102-02](report.md#scope-102-02--durable-prometheus--alertmanager--ntfy-routing).

- [ ] After a fresh apply (no manual standup script), a generated bundle's
  `prometheus.yml` has an `alerting:` block and the deploy stack has an
  `alertmanager` service under the `monitoring` profile.
- [ ] Re-applying the stack does NOT lose the alerting block — alerting survives
  every deploy with zero manual re-run (the fragile `alertmanager-standup.sh` is
  gone).
- [ ] A fired alert arrives at the `self-hosted-alerts` ntfy topic as a **titled**
  message with a priority derived from its severity — not the raw Alertmanager
  JSON body. *(Live delivery to the operator's real ntfy = operator-gated,
  R-102-A/C; the Docker-host proof uses a test ntfy sink.)*
- [ ] The product repo contains NO operator ntfy host / IP / topic value — the
  real endpoint is injected by the knb adapter at apply time.

### SCN-102-C3 — Model-envelope correctness + BUG-026-006

Evidence linkage: [report.md § SCOPE-102-03](report.md#scope-102-03--model-envelope-correctness--bug-026-006).

**Implementation handoff (2026-07-10):** current-session evidence now covers
13/13 Python builders, strict profile/merge/hosted-provider behavior, typed Go
`/llm/chat` plus GET `/api/tags` boundaries, KV-envelope tests, and the
BUG-026-006 output budget. TP-C3-21 itself passes against real ephemeral NATS via
`integration-light`; its exact full integration command exits during the missing
Ollama test-image pull before the selector runs. These acceptance boxes remain
unchecked for `bubbles.validate`; no implementation run self-certifies them.

- [ ] Every one of the 13 production Python Ollama inference builders applies the
  selected model's SST `num_ctx` as `options.num_ctx` through one shared request-
  profile contract; Go only validates/routes typed compute requests and performs
  bounded read-only `/api/tags` probes, so no Go inference client or host-side
  `ollama create ... PARAMETER num_ctx` override exists.
- [ ] Missing, malformed, duplicate, or non-positive profile data, and a selected
  Ollama model with no profile, fail loud before any inference network call.
- [ ] Both LiteLLM kwargs and native `/api/generate` JSON preserve existing options,
  `think`, determinism settings, tools/format/token budgets, and top-level request
  `keep_alive`; hosted-provider branches carry no Ollama-only fields.
- [ ] Config generation REFUSES (fail-loud) a model profile that understates its
  real resident footprint, naming the model, the required MiB, and the envelope —
  i.e. the validator is honest about weights + KV(num_ctx, num_parallel).
- [ ] An uncapped context (e.g. `gemma4:26b` default) is refused before it can fail
  to load; the SST-capped `num_ctx` that fits the host is accepted.
- [ ] The qwen3+gemma4 co-residency-vs-swap posture is a documented SST decision
  (`max_loaded_models`); default is the safe on-demand swap (`1`) until live
  `ollama ps` co-residency is proven on <deploy-host>. *(Live proof = operator-gated,
  R-102-D.)*
- [ ] BUG-026-006's output-token budget is read from SST (not a hardcoded `2000`),
  and the bug advances toward closure with real before/after evidence.

### SCN-102-C4 — Backup-adapter durability formalization

Evidence linkage: [report.md § SCOPE-102-04](report.md#scope-102-04--backup-adapter-durability-formalization).

- [ ] A backup where the NATS volume is present but the capture fails is recorded
  as `warning` (degraded) — never a clean `success` — and any stale prior
  `nats-data` is rotated out, not re-shipped as fresh.
- [ ] A `root:root 0600` (unreadable) `manifest.yaml` does NOT hard-fail the backup:
  the manifest degrades to not-captured (non-fatal) and the backup still succeeds on
  its critical postgres capture; `apply.sh` has chowned the manifest back to the
  operator so it is readable next time.
- [ ] The backup-status gauge (`smackerel_backup_last_success_unixtime`) ADVANCES on
  a successful postgres capture and HOLDS its prior value on a failed one; the status
  JSON matches the `internal/backup.Status` schema (schema_version 1).

## Verification note

- In-repo proof (Docker host, macOS) covers everything except the four
  operator-gated live-host steps: the live <deploy-host> re-apply (R-102-A), the rollback
  drill (R-102-B), the knb git reconcile/push (R-102-C), and the live `ollama ps`
  co-residency proof (R-102-D). Those are confirmed by the operator on <deploy-host> and
  recorded as external blockers — never faked in-repo.
- Planning coverage and ownership declarations are in
  [scopes.md § Planning Contract Reconciliation](scopes.md#planning-contract-reconciliation-2026-07-10);
  no user-acceptance checkbox was changed by this planning pass.
