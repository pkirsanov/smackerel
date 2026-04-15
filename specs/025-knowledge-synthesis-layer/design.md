# Design: 025 — Knowledge Synthesis Layer

## Overview

Pending design phase. This document will be populated by `bubbles.design` based on the spec.md requirements.

Key architectural decisions required:
- PostgreSQL schema for knowledge_concepts, knowledge_entities, knowledge_lint_reports tables
- NATS subject topology for synthesis pipeline (artifacts.synthesize, artifacts.synthesize.result)
- Prompt contract YAML schema and validation framework
- ML sidecar synthesis endpoint design
- Incremental update transaction boundaries
- Lint scheduler integration with existing cron system
