# BUG-016-001 — Scopes

Status: done

---

## Scope 01 — Bump `PerCallTimeoutMs` 2000 → 8000

**Status:** Done (shipped in commit `96acf294`, deployed
2026-06-09T15:33:39Z)

**Depends on:** none

**Implementation:** see `design.md`. Single-line value change in
`internal/agent/tools/weather/tool.go::init()` + 8-line doc comment.

**Verification (already exercised):**

```bash
$ cd ~/smackerel && go test -count=1 -timeout 30s ./internal/agent/tools/weather/... 2>&1 | tail
ok  github.com/smackerel/smackerel/internal/agent/tools/weather  0.036s

$ go build ./... 2>&1 | tail
[exit 0]
```

CI run `27214998293`: success. Deployed to <deploy-host> via ci-keyless
promote.

**Definition of Done:**

- [x] `PerCallTimeoutMs: 8000` in tool registration
- [x] Doc comment explaining measurement + rationale
- [x] Weather unit tests pass
- [x] Build clean
- [x] Committed + pushed
- [x] CI green
- [x] Deployed to <deploy-host>
- [ ] Live verification — user sends `/weather <city>` after
      BUG-015-001 ollama recovery and gets a real forecast.
      **Pending user test.**
- [x] Build Quality Gate: go build + tests clean

---

## Scope 02 — Live verification (pending user test)

**Status:** Not started — depends on user sending `/weather` after
the ollama recovery from BUG-015-001 lands.

**Expected outcome:**

- `assistant_turn` log records `status: "answered"` (not
  `saved_as_idea`)
- `error_cause` is empty
- The Telegram reply contains a real forecast line + provider
  attribution (`open-meteo`)
- `latency_ms` is between 1000 and 8000 (cold cache; less for warm)
