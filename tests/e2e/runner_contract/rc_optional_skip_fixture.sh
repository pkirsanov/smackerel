#!/usr/bin/env bash
# Synthetic skip fixture for BUG-061-014.
#
# Emits the same structured skip record shape that
# tests/e2e/assistant_regression/lib/regression_helpers.sh::reg_skip_with_blocker
# produces, and exits 77. It asserts nothing about the product; its only job is
# to hand a controlled exit-77 outcome to a real classifier so the classifier's
# reported outcome can be asserted.
#
# It deliberately lives outside the `tests/e2e/test_*.sh` glob and outside the
# CLI lane arrays, so no product lane picks it up implicitly.
set -euo pipefail

echo "=== BUG-061-014 runner-contract synthetic skip fixture ==="
echo "RESULT: SKIPPED"
echo "SKIP_REASON: RC-SYNTHETIC-BLOCKER-TOKEN"
echo "FIXTURE_PATH: ${BASH_SOURCE[0]}"
exit 77
