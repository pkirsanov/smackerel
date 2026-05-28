import { describe, expect, it } from "vitest";
import { passesDwellGate } from "../../src/background/dwell_gate.js";

describe("dwell gate", () => {
  it("SCN-058-011 dropsVisitBelowThreshold: dwell=30, threshold=120 → false", () => {
    expect(passesDwellGate(30, 120)).toBe(false);
  });

  it("dwell exactly at threshold passes", () => {
    expect(passesDwellGate(120, 120)).toBe(true);
  });

  it("dwell above threshold passes", () => {
    expect(passesDwellGate(180, 120)).toBe(true);
  });

  it("threshold of 0 admits every non-negative dwell", () => {
    expect(passesDwellGate(0, 0)).toBe(true);
    expect(passesDwellGate(1, 0)).toBe(true);
  });

  it("negative or non-finite dwell never passes", () => {
    expect(passesDwellGate(-1, 0)).toBe(false);
    expect(passesDwellGate(Number.NaN, 0)).toBe(false);
    // Infinity is not finite → guard rejects it as malformed sensor data.
    expect(passesDwellGate(Number.POSITIVE_INFINITY, 0)).toBe(false);
  });
});
