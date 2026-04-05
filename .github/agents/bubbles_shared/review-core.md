# Review Core

Shared baseline for `bubbles.code-review`, `bubbles.system-review`, and `bubbles.spec-review`.

Always apply these rules:
- Start from the smallest declared review scope that answers the request.
- Use the review-specific config registry (`bubbles/code-review.yaml` or `bubbles/system-review.yaml`) when present instead of hardcoding lens behavior.
- Dispatch specialist lenses with `runSubagent`; do not impersonate those specialists inline.
- Keep findings evidence-backed and scoped to observed behavior, code, or artifact drift.
- Separate diagnosis from execution: reviews identify and prioritize work; owning specialists update artifacts or code.
- Promote findings through the owning agent when the review reveals required spec/design/plan/documentation changes.
- Keep the output stable and comparable across runs: scope, summary, findings, prioritized actions, and explicit next move.

Review surface split:
- `bubbles.code-review` = engineering-only review of code slices and repositories.
- `bubbles.system-review` = cross-domain product/runtime/trust/UX/system review.
- `bubbles.spec-review` = spec freshness and trust classification against implementation reality.