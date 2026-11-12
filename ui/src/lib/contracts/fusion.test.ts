import { describe, expect, it } from "vitest";
import { parseFusionResponse } from "./fusion";

const response = {
  contract_version: "2",
  provenance: "embedding",
  nodes: [
    {
      name: "loadWorkbench",
      kind: "function",
      path: "ui/src/lib/api/workbench.ts",
      lines: [21, 63],
      body: "export async function loadWorkbench() {}",
      relations: {
        caller: [
          { name: "refresh", path: "ui/src/routes/+page.svelte", line: 18 },
        ],
      },
      relations_truncated: true,
      handle: "opaque-do-not-address",
    },
  ],
  paths: [{ names: ["loadWorkbench", "refresh"], truncated: true }],
  impact: { nodes: 5, files: 3, truncated: false },
  unreadable_records: 2,
  failed_records: 1,
  truncated: false,
  paths_truncated: true,
  additive_future_field: { accepted: true },
};

describe("parseFusionResponse", () => {
  // spec: qualify-semengine-tier0 / Adopt Fusion Contract v2 and Exact Seed Resolution
  it("parses semengine fusion v2 while ignoring additive unknown fields", () => {
    expect(parseFusionResponse(response)).toMatchObject({
      contract_version: "2",
      provenance: "embedding",
      deferred: false,
      nodes: [
        {
          name: "loadWorkbench",
          path: "ui/src/lib/api/workbench.ts",
          lines: [21, 63],
          relations_truncated: true,
          handle: "opaque-do-not-address",
        },
      ],
      misses: [],
      paths: [{ names: ["loadWorkbench", "refresh"], truncated: true }],
      impact: { nodes: 5, files: 3, truncated: false },
      unreadable_records: 2,
      failed_records: 1,
      truncated: false,
      paths_truncated: true,
    });
  });

  it.each([
    [{ ...response, contract_version: "1" }, "contract version"],
    [{ ...response, provenance: "guessed" }, "provenance"],
    [{ ...response, truncated: "no" }, "truncated"],
    [{ ...response, paths_truncated: "no" }, "paths_truncated"],
    [{ ...response, deferred: "yes" }, "deferred"],
    [
      { ...response, deferred: true, defer_reason: "unknown_reason" },
      "defer_reason",
    ],
    [
      { ...response, deferred: false, defer_reason: "index_not_ready" },
      "defer_reason",
    ],
    [{ ...response, unreadable_records: -1 }, "unreadable_records"],
    [{ ...response, failed_records: 1.5 }, "failed_records"],
    [{ ...response, nodes: [{ name: "bad", lines: [10] }] }, "node lines"],
    [
      {
        ...response,
        nodes: [{ name: "bad", relations_truncated: "yes" }],
      },
      "relations_truncated",
    ],
    [
      { ...response, misses: [{ query: "thing", did_you_mean: [42] }] },
      "miss suggestions",
    ],
    [{ ...response, paths: [{ names: ["good", 42] }] }, "path"],
    [
      { ...response, impact: { nodes: 2, files: "one", truncated: false } },
      "impact",
    ],
  ])("rejects malformed known fields", (value, message) => {
    expect(() => parseFusionResponse(value)).toThrow(message);
  });

  it("keeps absent evidence absent", () => {
    const parsed = parseFusionResponse({
      contract_version: "2",
      deferred: true,
      defer_reason: "index_not_ready",
      provenance: "embedding",
      truncated: false,
    });
    expect(parsed.nodes).toEqual([]);
    expect(parsed.misses).toEqual([]);
    expect(parsed.deferred).toBe(true);
    expect(parsed.defer_reason).toBe("index_not_ready");
  });

  it("accepts all_seeds_unhydrated defer_reason when deferred is true", () => {
    const parsed = parseFusionResponse({
      ...response,
      deferred: true,
      defer_reason: "all_seeds_unhydrated",
    });
    expect(parsed.deferred).toBe(true);
    expect(parsed.defer_reason).toBe("all_seeds_unhydrated");
  });
});
