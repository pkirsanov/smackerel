// chrome.bookmarks event mapping → RawArtifact. Listeners registered at top
// level of background/index.ts so they survive SW eviction (design §4.3).
//
// Bookmark URL is the canonical dedup key; the folder path is captured for
// downstream knowledge-graph use. Removed bookmarks emit a tombstone with
// bookmark_event="removed" so the server can mark the existing artifact
// inactive (server-side semantics owned by spec 058 Scope 2 publisher).

import type { BookmarkMetadata, RawArtifact } from "../common/schema.js";
import { uuidv7 } from "../common/uuid.js";

export interface BookmarkEventInput {
  bookmark_id: string;
  parent_id?: string;
  url: string;
  title: string;
  folder_path: string[];
  event: "created" | "updated" | "removed";
  captured_at: string; // RFC3339
}

export interface BookmarkBuildContext {
  source_device_id: string;
  extension_version: string;
  privacy_filter_version: string;
  now: () => Date;
}

export function buildBookmarkArtifact(
  input: BookmarkEventInput,
  ctx: BookmarkBuildContext,
): RawArtifact {
  const metadata: BookmarkMetadata = {
    source_device_id: ctx.source_device_id,
    extension_version: ctx.extension_version,
    privacy_filter_version: ctx.privacy_filter_version,
    client_event_id: uuidv7(ctx.now().getTime()),
    bookmark_id: input.bookmark_id,
    bookmark_folder_path: input.folder_path,
    bookmark_event: input.event,
    parent_id: input.parent_id,
  };
  return {
    source_id: "browser-extension",
    source_ref: `bookmark:${input.bookmark_id}`,
    content_type: "bookmark",
    title: input.title,
    url: input.url,
    raw_content: "",
    captured_at: input.captured_at,
    metadata,
  };
}

// Walks the bookmarks tree from a node id upward to produce the human-readable
// folder path. Defensive against transient API failures (returns []).
export async function resolveFolderPath(parentId?: string): Promise<string[]> {
  if (!parentId) return [];
  const path: string[] = [];
  let cursor: string | undefined = parentId;
  for (let i = 0; i < 32 && cursor; i++) {
    let node: chrome.bookmarks.BookmarkTreeNode | undefined;
    try {
      const nodes = (await chrome.bookmarks.get(
        cursor,
      )) as chrome.bookmarks.BookmarkTreeNode[];
      node = nodes[0];
    } catch {
      break;
    }
    if (!node) break;
    if (node.title) path.unshift(node.title);
    cursor = node.parentId;
  }
  return path;
}
