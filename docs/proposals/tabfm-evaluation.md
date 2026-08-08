# Proposal: Evaluate TabFM For Topic Engagement Prediction

> **STATUS: PROPOSAL — NOT ADOPTED. Blocked on a data prerequisite.**
> Nothing here has been installed or wired into the ML sidecar. No dependency was
> added to `ml/pyproject.toml` or `ml/requirements.txt`. This document records a
> candidate experiment and — more usefully — why the obvious version of that
> experiment would be worthless.
>
> | Field | Value |
> | --- | --- |
> | **Subject** | [`google-research/tabfm`](https://github.com/google-research/tabfm) v1.0.1 — zero-shot tabular foundation model |
> | **Scope** | Offline evaluation of forward topic-engagement prediction |
> | **Production use** | **Prohibited by the model's weight licence.** See [§5](#5-blockers). |
> | **Date** | 2026-08-07 |
> | **Next step if pursued** | `/bubbles.workflow` — this is not a spec and grants no implementation authority |

---

## 1. Summary

Smackerel's topic lifecycle is driven by a weighted momentum formula with six
inputs and fixed coefficients. Those six inputs are already stored per topic, and
a Python ML sidecar already exists. On the surface this looks like an ideal
tabular-classification target.

It is not — at least not in its obvious form. The lifecycle state is a
deterministic function of the momentum score, so training a model to predict the
state would only teach it to imitate the formula it was meant to challenge.

There is a legitimate target underneath: predicting **future engagement** rather
than reproducing the current label. That target has no training data yet, because
topic counters are stored as current values with no history.

---

## 2. What TabFM Is (grounded)

| Property | Value |
| --- | --- |
| Task support | Classification (max 10 classes) and regression on continuous targets |
| Method | In-context learning; context rows in, prediction out in one forward pass, no per-dataset training |
| Interface | `TabFMClassifier` / `TabFMRegressor`, scikit-learn compatible |
| Practical limits | `max_num_rows` default 100 context rows; `max_num_features` default 500 |
| Runtime | Python >= 3.11, JAX or PyTorch backend |
| Checkpoint size | 6.56 GB (classification), 6.59 GB (regression) |
| Training data | Entirely synthetic, from structural causal models |
| Code licence | Apache-2.0 |
| Weight licence | `tabfm-non-commercial-v1.0` — non-commercial, non-production |
| Maturity | v1.0.0 released 2026-06-29; v1.0.1 (2026-07-09) fixed weight loading, multi-device crashes, dtype, and a mismatched-checkpoint path that previously produced silently wrong predictions. No technical report. Not an officially supported Google product. |

The six-state lifecycle vocabulary (`emerging`, `active`, `hot`, `cooling`,
`dormant`, `archived`) fits comfortably inside the 10-class ceiling, and six
features are far below the 500-feature limit. The mechanical fit is genuine; the
problem is the label, not the shape.

---

## 3. Why The Obvious Experiment Is Invalid

[`lifecycle.go`](../../internal/topics/lifecycle.go) computes momentum from six
inputs with fixed weights, then maps the score to a state through fixed
thresholds:

```go
func CalculateMomentum(captures30d, captures90d, searchHits30d, starCount, connectionCount int, daysSinceActive int, cfg MomentumConfig) float64 {
    raw := float64(captures30d)*cfg.CaptureWeight30d +
        float64(captures90d)*cfg.CaptureWeight90d +
        float64(searchHits30d)*cfg.SearchWeight30d +
        float64(starCount)*cfg.StarWeight +
        float64(connectionCount)*cfg.ConnectionWeight

    decay := math.Exp(-cfg.DecayFactor * float64(daysSinceActive))
    return raw * decay
}
```

`TransitionState` then thresholds that score. Because the stored `state` is a
pure function of the same six features the model would consume, a model trained
on `(features → state)` is performing **function approximation of a known
formula**, not learning anything about the world. A high score would prove only
that the formula is learnable, which is not in doubt.

Any proposal that reports such a result as "TabFM matches the R-208 heuristic"
would be measuring the wrong thing.

---

## 4. The Legitimate Target And Its Prerequisite

**Question worth asking.** Given a topic's current features, will it actually
receive engagement — new captures, searches, or stars — in the next N days?

That label is independently observable. It is not derived from the momentum
formula, so a model can genuinely beat, or fail to beat, the heuristic at
predicting it. This reframes momentum from a scoring rule into a *forecast* that
can be validated.

**Why it cannot be evaluated today.** The `topics` table stores only current
counters:

```sql
state                   TEXT DEFAULT 'emerging',
momentum_score          REAL DEFAULT 0.0,
capture_count_total     INTEGER DEFAULT 0,
capture_count_30d       INTEGER DEFAULT 0,
capture_count_90d       INTEGER DEFAULT 0,
search_hit_count_30d    INTEGER DEFAULT 0,
last_active             TIMESTAMPTZ,
```

— [`001_initial_schema.sql`](../../internal/db/migrations/001_initial_schema.sql)

`UpdateAllMomentum` overwrites `momentum_score` and `state` in place. There is no
per-topic history table, so there is no way to reconstruct "what the features
looked like 30 days ago" or "what happened next". Both sides of the training pair
are missing.

**Prerequisite.** A periodic snapshot of per-topic features, retained over time.
Only after several months of accumulation would there be enough
`(features at time T → engagement during T..T+N)` pairs to evaluate anything.

---

## 5. Blockers

### 5.1 No historical data (blocking, and slow to resolve)

Described in [§4](#4-the-legitimate-target-and-its-prerequisite). Unlike a
missing dependency, this cannot be fixed in an afternoon — it requires
instrumentation followed by elapsed calendar time.

### 5.2 The weight licence forbids production use (decisive)

`tabfm-non-commercial-v1.0` defines "Non-Commercial Purpose" as testing,
evaluation, or research **not** tied to commercial gain or production deployment,
and explicitly excludes use "in direct or indirect interactions with end users or
production systems". A self-hosted personal instance is a production system that
interacts with its user, so the released weights cannot drive the live lifecycle
engine even in a single-user deployment.

### 5.3 Footprint conflicts with self-hosting

Constitution Principle 1 requires core product value to work on user-controlled
hardware, and Principle 6 requires the committed runtime to boot as a local,
self-hosted stack. Adding a 6.6 GB checkpoint alongside the existing
`sentence-transformers` and `transformers` stack materially raises the minimum
hardware bar for every self-hoster, to improve one internal scoring heuristic.
That trade is poor.

### 5.4 Explainability

Constitution Principle 4 requires synthesis to be traceable to source artifacts.
The current momentum formula is fully transparent — every term is inspectable and
its contribution is arithmetic. An in-context foundation model is not
interpretable in the same way. If topic state ever surfaces in user-facing
explanation, this is a real regression in traceability, not a technicality.

### 5.5 The Go/Python boundary

Constitution Principle 2 keeps Go as the primary runtime and confines Python to
ML sidecar responsibilities. The lifecycle engine is Go and writes topic state
directly. Routing lifecycle decisions through the Python sidecar would move a
core state-write decision across that boundary, which needs explicit design, not
an incidental consequence of an experiment.

---

## 6. Decision Criteria

Pursue further only if **all** of the following hold:

- [ ] Per-topic feature snapshots have been collected for long enough to build forward-engagement labels.
- [ ] The evaluated target is future engagement, never the formula-derived state.
- [ ] TabFM beats **both** the momentum heuristic and a simple logistic-regression baseline at predicting that engagement.
- [ ] The accuracy gain justifies the footprint and the loss of arithmetic explainability.
- [ ] The result is accepted as research insight only, given the licence.

---

## 7. Non-Goals

- Replacing `CalculateMomentum` or `TransitionState`.
- Adding TabFM to `ml/pyproject.toml` or `ml/requirements.txt`.
- Moving lifecycle state writes out of the Go runtime.
- Reporting "the model reproduced the heuristic" as a meaningful result.
- Any cloud inference path for topic scoring.

---

## 8. Honest Alternative

The valuable step here is independent of TabFM: **start snapshotting topic
features over time.** That single change turns the momentum formula from an
unfalsifiable scoring rule into a testable forecast, and would let anyone ask
whether the six hand-chosen coefficients actually predict engagement.

If the answer turns out to be no, the fix is likely to be re-fitting those
coefficients — a small, explainable, dependency-free change that respects both
the local-first and explainability principles. On the evidence available today,
this repository is the weakest of the workspace candidates for TabFM, and the
strongest candidate for simply measuring the heuristic it already has.
