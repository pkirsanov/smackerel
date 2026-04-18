# User Validation Checklist

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md)

## Checklist

- [x] Baseline checklist initialized for expense tracking feature
- [x] Spec covers all 10 use cases (UC-001 through UC-010)
- [x] Spec covers all 28 business scenarios (BS-001 through BS-028)
- [x] Design covers all 13 sections (overview through risks)
- [x] Scopes break feature into 9 independently testable units
- [x] Scope dependency graph has no circular dependencies
- [x] Every scope has Gherkin scenarios mapped to spec business scenarios
- [x] Every scope has a Test Plan with unit, integration, and regression E2E rows
- [x] Every scope has a Definition of Done with markdown checkboxes
- [x] Configuration scope (01) establishes SST compliance foundation
- [x] Receipt extraction scope (02) covers heuristic detection and prompt contract
- [x] Data model scope (03) covers migrations, indexes, and JSONB schema
- [x] Classification scope (04) implements 7-level rule priority chain
- [x] Vendor normalization scope (05) covers aliases, suggestions, and batch reclassification
- [x] API scope (06) covers all 7 REST endpoints (A-001 through A-007)
- [x] CSV export scope (07) covers standard and QuickBooks formats
- [x] Telegram scope (08) covers all 11 interaction formats (T-001 through T-011)
- [x] Digest scope (09) covers all 5 digest blocks with word limit enforcement
- [x] All scopes include adversarial test cases for key bugs
- [x] No TODO, FIXME, or deferral language in scopes
