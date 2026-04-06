# Smackerel — Project Constitution

> Version 1.2.0

---

## Core Principles

### 1. Local-First Knowledge Ownership
- Core product value must work on user-controlled hardware.
- Artifacts, summaries, embeddings, graph relationships, and digests are stored locally by default.
- Cloud LLMs may be optional helpers, never the only way to use the product.

### 2. Go-First Runtime, Python-Only ML Sidecar
- The primary runtime lives in Go: API, connectors, scheduler, knowledge graph, lifecycle engine, digest generation, and delivery channels.
- Python is reserved for ML-heavy sidecar responsibilities such as embeddings, model gateway work, transcript retrieval, and extraction fallback paths.
- Python does not become the primary orchestrator or primary data-write surface.

### 3. Processed Knowledge Beats Raw Dumps
- Every captured artifact must become usable knowledge: summary, entities, tags, and graph connections.
- Raw content is retained for audit and replay, but processed output is the primary user-facing representation.

### 4. Explainable Synthesis
- Digests, syntheses, and proactive prompts must be traceable to source artifacts.
- The system may infer and connect, but it must preserve the path back to evidence.

### 5. Passive by Default, Explicit on Action
- Observation, capture, and summarization can be passive.
- Outbound or state-changing actions that affect external systems require explicit user intent.
- The product should reduce cognitive load, not create workflow guilt.

### 6. Docker-First Self-Hosting
- The committed runtime must boot as a local, self-hosted stack.
- Stateful services must be isolated and restart-safe.
- Deployment documentation and committed configuration must describe the same topology.

### 7. Single CLI Operations
- Build, test, lint, format, stack lifecycle, config generation, logs, and cleanup must flow through one repo CLI.
- Ad-hoc operational commands are not the documented runtime interface.
- The CLI must own environment selection and safety checks.

### 8. Single Source Of Truth Configuration
- All runtime configuration must originate from one committed source file.
- Generated env files and Compose files are derived outputs, not handwritten truth.
- Missing required config must fail loudly; hidden defaults are forbidden.

### 9. Isolated Test Environments
- Persistent development state is separate from automated test and validation state.
- Integration, E2E, chaos, and validation flows must use disposable storage.
- Automated test writes must never target the primary development store.

### 10. Docker Lifecycle Safety And Freshness
- Cleanup defaults must preserve persistent data and useful caches.
- Build freshness must be proven through image identity and input hashes, not timestamps or `latest` tags.
- Project-scoped cleanup is required before any broader Docker cleanup.

---

## Testing Doctrine

- **Go unit tests:** cover the core runtime, connector logic, graph logic, and delivery orchestration.
- **Python unit tests:** cover ML sidecar transforms, schema validation, model gateway behavior, and fallback extractors.
- **Integration tests:** exercise Go, NATS, Python, PostgreSQL, and Ollama boundaries together.
- **End-to-end tests:** prove capture, retrieval, synthesis, and digest workflows from user input to surfaced output.
- **Stress tests:** required for ingestion, retrieval, and synthesis hot paths.
- **E2E environment isolation:** end-to-end tests must run against disposable test state, never the primary dev store.
- **Live-stack authenticity:** integration and end-to-end tests must hit the real running stack; mocked request interception in live categories is forbidden.

---

## Documentation Doctrine

- `docs/smackerel.md` is the authoritative product and architecture design.
- `docs/Development.md` is the authoritative runtime command and configuration contract.
- `docs/Testing.md` is the authoritative test taxonomy and environment isolation guide.
- `docs/Docker_Best_Practices.md` is the authoritative Docker lifecycle, cleanup, and freshness guide.
- Specs under `specs/` define phased behavior and scope sequencing.
- Any change to the planned runtime stack must update `docs/smackerel.md`, this constitution, and the project-owned Bubbles config in the same change set.

---

## Configuration Management

- No hidden defaults or fallback runtime topology.
- Runtime service boundaries, storage choices, and deployment commands must be committed and documented.
- Runtime configuration must flow from a single committed source-of-truth file.
- Secrets stay out of the repository; missing required runtime configuration must fail loudly.

---

## Business Invariants

- User data remains local by default; remote services are optional integrations, not the core system of record.
- Every derived insight, digest item, or synthesis claim must be traceable back to source artifacts.
- Passive observation must never cause outbound side effects without explicit user initiation.
- Processed knowledge is primary; raw payloads support audit, replay, and debugging only.
- The Go core remains the authoritative orchestrator even when Python sidecars are present.

---

## Model Compensations

| Compensation | Limitation It Addresses | Review Date |
|---|---|---|
| Require raw execution evidence for pass/fail claims | Models summarize expected behavior instead of proving executed behavior | Next model upgrade |
| Keep sequential scope completion enforced | Models tend to jump ahead before current work is verified | Next model upgrade |
| Restrict Python to ML-sidecar responsibilities unless design docs change | Models tend to sprawl ML-friendly code into the primary runtime | After the first runtime milestone |
| Persist synthesized output only after schema validation and source-link attachment | Models can hallucinate structure or unsupported claims | After the first end-to-end implementation milestone |
