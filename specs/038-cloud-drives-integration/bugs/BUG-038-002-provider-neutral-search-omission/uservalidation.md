# User Validation: BUG-038-002 Provider-neutral Drive search omission

## Checklist

### Multi-provider recall

- [x] **What:** A matching Google Drive file and matching second-provider file are returned together by canonical search after ingestion completes.
  - **Steps:** Connect disposable Google and memdrive fixtures, scan/extract one matching file from each, then search their shared terms.
  - **Expected:** Both exact artifacts appear with the correct provider chip and metadata.
  - **Verify:** Run the live Drive regression through the repository CLI.
  - **Evidence:** `report.md#test-evidence`

### Strict policy behavior

- [x] **What:** Provider, owner/audience, sharing, and sensitivity behavior remains strict while retrieval sources converge.
  - **Steps:** Exercise the provider/audience filter variants in the real disposable stack.
  - **Expected:** Eligible rows are returned and ineligible rows remain excluded without weakened assertions.
  - **Verify:** Run focused integration and Drive-package E2E regressions.
  - **Evidence:** `report.md#test-evidence`
