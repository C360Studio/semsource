export const completeGraphResponse = {
  contract_version: "1",
  index: {
    ready: true,
    state: "ready",
    indexed_revision: 41,
    target_revision: 41,
    lag: 0,
  },
  provenance: "deterministic",
  nodes: [
    { name: "Alpha", kind: "function", path: "alpha.go", handle: "handle-a" },
    { name: "Beta", kind: "function", path: "beta.go", handle: "handle-b" },
  ],
  graph: {
    nodes: [
      {
        handle: "handle-a",
        facts: [
          {
            predicate: "source.literal",
            value: "org.platform.domain.system.type.instance",
            datatype: "xsd:string",
            evidence: [{ source: "ast", timestamp: "2026-07-19T12:00:00Z" }],
          },
          {
            predicate: "source.lines",
            value: [10, 14],
            evidence: [{ confidence: 0.9 }],
          },
        ],
      },
      { handle: "handle-b", facts: [] },
    ],
    edges: [
      {
        id: "handle-a|calls|handle-b",
        source: "handle-a",
        target: "handle-b",
        predicate: "calls",
        direction: "outgoing",
        evidence: [{ source: "ast", confidence: 0.8 }],
      },
      {
        id: "handle-a|imports|handle-b",
        source: "handle-a",
        target: "handle-b",
        predicate: "imports",
        direction: "outgoing",
        evidence: [{}],
      },
      {
        id: "handle-b|references|handle-a",
        source: "handle-b",
        target: "handle-a",
        predicate: "references",
        direction: "incoming",
      },
      {
        id: "handle-a|returns|missing-handle",
        source: "handle-a",
        target: "missing-handle",
        predicate: "returns",
        direction: "outgoing",
      },
    ],
    view_revision: { start: 41, end: 41, coherent: true },
    truncated: false,
  },
  truncated: false,
};
