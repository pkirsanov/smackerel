# User Validation Checklist: BUG-031-010

## Automation Readiness

- [ ] Exact `go test -list` failure status is proven by red-stage and green-stage automation.
- [ ] JSON warning, quoted benign literal, grep failure, capture failure, and cleanup cases are proven by automation.
- [ ] The disposable live stress gate is green after the active E2E lane is idle.

## Checklist

- [ ] A forced workload discovery failure is shown as that failure, not as a selector miss or zero-match summary.
- [ ] A direct JSON WARN record fails the stress harness.
- [ ] Quoted warning-shaped example text remains accepted when no real warning record exists.
- [ ] Classifier and capture tool failures fail loud and leave no temporary capture files.
- [ ] Existing readiness, workload failure, skip, logfmt warning, and timestamp warning behavior remains intact.

## Human Acceptance Record

No human acceptance has been recorded because the bug is still in progress.

## Goal

- Goal: Make the stress harness report discovery, warning, and infrastructure failures truthfully.
- Success signal: The operator can distinguish selector no-match from discovery failure, real warnings from quoted examples, and trusted output from failed classification while every temporary capture is cleaned.

## Journey Steps

This bug affects a command-line validation harness rather than a product UI. Human acceptance uses the repository commands and observable exit statuses listed in the checklist.

## Open Refinements

None recorded during planning.