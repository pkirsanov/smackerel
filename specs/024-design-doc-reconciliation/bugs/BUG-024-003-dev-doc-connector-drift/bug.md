# Bug: BUG-024-003 docs/Development.md L31 understates passive connector count (15→16, missing QF Decisions) + no automated guard catches docs↔runtime connector-count drift

## Summary

Sweep round 9 of `sweep-2026-05-24-r10` (`mode: chaos-hardening`, parent-expanded) ran the chaos trigger probe on `specs/024-design-doc-reconciliation` and surfaced two real implementation-vs-design drifts plus one artifact-internal inconsistency in spec 024's own `spec.md`:

1. **F1 (HIGH) — Real docs↔runtime drift in `docs/Development.md` line 31.** The "Current Capabilities" bullet says `15 passive connectors` and enumerates a 15-item parenthetical list (IMAP, CalDAV, YouTube, RSS, Bookmarks, Browser, Keep, Maps, Hospitable, GuestHost, Discord, Twitter, Weather, Alerts, Markets) — `QF Decisions` (`internal/connector/qfdecisions/`, registered in `cmd/core/connectors.go` lines 22 + 47 + 50) is missing. `cmd/core/connectors.go` registers **16** connectors today. This is exactly the kind of doc↔runtime drift spec 024's R-006 contract is supposed to prevent ("All connector lists in the document must account for the implemented connectors"). It is the same drift class that BUG-024-002 closed for `docs/smackerel.md` §22.7 + §24-A on 2026-05-24 — `docs/Development.md` was overlooked by that pass because R-006 only enumerated `docs/smackerel.md` surfaces.
2. **F2 (MEDIUM) — No automated guard catches docs↔runtime connector-count drift.** Scratch simulation: cloning `docs/smackerel.md` to `/tmp/`, replacing `### 22.7 Committed Connector Inventory (16 connectors)` with `(17 connectors)`, and running every Bubbles guard against the parent spec produces **zero diagnostic output** — `state-transition-guard.sh`, `artifact-freshness-guard.sh`, `artifact-lint.sh`, and `traceability-guard.sh` do not parse the §22.7 header for runtime agreement. No Go contract test under `internal/deploy/` pins agreement either. The drift class that produced F1 here (and BUG-024-002 last round) can recur the moment any future spec adds a 17th connector without invoking spec 024 reconciliation.
3. **F3 (LOW) — Spec 024 `spec.md` internal inconsistency between BS-004 and R-006.** `BS-004` (lines 119-121) lists **16** connectors including `qfdecisions`. `R-006` (lines 217-235) says `the 15 implemented connectors` and enumerates **15** without `qfdecisions`. BUG-024-002 updated BS-004 on 2026-05-24 but did not also propagate to R-006, leaving the spec's own acceptance criterion internally inconsistent.

## Severity

- [ ] Critical — System unusable, data loss
- [x] High — F1 is a real docs↔runtime drift that breaks spec 024's R-006 contract; F2 is the hardening gap that allowed F1 and BUG-024-002 to land in the first place; F3 is an artifact-internal inconsistency in the spec that owns the reconciliation contract
- [ ] Medium
- [ ] Low

## Status

- [x] Reported
- [x] Confirmed by sweep round 9 chaos-hardening probe
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. From clean HEAD `2c8e3242` (sweep round 8 close-out), run `grep -nE '15 passive connectors' docs/Development.md`.
2. Observe `docs/Development.md:31:- 15 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko)` — 15 connectors enumerated, `qfdecisions` absent.
3. Run `grep -cE 'qfdecisions|QF Decisions' docs/Development.md` → `0` hits.
4. Run `grep -nE '^\s+qfDecisionsConn|qfDecisionsConnector' cmd/core/connectors.go`.
5. Observe registration of `qfDecisionsConn` at lines 22 (import), 47 (instantiate), 50 (slice append for `svc.registry.Register`). Live registry registers 16 connectors.
6. Run `find internal/connector -maxdepth 1 -mindepth 1 -type d -not -name photos -not -name adapters | wc -l` → 16 directories with wired connectors.
7. Run `grep -nE 'BS-004|R-006' specs/024-design-doc-reconciliation/spec.md`.
8. Observe BS-004 (line 119) lists 16 connectors with `qfdecisions`; R-006 (line 217) says `15 implemented connectors` with 15-item list missing `qfdecisions`. Spec is internally inconsistent.
9. Scratch simulation (no production write): `cp docs/smackerel.md /tmp/scratch.md && sed -i 's|### 22.7 Committed Connector Inventory (16 connectors)|### 22.7 Committed Connector Inventory (17 connectors)|' /tmp/scratch.md` then run the four Bubbles guards against parent spec 024 → all four exit 0 because no guard reads `docs/smackerel.md`. Delete scratch.
10. Run `grep -rEn 'connector_count|connectors_total|16 connectors|All 16 connectors are implemented|connector inventory' .github/bubbles/scripts/` → 0 hits. No framework guard pins the count.

