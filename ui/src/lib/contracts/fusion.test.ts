import { describe, expect, it } from "vitest";
import { parseFusionResponse } from "./fusion";

const response = {
  contract_version: "1",
  index: {
    ready: true,
    state: "ready",
    indexed_revision: 14,
    target_revision: 14,
    lag: 0,
  },
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
      handle: "opaque-do-not-address",
    },
  ],
  paths: [{ names: ["loadWorkbench", "refresh"], truncated: true }],
  impact: { nodes: 5, files: 3, truncated: false },
  truncated: false,
  additive_future_field: { accepted: true },
};

describe("parseFusionResponse", () => {
  it("parses beta.145 fusion v1 while ignoring additive unknown fields", () => {
    expect(parseFusionResponse(response)).toMatchObject({
      contract_version: "1",
      provenance: "embedding",
      index: { ready: true, state: "ready", lag: 0 },
      nodes: [
        {
          name: "loadWorkbench",
          path: "ui/src/lib/api/workbench.ts",
          lines: [21, 63],
          handle: "opaque-do-not-address",
        },
      ],
      misses: [],
      paths: [{ names: ["loadWorkbench", "refresh"], truncated: true }],
      impact: { nodes: 5, files: 3, truncated: false },
      truncated: false,
    });
  });

  it.each([
    [{ ...response, contract_version: "2" }, "contract version"],
    [{ ...response, provenance: "guessed" }, "provenance"],
    [{ ...response, truncated: "no" }, "truncated"],
    [{ ...response, index: { ready: "yes", state: "ready" } }, "index"],
    [{ ...response, nodes: [{ name: "bad", lines: [10] }] }, "node lines"],
    [
      { ...response, misses: [{ query: "thing", did_you_mean: [42] }] },
      "miss suggestions",
    ],
    [{ ...response, paths: [{ names: ["good", 42] }] }, "path"],
    [
      { ...response, impact: { nodes: 2, files: "one", truncated: false } },
      "impact",
    ],
    [{ ...response, index: { ready: true, state: "building" } }, "index"],
    [
      {
        ...response,
        index: {
          ready: false,
          state: "building",
          indexed_revision: 5,
          target_revision: 8,
          lag: 2,
        },
      },
      "lag",
    ],
    [
      {
        ...response,
        index: {
          ready: true,
          state: "ready",
          indexed_revision: 5,
          target_revision: 6,
          lag: 0,
        },
      },
      "caught up",
    ],
    [
      {
        ...response,
        index: {
          ready: true,
          state: "ready",
          indexed_revision: 9,
          target_revision: 8,
          lag: 0,
        },
      },
      "target",
    ],
    [
      {
        ...response,
        index: {
          ready: true,
          state: "ready",
          indexed_revision: 8,
          revision: "7",
        },
      },
      "revision",
    ],
  ])("rejects malformed known fields", (value, message) => {
    expect(() => parseFusionResponse(value)).toThrow(message);
  });

  it("keeps absent evidence absent", () => {
    const parsed = parseFusionResponse({
      contract_version: "1",
      index: { ready: false, state: "building" },
      provenance: "embedding",
      truncated: false,
    });
    expect(parsed.nodes).toEqual([]);
    expect(parsed.misses).toEqual([]);
    expect(parsed.index.lag).toBeUndefined();
  });

  it("accepts matching unsigned decimal revision evidence", () => {
    expect(
      parseFusionResponse({
        ...response,
        index: {
          ready: true,
          state: "ready",
          indexed_revision: 8,
          revision: "8",
        },
      }).index.revision,
    ).toBe("8");
  });
});
