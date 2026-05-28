// HTTP transport for POST /v1/connectors/extension/ingest. Maps the status
// codes from design §3.1 to per-item retry classification consumed by the
// drainer.
//
// Status mapping (SCN-058-014):
//   200          → per-item outcomes from response body (server-authoritative)
//   401, 403     → "auth_terminal": all queued items terminal, badge AUTH
//   400, 413, 422 → "batch_terminal": the offending batch is rejected per-item;
//                   the drainer drops the items in this batch (no retry)
//   5xx, network → "retryable": leave items queued, schedule next backoff
//
// The bearer token is sent in the Authorization header and is NEVER logged.

import type {
  IngestItemOutcome,
  IngestResponse,
  RawArtifact,
} from "../common/schema.js";

export type TransportClassification =
  | { kind: "ok"; outcomes: IngestItemOutcome[] }
  | { kind: "auth_terminal"; status: number; code: string }
  | { kind: "batch_terminal"; status: number; code: string }
  | { kind: "retryable"; status: number; code: string };

export interface TransportConfig {
  baseURL: string;
  bearerToken: string;
  fetchImpl?: typeof fetch;
}

const INGEST_PATH = "/v1/connectors/extension/ingest";

export type TerminalKind = "ok" | "auth_terminal" | "batch_terminal" | "retryable";

export function classifyStatus(status: number): TerminalKind {
  if (status === 401 || status === 403) return "auth_terminal";
  if (status === 400 || status === 413 || status === 422) return "batch_terminal";
  return "retryable";
}

export async function postBatch(
  cfg: TransportConfig,
  items: RawArtifact[],
): Promise<TransportClassification> {
  const fetchImpl = cfg.fetchImpl ?? fetch;
  const url = cfg.baseURL.replace(/\/+$/, "") + INGEST_PATH;
  let resp: Response;
  try {
    resp = await fetchImpl(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${cfg.bearerToken}`,
      },
      body: JSON.stringify(items),
    });
  } catch (err) {
    return {
      kind: "retryable",
      status: 0,
      code: `network: ${(err as Error).message}`,
    };
  }

  if (resp.status === 200) {
    let body: IngestResponse;
    try {
      body = (await resp.json()) as IngestResponse;
    } catch (err) {
      return {
        kind: "retryable",
        status: 200,
        code: `invalid_response_json: ${(err as Error).message}`,
      };
    }
    if (!body || !Array.isArray(body.items)) {
      return { kind: "retryable", status: 200, code: "invalid_response_shape" };
    }
    return { kind: "ok", outcomes: body.items };
  }

  let code = "";
  try {
    const errBody = (await resp.json()) as { code?: string };
    if (errBody && typeof errBody.code === "string") code = errBody.code;
  } catch {
    // ignore — non-JSON error body
  }
  const kind = classifyStatus(resp.status);
  if (kind === "auth_terminal") return { kind, status: resp.status, code };
  if (kind === "batch_terminal") return { kind, status: resp.status, code };
  return { kind: "retryable", status: resp.status, code };
}
