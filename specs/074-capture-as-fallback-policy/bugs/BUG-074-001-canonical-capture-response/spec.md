# Specification: BUG-074-001 Canonical capture response

## Expected Behavior

When open-knowledge no-ground persistence succeeds, the facade MUST return the same canonical capture acknowledgement used by other fallback causes. Capture failure remains unavailable with an explicit error.

## Acceptance Criteria

1. Successful capture clears upstream error cause and sources.
2. Status is saved-as-idea, capture route is true, and body is canonical.
3. Confirm/disambiguation payloads are absent.
4. A failed capture remains a fail-loud unavailable response.
5. HTTP and Telegram-visible shape remain aligned.

## Release Train

Target: `mvp`. No feature flag introduced.

## Test Isolation

Live tests use disposable backing services and unique message IDs.

## Deployment Boundary

No deployment, release-train, host, or secret changes.
