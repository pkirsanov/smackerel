# User Validation: 071 IntentTrace Observability Surface

## Checklist

- [x] Baseline planning checklist exists for operator dashboard, privacy review, replay, and bypass-guard use cases.
- [x] Baseline scenario coverage maps SCN-071-A01 through SCN-071-A10 into scopes, scenario manifest, and test plan artifacts.
- [x] Baseline validation acknowledges that no runtime completion or test-pass claim is made by this planning artifact.

## Acceptance Review Checklist

- [ ] Operator can reconcile total turns as sampled plus sampled-out envelopes.
- [ ] Privacy reviewer can confirm raw text and sensitive slots are absent when source policy forbids persistence.
- [ ] Scenario author can run read-only replay and compare route/tool calls without side effects.
- [ ] Operator can join refusal counters and IntentTrace rows by cause label.
- [ ] Retention behavior is observable and configurable through required SST keys.
