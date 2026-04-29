# User Validation Checklist

## Checklist

- [x] Bug packet exists for empty-store knowledge stats 500.
- [ ] `/api/knowledge/stats` returns HTTP 200 on a fresh empty knowledge store.
- [ ] Empty-store stats contain zero counts and explicit empty prompt contract version.
- [ ] Broad E2E no longer reports the empty-store knowledge stats 500.
