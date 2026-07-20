# Bug Fix Design: BUG-074-001

## Root Cause Analysis

The no-ground hook persists the fallback idea but only replaces `resp` when persistence fails. On success, a refusal response can retain `provider_unavailable` and an unsourced-answer body while also claiming `CaptureRoute=true` and saved-as-idea.

## Fix Design

Add a pure canonicalization helper that preserves routing/emission metadata while setting the closed capture response shape. Invoke it only after successful no-ground persistence. Unit-test adversarial input containing upstream error, sources, and control payloads.

## Complexity Tracking

None - simplest viable fix used.
