// Minimal UUID v4 and v7 generators. v7 is required for client_event_id
// (idempotency key) per design §2.2; v4 is used for auto-<uuidv4> device-id
// fallback per design §4.4.

function getRandomBytes(n: number): Uint8Array {
  const buf = new Uint8Array(n);
  // crypto.getRandomValues exists in service workers, page contexts, and
  // Node 19+. Tests run on Node ≥ 20.
  crypto.getRandomValues(buf);
  return buf;
}

function bytesToHex(bytes: Uint8Array): string {
  let out = "";
  for (const b of bytes) {
    out += b.toString(16).padStart(2, "0");
  }
  return out;
}

export function uuidv4(): string {
  const b = getRandomBytes(16);
  b[6] = (b[6] & 0x0f) | 0x40; // version 4
  b[8] = (b[8] & 0x3f) | 0x80; // RFC4122 variant
  const h = bytesToHex(b);
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

// UUIDv7: 48-bit unix-ms timestamp + 4-bit version + 12 random bits + 2-bit
// variant + 62 random bits. Strictly monotonic ordering within a process is
// not required by the spec; we rely on the timestamp prefix for k-sortability
// and the random tail for collision resistance.
export function uuidv7(nowMs: number = Date.now()): string {
  const rand = getRandomBytes(10);
  const ts = BigInt(nowMs);
  const b = new Uint8Array(16);
  b[0] = Number((ts >> 40n) & 0xffn);
  b[1] = Number((ts >> 32n) & 0xffn);
  b[2] = Number((ts >> 24n) & 0xffn);
  b[3] = Number((ts >> 16n) & 0xffn);
  b[4] = Number((ts >> 8n) & 0xffn);
  b[5] = Number(ts & 0xffn);
  b[6] = 0x70 | (rand[0] & 0x0f); // version 7
  b[7] = rand[1];
  b[8] = 0x80 | (rand[2] & 0x3f); // variant
  for (let i = 0; i < 7; i++) {
    b[9 + i] = rand[3 + i];
  }
  const h = bytesToHex(b);
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}
