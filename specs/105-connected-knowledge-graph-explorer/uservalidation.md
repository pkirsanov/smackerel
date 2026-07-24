# User Validation: 105 Connected Knowledge Graph Explorer

## Checklist

- [x] Planning-fidelity baseline: the packet preserves the specification's real-topology, bounded-query, equivalent-access, privacy, mobile, and honest-state contracts; this is not runtime acceptance.
- [x] Dependency baseline: BUG-080-001 is recorded as a blocking prerequisite and is not claimed complete by this packet.
- [x] Test-integrity baseline: every SCN-105 scenario has concrete unit, integration, E2E API, and E2E UI planning with no mocked live-stack path.

## Goal

An authenticated user can explore only authorized stored knowledge through a
bounded visual graph and equivalent semantic projections, understand why
relationships exist, and preserve context across input modes and viewports.

## Acceptance Journeys

| Journey | Planned Evidence | Runtime Status |
|---|---|---|
| Open and inspect a real bounded graph | SCN-105-001 matrix | Not executed |
| Distinguish a connected overview from an isolated-only corpus honestly | SCN-105-014, 015 | Not executed |
| Expand, filter, find a path, and return through history | SCN-105-003, 005, 006, 007 | Not executed |
| Complete the same outcome by keyboard and semantic outline with equivalent Graph, Outline, and Table views | SCN-105-008, 009, 016 | Not executed |
| Complete the flow on mobile with reduced motion and themes | SCN-105-010 | Not executed |
| Preserve privacy and distinguish honest failure/empty states | SCN-105-004, 011, 012, 013 | Not executed |

## Planning Boundary

Checked items above validate packet fidelity only. They do not validate source,
tests, browser behavior, migration, deployment, or operated data.