import { render, screen, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { parseFusionResponse } from "$lib/contracts/fusion";
import { FusionSearchError } from "$lib/api/search";
import type {
  GraphRendererFactory,
  GraphRendererSession,
} from "$lib/graph/renderer";
import type { CanonicalGraph } from "$lib/graph/model";
import { completeGraphResponse } from "../../tests/fixtures/graph";
import GraphPanel from "./GraphPanel.svelte";

const ready = {
  availability: "ready" as const,
  method: "POST",
  href: "/code-context/context",
};

class FakeRenderer implements GraphRendererSession {
  setGraph = vi.fn<(graph: CanonicalGraph) => void>();
  setSelection = vi.fn();
  kill = vi.fn();
  constructor(readonly select: (handle: string) => void) {}
}

function rendererFactory(renderers: FakeRenderer[]): GraphRendererFactory {
  return async (_container, _graph, onSelect) => {
    const renderer = new FakeRenderer(onSelect);
    renderers.push(renderer);
    return renderer;
  };
}

async function submitGraph(): Promise<void> {
  await userEvent.type(
    screen.getByRole("searchbox", { name: /graph query/i }),
    "Alpha",
  );
  await userEvent.click(
    screen.getByRole("button", { name: /investigate graph/i }),
  );
}

describe("GraphPanel", () => {
  it("does not request or infer a graph while the capability is unavailable", () => {
    const query = vi.fn();
    render(GraphPanel, {
      capability: {
        availability: "unsupported",
        reason: {
          code: "upstream_contract_pending",
          message: "Projection unavailable",
          retryable: false,
        },
      },
      errorContract: "1",
      graphContract: undefined,
      query,
    });
    expect(
      screen.getByRole("region", { name: /graph drill-down/i }),
    ).toHaveTextContent("Projection unavailable");
    expect(query).not.toHaveBeenCalled();
  });

  it("does not use a ready route without the advertised graph contract", () => {
    const query = vi.fn();
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: undefined,
      query,
    });
    expect(screen.getByText("Incompatible graph contract")).toBeInTheDocument();
    expect(
      screen.queryByRole("searchbox", { name: /graph query/i }),
    ).not.toBeInTheDocument();
    expect(query).not.toHaveBeenCalled();
  });

  it("keeps Sigma, accessible navigator, and detail selection synchronized", async () => {
    const renderers: FakeRenderer[] = [];
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query: vi
        .fn()
        .mockResolvedValue(parseFusionResponse(completeGraphResponse)),
      rendererFactory: rendererFactory(renderers),
    });
    await submitGraph();
    const navigator = await screen.findByRole("list", {
      name: /graph entities/i,
    });
    expect(within(navigator).getAllByRole("button")).toHaveLength(3);
    expect(
      within(navigator).getByRole("button", { name: /^Alpha/ }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      screen.getByRole("region", { name: /selected entity details/i }),
    ).toHaveTextContent("org.platform.domain.system.type.instance");
    await vi.waitFor(() => expect(renderers).toHaveLength(1));
    expect(renderers[0]?.setGraph.mock.calls.at(-1)?.[0].edges).toHaveLength(4);

    const betaButton = within(navigator).getByRole("button", { name: /^Beta/ });
    betaButton.focus();
    await userEvent.keyboard("{Enter}");
    expect(
      screen.getByRole("region", { name: /selected entity details/i }),
    ).toHaveTextContent("Beta");
    expect(betaButton).toHaveAttribute("aria-pressed", "true");
    renderers[0]?.select("handle-a");
    await vi.waitFor(() =>
      expect(
        within(navigator).getByRole("button", { name: /^Alpha/ }),
      ).toHaveAttribute("aria-pressed", "true"),
    );
  });

  it("retains prior items and visibly asks for retry on a partial revision", async () => {
    const exact = parseFusionResponse(completeGraphResponse);
    const partialValue = structuredClone(completeGraphResponse);
    partialValue.nodes = [partialValue.nodes[0]];
    partialValue.graph.nodes = [partialValue.graph.nodes[0]];
    partialValue.graph.edges = [];
    partialValue.graph.view_revision = { start: 41, end: 42, coherent: false };
    const query = vi
      .fn()
      .mockResolvedValueOnce(exact)
      .mockResolvedValueOnce(parseFusionResponse(partialValue));
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query,
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    await screen.findByRole("list", { name: /graph entities/i });
    await userEvent.click(
      screen.getByRole("button", { name: /investigate graph/i }),
    );
    expect(
      await screen.findByRole("status", { name: /partial graph projection/i }),
    ).toHaveTextContent(/retained.*retry/i);
    expect(
      within(screen.getByRole("list", { name: /graph entities/i })).getByRole(
        "button",
        { name: /^Beta/ },
      ),
    ).toBeInTheDocument();
  });

  it("preserves a retryable classified graph error and retries", async () => {
    const query = vi
      .fn()
      .mockRejectedValueOnce(
        new FusionSearchError(
          "Graph index timed out",
          "upstream_timeout",
          true,
          504,
        ),
      )
      .mockResolvedValueOnce(parseFusionResponse(completeGraphResponse));
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query,
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Graph index timed out");
    expect(alert).toHaveTextContent("upstream_timeout");
    await userEvent.click(
      screen.getByRole("button", { name: /retry graph query/i }),
    );
    expect(
      await screen.findByRole("list", { name: /graph entities/i }),
    ).toBeInTheDocument();
    expect(query).toHaveBeenCalledTimes(2);
  });

  it("retries the last errored query after the input is cleared, labeling the retry when it differs", async () => {
    const query = vi
      .fn()
      .mockRejectedValueOnce(
        new FusionSearchError(
          "Graph index timed out",
          "upstream_timeout",
          true,
          504,
        ),
      )
      .mockResolvedValueOnce(parseFusionResponse(completeGraphResponse));
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query,
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    await screen.findByRole("alert");
    await userEvent.clear(
      screen.getByRole("searchbox", { name: /graph query/i }),
    );
    const retryButton = await screen.findByRole("button", {
      name: /retry graph query.*alpha/i,
    });
    await userEvent.click(retryButton);
    expect(query).toHaveBeenCalledTimes(2);
    expect(query.mock.calls[1]?.[1]).toBe("Alpha");
    expect(
      await screen.findByRole("list", { name: /graph entities/i }),
    ).toBeInTheDocument();
  });

  it("does not offer retry for a nonretryable classified graph error", async () => {
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query: vi
        .fn()
        .mockRejectedValue(
          new FusionSearchError("Bad graph query", "invalid_query", false, 400),
        ),
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Bad graph query");
    expect(alert).toHaveTextContent("invalid_query");
    expect(
      screen.queryByRole("button", { name: /retry graph query/i }),
    ).not.toBeInTheDocument();
  });

  it.each([
    { graphContract: "2", errorContract: "1", changed: "graph" },
    { graphContract: "1", errorContract: "2", changed: "error" },
  ])(
    "aborts, resets, and suppresses retained state when the $changed contract changes",
    async (nextContracts) => {
      let resolvePending:
        ((value: ReturnType<typeof parseFusionResponse>) => void) | undefined;
      let pendingSignal: AbortSignal | undefined;
      const query = vi
        .fn()
        .mockResolvedValueOnce(parseFusionResponse(completeGraphResponse))
        .mockImplementationOnce(
          (
            _href: string,
            _query: string,
            _contract: string | undefined,
            signal: AbortSignal,
          ) => {
            pendingSignal = signal;
            return new Promise<ReturnType<typeof parseFusionResponse>>(
              (resolve) => {
                resolvePending = resolve;
              },
            );
          },
        );
      const view = render(GraphPanel, {
        capability: ready,
        errorContract: "1",
        graphContract: "1",
        query,
        rendererFactory: rendererFactory([]),
      });
      await submitGraph();
      await screen.findByRole("list", { name: /graph entities/i });
      await userEvent.click(
        screen.getByRole("button", { name: /investigate graph/i }),
      );
      expect(pendingSignal?.aborted).toBe(false);

      await view.rerender({
        capability: ready,
        errorContract: nextContracts.errorContract,
        graphContract: nextContracts.graphContract,
        query,
        rendererFactory: rendererFactory([]),
      });
      expect(pendingSignal?.aborted).toBe(true);
      expect(
        screen.queryByRole("list", { name: /graph entities/i }),
      ).not.toBeInTheDocument();

      resolvePending?.(parseFusionResponse(completeGraphResponse));
      await Promise.resolve();
      expect(
        screen.queryByRole("list", { name: /graph entities/i }),
      ).not.toBeInTheDocument();
    },
  );

  it.each([
    [
      {
        contract_version: "1",
        index: { ready: false, state: "building" },
        provenance: "deterministic",
        nodes: [],
        truncated: false,
      },
      /index is building.*retained.*retry/i,
    ],
    [
      {
        contract_version: "1",
        index: { ready: true, state: "ready" },
        provenance: "deterministic",
        nodes: [],
        misses: [{ query: "Alpha", did_you_mean: ["Alfa"] }],
        truncated: false,
      },
      /no governed graph entities.*Alfa/i,
    ],
  ])(
    "renders a valid no-graph fusion envelope truthfully",
    async (value, message) => {
      render(GraphPanel, {
        capability: ready,
        errorContract: "1",
        graphContract: "1",
        query: vi.fn().mockResolvedValue(parseFusionResponse(value)),
        rendererFactory: rendererFactory([]),
      });
      await submitGraph();
      expect(await screen.findByRole("status")).toHaveTextContent(message);
      expect(
        screen.queryByRole("list", { name: /graph entities/i }),
      ).not.toBeInTheDocument();
    },
  );

  it("renders node, fact, and edge truncation evidence", async () => {
    const value = {
      ...completeGraphResponse,
      graph: {
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
        edges: completeGraphResponse.graph.edges.map((edge, edgeIndex) =>
          edgeIndex === 0 ? { ...edge, truncated: true } : edge,
        ),
      },
    };
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query: vi.fn().mockResolvedValue(parseFusionResponse(value)),
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    const detail = await screen.findByRole("region", {
      name: /selected entity details/i,
    });
    expect(detail).toHaveTextContent("3 facts were omitted");
    expect(detail).toHaveTextContent("Fact evidence was truncated");
    expect(detail).toHaveTextContent("Relationship evidence was truncated");
  });

  it("shows every explicit unresolved opaque handle verbatim", async () => {
    const value = {
      ...completeGraphResponse,
      graph: {
        ...completeGraphResponse.graph,
        edges: [
          ...completeGraphResponse.graph.edges,
          {
            id: "handle-b|calls|other-missing-handle",
            source: "handle-b",
            target: "other-missing-handle",
            predicate: "calls",
            direction: "outgoing",
          },
        ],
      },
    };
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query: vi.fn().mockResolvedValue(parseFusionResponse(value)),
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    const navigator = await screen.findByRole("list", {
      name: /graph entities/i,
    });
    expect(
      within(navigator).getByText("Unresolved: missing-handle"),
    ).toBeVisible();
    expect(
      within(navigator).getByText("Unresolved: other-missing-handle"),
    ).toBeVisible();
  });

  it("groups unresolved endpoints below resolved entities, labels builtin/external markers, and selects the queried symbol", async () => {
    const value = {
      ...completeGraphResponse,
      graph: {
        ...completeGraphResponse.graph,
        edges: [
          ...completeGraphResponse.graph.edges,
          {
            id: "handle-b|calls|builtin:len",
            source: "handle-b",
            target: "builtin:len",
            predicate: "calls",
            direction: "outgoing",
          },
        ],
      },
    };
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query: vi.fn().mockResolvedValue(parseFusionResponse(value)),
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    const navigator = await screen.findByRole("list", {
      name: /graph entities/i,
    });
    expect(
      within(navigator).getByText("Resolved entities (2)"),
    ).toBeInTheDocument();
    expect(
      within(navigator).getByText("Unresolved endpoints (2)"),
    ).toBeInTheDocument();
    expect(within(navigator).getByText("Builtin: len")).toBeInTheDocument();
    const buttons = within(navigator).getAllByRole("button");
    // Within each group, order is the canonical graph's stable handle sort
    // ("builtin:len" < "missing-handle"), not insertion order.
    expect(buttons.map((button) => button.textContent)).toEqual([
      expect.stringContaining("Alpha"),
      expect.stringContaining("Beta"),
      expect.stringContaining("Builtin: len"),
      expect.stringContaining("Unresolved: missing-handle"),
    ]);
    expect(buttons[0]).toHaveAttribute("aria-pressed", "true");
    expect(buttons[2]).toHaveAttribute("aria-pressed", "false");
    expect(buttons[3]).toHaveAttribute("aria-pressed", "false");
  });

  it("never auto-selects an unresolved endpoint when no resolved node is present", async () => {
    const value = {
      contract_version: "1",
      index: { ready: true, state: "ready" },
      provenance: "deterministic",
      nodes: [],
      graph: {
        nodes: [],
        edges: [
          {
            id: "handle-a|calls|builtin:len",
            source: "handle-a",
            target: "builtin:len",
            predicate: "calls",
            direction: "outgoing",
          },
        ],
        view_revision: { start: 1, end: 1, coherent: true },
        truncated: false,
      },
      truncated: false,
    };
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query: vi.fn().mockResolvedValue(parseFusionResponse(value)),
      rendererFactory: rendererFactory([]),
    });
    await submitGraph();
    const navigator = await screen.findByRole("list", {
      name: /graph entities/i,
    });
    for (const button of within(navigator).getAllByRole("button"))
      expect(button).toHaveAttribute("aria-pressed", "false");
    expect(
      screen.getByRole("region", { name: /selected entity details/i }),
    ).toHaveTextContent(
      "Select an entity to inspect its supplied facts and relationships.",
    );
  });

  it("reports renderer initialization failure while preserving the accessible surface", async () => {
    render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query: vi
        .fn()
        .mockResolvedValue(parseFusionResponse(completeGraphResponse)),
      rendererFactory: async () => {
        throw new Error("WebGL blocked");
      },
    });
    await submitGraph();
    expect(
      await screen.findByRole("alert", { name: /graph visualization/i }),
    ).toHaveTextContent(/visualization unavailable/i);
    expect(
      screen.getByRole("list", { name: /graph entities/i }),
    ).toBeInTheDocument();
  });

  it("refreshes the renderer for revision changes and kills it on unmount", async () => {
    const secondValue = structuredClone(completeGraphResponse);
    secondValue.graph.view_revision = { start: 42, end: 42, coherent: true };
    const query = vi
      .fn()
      .mockResolvedValueOnce(parseFusionResponse(completeGraphResponse))
      .mockResolvedValueOnce(parseFusionResponse(secondValue));
    const renderers: FakeRenderer[] = [];
    const view = render(GraphPanel, {
      capability: ready,
      errorContract: "1",
      graphContract: "1",
      query,
      rendererFactory: rendererFactory(renderers),
    });
    await submitGraph();
    await vi.waitFor(() => expect(renderers).toHaveLength(1));
    await userEvent.click(
      screen.getByRole("button", { name: /investigate graph/i }),
    );
    await vi.waitFor(() =>
      expect(renderers[0]?.setGraph).toHaveBeenCalledTimes(2),
    );
    view.unmount();
    expect(renderers[0]?.kill).toHaveBeenCalledOnce();
  });
});
