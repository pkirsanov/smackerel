// Wire-schema typings mirrored from internal/connector/connector.go::RawArtifact
// and the spec 058 design §2.1/§2.2 Metadata enumeration. These are the source
// of truth on the extension side; any drift from the server-side Go types is
// a spec amendment, not a refactor.

export type ContentType = "bookmark" | "browser_history_visit";

export type BookmarkEvent = "created" | "updated" | "removed";

export type IngestOutcome = "accepted" | "deduped" | "rejected";

export interface CommonMetadata {
  source_device_id: string;
  extension_version: string;
  privacy_filter_version: string;
  client_event_id: string;
  // Optional per-request override for the server's history dedup window
  // (resolved against SST default_dedup_window_seconds when absent).
  dedup_window_seconds?: number;
}

export interface BookmarkMetadata extends CommonMetadata {
  bookmark_id: string;
  bookmark_folder_path: string[];
  bookmark_event: BookmarkEvent;
  parent_id?: string;
}

export interface HistoryMetadata extends CommonMetadata {
  dwell_estimate_seconds: number;
  transition_type: string;
  referrer_url?: string;
  visit_started_at: string;
}

export type Metadata = BookmarkMetadata | HistoryMetadata;

// JSON tags match internal/connector/connector.go::RawArtifact.
export interface RawArtifact {
  source_id: "browser-extension";
  source_ref: string;
  content_type: ContentType;
  title: string;
  url: string;
  raw_content: string;
  captured_at: string; // RFC3339
  metadata: Metadata;
}

export interface IngestItemOutcome {
  client_event_id: string;
  outcome: IngestOutcome;
  artifact_id?: string;
  error?: string;
}

export interface IngestResponse {
  items: IngestItemOutcome[];
}

// WAL row shape persisted in IndexedDB. Stored as a structured-cloneable object
// so a corrupted row (e.g. unparseable artifact body) is detectable when we
// validate the shape during drain (SCN-058-015).
export interface WALRow {
  client_event_id: string;
  enqueued_at: number; // epoch ms
  artifact: RawArtifact;
  attempts: number;
  next_attempt_at: number; // epoch ms
}
