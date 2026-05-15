# BUG-020-004: ML NATS client auth token read must use canonical fail-loud `_AUTH_TOKEN`

## Status

Fixed and validated on 2026-05-15.

## Summary

`ml/app/nats_client.py` previously re-read `SMACKEREL_AUTH_TOKEN` inside `NATSClient.connect()` using the forbidden silent-default pattern `os.environ.get("SMACKEREL_AUTH_TOKEN", "")`. That bypassed the canonical fail-loud read in `ml/app/auth.py` and violated Smackerel Gate G028 / NO-DEFAULTS SST policy.

The working tree now routes NATS auth-token plumbing through `from .auth import _AUTH_TOKEN` and uses `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN`. The companion tests patch `app.nats_client._AUTH_TOKEN` directly and include a persistent source-contract regression test that fails if the forbidden token read returns.

## Reproduction

Pre-fix at HEAD `ad512fc6`, the secondary call site existed in `ml/app/nats_client.py`:

```python
# Token authentication — mirrors Go core's NATS auth enforcement
auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
if auth_token:
    connect_opts["token"] = auth_token
```

The current fixed state is verified by the evidence in `report.md`:

- forbidden grep for `os.environ.get/getenv("SMACKEREL_AUTH_TOKEN"...)` returns no matches with `grep_exit=1`
- positive grep returns exactly the `_AUTH_TOKEN` import and connect-time assignment lines
- `./smackerel.sh test unit --python` exits successfully with `449 passed`
- regression-quality guard exits successfully in normal and bugfix modes

## Scope

In scope:

- `ml/app/nats_client.py` current auth-token plumbing
- `ml/tests/test_nats_client.py` behavioural tests and Gate G028 source-contract test
- this bug packet's Bubbles artifacts

Out of scope:

- `ml/app/main.py` `_check_required_config()` sequel-cleanup concerns
- unrelated dirty worktree files in Go metrics/auth and other ML tests
- deployment adapters, Compose, generated config, and parent spec artifacts

## Resolution

The bug is closed by the existing `_AUTH_TOKEN` implementation in `ml/app/nats_client.py` plus the new persistent `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` regression test in `ml/tests/test_nats_client.py`.
