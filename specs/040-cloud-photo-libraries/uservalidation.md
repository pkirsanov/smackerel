# User Validation Checklist: 040 Cloud Photo Libraries

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] Baseline checklist initialized for feature 040 planning.
- [ ] User can connect Immich, choose scope, and see scan progress with visible skipped-photo reasons.
- [ ] User can search a scanned photo by natural-language content, OCR text, provider metadata, and EXIF-derived context.
- [ ] RAW-to-processed lifecycle links, editor signatures, duplicate clusters, best-pick rationale, and removal candidates are visible in Photo Health.
- [ ] Destructive or low-confidence photo actions require exact user confirmation before provider mutation.
- [ ] Telegram, mobile, and web uploads enter one shared photo pipeline and preserve source-channel plus provider refs.
- [ ] Receipt, recipe, document, product, place, list, annotation, meal-plan, and intelligence routes are created only when confidence and sensitivity policy allow.
- [ ] Sensitive photo retrieval through Telegram or preview APIs blocks raw photo delivery unless reveal policy is satisfied.
- [ ] A second provider path uses the same provider-neutral classification, search, lifecycle, dedupe, routing, and capability governance contracts.
- [ ] Provider limitations return a visible `PROVIDER_LIMITATION` reason and never become silent no-ops.
- [ ] Synthetic large-library stress validation proves photo health and search readiness without using user-owned libraries.

Unchecked items are acceptance items for implementation and validation phases. The checked baseline item only records that this checklist exists.