## Expected Behavior

- `grep -nE '15 passive connectors' docs/Development.md` → 0 hits; line 31 reads `- 16 passive connectors (...QF Decisions companion ingestion via spec 041 read-only packet flow)`.
- `grep -cE 'qfdecisions|QF Decisions' docs/Development.md` ≥ 1.
- `grep -nE 'R-006' specs/024-design-doc-reconciliation/spec.md` followed by the connector list shows 16 enumerated connectors including `qfdecisions`, matching BS-004 verbatim.
- A new Go contract test under `internal/deploy/docs_connector_count_contract_test.go` asserts that the count of registered connectors in `cmd/core/connectors.go` matches both (a) `docs/smackerel.md` §22.7 header `### 22.7 Committed Connector Inventory (N connectors)` and (b) `docs/Development.md` line 31 `- N passive connectors (...)`. The test has at least three adversarial sub-tests proving non-tautological: simulated count mismatch in `connectors.go`, simulated mismatch in `smackerel.md` header, simulated mismatch in `Development.md` line.
- `./smackerel.sh test unit --go` continues to pass with the new test included.
- Spec 024 `status` remains `done`. `cmd/core/connectors.go`, `internal/connector/qfdecisions/`, schema, NATS topology, web template, prompt contract, Telegram command, deploy script, compose file, and `smackerel.yaml` are unchanged.

## Actual Behavior

- `docs/Development.md` line 31 says `15 passive connectors` with a 15-item parenthetical missing QF Decisions; runtime registers 16.
- Spec 024 `spec.md` BS-004 says 16 connectors with qfdecisions; R-006 says 15 without qfdecisions. Internal inconsistency.
- Scratch simulation confirms: a future doc edit that introduces a 15-vs-runtime-N drift (or 17-vs-runtime-N drift) in any of `docs/smackerel.md` §22.7 / `docs/smackerel.md` §24-A / `docs/Development.md` line 31 would NOT be detected by any existing framework guard or Go contract test.

## Environment

- Branch: `main`, HEAD `2c8e3242`
- Sweep: `sweep-2026-05-24-r10` round 9, mode `chaos-hardening`, executionModel `parent-expanded-child-mode`
- Parent feature: `specs/024-design-doc-reconciliation` (currently `status: done` end-to-end since BUG-024-002 round 29 close-out on 2026-05-24; BS-004 was updated in that pass, R-006 was not)
- Source-of-truth: `cmd/core/connectors.go` lines 11-26 (imports) + lines 30-50 (instantiation + registration slice) — 16 connectors wired
- `internal/connector/photos/` exists as a photo-library package (used by `cmd/core/wiring.go` as `photolib`) and is intentionally not a registered connector
- Real-drift surface: `docs/Development.md` line 31; secondary inconsistency: `specs/024-design-doc-reconciliation/spec.md` R-006

## Error Output

```text
$ grep -nE '15 passive connectors' docs/Development.md
31:- 15 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko)
$ grep -cE 'qfdecisions|QF Decisions' docs/Development.md
0
$ grep -nE '^\s+qfDecisions' cmd/core/connectors.go
22:	qfDecisionsConnector "github.com/smackerel/smackerel/internal/connector/qfdecisions"
47:	qfDecisionsConn := qfDecisionsConnector.New("qf-decisions")
50:		discordConn, twitterConn, weatherConn, alertsConn, marketsConn, qfDecisionsConn,
$ find internal/connector -maxdepth 1 -mindepth 1 -type d -not -name photos -not -name adapters | wc -l
16
$ grep -rEn 'connector_count|connectors_total|All 16 connectors are implemented' .github/bubbles/scripts/
(zero output)
```

