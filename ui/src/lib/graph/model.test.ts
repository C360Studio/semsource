import { describe, expect, it } from "vitest";
import { parseFusionResponse } from "$lib/contracts/fusion";
import { completeGraphResponse } from "../../tests/fixtures/graph";
import { emptyGraph, syncGraph } from "./model";

function parsed(value = completeGraphResponse) {
  const response = parseFusionResponse(value);
  if (!response.graph) throw new Error("fixture graph missing");
  return response;
}

describe("syncGraph", () => {
  it("creates unresolved stubs only for explicit edge endpoints", () => {
    const response = parsed();
    const result = syncGraph(emptyGraph(), response.graph!, response.nodes);
    expect(
      result.graph.nodes.map((node) => [node.handle, node.resolved]),
    ).toEqual([
      ["handle-a", true],
      ["handle-b", true],
      ["missing-handle", false],
    ]);
    expect(
      result.graph.nodes.some((node) => node.handle.includes("org.platform")),
    ).toBe(false);
  });

  it("refreshes same-handle attributes and revision-only updates", () => {
    const first = parsed();
    const state = syncGraph(emptyGraph(), first.graph!, first.nodes).graph;
    const changed = structuredClone(completeGraphResponse);
    changed.nodes[0].name = "Alpha renamed";
    changed.graph.view_revision = { start: 42, end: 42, coherent: true };
    const second = parsed(changed);
    const next = syncGraph(state, second.graph!, second.nodes).graph;
    expect(next.nodes.find((node) => node.handle === "handle-a")?.name).toBe(
      "Alpha renamed",
    );
    expect(next.revision).toEqual({ start: 42, end: 42, coherent: true });
    expect(next).not.toBe(state);
  });

  it("deletes absent items only for a complete coherent nonzero projection", () => {
    const first = parsed();
    const state = syncGraph(emptyGraph(), first.graph!, first.nodes).graph;
    const exact = structuredClone(completeGraphResponse);
    exact.nodes = [exact.nodes[0]];
    exact.graph.nodes = [exact.graph.nodes[0]];
    exact.graph.edges = [];
    exact.graph.view_revision = { start: 43, end: 43, coherent: true };
    const response = parsed(exact);
    const result = syncGraph(state, response.graph!, response.nodes);
    expect(result.mode).toBe("complete");
    expect(result.graph.nodes.map((node) => node.handle)).toEqual(["handle-a"]);
    expect(result.graph.edges).toEqual([]);
  });

  it.each([
    [{ truncated: true }, "truncated"],
    [{ view_revision: { start: 41, end: 42, coherent: false } }, "incoherent"],
    [
      { view_revision: { start: 0, end: 0, coherent: true } },
      "revision unavailable",
    ],
  ])("retains absent items for partial projections: %s", (override, reason) => {
    const first = parsed();
    const state = syncGraph(emptyGraph(), first.graph!, first.nodes).graph;
    const partial = structuredClone(completeGraphResponse);
    partial.nodes = [partial.nodes[0]];
    partial.graph.nodes = [partial.graph.nodes[0]];
    partial.graph.edges = [];
    Object.assign(partial.graph, override);
    const response = parsed(partial);
    const result = syncGraph(state, response.graph!, response.nodes);
    expect(result.mode).toBe("partial");
    expect(result.reason).toContain(reason);
    expect(result.graph.nodes.some((node) => node.handle === "handle-b")).toBe(
      true,
    );
    expect(result.graph.edges.length).toBe(4);
  });

  it("retains newer state when a lower complete revision arrives late", () => {
    const first = parsed();
    const state = syncGraph(emptyGraph(), first.graph!, first.nodes).graph;
    const stale = structuredClone(completeGraphResponse);
    stale.nodes = [stale.nodes[0]];
    stale.graph.nodes = [stale.graph.nodes[0]];
    stale.nodes[0].name = "Stale Alpha";
    stale.graph.nodes[0].facts[0].value = "stale fact";
    stale.graph.nodes[0].facts[0].evidence = [
      { source: "stale source", timestamp: "2026-07-19T12:00:00Z" },
    ];
    stale.graph.edges = [stale.graph.edges[0]];
    stale.graph.edges[0].evidence = [{ source: "stale edge", confidence: 0.1 }];
    stale.graph.view_revision = { start: 40, end: 40, coherent: true };
    const response = parsed(stale);
    const result = syncGraph(state, response.graph!, response.nodes);
    expect(result.mode).toBe("partial");
    expect(result.reason).toContain("stale");
    expect(result.graph).toBe(state);
    expect(
      result.graph.nodes.find((node) => node.handle === "handle-a"),
    ).toEqual(state.nodes.find((node) => node.handle === "handle-a"));
    expect(
      result.graph.nodes.find((node) => node.handle === "handle-a")?.name,
    ).toBe("Alpha");
    expect(
      result.graph.nodes.find((node) => node.handle === "handle-a")?.facts[0],
    ).toEqual({
      predicate: "source.literal",
      value: "org.platform.domain.system.type.instance",
      datatype: "xsd:string",
      evidence: [{ source: "ast", timestamp: "2026-07-19T12:00:00Z" }],
      truncated: false,
    });
    expect(result.graph.edges).toEqual(state.edges);
    expect(result.graph.edges).toHaveLength(4);
    expect(result.graph.edges[0]?.evidence).toEqual([
      { source: "ast", confidence: 0.8 },
    ]);
    expect(result.graph.nodes.some((node) => node.handle === "handle-b")).toBe(
      true,
    );
    expect(result.graph.revision).toEqual({
      start: 41,
      end: 41,
      coherent: true,
    });
  });
});
