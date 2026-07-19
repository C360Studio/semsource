import { describe, expect, it, vi } from "vitest";
import { completeGraphResponse } from "../../tests/fixtures/graph";
import { queryGraph } from "./graph";

describe("queryGraph", () => {
  it("posts a graph-only fusion request to the advertised same-origin route", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(JSON.stringify(completeGraphResponse), { status: 200 }),
      );
    await queryGraph(fetcher, "/code-context/context", "Alpha", "1");
    expect(fetcher).toHaveBeenCalledWith(
      "/code-context/context",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ query: "Alpha", want: ["graph"] }),
      }),
    );
  });

  it("rejects a successful response that omits the requested graph", async () => {
    const withoutGraph = structuredClone(completeGraphResponse);
    delete (withoutGraph as { graph?: unknown }).graph;
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(JSON.stringify(withoutGraph), { status: 200 }),
      );
    await expect(
      queryGraph(fetcher, "/code-context/context", "Alpha", "1"),
    ).rejects.toMatchObject({
      code: "invalid_payload",
      retryable: false,
    });
  });

  it.each([
    {
      label: "not-ready",
      index: { ready: false, state: "building" },
      nodes: [],
      misses: [],
    },
    {
      label: "ready miss",
      index: { ready: true, state: "ready" },
      nodes: [],
      misses: [{ query: "Alpha", did_you_mean: ["Alfa"] }],
    },
    {
      label: "ready empty",
      index: { ready: true, state: "ready" },
      nodes: [],
      misses: [],
    },
  ])(
    "accepts a valid no-graph $label envelope",
    async ({ index, nodes, misses }) => {
      const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
        new Response(
          JSON.stringify({
            contract_version: "1",
            index,
            provenance: "deterministic",
            nodes,
            misses,
            truncated: false,
          }),
          { status: 200 },
        ),
      );
      await expect(
        queryGraph(fetcher, "/code-context/context", "Alpha", "1"),
      ).resolves.toMatchObject({ index, nodes, misses, graph: undefined });
    },
  );

  it.each([
    [400, "invalid_request", false],
    [503, "dependency_unavailable", true],
    [504, "upstream_timeout", true],
  ])(
    "honors the advertised fusion error contract for HTTP %i",
    async (status, code, retryable) => {
      const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: {
              contract_version: "1",
              code,
              class: retryable ? "transient" : "invalid",
              message: "Safe graph error",
              retryable,
            },
          }),
          { status },
        ),
      );
      await expect(
        queryGraph(fetcher, "/code-context/context", "Alpha", "1"),
      ).rejects.toMatchObject({
        status,
        code,
        message: "Safe graph error",
        retryable,
      });
    },
  );

  it("sanitizes a malformed graph error envelope", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response('{"error":{"message":"/private/path"}}', { status: 503 }),
      );
    await expect(
      queryGraph(fetcher, "/code-context/context", "Alpha", "1"),
    ).rejects.toMatchObject({
      code: "http_error",
      message: "Graph query failed with HTTP 503",
      retryable: true,
    });
  });
});
