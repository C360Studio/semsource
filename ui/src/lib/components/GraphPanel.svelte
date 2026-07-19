<script lang="ts">
  import type { Capability } from "$lib/contracts/capabilities";
  import { queryGraph, type GraphQuery } from "$lib/api/graph";
  import { FusionSearchError, isSameOriginRelativeHref } from "$lib/api/search";
  import {
    emptyGraph,
    syncGraph,
    type GraphSyncResult,
  } from "$lib/graph/model";
  import { groupEntities } from "$lib/graph/drilldown";
  import type { GraphRendererFactory } from "$lib/graph/renderer";
  import GraphView from "./GraphView.svelte";

  const defaultQuery: GraphQuery = (href, query, errorContract, signal) =>
    queryGraph(globalThis.fetch, href, query, errorContract, signal);

  let {
    capability,
    errorContract,
    graphContract,
    query = defaultQuery,
    rendererFactory,
  }: {
    capability: Capability | undefined;
    errorContract: string | undefined;
    graphContract: string | undefined;
    query?: GraphQuery;
    rendererFactory?: GraphRendererFactory;
  } = $props();

  let input = $state("");
  let loading = $state(false);
  let error = $state<{
    message: string;
    code?: string;
    retryable: boolean;
  } | null>(null);
  let result = $state<GraphSyncResult>({
    graph: emptyGraph(),
    mode: "complete",
  });
  let selectedHandle = $state<string | null>(null);
  let focusedHandles = $state<string[]>([]);
  let queryNotice = $state<string | null>(null);
  // The query behind the currently displayed result (drives queried-symbol
  // selection) and the query behind the last failed attempt (drives Retry —
  // D5), tracked separately from the live `input` so neither is at the mercy
  // of the user editing the box afterward.
  let resultQuery = $state("");
  let erroredQuery = $state<string | null>(null);
  let controller: AbortController | null = null;
  let generation = 0;
  const usable = $derived(
    capability?.availability === "ready" &&
      graphContract === "1" &&
      capability.method === "POST" &&
      isSameOriginRelativeHref(capability.href),
  );
  const resetKey = $derived(
    `${JSON.stringify(capability ?? null)}:${graphContract ?? ""}:${errorContract ?? ""}`,
  );
  const retryLabel = $derived(
    erroredQuery !== null && erroredQuery !== input.trim()
      ? `Retry graph query: "${erroredQuery}"`
      : "Retry graph query",
  );

  function cancel(): void {
    generation += 1;
    controller?.abort();
    controller = null;
    loading = false;
  }

  function resetForContract(resetKeyValue: string): void {
    if (!resetKeyValue) return;
    cancel();
    error = null;
    erroredQuery = null;
    queryNotice = null;
    result = { graph: emptyGraph(), mode: "complete" };
    resultQuery = "";
    selectedHandle = null;
    focusedHandles = [];
  }

  $effect(() => {
    resetForContract(resetKey);
    return cancel;
  });

  async function runQuery(graphQuery: string): Promise<void> {
    cancel();
    error = null;
    erroredQuery = null;
    queryNotice = null;
    if (!graphQuery || !usable || !capability?.href) return;
    const request = new AbortController();
    controller = request;
    const current = ++generation;
    loading = true;
    try {
      const response = await query(
        capability.href,
        graphQuery,
        errorContract,
        request.signal,
      );
      if (request.signal.aborted || current !== generation) return;
      if (!response.index.ready) {
        queryNotice = `Graph index is ${response.index.state.replaceAll("_", " ")}; existing graph state was retained. Retry when the index is ready.`;
        return;
      }
      if (!response.graph) {
        const suggestions = response.misses.flatMap(
          (miss) => miss.did_you_mean ?? [],
        );
        queryNotice = suggestions.length
          ? `No governed graph entities matched. Suggested queries: ${suggestions.join(", ")}.`
          : "No governed graph entities matched this query.";
        return;
      }
      result = syncGraph(result.graph, response.graph, response.nodes);
      resultQuery = graphQuery;
      focusedHandles = response.graph.nodes.map((node) => node.handle);
      if (
        !selectedHandle ||
        !result.graph.nodes.some((node) => node.handle === selectedHandle)
      )
        selectedHandle = groupEntities(
          result.graph.nodes,
          resultQuery,
        ).selectedHandle;
    } catch (cause) {
      if (request.signal.aborted || current !== generation) return;
      error =
        cause instanceof FusionSearchError
          ? {
              message: cause.message,
              code: cause.code,
              retryable: cause.retryable,
            }
          : {
              message: "Graph investigation could not be completed",
              retryable: false,
            };
      erroredQuery = graphQuery;
    } finally {
      if (current === generation) {
        loading = false;
        controller = null;
      }
    }
  }

  async function submit(): Promise<void> {
    await runQuery(input.trim());
  }

  async function retryErrored(): Promise<void> {
    if (erroredQuery !== null) await runQuery(erroredQuery);
  }