## Workaround

None for F1 — drift between docs and runtime cannot be worked around by readers; the doc must be reconciled. For F2, downstream specs adding a new connector currently rely on remembering to also update `docs/smackerel.md` §22.7 + §24-A + `docs/Development.md` L31 + spec 024 R-006 — that human reminder failed for spec 041 (caught a sweep later by BUG-024-002) and for `docs/Development.md` here. For F3, anyone reading spec 024 cannot trust BS-004 and R-006 simultaneously because they disagree.

## Root Cause Analysis (Five Whys)

- **Why did F1 land?** Because spec 041 added `internal/connector/qfdecisions/` on 2026-05-22 without invoking spec 024 reconciliation. BUG-024-002 (round 29 of the prior sweep) caught the drift in `docs/smackerel.md` §22.7 and §24-A but R-006's enumeration of doc surfaces only listed `docs/smackerel.md`, not `docs/Development.md` — so `docs/Development.md` L31 was never inspected and stayed at 15.
- **Why did R-006 only enumerate `docs/smackerel.md`?** Because the original 2026-04-10 spec 024 reconciliation scoped itself to `docs/smackerel.md` (the design document); `docs/Development.md` was treated as an operational/developer-onboarding doc rather than a product-truth doc. The R-006 contract was written under that assumption.
- **Why did the chaos probe catch F1 when BUG-024-002 didn't?** Because chaos-hardening's job is to probe for drift the prior probe missed. BUG-024-002 ran the reconcile probe over `docs/smackerel.md` only. Sweep round 9's chaos probe enumerated *every* docs/*.md that references "connector" and counted them against `cmd/core/connectors.go` — that is the kind of stochastic abuse `chaos` triggers do.
- **Why did F2 (no automated guard) survive every prior round?** Because the prior round-29 fix (BUG-024-002) was a reconcile-to-doc fastlane — it brought the artifact set to current gate standards and corrected the live drift but did not invest in forward-detection. The chaos-hardening mode is the correct vehicle to add forward-detection so that the next time someone adds a 17th connector, a Go contract test fails immediately at `./smackerel.sh test unit --go` time instead of waiting for a future stochastic sweep round to catch it.
- **Why did F3 (BS-004/R-006 inconsistency) survive BUG-024-002?** BUG-024-002 updated BS-004 from 15 to 16 connectors with `qfdecisions` added but did not also propagate the same change to R-006. Two acceptance-criterion lists in the same `spec.md` disagreeing is exactly the kind of internal artifact drift Gate G068 / Check 22 cannot catch because both surfaces are valid Gherkin/markdown — the disagreement is semantic.
- **Why do these missed-during-prior-pass drifts keep happening?** Because spec 024 owns a doc↔runtime reconciliation contract that, by its nature, must be re-verified every time the runtime grows. Without an automated forward-detection guard, the contract is checked only when someone manually invokes spec 024 reconciliation. The chaos-hardening close-out for this round adds the missing automated guard.

## Related

- Parent: `specs/024-design-doc-reconciliation/`
- Prior sibling bugs:
  - `specs/024-design-doc-reconciliation/bugs/BUG-024-001-dod-scenario-fidelity-gap/` (G068 DoD fidelity, closed)
  - `specs/024-design-doc-reconciliation/bugs/BUG-024-002-reconcile-artifact-drift/` (32 BLOCKS + 19 freshness + §22.7/§24-A docs drift, closed 2026-05-24; this packet handles the `docs/Development.md` L31 surface that BUG-024-002 missed)
- Real-drift source spec: `specs/041-qf-companion-connector/` (`status: done_with_concerns`; introduced `internal/connector/qfdecisions/` on 2026-05-22 via commits `39ca4fcb`, `c22151a5`, `43ce5096`)
- Sweep ledger: `.specify/memory/sweep-2026-05-24-r10.json` round 9 (this packet)
- Reference pattern (Go contract test + adversarial sub-tests): `internal/deploy/monitoring_docs_contract_test.go` (T-049-005, asserts every alert in `config/prometheus/alerts.yml` is mentioned in `docs/Operations.md`); `internal/deploy/compose_contract_test.go` (asserts SST host-bind invariants with adversarial sub-tests); `internal/deploy/state_concerns_contract_test.go` (round 6 of this sweep, asserts `done_with_concerns` schema with adversarial sub-tests)
