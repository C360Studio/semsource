import { describe, expect, it } from "vitest";
import type { WorkbenchCapabilities } from "$lib/contracts/capabilities";
import { deriveReadinessCoverage, isFullyReady } from "./readiness";

function readiness(
  overrides: Partial<WorkbenchCapabilities["readiness"]> = {},
): WorkbenchCapabilities["readiness"] {
  return {
    overall: "ready",
    source: { available: true, ready: true, state: "ready" },
    structural_index: { available: true, ready: true, state: "ready" },
    semantic_index: { available: true, ready: true, state: "ready" },
    ...overrides,
  };
}

describe("deriveReadinessCoverage", () => {
  it("is ready only when every advertised signal is ready", () => {
    expect(deriveReadinessCoverage(readiness()).ready).toBe(true);
    expect(
      deriveReadinessCoverage(
        readiness({
          semantic_index: { available: true, ready: false, state: "building" },
        }),
      ).ready,
    ).toBe(false);
  });

  it("does not let a backend overall=ready mask a building semantic index", () => {
    // The backend's own overall field only gates on source + structural index;
    // this is the exact "Ready while Building" defect the coverage must close.
    const value = readiness({
      overall: "ready",
      semantic_index: { available: true, ready: false, state: "building" },
    });
    const coverage = deriveReadinessCoverage(value);
    expect(coverage.ready).toBe(false);
    expect(coverage.building).toEqual(["Semantic index"]);
  });

  it("does not block on a signal that is not advertised at all", () => {
    const value = readiness({
      semantic_index: { available: false, ready: false, state: "unknown" },
    });
    const coverage = deriveReadinessCoverage(value);
    expect(coverage.ready).toBe(true);
    expect(coverage.covered).toEqual(["Sources", "Structural index"]);
  });

  it("names every building signal, not just the first", () => {
    const value = readiness({
      structural_index: { available: true, ready: false, state: "building" },
      semantic_index: { available: true, ready: false, state: "building" },
    });
    expect(deriveReadinessCoverage(value).building).toEqual([
      "Structural index",
      "Semantic index",
    ]);
  });
});

describe("isFullyReady", () => {
  it("is false while capabilities have not loaded", () => {
    expect(isFullyReady(null)).toBe(false);
  });

  it("mirrors the coverage verdict once capabilities are present", () => {
    const capabilities = {
      readiness: readiness({
        semantic_index: { available: true, ready: false, state: "building" },
      }),
    } as WorkbenchCapabilities;
    expect(isFullyReady(capabilities)).toBe(false);
  });
});