</script>

<section class="graph-panel" aria-labelledby="graph-heading">
  <div class="section-heading">
    <div>
      <p class="eyebrow">Investigation drill-down</p>
      <h2 id="graph-heading">Graph drill-down</h2>
    </div>
    {#if result.graph.revision}
      <span
        >Revision {result.graph.revision.start}-{result.graph.revision
          .end}</span
      >
    {/if}
  </div>

  {#if !capability}
    <div class="truth-state">
      <strong>Not advertised</strong>
      <p>Graph projection was not advertised by SemSource.</p>
    </div>
  {:else if capability.availability !== "ready"}
    <div class="truth-state" data-state={capability.availability}>
      <strong
        >{capability.availability === "not_ready"
          ? "Not ready"
          : "Unsupported"}</strong
      >
      <p>{capability.reason?.message ?? "Graph projection is unavailable."}</p>
    </div>
  {:else if graphContract !== "1"}
    <div class="truth-state">
      <strong>Incompatible graph contract</strong>
      <p>
        SemSource did not advertise fusion graph projection contract version 1.
      </p>
    </div>
  {:else if !usable}
    <div class="truth-state">
      <strong>Unavailable</strong>
      <p>
        Invalid graph contract: expected an advertised POST route on this
        origin.
      </p>
    </div>
  {:else}
    <form
      onsubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <label for="graph-query">Graph query</label>
      <div class="controls">
        <input
          id="graph-query"
          type="search"
          bind:value={input}
          placeholder="Investigate a symbol or source concept"
          autocomplete="off"
        />
        <button type="submit">Investigate graph</button>
        {#if loading}<button type="button" onclick={cancel}>Cancel</button>{/if}
      </div>
    </form>
    <div aria-live="polite">
      {#if loading}<p role="status">Loading governed graph projection…</p>{/if}
      {#if queryNotice}<p class="truth-state" role="status">
          {queryNotice}
        </p>{/if}
      {#if error}<div class="truth-state error" role="alert">
          <strong>Graph query failed</strong>
          <p>{error.message}</p>
          {#if error.code}<p>Error code: {error.code}</p>{/if}
          {#if error.retryable}<button
              type="button"
              onclick={() => void retryErrored()}>{retryLabel}</button
            >{/if}
        </div>{/if}
      {#if result.mode === "partial"}
        <p class="warning" role="status" aria-label="Partial graph projection">
          {result.reason}. Existing graph items were retained; retry for a
          complete coherent revision.
        </p>
      {/if}
    </div>
    {#if result.graph.nodes.length > 0}
      <GraphView
        graph={result.graph}
        {focusedHandles}
        bind:selectedHandle
        {rendererFactory}
        queriedName={resultQuery}
      />
    {/if}
  {/if}
</section>

<style>
  .graph-panel {
    margin: 1rem 0;
    padding: 1.35rem;
    border: 1px solid var(--border);
    border-radius: 1rem;
    background: rgb(16 28 47 / 92%);
  }
  .section-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1rem;
  }
  .section-heading h2,
  p {
    margin-top: 0;
  }
  .section-heading h2 {
    margin-bottom: 0;
    font-size: 1.45rem;
  }
  .section-heading span {
    color: var(--muted);
    font-size: 0.85rem;
  }
  .eyebrow {
    margin-bottom: 0.55rem;
    color: var(--accent);
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.13em;
    text-transform: uppercase;
  }
  form {
    margin-bottom: 1rem;
  }
  label {
    display: block;
    margin-bottom: 0.45rem;
    font-weight: 700;
  }
  .controls {
    display: flex;
    gap: 0.65rem;
  }
  input {
    flex: 1;
    min-width: 0;
    padding: 0.7rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: 0.6rem;
    color: var(--ink);
    background: #091524;
  }
  button {
    padding: 0.65rem 0.9rem;
    border: 1px solid color-mix(in srgb, var(--accent) 55%, var(--border));
    border-radius: 0.6rem;
    color: var(--ink);
    background: rgb(99 214 199 / 12%);
    font: inherit;
    font-weight: 700;
    cursor: pointer;
  }
  .truth-state,
  .warning {
    padding: 0.9rem 1rem;
    border-radius: 0.75rem;
    color: var(--muted);
    background: var(--surface-raised);
  }
  .truth-state p,
  .warning {
    margin-bottom: 0;
  }
  .warning {
    color: var(--warning);
  }
  .error {
    color: var(--danger);
  }
  @media (max-width: 760px) {
    .controls {
      flex-direction: column;
    }
  }
</style>
