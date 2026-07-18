import { fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Capability } from "$lib/contracts/capabilities";
import type { FusionResponse } from "$lib/contracts/fusion";
import { FusionSearchError } from "$lib/api/search";
import SearchPanel from "./SearchPanel.svelte";

const ready: Capability = {
  availability: "ready",
  method: "POST",
  href: "/code-context/search",
};

function result(overrides: Partial<FusionResponse> = {}): FusionResponse {
  return {
    contract_version: "1",
    index: { ready: true, state: "ready" },
    provenance: "embedding",
    nodes: [
      {
        name: "loadWorkbench",
        kind: "function",
        path: "ui/src/lib/api/workbench.ts",
        lines: [21, 63],
        body: "export async function loadWorkbench() {}",
      },
      { name: "refresh", path: "ui/src/routes/+page.svelte" },
    ],
    misses: [],
    truncated: false,
    ...overrides,
  };
}

describe("SearchPanel", () => {
  it("shows missing, unsupported, not-ready, and invalid advertised states without probing", async () => {
    const search = vi.fn();
    const view = render(SearchPanel, {
      capability: undefined,
      errorContract: "1",
      search,
    });
    expect(screen.getByText("Not advertised")).toBeInTheDocument();

    await view.rerender({
      capability: {
        availability: "unsupported",
        reason: {
          code: "not_implemented",
          message: "Search is unavailable in this deployment",
          retryable: false,
        },
      },
      errorContract: "1",
      search,
    });
    expect(
      screen.getByText("Search is unavailable in this deployment"),
    ).toBeInTheDocument();

    await view.rerender({
      capability: {
        availability: "not_ready",
        method: "POST",
        href: "/code-context/search",
        reason: {
          code: "semantic_index_not_ready",
          message: "Semantic index is still building",
          retryable: true,
        },
      },
      errorContract: "1",
      search,
    });
    expect(
      screen.getByText("Semantic index is still building"),
    ).toBeInTheDocument();

    await view.rerender({
      capability: { availability: "ready", method: "GET", href: "/search" },
      errorContract: "1",
      search,
    });
    expect(screen.getByText(/invalid search contract/i)).toBeInTheDocument();
    expect(search).not.toHaveBeenCalled();
  });

  it("renders supplied results, provenance, truncation, and accessible list/detail selection", async () => {
    const search = vi.fn().mockResolvedValue(result({ truncated: true }));
    render(SearchPanel, { capability: ready, errorContract: "1", search });
    const user = userEvent.setup();
    await user.type(
      screen.getByLabelText(/search code/i),
      "workbench bootstrap",
    );
    await user.click(screen.getByRole("button", { name: "Search" }));

    expect(search).toHaveBeenCalledWith(
      "/code-context/search",
      "workbench bootstrap",
      "1",
      expect.any(AbortSignal),
    );
    expect(
      await screen.findByText(/results were truncated/i),
    ).toBeInTheDocument();
    expect(screen.getAllByText(/provenance: embedding/i)).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: /refresh/i }));
    expect(
      screen.getByRole("article", { name: /result detail/i }),
    ).toHaveTextContent("ui/src/routes/+page.svelte");
    expect(screen.queryByText("opaque-do-not-address")).not.toBeInTheDocument();
  });

  it("distinguishes an unready response from a ready empty response and renders misses", async () => {
    const search = vi
      .fn()
      .mockResolvedValueOnce(
        result({
          index: { ready: false, state: "building", lag: 7 },
          nodes: [],
        }),
      )
      .mockResolvedValueOnce(
        result({
          nodes: [],
          misses: [{ query: "rettry", did_you_mean: ["retry", "backoff"] }],
        }),
      )
      .mockResolvedValueOnce(result({ nodes: [], misses: [] }));
    render(SearchPanel, { capability: ready, errorContract: "1", search });
    const user = userEvent.setup();
    const input = screen.getByLabelText(/search code/i);

    await user.type(input, "first");
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByText(/index is building/i)).toBeInTheDocument();
    expect(screen.queryByText(/no code results/i)).not.toBeInTheDocument();

    await user.clear(input);
    await user.type(input, "rettry");
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByText(/did you mean/i)).toBeInTheDocument();
    expect(screen.getByText("retry")).toBeInTheDocument();

    await user.clear(input);
    await user.type(input, "nothing");
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByText(/no code results/i)).toBeInTheDocument();
  });

  it("keeps blank queries idle", async () => {
    const search = vi.fn();
    render(SearchPanel, { capability: ready, errorContract: "1", search });
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    expect(search).not.toHaveBeenCalled();
    expect(screen.getByText(/enter a code search query/i)).toBeInTheDocument();
  });

  it("renders sanitized search errors without stale result detail", async () => {
    const search = vi
      .fn()
      .mockResolvedValueOnce(result())
      .mockRejectedValueOnce(new Error("Safe server message"));
    render(SearchPanel, { capability: ready, errorContract: "1", search });
    const user = userEvent.setup();
    const input = screen.getByLabelText(/search code/i);
    await user.type(input, "first");
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(
      await screen.findByRole("article", { name: /result detail/i }),
    ).toBeInTheDocument();

    await user.clear(input);
    await user.type(input, "second");
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Safe server message",
    );
    expect(
      screen.queryByRole("article", { name: /result detail/i }),
    ).not.toBeInTheDocument();
  });

  it("preserves classified transient search error code and retries", async () => {
    const search = vi
      .fn()
      .mockRejectedValueOnce(
        new FusionSearchError("Index timed out", "upstream_timeout", true, 504),
      )
      .mockResolvedValueOnce(result({ nodes: [{ name: "Recovered" }] }));
    render(SearchPanel, { capability: ready, errorContract: "1", search });
    const user = userEvent.setup();
    await user.type(screen.getByLabelText(/search code/i), "retry");
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "upstream_timeout",
    );
    await user.click(screen.getByRole("button", { name: /retry search/i }));
    expect(await screen.findAllByText("Recovered")).not.toHaveLength(0);
  });

  it("aborts predecessors and suppresses stale or aborted completions", async () => {
    const pending: Array<{
      resolve: (value: FusionResponse) => void;
      signal: AbortSignal;
    }> = [];
    const search = vi.fn(
      (
        _href: string,
        _query: string,
        _contract: string | undefined,
        signal: AbortSignal,
      ) =>
        new Promise<FusionResponse>((resolve) =>
          pending.push({ resolve, signal }),
        ),
    );
    render(SearchPanel, { capability: ready, errorContract: "1", search });
    const user = userEvent.setup();
    const input = screen.getByLabelText(/search code/i);

    await user.type(input, "old");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await user.clear(input);
    await user.type(input, "new");
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(pending[0]?.signal.aborted).toBe(true);

    pending[1]?.resolve(result({ nodes: [{ name: "New result" }] }));
    await screen.findAllByText("New result");
    pending[0]?.resolve(result({ nodes: [{ name: "Old result" }] }));
    await waitFor(() =>
      expect(screen.queryByText("Old result")).not.toBeInTheDocument(),
    );
  });

  it("aborts on capability change and resets selected detail for a new search", async () => {
    let resolveFirst!: (value: FusionResponse) => void;
    let firstSignal!: AbortSignal;
    const search = vi
      .fn()
      .mockImplementationOnce(
        (
          _href: string,
          _query: string,
          _contract: string | undefined,
          signal: AbortSignal,
        ) => {
          firstSignal = signal;
          return new Promise<FusionResponse>(
            (resolve) => (resolveFirst = resolve),
          );
        },
      )
      .mockResolvedValueOnce(result({ nodes: [{ name: "Only result" }] }));
    const view = render(SearchPanel, {
      capability: ready,
      errorContract: "1",
      search,
    });
    const user = userEvent.setup();
    const input = screen.getByLabelText(/search code/i);
    await user.type(input, "old");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await view.rerender({
      capability: { availability: "unsupported" },
      errorContract: "1",
      search,
    });
    expect(firstSignal.aborted).toBe(true);
    resolveFirst(result());
  });

  it("aborts an active request when the panel is torn down", async () => {
    let activeSignal!: AbortSignal;
    const search = vi.fn(
      (
        _href: string,
        _query: string,
        _contract: string | undefined,
        signal: AbortSignal,
      ) => {
        activeSignal = signal;
        return new Promise<FusionResponse>(() => undefined);
      },
    );
    const view = render(SearchPanel, {
      capability: ready,
      errorContract: "1",
      search,
    });
    await userEvent.type(screen.getByLabelText(/search code/i), "active");
    await userEvent.click(screen.getByRole("button", { name: "Search" }));
    view.unmount();
    expect(activeSignal.aborted).toBe(true);
  });

  it("shows elapsed active state and cancels explicitly back to idle", async () => {
    vi.useFakeTimers();
    let activeSignal!: AbortSignal;
    const search = vi.fn(
      (
        _href: string,
        _query: string,
        _contract: string | undefined,
        signal: AbortSignal,
      ) => {
        activeSignal = signal;
        return new Promise<FusionResponse>(() => undefined);
      },
    );
    render(SearchPanel, { capability: ready, errorContract: "1", search });
    await fireEvent.input(screen.getByLabelText(/search code/i), {
      target: { value: "active" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Search" }));
    expect(screen.getByRole("button", { name: "Cancel search" })).toBeVisible();
    await vi.advanceTimersByTimeAsync(2100);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Searching… 2 seconds elapsed",
    );
    await fireEvent.click(
      screen.getByRole("button", { name: "Cancel search" }),
    );
    expect(activeSignal.aborted).toBe(true);
    expect(screen.getByLabelText(/search code/i)).toHaveFocus();
    expect(screen.getByText(/enter a code search query/i)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Cancel search" }),
    ).not.toBeInTheDocument();
    vi.useRealTimers();
  });

  it("clears the elapsed timer when torn down", async () => {
    vi.useFakeTimers();
    const clearIntervalSpy = vi.spyOn(globalThis, "clearInterval");
    const view = render(SearchPanel, {
      capability: ready,
      errorContract: "1",
      search: vi.fn(() => new Promise<FusionResponse>(() => undefined)),
    });
    await fireEvent.input(screen.getByLabelText(/search code/i), {
      target: { value: "active" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Search" }));
    view.unmount();
    expect(clearIntervalSpy).toHaveBeenCalled();
    clearIntervalSpy.mockRestore();
    vi.useRealTimers();
  });
});
