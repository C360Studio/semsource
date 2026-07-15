import { describe, expect, it } from "vitest";
import { parseProjectSummary } from "./projectSummary";

const summary = {
  namespace: "acme",
  phase: "ready",
  entity_id_format: "org.platform.domain.entity_type.source_id.entity_id",
  total_entities: 42,
  domains: [
    {
      domain: "code",
      entity_count: 42,
      types: [{ type: "code.symbol", count: 40 }],
      sources: ["ast-source-repo"],
    },
  ],
  predicates: [
    {
      source_type: "ast",
      predicates: [
        {
          name: "code.name",
          description: "Symbol name",
          data_type: "string",
          role: "identity",
        },
      ],
    },
  ],
  timestamp: "2026-07-15T12:00:00Z",
};

describe("parseProjectSummary", () => {
  it("parses the exact live summary and accepts null empty slices", () => {
    expect(parseProjectSummary(summary)).toMatchObject({ total_entities: 42 });
    expect(
      parseProjectSummary({ ...summary, domains: null, predicates: null }),
    ).toMatchObject({ domains: [], predicates: [] });
  });

  it.each([
    [{ ...summary, phase: "paused" }, /phase/i],
    [{ ...summary, total_entities: -1 }, /total_entities/i],
    [{ ...summary, timestamp: "yesterday" }, /timestamp/i],
    [
      {
        ...summary,
        predicates: [
          {
            source_type: "ast",
            predicates: [
              {
                name: "x",
                description: "x",
                data_type: "string",
                role: "authority",
              },
            ],
          },
        ],
      },
      /role/i,
    ],
  ])("rejects malformed summaries", (value, message) => {
    expect(() => parseProjectSummary(value)).toThrow(message);
  });
});
