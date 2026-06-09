# BUG-016-001 — Execution evidence

## Pre-fix observation

Two failed `/weather` invocations on the OLD code (before deploy of `96acf294`):

```json
{"time":"2026-06-09T14:35:31Z","scenario_id":"weather_query","status":"saved_as_idea","error_cause":"provider_unavailable","latency_ms":3863}
{"time":"2026-06-09T14:40:51Z","scenario_id":"weather_query","status":"saved_as_idea","error_cause":"provider_unavailable","latency_ms":4448}
```

Both above the 2s tool budget by 2x. The 14:35 turn is from when
ollama was still up (so this is the pure weather-tool-timeout case);
the 14:40 turn overlapped with the ollama failure (BUG-015-001) so
the fix wouldn't have helped that one alone.

## Open-Meteo latency measurement (read-only)

```bash
$ ssh <deploy-host> 'docker exec smackerel-home-lab-smackerel-core-1 sh -c \
   "for i in 1 2 3; do t0=\$(date +%s.%N); \
                       wget -qO- --timeout=3 \\
                         \"https://geocoding-api.open-meteo.com/v1/search?name=Seattle&count=1\" > /dev/null; \
                       t1=\$(date +%s.%N); \
                       echo \\\"geocode \\$i: \\$(echo \\$t1 - \\$t0 | bc)s\\\"; \
                       done"'
geocode 1: 1s
geocode 2: 1s
geocode 3: 2s

# Same for forecast endpoint:
forecast 1: 1s
forecast 2: 2s
forecast 3: 1s
```

Cold-cache lookup is `geocode + forecast` = 2-4s end-to-end. The
prior 2s `PerCallTimeoutMs` was structurally unreachable for this
workload.

## Fix application

Commit:

```
fix(weather,telegram): /weather provider_unavailable + bot DNS-race silent disable
SHA: 96acf29459e9c972005e0c9d95d365941e1bda28
Date: 2026-06-09T05:53 UTC
```

Diff (relevant section):

```diff
- PerCallTimeoutMs: 2000,
+ // A single lookup is geocode + forecast, two sequential HTTPS
+ // round trips to open-meteo. Measured worst case from the
+ // home-lab container is ~2s per call (so ~4s end-to-end on a
+ // cold cache). The previous 2000 ms cap was tighter than a
+ // single HTTP call and made /weather fail with
+ // `provider_unavailable` on most cold-cache invocations. 8s
+ // gives ~2x headroom over the observed worst case while still
+ // failing fast if open-meteo or DNS is degraded.
+ PerCallTimeoutMs: 8000,
```

## Verification

```bash
$ cd ~/smackerel && go test -count=1 -timeout 30s ./internal/agent/tools/weather/... 2>&1 | tail
ok  github.com/smackerel/smackerel/internal/agent/tools/weather  0.036s

$ go build ./... 2>&1
[exit 0]

$ gh run view 27214998293 --json status,conclusion
status=completed conclusion=success
```

Deployed via ci-keyless promote to <deploy-host>; manifest pointer swapped
to source `96acf294` at 2026-06-09T15:33:39Z.

## Live verification (pending)

The first post-deploy `/weather` test (at 15:37:18Z) STILL returned
`provider_unavailable` — but for a DIFFERENT reason: ollama itself
was down (BUG-015-001). The weather tool timeout fix doesn't help
when the LLM that DECIDES to call the tool is unreachable.

After BUG-015-001 recovery (ollama restarted at 15:48Z with `gemma4:26b`
warm-loadable), the end-to-end pipeline is now intact:

```
Telegram → smackerel-core → assistant facade → router
        → gemma4:26b (running) → weather_lookup tool (8s budget)
        → open-meteo geocode (1-2s) + forecast (1-2s)
        → response → provenance gate → forecast reply
```

User test of `/weather <city>` after BUG-015-001 recovery will
confirm. The unit-level evidence (build + tests + measured open-meteo
latency vs new cap) is sufficient to merge.

## Files changed

- `internal/agent/tools/weather/tool.go` — 1 line value change + 8 lines doc comment
