# Design: 032 — Documentation Freshness & Operational Guides

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Author:** bubbles.design
> **Date:** April 18, 2026
> **Status:** Draft

---

## Overview

Updates all managed documentation to reflect current codebase reality. Creates an operational runbook for common tasks. Documents system requirements, TLS setup, prompt contract format, and error troubleshooting.

### Key Design Decisions

1. **Measured, not estimated** — System requirements derived from `docker stats` measurements
2. **Docs-as-code** — All documentation in Markdown, version-controlled, verifiable against codebase
3. **Error lookup table** — Common error messages mapped to causes and resolutions
4. **TLS via reverse proxy** — No application-level TLS; recommend Caddy (auto-HTTPS) or nginx

---

## Architecture

### Documentation Structure

```
docs/
├── smackerel.md          # Architecture design (existing, no changes)
├── Development.md        # Developer guide (UPDATE: add new packages, migrations, contracts)
├── Testing.md            # Testing guide (existing, minor updates)
├── Docker_Best_Practices.md  # Docker ops (existing, no changes)
├── Connector_Development.md  # Connector guide (existing, no changes)
└── Operations.md         # NEW: Operational runbook

README.md                 # UPDATE: Add system requirements section
```

### Development.md Update Scope

**New packages to document:**
| Package | Purpose |
|---------|---------|
| `internal/domain/` | Domain extraction schema registry |
| `internal/annotation/` | User annotations, parser, store |
| `internal/list/` | Actionable list model, aggregators, store |

**New migrations to document:**
| Migration | Purpose |
|-----------|---------|
| 015 | Domain extraction columns + indexes |
| 016 | Annotations table + materialized view |
| 017 | Lists + list items tables |

**New prompt contracts to document:**
| Contract | Purpose |
|----------|---------|
| recipe-extraction-v1.yaml | Recipe structured extraction |
| product-extraction-v1.yaml | Product structured extraction |

### Operations.md Structure

```markdown
# Operations Runbook

## System Requirements
- Minimum: 16GB RAM, 15GB disk, Docker 24+
- Recommended: 32GB RAM, 30GB disk, Docker 24+

## Deployment
- First-time setup
- Configuration (config/smackerel.yaml)
- Starting: ./smackerel.sh up
- Stopping: ./smackerel.sh down

## Connector Management
- List active connectors: ./smackerel.sh status
- Restart stuck connector: POST /api/connectors/{id}/trigger
- Disable connector: edit smackerel.yaml → config generate → restart

## Troubleshooting
### Error Lookup Table
| Error | Cause | Resolution |
|-------|-------|------------|
| "NATS connection refused" | NATS not started | ./smackerel.sh up |
| "migration NNN failed" | Schema conflict | Check migration SQL, run rollback |
| "ML sidecar unhealthy" | Sidecar not ready | Wait 120s or check logs |
| "LLM timeout" | Provider rate limit | Check LLM_API_KEY, retry |

## TLS Setup
- Caddy reverse proxy (recommended)
- nginx reverse proxy (alternative)

## Backup & Restore
- PostgreSQL: pg_dump/pg_restore
- Volumes: docker volume inspect + backup
```

---

## Testing Strategy

| Test Type | Coverage | Evidence |
|-----------|----------|----------|
| Manual | All documented commands verified against real stack | Command execution screenshots |
| CI | Optional docs-freshness check comparing documented packages to `go list ./...` | CI step (spec 029) |

---

## Risks & Open Questions

| # | Risk | Mitigation |
|---|------|------------|
| 1 | Docs go stale again after initial update | CI freshness check (spec 029 coordination) |
| 2 | System requirements change as features grow | Remeasure on each major release |
