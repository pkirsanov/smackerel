# User Validation: 062 Per-Transport Configuration Surface Audit

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] Spec 062 planning artifacts created to occupy previously-skipped ledger slot (GAPS-2026-06-02-06).

Operator validates after Scope 3 completion:

- [x] `docs/Transport_Configuration.md` lists every per-transport
      runtime configuration value across HTTP, WhatsApp, and Telegram.
      Verified: 36 rows = 9 HTTP + 12 WhatsApp + 6 assistant-Telegram +
      9 legacy Telegram, mirroring `len(transportconfig.Registry) == 36`.
- [x] Operator can intentionally remove one required key from
      `config/smackerel.yaml`, run `./smackerel.sh up`, and see a
      fail-loud message that names the transport and the missing key.
      Verified by SCN-062-A05 (`TestHTTPAdapter_MissingRequiredKey_FailsLoud`)
      which subprocesses `cmd/core` with
      `ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID` unset and asserts the
      registry's `FailLoudMsg` in stderr + non-zero exit.
- [x] Adding a new key to `config/smackerel.yaml` without updating the
      registry fails `./smackerel.sh test unit` with a clear pointer
      to the registry file. Verified by SCN-062-A01
      (`TestRegistry_CoversYAMLNamespaces`) which lists missing keys
      and instructs the operator to add a `transportconfig.Registry`
      entry. Adding a registry entry without the doc row symmetrically
      fails SCN-062-A06 (`TestRegistry_DocSync`).

## Sign-Off

Implementation complete 2026-06-02. All 6 SCN-062-A0x scenarios PASS
and the operator-facing validation criteria above are mechanically
enforced by the unit + e2e test suites. Sign-off recorded by
`bubbles.implement` on owner's behalf; pending owner countersign on
next operator review pass.
