# User Validation: [BUG-018-001] Reconcile Artifact-Governance Drift on Spec 018

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Checklist

- [x] AC-01 — state-transition-guard on parent spec 018 returns Exit 0 (≤2 documented residuals)
- [x] AC-02 — artifact-lint on parent spec 018 returns Exit 0
- [x] AC-03 — traceability-guard on parent spec 018 returns Exit 0 with 11/11 G068 fidelity
- [x] AC-04 — state-transition-guard on BUG-018-001 packet returns Exit 0
- [x] AC-05 — artifact-lint on BUG-018-001 packet returns Exit 0
- [x] AC-06 — traceability-guard on BUG-018-001 packet returns Exit 0
- [x] AC-07 — `go test ./internal/connector/markets/... -count=1 -cover` reports 151 PASS / 0 FAIL / 97.2% coverage
- [x] AC-08 — `git diff --name-only HEAD~1..HEAD -- internal/connector/markets/` returns empty
- [x] AC-09 — Closing commit message starts with `bubbles(018/bug-018-001)` or `spec(018)`
- [x] AC-10 — `git diff --name-only HEAD~1..HEAD` returns only paths under `specs/018-financial-markets-connector/` (or sweep ledger)

## Acceptance Criteria Validation Checklist

| AC | Criterion | Status | Evidence |
|---|---|---|---|
| AC-01 | state-transition-guard.sh on parent spec 018 returns Exit 0 (or ≤2 documented residuals) | ✅ Validated | [report.md § Verification Evidence → Guard Pass — State-Transition-Guard (Parent Spec 018)](report.md#guard-pass--state-transition-guard-parent-spec-018) |
| AC-02 | artifact-lint.sh on parent spec 018 returns Exit 0 | ✅ Validated | [report.md § Verification Evidence → Guard Pass — Artifact-Lint (Parent Spec 018)](report.md#guard-pass--artifact-lint-parent-spec-018) |
| AC-03 | traceability-guard.sh on parent spec 018 returns Exit 0 with 11/11 G068 fidelity | ✅ Validated | [report.md § Verification Evidence → Guard Pass — Traceability-Guard (Parent Spec 018)](report.md#guard-pass--traceability-guard-parent-spec-018) |
| AC-04 | state-transition-guard.sh on BUG-018-001 packet returns Exit 0 (or ≤1 documented residual) | ✅ Validated | [report.md § Verification Evidence → Guard Pass — BUG-018-001 Packet](report.md#guard-pass--bug-018-001-packet) |
| AC-05 | artifact-lint.sh on BUG-018-001 packet returns Exit 0 | ✅ Validated | [report.md § Verification Evidence → Guard Pass — BUG-018-001 Packet](report.md#guard-pass--bug-018-001-packet) |
| AC-06 | traceability-guard.sh on BUG-018-001 packet returns Exit 0 | ✅ Validated | [report.md § Verification Evidence → Guard Pass — BUG-018-001 Packet](report.md#guard-pass--bug-018-001-packet) |
| AC-07 | `go test ./internal/connector/markets/... -count=1 -cover` reports 151 PASS / 0 FAIL / 97.2% coverage | ✅ Validated | [report.md § Verification Evidence → Regression Baseline (markets-suite)](report.md#regression-baseline-markets-suite) |
| AC-08 | `git diff --name-only HEAD~1..HEAD -- internal/connector/markets/` returns empty | ✅ Validated | [report.md § Git-Backed Proof](report.md#git-backed-proof) |
| AC-09 | Closing commit message starts with `bubbles(018/bug-018-001)` or `spec(018)` | ✅ Validated | git log of the closing commit |
| AC-10 | `git diff --name-only HEAD~1..HEAD` returns only paths under `specs/018-financial-markets-connector/` (or sweep ledger) | ✅ Validated | git diff inspection of the closing commit |

## Reviewer Sign-Off

- Bug status: **resolved** (artifact-only reconcile applied at HEAD `381cc0e9388c49a7a2fa698a70b1feca7f6c8422` → reconciled HEAD)
- Production-code regression: **zero** (151 PASS, 97.2% coverage, exact match to R09 and R12 baselines)
- Parent spec 018 status: remains **done** with restored guard-clean state
- Sweep round: `sweep-2026-05-23-r30` round 27 of 30 closes to `completed_owned`
- No further routing required.
