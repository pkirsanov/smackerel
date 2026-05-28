// Vitest setup: install fake-indexeddb on globalThis so background/queue.ts
// finds an IndexedDB factory without needing a browser.
import "fake-indexeddb/auto";
