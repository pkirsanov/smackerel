# User Validation: BUG-017-002

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] Traceability-guard Gate G068 reports `13/13` Gherkin scenarios mapped to DoD for spec 017 under framework v3.8.0
- [x] Parent feature `scopes.md` Scope 03 has a new DoD bullet explicitly prefixed `Scenario "SCN-GA-NWS-002 NWS severity and event classification":`
- [x] Existing Scope 03 DoD bullets (`Severity mapped …`, `Event types classified …`, `17 unit tests pass …`) are preserved byte-identical
- [x] All 13 `SCN-GA-*` scenarios are listed in `specs/017-gov-alerts-connector/scenario-manifest.json` with linked tests and evidence refs (manifest unchanged by this fix)
- [x] Parent `report.md` cross-references BUG-017-002 in addition to the existing BUG-017-001 entry
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector` and the bug folder both PASS
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector` PASSES with 0 failures
- [x] `go test ./internal/connector/alerts/ -count=1` continues to report 175 tests PASS; race-detector subset remains clean

## Notes

Artifact-only documentation/traceability bug fix triggered by the May 12, 2026 framework upgrade (commit `3037eb8c`) which tightened G068's significant-word matcher (length floor 4 → 3, trimmed stop list). The behavior the SCN-GA-NWS-002 Gherkin describes (joint severity + event-type classification for NWS alerts) is already delivered in `internal/connector/alerts/alerts.go::mapNWSSeverity` + `classifyNWSEventType` and exercised by `TestMapNWSSeverity` + `TestClassifyNWSEventType`. The only gap was that the existing DoD bullets in Scope 03 split severity and event-type into two separate bullets without embedding the SCN-GA-NWS-002 trace ID, which the v3.8.0 matcher no longer accepts. The fix adds one scenario-prefix bullet and changes nothing else.
