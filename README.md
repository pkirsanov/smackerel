# Smackerel

> *"What I like best is just doing nothing... and having a little smackerel of something." — Winnie-the-Pooh*

A passive intelligence layer across your entire digital life. It observes, captures, connects, and synthesizes information so you don't have to organize anything yourself.

## What It Does

- **Observes** everything — email, videos, maps, calendar, browsing, notes, purchases
- **Captures** anything via zero-friction input from any device
- **Connects** across domains — cross-links, detects themes, builds a living knowledge graph
- **Searches** by meaning, not keywords — "that pricing video" just works
- **Synthesizes** patterns, proposes ideas, identifies blind spots
- **Evolves** — promotes hot topics, archives cold ones, tracks expertise growth
- **Surfaces** the right information at the right time
- **Runs locally** — you own your data, always

## Docs

- [Design Document](docs/smackerel.md)
- [Development Guide](docs/Development.md)
- [Testing Guide](docs/Testing.md)
- [Docker Best Practices](docs/Docker_Best_Practices.md)

## Runtime Standards

Smackerel does not have a committed runtime yet. When it lands, the implementation must follow the same operational standards used in the stronger repos in this workspace:

- Docker-only runtime and test execution
- One repo CLI for build, test, config generation, stack lifecycle, logs, and cleanup
- A single configuration source of truth with generated runtime artifacts
- Persistent development state separated from disposable test and validation state
- Smart cleanup and build-freshness verification instead of destructive default cleanup
- Live-stack integration and E2E requirements with isolated test environments
