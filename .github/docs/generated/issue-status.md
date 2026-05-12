# Issue Status

Tracked gaps: 2 issue-backed capabilities.

| Issue | Ledger Status | Related Capability | Summary |
| --- | --- | --- | --- |
| [session-aware-runtime-coordination.md](../issues/session-aware-runtime-coordination.md) | shipped | Session-aware runtime coordination | Runtime lease coordination is shipped — `runtime-leases.sh` provides acquire/attach/release/heartbeat/doctor/reclaim-stale operations with compatibility fingerprints, share modes (shared-compatible / exclusive / disposable / persistent-protected), and stale-lease takeover semantics; wired into `bubbles/scripts/cli.sh` (`runtime` subcommand, `status`, `doctor`) and `framework-validate.sh` (selftest). |
| [G068-word-overlap-threshold.md](../issues/G068-word-overlap-threshold.md) | shipped | G068 DoD-Gherkin fidelity threshold tuning | G068 overlap heuristic now uses 3-char min word length, a trimmed true-stop-word exclusion list, and a percentage threshold (>=50%) with a >=3 absolute floor — false-positive matching for legitimate DoD items is resolved. |
