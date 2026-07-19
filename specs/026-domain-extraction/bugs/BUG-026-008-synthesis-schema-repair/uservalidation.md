# User Validation: BUG-026-008 Bounded synthesis schema repair

## Checklist

### Bounded correction

- [x] **What:** A card-rewards extraction missing required `concepts` receives one corrective schema-repair request instead of becoming permanently failed after the first model response.
  - **Steps:** Capture a representative card-rewards artifact through the normal product flow after the certified build is deployed.
  - **Expected:** Synthesis succeeds when the corrective response validates, and repair-attempt telemetry records one content-free attempt class.
  - **Verify:** Inspect the artifact's synthesis status and content-free runtime telemetry.
  - **Evidence:** `report.md#outcome-contract-evidence`

### Truthful terminal failure

- [x] **What:** A repair that remains invalid, malformed, or raises an LLM error stays explicitly failed after exactly two total calls.
  - **Steps:** Exercise the controlled regression cases in the test contract.
  - **Expected:** `success: false` reports the final error class, with no third call and no sensitive content in logs or result.
  - **Verify:** Run the focused Python regression through the repository CLI.
  - **Evidence:** `report.md#test-evidence`

### Existing extraction behavior

- [x] **What:** Initially valid extraction, malformed-JSON capture preservation, and qwen3 thinking/profile controls retain their prior behavior.
  - **Steps:** Run the full impacted Python suite and broader disposable-stack E2E lane.
  - **Expected:** Existing regressions remain green and initially valid synthesis still uses one LLM call.
  - **Verify:** Use the repository test CLI and linked evidence.
  - **Evidence:** `report.md#test-evidence`
