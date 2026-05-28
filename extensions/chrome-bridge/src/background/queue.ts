// IndexedDB write-ahead log for outgoing artifacts. Service workers are
// evicted aggressively under MV3, so all queue state lives here; the
// background entrypoint re-binds listeners + alarms at top level and the
// drainer picks up where it left off.
//
// Schema: a single object store "wal" keyed on client_event_id.
// Each row is a WALRow (see common/schema.ts).
//
// SCN-058-012: persistsAcrossSWEviction — round-trip via IndexedDB only.
// SCN-058-015: skipsCorruptedRow — drain() validates the shape and removes
//              malformed rows without losing neighbors.

import type { IngestOutcome, RawArtifact, WALRow } from "../common/schema.js";

const DB_NAME = "smackerel-chrome-bridge";
const DB_VERSION = 1;
const STORE = "wal";

// IndexedDB opener is parameterised so tests can inject fake-indexeddb's
// factory without polluting the global namespace.
export interface IDBEnv {
  indexedDB: IDBFactory;
}

function defaultEnv(): IDBEnv {
  // In an SW context, indexedDB is on self; in tests, fake-indexeddb installs
  // it on globalThis.
  const g = globalThis as { indexedDB?: IDBFactory };
  if (!g.indexedDB) {
    throw new Error("indexedDB is not available in this environment");
  }
  return { indexedDB: g.indexedDB };
}

function openDB(env: IDBEnv = defaultEnv()): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = env.indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) {
        db.createObjectStore(STORE, { keyPath: "client_event_id" });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function promisifyRequest<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function txDone(tx: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onabort = () => reject(tx.error);
    tx.onerror = () => reject(tx.error);
  });
}

function isValidArtifact(a: unknown): a is RawArtifact {
  if (!a || typeof a !== "object") return false;
  const o = a as Record<string, unknown>;
  return (
    o.source_id === "browser-extension" &&
    typeof o.source_ref === "string" &&
    (o.content_type === "bookmark" || o.content_type === "browser_history_visit") &&
    typeof o.title === "string" &&
    typeof o.url === "string" &&
    typeof o.raw_content === "string" &&
    typeof o.captured_at === "string" &&
    !!o.metadata &&
    typeof o.metadata === "object"
  );
}

function isValidRow(r: unknown): r is WALRow {
  if (!r || typeof r !== "object") return false;
  const o = r as Record<string, unknown>;
  return (
    typeof o.client_event_id === "string" &&
    typeof o.enqueued_at === "number" &&
    typeof o.attempts === "number" &&
    typeof o.next_attempt_at === "number" &&
    isValidArtifact(o.artifact)
  );
}

export class Queue {
  private readonly env: IDBEnv;
  constructor(env: IDBEnv = defaultEnv()) {
    this.env = env;
  }

  async enqueue(artifact: RawArtifact, nowMs: number = Date.now()): Promise<void> {
    const row: WALRow = {
      client_event_id: artifact.metadata.client_event_id,
      enqueued_at: nowMs,
      artifact,
      attempts: 0,
      next_attempt_at: nowMs,
    };
    const db = await openDB(this.env);
    try {
      const tx = db.transaction(STORE, "readwrite");
      tx.objectStore(STORE).put(row);
      await txDone(tx);
    } finally {
      db.close();
    }
  }

