# Design: 062 Per-Transport Configuration Surface Audit

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [scenario-manifest.json](scenario-manifest.json) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## 1. Approach

Treat the per-transport configuration surface as a first-class
in-repo artifact. Introduce a single registry package under
`internal/assistant/transportconfig/` that enumerates each transport's
required and optional keys, the env-var binding, the human-readable
fail-loud message, and the owning adapter package. Adapters MUST
consume the registry at startup to drive their fail-loud checks instead
of hand-rolling per-key `os.Getenv` reads.

A unit test in the same package walks the registry and:

1. Parses `config/smackerel.yaml` and confirms every key under each
   transport namespace (`assistant.http.*`, `assistant.whatsapp.*`,
   `telegram.*`) is registered.
2. Confirms every required registry entry has a corresponding env-var
   binding produced by the config-generation pipeline.
3. Greps the owning adapter package source for forbidden fallback
   patterns (`os.Getenv(`…`, "`…`"`, `os.LookupEnv` followed by
   `if !ok { return "…" }`, `${VAR:-`, `${VAR-`).

Operator-facing rendering lives in a new
`docs/Transport_Configuration.md` generated-by-hand-but-test-asserted
table. The test confirms every registry entry has a matching row in the
doc and vice versa.

## 2. Package Layout

```
internal/assistant/transportconfig/
  registry.go            // Entry struct + Registry slice (init or package-level var)
  registry_test.go       // walk registry + parse smackerel.yaml + grep adapters
  doc_sync_test.go       // assert docs/Transport_Configuration.md mirrors registry
  http.go                // HTTP transport entries (cites spec 069)
  whatsapp.go            // WhatsApp transport entries (cites spec 072)
  telegram.go            // Legacy Telegram transport entries
```

## 3. Registry Entry Shape

```go
type Entry struct {
    Transport      string   // "http" | "whatsapp" | "telegram"
    YAMLKey        string   // "assistant.http.bearer.shared_user_id"
    EnvVar         string   // "ASSISTANT_HTTP_BEARER_SHARED_USER_ID"
    Required       bool
    FailLoudMsg    string   // exact message the adapter must print
    OwningPackage  string   // "internal/assistant/httpadapter"
    IntroducedBy   string   // "specs/069-assistant-http-transport SCOPE-2"
    DefaultedFor   string   // "" if no default; ratified-reason otherwise
}
```

The registry is the SST for which keys exist *per transport*. The YAML
file remains the SST for the *values*. The registry test enforces
bidirectional coverage.

## 4. Migration Strategy

1. **Inventory pass (Scope 1):** read the existing adapter packages and
   `config/smackerel.yaml`, produce the registry entries verbatim from
   what is already there. Do not change runtime behavior in this scope.
2. **Adapter refactor (Scope 2):** rewrite each adapter's startup
   fail-loud checks to drive off the registry. Any discovered default
   that cannot be removed is annotated `DefaultedFor: "<reason>"` and
   reviewed during scope DoD.
3. **Docs + test enforcement (Scope 3):** publish
   `docs/Transport_Configuration.md`, add the doc-sync test, and wire
   the registry test into `./smackerel.sh test unit`.

## 5. Test Plan Summary

| Test | Type | Asserts |
|------|------|---------|
| `TestRegistry_CoversYAMLNamespaces` | unit | every `assistant.http.*` / `assistant.whatsapp.*` / `telegram.*` key in `config/smackerel.yaml` has a registry entry |
| `TestRegistry_NoOrphanedEntries` | unit | every registry entry maps to a key present in `config/smackerel.yaml` |
| `TestRegistry_RequiredEntriesHaveFailLoud` | unit | grep owning package for the exact `FailLoudMsg` literal |
| `TestRegistry_NoForbiddenFallbacks` | unit | grep owning packages reject `os.Getenv(k, "default")` and `${VAR:-` in any companion env file |
| `TestRegistry_DocSync` | unit | `docs/Transport_Configuration.md` table mirrors the registry row-for-row |
| `TestHTTPAdapter_MissingRequiredKey_FailsLoud` | e2e-api | start `smackerel-core` with one required HTTP key removed; assert non-zero exit + exact message |

## 6. Risks

- **Hidden defaults discovered:** the inventory pass may surface
  defaults that pre-date the NO-DEFAULTS doctrine. Each must be
  removed in-scope or explicitly ratified with `DefaultedFor`. No
  silent carry-over.
- **YAML schema drift:** if a transport namespace is renamed, the
  registry namespace mapping must move in lockstep — the test will
  catch this only if the namespace prefix list is itself part of the
  registry.

## 7. Cross-Spec Boundaries

This spec does NOT modify the runtime semantics of any transport. It
only audits and codifies the configuration contract. If the audit
discovers a runtime bug (e.g. a transport accepts traffic before
fail-loud checks complete), that becomes a separate bug under the
owning transport's spec, not part of 062.
