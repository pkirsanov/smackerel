import { describe, expect, it } from "vitest";
import { classifyStatus, postBatch } from "../../src/background/transport.js";
import type { RawArtifact } from "../../src/common/schema.js";

const sampleArtifact: RawArtifact = {
  source_id: "browser-extension",
  source_ref: "bookmark:abc",
  content_type: "bookmark",
  title: "t",
  url: "https://example.com/",
  raw_content: "",
  captured_at: "2026-05-28T12:00:00Z",
  metadata: {
    source_device_id: "laptop",
    extension_version: "0.1.0",
    privacy_filter_version: "pf-x",
    client_event_id: "0190d400-0000-7000-8000-000000000001",
    bookmark_id: "abc",
    bookmark_folder_path: [],
    bookmark_event: "created",
  },
};

describe("transport.classifyStatus", () => {
  it("SCN-058-014 mapsHTTPStatusToOutcome: 401/403 → auth_terminal", () => {
    expect(classifyStatus(401)).toBe("auth_terminal");
    expect(classifyStatus(403)).toBe("auth_terminal");
  });
  it("SCN-058-014: 400/413/422 → batch_terminal (per-item rejection, no retry)", () => {
    expect(classifyStatus(400)).toBe("batch_terminal");
    expect(classifyStatus(413)).toBe("batch_terminal");
    expect(classifyStatus(422)).toBe("batch_terminal");
  });
  it("SCN-058-014: 5xx → retryable", () => {
    expect(classifyStatus(500)).toBe("retryable");
    expect(classifyStatus(503)).toBe("retryable");
  });
});

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("transport.postBatch", () => {
  it("returns ok with per-item outcomes on 200", async () => {
    const fakeFetch = async () =>
      jsonResponse(200, {
        items: [
          {
            client_event_id: "0190d400-0000-7000-8000-000000000001",
            outcome: "accepted",
            artifact_id: "art_1",
          },
        ],
      });
    const r = await postBatch(
      {
        baseURL: "https://h.example",
        bearerToken: "tok",
        fetchImpl: fakeFetch as unknown as typeof fetch,
      },
      [sampleArtifact],
    );
    expect(r.kind).toBe("ok");
    if (r.kind === "ok") {
      expect(r.outcomes).toHaveLength(1);
      expect(r.outcomes[0].outcome).toBe("accepted");
    }
  });

  it("maps 401 to auth_terminal carrying the server code", async () => {
    const fakeFetch = async () => jsonResponse(401, { code: "auth_invalid" });
    const r = await postBatch(
      {
        baseURL: "https://h.example",
        bearerToken: "tok",
        fetchImpl: fakeFetch as unknown as typeof fetch,
      },
      [sampleArtifact],
    );
    expect(r.kind).toBe("auth_terminal");
    if (r.kind === "auth_terminal") expect(r.code).toBe("auth_invalid");
  });

  it("maps 413 to batch_terminal", async () => {
    const fakeFetch = async () =>
      jsonResponse(413, { code: "body_too_large" });
    const r = await postBatch(
      {
        baseURL: "https://h.example",
        bearerToken: "tok",
        fetchImpl: fakeFetch as unknown as typeof fetch,
      },
      [sampleArtifact],
    );
    expect(r.kind).toBe("batch_terminal");
  });

  it("maps 503 to retryable", async () => {
    const fakeFetch = async () =>
      jsonResponse(503, { code: "pipeline_unavailable" });
    const r = await postBatch(
      {
        baseURL: "https://h.example",
        bearerToken: "tok",
        fetchImpl: fakeFetch as unknown as typeof fetch,
      },
      [sampleArtifact],
    );
    expect(r.kind).toBe("retryable");
  });

  it("maps network failure to retryable", async () => {
    const fakeFetch = async () => {
      throw new Error("ECONNREFUSED");
    };
    const r = await postBatch(
      {
        baseURL: "https://h.example",
        bearerToken: "tok",
        fetchImpl: fakeFetch as unknown as typeof fetch,
      },
      [sampleArtifact],
    );
    expect(r.kind).toBe("retryable");
    if (r.kind === "retryable") expect(r.status).toBe(0);
  });

  it("sends bearer token in Authorization header (never in body)", async () => {
    let seen: { url?: string; init?: RequestInit } = {};
    const fakeFetch = async (url: string, init?: RequestInit) => {
      seen = { url, init };
      return jsonResponse(200, { items: [] });
    };
    await postBatch(
      {
        baseURL: "https://h.example",
        bearerToken: "supersecret",
        fetchImpl: fakeFetch as unknown as typeof fetch,
      },
      [sampleArtifact],
    );
    const auth = (seen.init?.headers as Record<string, string>).Authorization;
    expect(auth).toBe("Bearer supersecret");
    expect(seen.url).toBe("https://h.example/v1/connectors/extension/ingest");
    const body = String(seen.init?.body ?? "");
    expect(body.includes("supersecret")).toBe(false);
  });
});
