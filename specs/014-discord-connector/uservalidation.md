# User Validation: 014 — Discord Connector

> **Feature:** [specs/014-discord-connector](.)
> **Status:** Done

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Spec reviewed and approved
- [x] Design reviewed and approved
- [x] Scopes planned (6 scopes)
- [x] Normalizer handles all Discord message content types
- [x] REST backfill fetches message history with pagination
- [x] Connector implements standard Connector interface
- [x] Config schema follows smackerel.yaml conventions
- [x] Gateway captures real-time messages from monitored channels
- [x] Thread detection and ingestion creates linked artifact chains
- [x] Bot command capture supports explicit !save/!capture
- [x] Rate limit handling respects Discord API limits
- [x] Per-channel cursors track sync progress independently
- [x] Pinned messages ingested as high-priority artifacts
