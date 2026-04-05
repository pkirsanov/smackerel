---
name: bubbles-docker-port-standards
description: Enforce workspace-wide standards for Docker configuration, port allocation (the 10k Rule), and URL binding (Dual-URL Standard). Use when generating or modifying docker-compose.yml or service configurations.
---

# Bubbles Docker & Port Standards

## Goal
Prevent port conflicts and ensure reliable host and container connectivity.

## Non-negotiables
- Never assign standard ports such as `80`, `5432`, or `6379` to host mappings.
- Never use `localhost` for external or host bindings. Use `127.0.0.1`.
- Always separate internal URLs from external URLs.
- Always use the assigned port block for the specific project.

## Port Block Allocation (The 10k Rule)

Define project-specific port blocks in project setup docs, not here, to keep this skill project-agnostic.

## Dual-URL Configuration Standard

All configuration systems MUST generate two URL variables per service.

### 1. Internal Binding (Docker-to-Docker)
- Format: `http://<service_name>:<internal_port>`

### 2. External Binding (Host-to-Docker)
- Format: `http://127.0.0.1:<allocated_host_port>`

## Docker Isolation Rules
- Container names must be prefixed with the project name.
- Networks must use explicit names.
- Volumes must use explicit names.

## Operational Maturity Standards
- Add health checks for stateful services.
- Use `depends_on: condition: service_healthy` when supported.
- Use bounded logging such as `json-file` with `max-size: "10m"`.
- Prefer `unless-stopped` restart policy.
- Pin image tags. Do not rely on `:latest`.