  // Returns up to maxItems WAL rows whose next_attempt_at <= nowMs and whose
  // serialised JSON fits inside maxBytes. Corrupted rows (failed shape check)
  // are removed in a separate writable transaction and logged once each by
  // the drainer that calls this.
  async peekBatch(
    maxItems: number,
    maxBytes: number,
    nowMs: number = Date.now(),
  ): Promise<{ ready: WALRow[]; corrupted: string[] }> {
    const db = await openDB(this.env);
    const ready: WALRow[] = [];
    const corrupted: string[] = [];
    let used = 0;
    try {
      const tx = db.transaction(STORE, "readonly");
      const store = tx.objectStore(STORE);
      const all = await promisifyRequest(store.getAll());
      for (const raw of all) {
        if (!isValidRow(raw)) {
          // We still try to extract the key for cleanup so neighbors survive
          // (SCN-058-015).
          const maybeKey =
            raw && typeof raw === "object" && typeof (raw as Record<string, unknown>).client_event_id === "string"
              ? ((raw as Record<string, unknown>).client_event_id as string)
              : "";
          if (maybeKey) corrupted.push(maybeKey);
          continue;
        }
        if (raw.next_attempt_at > nowMs) continue;
        const json = JSON.stringify(raw.artifact);
        const size = json.length; // proxy for byte size; UTF-8 is bounded by 4x
        if (ready.length > 0 && used + size > maxBytes) break;
        ready.push(raw);
        used += size;
        if (ready.length >= maxItems) break;
      }
      await txDone(tx);
    } finally {
      db.close();
    }
    if (corrupted.length > 0) {
      await this.removeCorrupted(corrupted);
    }
    return { ready, corrupted };
  }

  async removeCorrupted(keys: string[]): Promise<void> {
    if (keys.length === 0) return;
    const db = await openDB(this.env);
    try {
      const tx = db.transaction(STORE, "readwrite");
      const store = tx.objectStore(STORE);
      for (const k of keys) store.delete(k);
      await txDone(tx);
    } finally {
      db.close();
    }
  }

  async markOutcome(
    clientEventID: string,
    outcome: IngestOutcome,
    nextAttemptAt?: number,
  ): Promise<void> {
    const db = await openDB(this.env);
    try {
      const tx = db.transaction(STORE, "readwrite");
      const store = tx.objectStore(STORE);
      if (outcome === "accepted" || outcome === "deduped") {
        store.delete(clientEventID);
      } else {
        // rejected: terminal — drop. (5xx / network failures go through
        // scheduleRetry instead and are mapped to outcome "rejected" only by
        // the transport layer when the server explicitly rejected the item.)
        store.delete(clientEventID);
      }
      await txDone(tx);
      if (nextAttemptAt !== undefined) {
        // No-op for terminal outcomes; included for symmetry with retry path.
      }
    } finally {
      db.close();
    }
  }

  async scheduleRetry(
    clientEventID: string,
    nextAttemptAt: number,
    attempts: number,
  ): Promise<void> {
    const db = await openDB(this.env);
    try {
      const tx = db.transaction(STORE, "readwrite");
      const store = tx.objectStore(STORE);
      const existing = (await promisifyRequest(store.get(clientEventID))) as
        | WALRow
        | undefined;
      if (!existing) {
        await txDone(tx);
        return;
      }
      existing.next_attempt_at = nextAttemptAt;
      existing.attempts = attempts;
      store.put(existing);
      await txDone(tx);
    } finally {
      db.close();
    }
  }

  async size(): Promise<number> {
    const db = await openDB(this.env);
    try {
      const tx = db.transaction(STORE, "readonly");
      const c = await promisifyRequest(tx.objectStore(STORE).count());
      await txDone(tx);
      return c;
    } finally {
      db.close();
    }
  }

  async getAllForTest(): Promise<WALRow[]> {
    const db = await openDB(this.env);
    try {
      const tx = db.transaction(STORE, "readonly");
      const all = (await promisifyRequest(tx.objectStore(STORE).getAll())) as WALRow[];
      await txDone(tx);
      return all.filter(isValidRow);
    } finally {
      db.close();
    }
  }

  // Test-only: insert an arbitrary row (used to seed corrupted-row scenarios).
  async putRawForTest(row: unknown): Promise<void> {
    const db = await openDB(this.env);
    try {
      const tx = db.transaction(STORE, "readwrite");
      tx.objectStore(STORE).put(row as WALRow);
      await txDone(tx);
    } finally {
      db.close();
    }
  }
}
