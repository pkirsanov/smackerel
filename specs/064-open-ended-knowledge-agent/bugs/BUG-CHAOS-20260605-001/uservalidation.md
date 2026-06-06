# User Validation: BUG-CHAOS-20260605-001

## Checklist

- [x] A developer running `go test ./tests/integration/agent/...` from the repo root (without the `./smackerel.sh` wrapper) sees the open-knowledge routing tests load the real scenarios instead of failing with a misleading "open_knowledge scenario absent" message.
- [x] When the live test stack is up, both routing tests still pass through the `./smackerel.sh test integration` wrapper.
- [x] If `AGENT_SCENARIO_DIR` is genuinely misconfigured (relative path that does not resolve to a real directory), the test fails loudly with an error string that names both the supplied relative path AND the resolved absolute path, so the operator can correct the env immediately.
- [x] No product runtime behavior changes — only test-side resolution.
