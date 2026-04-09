---
name: bubbles-config-sst
description: Enforce Configuration Single Source of Truth (SST) governance. Use when adding config values, creating services, modifying ports, reviewing env files, changing Docker Compose, or auditing config drift. Triggers include new service/port/endpoint, .env editing, hardcoded values in source, config generation pipeline changes, and environment variable additions.
---

# Bubbles Configuration Single Source of Truth (SST)

## Goal

Ensure every configuration value has exactly ONE canonical location (the SST file) and all other consumers either reference SST-generated artifacts or are produced by the config generation pipeline. Zero hardcoded values, zero manual edits to generated files, zero silent defaults.

## Use This Skill When

- Adding a new service, port, endpoint, or config value
- Modifying Docker Compose files or port mappings
- Reviewing or editing `.env` files
- Finding hardcoded ports, URLs, or credentials in source code
- Changing the config generation pipeline
- Adding environment variables to services
- Auditing config consistency across environments
- Setting up a new project with Bubbles
- Reviewing PRs that touch config-adjacent files

## SST Lifecycle

### Phase 1: Define in SST

Every config value enters the system through the SST config file.

```
config/<project>.yaml    ← Human edits happen HERE and ONLY here
```

The SST file is organized by concern:

| Section | Contains |
|---------|----------|
| `project` | Name, namespace, version |
| `ports` | Host and internal port mappings per service |
| `services` | Service-specific config (resources, env, deps) |
| `infrastructure` | Database, message bus, cache, observability |
| `auth` | JWT, encryption, session config |
| `environments` | Environment-specific overrides (dev, test, staging, prod) |

### Phase 2: Generate Derived Files

The config generator reads the SST and writes all derived files.

```bash
./<project>.sh config generate    # or: config compile
```

Generated files MUST:
- Contain a header comment marking them as auto-generated
- Be listed in the project's "DO NOT EDIT" manifest (copilot-instructions.md)
- Be reproducible (same SST input → same generated output)

### Phase 3: Consume at Runtime

Runtime code reads values from generated artifacts (env vars, config JSON), never from the SST file directly.

```
Backend  → reads env vars from .env or compiled JSON
Frontend → reads build-time env vars from .env
Docker   → reads Compose file + .env
Tests    → reads test-specific env from generated test config
```

## Classification Rules

### SST-Managed (MUST be in SST file)

| Value Type | Examples |
|------------|----------|
| Ports | Host ports, internal ports, mapped ports |
| URLs | Database URLs, service endpoints, external APIs |
| Credentials | DB passwords, JWT secrets, API keys (or secret refs) |
| Timeouts | Server timeouts, connection timeouts, rate limits |
| Resource limits | Memory, CPU, pool sizes |
| Feature flags | Enable/disable toggles |
| Service identity | Container names, image names, network names, volume names |

### NOT SST-Managed (OK to define elsewhere)

| Value Type | Where |
|------------|-------|
| Build-time constants | Source code constants module |
| Compile-time feature flags | Build system (Cargo features, webpack defines) |
| UI design tokens | Design token files (CSS vars, Tailwind config) |
| Test fixture data | Test files |
| Algorithm parameters | Domain config files (if not deployment-sensitive) |

## Verification Checklist

When reviewing config changes, verify ALL of these:

```
[ ] New value defined in SST file (config/<project>.yaml)
[ ] Config generator updated to parse and emit the new value
[ ] Generated files regenerated and committed (if checked in)
[ ] Source code reads from env var or config JSON, not hardcoded
[ ] No fallback/default pattern for the new value in source code
[ ] copilot-instructions.md updated if new generated file added
[ ] Port allocation follows project's assigned block (10k Rule)
[ ] Dual-URL standard followed (internal DNS vs external 127.0.0.1)
[ ] Secrets separated from non-secret config (not committed to git)
```

## Anti-Patterns (BLOCKING)

| Anti-Pattern | Why It's Wrong | Fix |
|--------------|---------------|-----|
| Editing a generated `.env` file | Will be overwritten on next generate | Edit SST file, regenerate |
| Hardcoded port `50001` in Go source | Bypasses SST pipeline | Read from `os.Getenv("PORT")` |
| `DB_HOST.unwrap_or("localhost")` | Silent default masks missing config | `DB_HOST?` with error propagation |
| Different port in YAML vs Compose | Config drift between SST and consumer | Regenerate, verify diff is clean |
| Adding env var to Dockerfile but not SST | Value exists outside SST control | Add to SST YAML first |
| Service URL hardcoded in frontend | Bypasses config pipeline | Use build-time env injection |
| Config value in two YAML files | Dual source of truth | Single SST with generation |

## Config Drift Detection

Run these checks to detect SST drift:

```bash
# 1. Regenerate all config from SST
./<project>.sh config generate

# 2. Check for uncommitted changes to generated files
git diff --name-only config/generated/ *.env docker-compose.yml

# 3. Scan for hardcoded port literals in source
grep -rn '<port>' src/ backend/ frontend/ services/ \
    --include='*.ts' --include='*.go' --include='*.rs' --include='*.py' \
    | grep -v '.env' | grep -v 'test' | grep -v 'node_modules'

# 4. Scan for default/fallback patterns
grep -rn ':-\|unwrap_or\|getOrElse\||| .\|?? .' src/ backend/ frontend/ services/ \
    --include='*.sh' --include='*.rs' --include='*.go' --include='*.ts' --include='*.py'

# 5. Verify all SST-managed ports appear in generated files
# (project-specific: compare SST ports section to generated .env)
```

## Secrets Handling

Secrets follow the SST principle but with an additional security layer:

| Approach | When to Use |
|----------|-------------|
| Separate `secrets.yaml` (gitignored) | Dev-only, no secret manager |
| Secret manager refs in SST | Production (Infisical, Vault, AWS SSM) |
| `.env.secrets` bootstrap | Local dev with secret manager backup |

**Rules:**
- Secrets MUST NOT be committed to git (even in dev)
- SST file MAY contain secret placeholders/refs but not actual values
- Config generator resolves secret refs at generation time
- Generated files with resolved secrets MUST be gitignored

## Environment Overlay Pattern

For multi-environment deployments:

```
config/
├── <project>.yaml              ← Base SST (shared values)
├── environments/
│   ├── dev.yaml                ← Dev overrides
│   ├── staging.yaml            ← Staging overrides
│   └── prod.yaml               ← Production overrides
└── secrets.yaml                ← Gitignored secrets
```

The config compiler merges: `base + environment overlay + secrets → generated config`.

Alternatively, for simpler projects, embed `environments:` section inline in the SST file with per-environment port blocks and compose project names.

## Integration with Bubbles Governance

| Bubbles Gate | SST Relevance |
|--------------|--------------|
| Implementation Reality Scan (G028) | Detects hardcoded values that should come from SST |
| Integration Completeness (G029) | New services must be wired into SST pipeline |
| Docker Bundle Freshness (Gate 9/10) | Config must be regenerated before freshness check |
| Pre-Completion Self-Audit | `config generate` + `git diff` is a required step |
| Vertical Slice (G035) | Config values must flow end-to-end (SST → backend → frontend) |

## References

- `.github/instructions/bubbles-config-sst.instructions.md`
- `.github/agents/bubbles_shared/project-config-contract.md`
- `.github/skills/bubbles-docker-port-standards/SKILL.md`
- `.github/instructions/bubbles-docker-lifecycle-governance.instructions.md`
