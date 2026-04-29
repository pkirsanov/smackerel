# User Validation Checklist

## Checklist

- [x] Bug packet exists for domain extraction E2E status timeout.
- [x] Recipe-like E2E capture reaches processed or completed status.
- [x] Domain extraction reaches completed status and persists structured domain data.
- [x] Broad E2E no longer reports the domain extraction status timeout.

## Evidence Notes

- Focused proof: `timeout 420 ./smackerel.sh --env test test e2e --go-run TestE2E_DomainExtraction` exited 0 and logged `processing=processed domain=completed`, recipe `domain_data` keys, and a search hit for artifact `01KQA4DMXN6CX7QW4VSF7Q1HKT`.
- Broad proof: `timeout 3600 ./smackerel.sh --env test test e2e` exited 1 because of unrelated `TestOperatorStatus_RecommendationProvidersEmptyByDefault`, but `TestE2E_DomainExtraction` passed in that run with artifact `01KQA5AD4QXMKGW5JRVDESB8N3`.
- Closure note: validation-owned certification is intentionally left open until the validation owner decides how to treat the remaining broad-suite failure.
