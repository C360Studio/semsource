<script lang="ts">
  import type { Capability } from "$lib/contracts/capabilities";
  import type { FusionNode, FusionResponse } from "$lib/contracts/fusion";
  import {
    isSameOriginRelativeHref,
    searchCode,
    FusionSearchError,
    type CodeSearch,
  } from "$lib/api/search";

  const defaultSearch: CodeSearch = (href, query, errorContract, signal) =>
    searchCode(globalThis.fetch, href, query, errorContract, signal);

  let {
    capability,
    errorContract,
    search = defaultSearch,
  }: {
    capability: Capability | undefined;
    errorContract: string | undefined;
    search?: CodeSearch;
  } = $props();

  let query = $state("");
  let searchInput = $state<HTMLInputElement>();
  let response = $state<FusionResponse | null>(null);
  let selectedIndex = $state<number | null>(null);
  let loading = $state(false);
  let error = $state<{
    message: string;
    code?: string;
    retryable: boolean;
  } | null>(null);
  let controller: AbortController | null = null;
  let elapsedSeconds = $state(0);
  let elapsedTimer: ReturnType<typeof setInterval> | null = null;
  let generation = 0;

  const usable = $derived(
    capability?.availability === "ready" &&
      capability.method === "POST" &&
      isSameOriginRelativeHref(capability.href),
  );
  const advertisedRouteValid = $derived(
    capability?.method === "POST" && isSameOriginRelativeHref(capability.href),
  );
  const capabilityKey = $derived(
    `${JSON.stringify(capability ?? null)}:${errorContract ?? ""}`,
  );

  function stopElapsedTimer(): void {
    if (elapsedTimer !== null) {
      clearInterval(elapsedTimer);
      elapsedTimer = null;
    }
  }

  function cancelActive(): void {
    controller?.abort();
    controller = null;
    stopElapsedTimer();
    generation += 1;
    loading = false;
    elapsedSeconds = 0;
  }

  function resetResults(): void {
    response = null;
    selectedIndex = null;
    error = null;
  }

  $effect(() => {
    if (!capabilityKey) return;
    cancelActive();
    resetResults();
    return cancelActive;
  });

  function isAbort(cause: unknown): boolean {
    return (
      (cause instanceof DOMException && cause.name === "AbortError") ||
      (cause instanceof Error && cause.name === "AbortError")
    );
  }

  async function submit(): Promise<void> {
    const nextQuery = query.trim();
    cancelActive();
    resetResults();
    if (!nextQuery || !usable || !capability?.href) return;

    controller = new AbortController();
    const request = controller;
    const requestGeneration = ++generation;
    loading = true;
    elapsedSeconds = 0;
    elapsedTimer = setInterval(() => {
      elapsedSeconds += 1;
    }, 1000);
    try {
      const result = await search(
        capability.href,
        nextQuery,
        errorContract,
        request.signal,
      );
      if (request.signal.aborted || requestGeneration !== generation) return;
      response = result;
      selectedIndex = result.nodes.length > 0 ? 0 : null;
    } catch (cause) {
      if (
        request.signal.aborted ||
        requestGeneration !== generation ||
        isAbort(cause)
      ) {
        return;
      }
      error =
        cause instanceof FusionSearchError
          ? {
              message: cause.message,
              code: cause.code,
              retryable: cause.retryable,
            }
          : {
              message:
                cause instanceof Error
                  ? cause.message
                  : "Code search could not be completed",
              retryable: false,
            };
    } finally {
      if (requestGeneration === generation) {
        stopElapsedTimer();
        loading = false;
        controller = null;
      }
    }
  }

  function cancelSearch(): void {
    cancelActive();
    resetResults();
    searchInput?.focus();
  }

  function chooseSuggestion(suggestion: string): void {
    query = suggestion;
  }

  function nodeLocation(node: FusionNode): string | null {
    if (!node.path) return null;
    return node.lines
      ? `${node.path}:${node.lines[0]}-${node.lines[1]}`
      : node.path;
  }
</script>

