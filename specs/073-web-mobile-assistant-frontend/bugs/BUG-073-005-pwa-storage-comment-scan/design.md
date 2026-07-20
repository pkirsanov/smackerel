# Design: BUG-073-005 - Reuse structured comment-aware JavaScript scanning

## Root Cause Analysis

The live E2E uses `strings.Contains(js, forbidden)` against the raw served file.
`assistant.js` deliberately documents forbidden APIs in its leading `//`
comments. The unit guard in `web/pwa/tests/assistant_storage_guard_test.go`
first calls `stripLineComments`, so it correctly distinguishes documentation
from executable source. The duplicated scans have drifted.

The existing helper is test-package-local and only strips from `//` to end of
line with `strings.Index`; that form can misread `//` inside JavaScript strings.
The reusable replacement should use a small lexical state machine that tracks
single-quoted, double-quoted, template-string, line-comment, and block-comment
states while preserving line boundaries. It is source inspection, not source
rewriting or execution.

## Fix Design

1. Extract a reusable JavaScript comment-removal helper into a test-support
   package importable by both test packages.
2. Preserve quoted string/template content and escaped delimiters; replace
   comment bytes with whitespace/newlines so token boundaries stay stable.
3. Update the existing storage guard and robustness consumers to use the shared
   helper, retaining the existing forbidden regex catalog.
4. Update the served-route E2E to scan only executable source using the helper.
5. Add adversarial table tests: comments ignored, executable access retained,
   block comments ignored, URL/string `//` preserved, and code after comments
   remains visible.

## Security Boundary

The forbidden API set is not reduced. The change only makes lexical treatment
consistent. Tests continue to inspect real committed/served production source
without mocks.

## Rollback

Revert the helper and test changes. No runtime or persisted state is touched.
