# User Validation: 062 Per-Transport Configuration Surface Audit

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] Spec 062 planning artifacts created to occupy previously-skipped ledger slot (GAPS-2026-06-02-06).

Operator validates after Scope 3 completion:

- [ ] `docs/Transport_Configuration.md` lists every per-transport
      runtime configuration value across HTTP, WhatsApp, and Telegram.
- [ ] Operator can intentionally remove one required key from
      `config/smackerel.yaml`, run `./smackerel.sh up`, and see a
      fail-loud message that names the transport and the missing key.
- [ ] Adding a new key to `config/smackerel.yaml` without updating the
      registry fails `./smackerel.sh test unit` with a clear pointer
      to the registry file.

## Sign-Off

_Pending — fill after Scope 3 DoD is met._