<section class="search-panel" aria-labelledby="code-search-heading">
  <div class="section-heading">
    <div>
      <p class="eyebrow">Supplied source evidence</p>
      <h2 id="code-search-heading">Code search</h2>
    </div>
    {#if response}
      <span class="provenance">Provenance: {response.provenance}</span>
    {/if}
  </div>

  {#if !capability}
    <div class="truth-state" data-state="missing">
      <strong>Not advertised</strong>
      <p>This SemSource version has not advertised code search.</p>
    </div>
  {:else if capability.availability === "unsupported"}
    <div class="truth-state" data-state="unsupported">
      <strong>Unsupported</strong>
      <p>{capability.reason?.message ?? "Code search is not supported."}</p>
    </div>
  {:else if !advertisedRouteValid}
    <div class="truth-state" data-state="invalid">
      <strong>Unavailable</strong>
      <p>
        Invalid search contract: expected an advertised POST route on this
        origin.
      </p>
    </div>
  {:else if capability.availability === "not_ready"}
    <div class="truth-state" data-state="not_ready">
      <strong>Not ready</strong>
      <p>{capability.reason?.message ?? "Code search is not ready."}</p>
    </div>
  {:else}
    <form
      onsubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <label for="code-query">Search code</label>
      <div class="search-controls">
        <input
          bind:this={searchInput}
          id="code-query"
          type="search"
          bind:value={query}
          placeholder="Where is retry logic handled?"
          autocomplete="off"
        />
        <button type="submit">Search</button>
      </div>
    </form>

    <div class="search-state" aria-live="polite">
      {#if loading}
        <div class="active-search">
          <p class="idle" role="status">
            Searching… {elapsedSeconds}
            {elapsedSeconds === 1 ? "second" : "seconds"} elapsed
          </p>
          <button type="button" onclick={cancelSearch}>Cancel search</button>
        </div>
      {:else if error}
        <div class="truth-state error" role="alert">
          <strong>Search failed</strong>
          <p>{error.message}</p>
          {#if error.code}<p>Error code: {error.code}</p>{/if}
          {#if error.retryable}<button
              type="button"
              onclick={() => void submit()}>Retry search</button
            >{/if}
        </div>
      {:else if response && !response.index.ready}
        <div class="truth-state" data-state="not_ready">
          <strong>Index is {response.index.state}</strong>
          <p>
            Code search has not reported a ready index, so this response is not
            treated as empty.
            {#if response.index.lag !== undefined}
              Revision lag: {response.index.lag}.
            {/if}
          </p>
        </div>
      {:else if response}
        {#if response.truncated}
          <p class="warning" role="status">
            Results were truncated by the server; the list is not complete.
          </p>
        {/if}

        {#if response.nodes.length > 0}
          <div class="result-layout">
            <div>
              <h3>Results</h3>
              <ul class="result-list" aria-label="Code search results">
                {#each response.nodes as node, index (`${node.name}:${node.path ?? ""}:${index}`)}
                  <li>
                    <button
                      type="button"
                      class:selected={selectedIndex === index}
                      aria-current={selectedIndex === index
                        ? "true"
                        : undefined}
                      onclick={() => (selectedIndex = index)}
                    >
                      <strong>{node.name}</strong>
                      {#if nodeLocation(node)}
                        <span>{nodeLocation(node)}</span>
                      {/if}
                    </button>
                  </li>
                {/each}
              </ul>
            </div>

            {#if selectedIndex !== null && response.nodes[selectedIndex]}
              {@const selected = response.nodes[selectedIndex]}
              <article class="result-detail" aria-label="Result detail">
                <h3>{selected.name}</h3>
                {#if nodeLocation(selected)}
                  <p class="path">{nodeLocation(selected)}</p>
                {/if}
                <p class="detail-provenance">
                  Provenance: {response.provenance}
                </p>
                {#if selected.body}
                  <pre><code>{selected.body}</code></pre>
                {:else}
                  <p class="unknown">No source body was supplied.</p>
                {/if}
              </article>
            {/if}
          </div>
        {:else if response.misses.length > 0}
          <div class="misses">
            <h3>No exact result</h3>
            {#each response.misses as miss, missIndex (`${miss.query}:${missIndex}`)}
              <p>No result was supplied for <strong>{miss.query}</strong>.</p>
              {#if miss.did_you_mean?.length}
                <div class="suggestions" aria-label="Did you mean suggestions">
                  <span>Did you mean:</span>
                  {#each miss.did_you_mean as suggestion, suggestionIndex (`${suggestion}:${suggestionIndex}`)}
                    <button
                      type="button"
                      onclick={() => chooseSuggestion(suggestion)}
                    >
                      {suggestion}
                    </button>
                  {/each}
                </div>
              {/if}
            {/each}
          </div>
        {:else}
          <p class="empty">No code results were supplied for this query.</p>
        {/if}
      {:else if !loading}
        <p class="idle">
          Enter a code search query to inspect supplied source evidence.
        </p>
      {/if}
    </div>
  {/if}
</section>

<style>
  .search-panel {
    margin-bottom: 1rem;
    padding: 1.35rem;
    border: 1px solid var(--border);
    border-radius: 1rem;
    background: rgb(16 28 47 / 92%);
  }

  .section-heading {
    display: flex;
    gap: 1rem;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
  }

  h2,
  h3,
  p {
    margin-top: 0;
  }

  h2 {
    margin-bottom: 0;
    font-size: 1.45rem;
  }

  h3 {
    font-size: 1rem;
  }

  .eyebrow {
    margin-bottom: 0.45rem;
    color: var(--accent);
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.13em;
    text-transform: uppercase;
  }

  .provenance,
  .detail-provenance,
  .path,
  .idle,
  .empty,
  .unknown {
    color: var(--muted);
    font-size: 0.84rem;
  }

  form label {
    display: block;
    margin-bottom: 0.4rem;
    font-weight: 700;
  }

  .search-controls {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.65rem;
  }

  input,
  button {
    border: 1px solid var(--border);
    border-radius: 0.65rem;
    color: var(--ink);
    font: inherit;
  }

  input {
    min-width: 0;
    padding: 0.75rem 0.85rem;
    background: #091424;
  }

  button {
    padding: 0.7rem 0.9rem;
    background: rgb(99 214 199 / 12%);
    cursor: pointer;
  }

  button:disabled {
    cursor: wait;
    opacity: 0.7;
  }

  .search-state {
    margin-top: 1rem;
  }

  .active-search {
    display: flex;
    gap: 0.65rem;
    align-items: center;
  }

  .active-search .idle {
    flex: 1;
  }

  .truth-state,
  .warning,
  .misses,
  .empty,
  .idle {
    padding: 0.9rem 1rem;
    border-radius: 0.75rem;
    background: var(--surface-raised);
  }

  .truth-state p,
  .warning,
  .empty,
  .idle {
    margin-bottom: 0;
  }

  .truth-state[data-state="not_ready"],
  .warning {
    border: 1px solid color-mix(in srgb, var(--warning) 45%, var(--border));
  }

  .truth-state.error {
    border: 1px solid color-mix(in srgb, var(--danger) 55%, var(--border));
  }

  .result-layout {
    display: grid;
    grid-template-columns: minmax(15rem, 0.8fr) minmax(0, 1.5fr);
    gap: 1rem;
  }

  .result-list {
    display: grid;
    gap: 0.45rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .result-list button {
    display: grid;
    gap: 0.25rem;
    width: 100%;
    text-align: left;
    background: var(--surface-raised);
  }

  .result-list button.selected {
    border-color: var(--accent);
    background: rgb(99 214 199 / 15%);
  }

  .result-list span {
    overflow-wrap: anywhere;
    color: var(--muted);
    font-size: 0.78rem;
  }

  .result-detail {
    min-width: 0;
    padding: 1rem;
    border-radius: 0.75rem;
    background: #091424;
  }

  .path {
    overflow-wrap: anywhere;
  }

  pre {
    max-height: 28rem;
    margin-bottom: 0;
    padding: 0.85rem;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: 0.6rem;
    background: #050b14;
    white-space: pre-wrap;
  }

  .suggestions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: center;
  }

  .suggestions span {
    color: var(--muted);
  }

  @media (max-width: 700px) {
    .section-heading,
    .result-layout {
      align-items: stretch;
      grid-template-columns: 1fr;
    }

    .section-heading {
      display: grid;
    }

    .search-controls {
      grid-template-columns: 1fr;
    }
  }
</style>
