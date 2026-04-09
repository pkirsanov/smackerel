# User Validation: 009 — Bookmarks Connector

> **Status:** Done

---

## Checklist

- [x] Bookmarks connector implements Connector interface (ID, Connect, Sync, Health, Close)
- [x] Chrome JSON exports parsed correctly via ParseChromeJSON
- [x] Netscape HTML exports (.html, .htm) parsed correctly via ParseNetscapeHTML
- [x] Incremental sync skips already-processed files using cursor
- [x] Corrupted export files logged and skipped without crashing sync
- [x] URL normalization strips tracking params (utm_*, fbclid, gclid, ref)
- [x] URL dedup prevents reprocessing same URL across exports
- [x] Folder-to-topic mapping creates/matches topics from bookmark folder hierarchy
- [x] Config section in smackerel.yaml with SST pipeline integration
- [x] Connector registered and auto-started in main.go
- [x] All 26 unit tests pass across connector_test.go, dedup_test.go, topics_test.go
- [x] ./smackerel.sh lint passes
- [x] ./smackerel.sh check passes
