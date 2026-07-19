import { describe, expect, it } from "vitest";
import { completeGraphResponse } from "../../tests/fixtures/graph";
import { parseGraphProjection } from "./graph";

describe("parseGraphProjection", () => {
  it("preserves typed facts, parallel/opposite edges, opaque handles, and absent evidence", () => {
    const graph = parseGraphProjection(completeGraphResponse.graph);
    expect(graph.nodes[0]?.facts[0]?.value).toBe(
      "org.platform.domain.system.type.instance",
    );
    expect(graph.nodes[0]?.facts[1]?.value).toEqual([10, 14]);
    expect(
      graph.edges.map(({ source, predicate, target }) => [
        source,
        predicate,
        target,
      ]),
    ).toEqual([
      ["handle-a", "calls", "handle-b"],
      ["handle-a", "imports", "handle-b"],
      ["handle-b", "references", "handle-a"],
      ["handle-a", "returns", "missing-handle"],
    ]);
    expect(graph.edges[1]?.evidence[0]).toEqual({});
    expect(graph.edges[1]?.evidence[0]?.confidence).toBeUndefined();
  });

  it("preserves every level of positive truncation evidence", () => {
    const value = {
      ...completeGraphResponse.graph,
      truncated: true,
      nodes: completeGraphResponse.graph.nodes.map((node, nodeIndex) =>
        nodeIndex === 0
          ? {
              ...node,
              facts_truncated: true,
              facts_dropped: 3,
              facts: node.facts.map((fact, factIndex) =>
                factIndex === 0 ? { ...fact, truncated: true } : fact,
              ),
            }
          : node,
      ),
      edges: completeGraphResponse.graph.edges.map((edge, index) =>
        index === 0 ? { ...edge, truncated: true } : edge,
      ),
    };
    const graph = parseGraphProjection(value);
    expect(graph.truncated).toBe(true);
    expect(graph.nodes[0]).toMatchObject({
      facts_truncated: true,
      facts_dropped: 3,
    });
    expect(graph.nodes[0]?.facts[0]?.truncated).toBe(true);
    expect(graph.edges[0]?.truncated).toBe(true);
  });

  it.each([
    [{ ...completeGraphResponse.graph, truncated: "no" }, "truncated"],
    [
      {
        ...completeGraphResponse.graph,
        nodes: [{ handle: "x", facts: [{ predicate: "p" }] }],
      },
      "fact",
    ],
    [
      {
        ...completeGraphResponse.graph,
        edges: [
          {
            id: "x",
            source: "a",
            target: "b",
            predicate: "p",
            direction: "sideways",
          },
        ],
      },
      "edge",
    ],
    [
      {
        ...completeGraphResponse.graph,
        view_revision: { start: 2, end: 3, coherent: true },
      },
      "revision",
    ],
    [
      {
        ...completeGraphResponse.graph,
        nodes: [{ handle: "x", facts_truncated: false, facts_dropped: 2 }],
      },
      "facts",
    ],
    [
      {
        ...completeGraphResponse.graph,
        edges: [
          {
            id: "a|p|b",
            source: "a",
            target: "b",
            predicate: "p",
            direction: "outgoing",
            evidence: [{ timestamp: "yesterday" }],
          },
        ],
      },
      "evidence",
    ],
  ])("rejects malformed known graph fields", (value, message) => {
    expect(() => parseGraphProjection(value)).toThrow(message);
  });
});
