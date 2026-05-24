# Bug: BUG-009-002 — NormalizeURL chaos R30 (control chars, default ports, trailing dot)

## Classification

- **Type:** Runtime defect (URL normalization)
- **Severity:** HIGH for F-CHAOS-R30-001 (PG insert failure + log-injection vector); MEDIUM for F-CHAOS-R30-002 and F-CHAOS-R30-003 (silent dedup misses).
- **Parent Spec:** 009 — Bookmarks Connector
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed

## Problem Statement

Stochastic-quality-sweep round 14 of `sweep-2026-05-23-r30` ran a chaos probe (parent-expanded child workflow mode `chaos-hardening`) against `NormalizeURL` in `internal/connector/bookmarks/dedup.go` using adversarial inputs that mirror real-world browser-export quirks. Three concrete defects surfaced. None had pre-existing coverage by the R16/R24/C17 chaos batches.

### F-CHAOS-R30-001 — ASCII control characters survive into SourceRef (HIGH)

`url.Parse` accepts and round-trips ASCII control characters (`0x00`-`0x1F`, `0x7F`) in the host and path. The connector then writes the normalized URL straight into the `artifacts.source_ref` TEXT column. Three concrete failure modes:

1. **PostgreSQL INSERT failure on NUL.** `source_ref TEXT NOT NULL` cannot store `\x00`. The whole `INSERT INTO artifacts (...) VALUES (...)` aborts, the file is never recorded in the dedup table, and the connector silently loses capture for that bookmark (and possibly the entire batch, depending on transaction scope).
2. **Log injection.** When the connector logs `slog.Info("dedup hit", "url", normalized)`, embedded `\n`/`\r` produce extra log lines that an attacker controls — forged log entries can hide real activity, evade alerting, or mislead the human reviewing the log.
3. **Dedup miss.** An attacker (or buggy exporter) introduces `\n`/`\r`/`\t` variants of the same URL; each variant hashes to a distinct `source_ref` and bypasses the `IsKnown` check, multiplying capture cost and polluting the topic graph.

Reproduction (in-package probe, captured by R30 test):

```text
input:  "http://example.com/path\x00more"   normalized: "http://example.com/path\x00more"   (NUL preserved → PG insert fails)
input:  "http://example.com/path\nmore"     normalized: "http://example.com/path\nmore"     (LF preserved → log injection)
input:  "http://example.com/\tpath"          normalized: "http://example.com/\tpath"        (TAB preserved → dedup miss)
```

### F-CHAOS-R30-002 — Default ports are not elided (MEDIUM)

`https://example.com:443/page` and `https://example.com/page` resolve to the same origin in every browser, but `url.Parse` preserves the explicit `:443` and `NormalizeURL` returned the port verbatim. Same for `http://...:80/...` and `ftp://...:21/...`. Result: two `source_ref` rows for one resource, dedup silently bypassed whenever a bookmark exporter chooses to include the default port.

Reproduction:

```text
input:  "https://example.com:443/page"   normalized: "https://example.com:443/page"  (should be "https://example.com/page")
input:  "http://example.com:80/page"     normalized: "http://example.com:80/page"    (should be "http://example.com/page")
input:  "ftp://files.example.com:21/pub" normalized: "ftp://files.example.com:21/pub" (should be "ftp://files.example.com/pub")
```

### F-CHAOS-R30-003 — Trailing DNS-root dot is not stripped (MEDIUM)

`http://example.com./foo` and `http://example.com/foo` resolve to the same origin (the trailing dot is the DNS root indicator), but `NormalizeURL` preserved it. Same dedup-miss class as R30-002.

Reproduction:

```text
input:  "http://example.com./foo"     normalized: "http://example.com./foo"   (should be "http://example.com/foo")
input:  "https://www.example.com./b"  normalized: "https://example.com./b"   (www stripped but trailing dot kept)
```

## Acceptance Criteria

- [x] `NormalizeURL` strips ASCII control characters (`0x00`-`0x1F` and `0x7F`) from the input before parsing so no control byte can survive into `SourceRef` (F-CHAOS-R30-001).
- [x] `NormalizeURL` elides the default port for `http` (`:80`), `https` (`:443`), and `ftp` (`:21`) so canonically-equivalent URLs share a `SourceRef`, while non-default ports remain preserved (F-CHAOS-R30-002).
- [x] `NormalizeURL` strips one or more trailing `.` characters from the hostname so the DNS-root form dedups against the bare form (F-CHAOS-R30-003).
- [x] Adversarial regression tests are added that FAIL when each fix is reverted (proven by toggling the fix in `dedup.go` and re-running the new R30 tests).
- [x] All pre-existing `NormalizeURL_*`, `ChaosR24_*`, and bookmarks-package tests continue to pass.
- [x] `go vet ./internal/connector/bookmarks/...` and `gofmt -l` are clean.
- [x] Parent `specs/009-bookmarks-connector/state.json` and `report.md` reference this bug under chaos R30 history.

## Boundary

- No DB schema change.
- No connector-config-shape change.
- No change to the `Connect/Sync/Health/Close` contract.
- Spec 055 work-in-progress files are NOT staged in the close-out commit.
