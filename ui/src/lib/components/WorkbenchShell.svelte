<script lang="ts">
  import type { InventoryState, ProjectSummaryState } from "$lib/api/workbench";
  import type { CodeSearch } from "$lib/api/search";
  import type { GraphQuery } from "$lib/api/graph";
  import type { GraphRendererFactory } from "$lib/graph/renderer";
  import type { WorkbenchCapabilities } from "$lib/contracts/capabilities";
  import type { ManifestSource } from "$lib/contracts/sourceManifest";
  import CapabilityCard from "./CapabilityCard.svelte";
  import SearchPanel from "./SearchPanel.svelte";
  import GraphPanel from "./GraphPanel.svelte";

  let {
    capabilities,
    inventory,
    projectSummary,
    loading,
    error,
    onRetry,
    search,
    graphQuery,
    graphRendererFactory,
  }: {
    capabilities: WorkbenchCapabilities | null;
    inventory: InventoryState | null;
    projectSummary: ProjectSummaryState | null;
    loading: boolean;
    error: string | { message: string; kind: string } | null;
    onRetry: () => void;
    search?: CodeSearch;
    graphQuery?: GraphQuery;
    graphRendererFactory?: GraphRendererFactory;
  } = $props();

  const overall = $derived(capabilities?.readiness.overall ?? "unknown");
  const errorMessage = $derived(
    typeof error === "string" ? error : error?.message,
  );
  const incompatible = $derived(
    typeof error === "object" && error?.kind === "incompatible_version",
  );
  const manifest = $derived(
    inventory?.status === "ready" || inventory?.status === "empty"
      ? inventory.data
      : null,
  );
  const summary = $derived(
    projectSummary?.status === "ready" || projectSummary?.status === "empty"
      ? projectSummary.data
      : null,
  );

  function label(value: string): string {
    return value
      .replaceAll("_", " ")
      .replace(/^./, (first) => first.toUpperCase());
  }

  function sourceLocation(source: ManifestSource): string {
    return (
      source.path ??
      source.paths?.join(", ") ??
      source.url ??
      source.urls?.join(", ") ??
      "Configured source"
    );
  }

  function stateLabel(status: string): string {
    return status === "not_ready" ? "Not ready" : label(status);
  }
</script>

<svelte:head>
  <title>SemSource Workbench</title>
  <meta
    name="description"
    content="Optional SemSource workbench for source readiness, inventory, search, and evidence"
  />
</svelte:head>

<main>
  <header>
    <div>
      <p class="eyebrow">Source knowledge workbench</p>
      <h1>{capabilities?.product.name ?? "SemSource"}</h1>
      <p class="subtitle">
        Project <strong>{capabilities?.project.key ?? "connecting"}</strong>
      </p>
    </div>
    {#if capabilities}
      <div
        class="overall"
        role="status"
        aria-label="Overall readiness"
        data-state={overall}
      >
        <span>Overall readiness</span>
        <strong>{label(overall)}</strong>
      </div>
    {/if}
  </header>

  {#if loading}
    <section class="notice" aria-live="polite">
      Connecting to SemSource…
    </section>
  {:else if error}
    <section class="notice error" role="alert">
      <h2>
        {incompatible
          ? "Incompatible workbench contract"
          : "Workbench disconnected"}
      </h2>
      <p>{errorMessage}</p>
      <p>
        Headless SemSource may still be available through its HTTP and MCP
        contracts.
      </p>
      {#if !incompatible}<button type="button" onclick={onRetry}
          >Retry connection</button
        >{/if}
    </section>
  {:else if capabilities}
    <section class="readiness" aria-labelledby="readiness-heading">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Authoritative state</p>
          <h2 id="readiness-heading">Readiness</h2>
        </div>
        <span>
          {capabilities.readiness.source.total_entities === undefined
            ? "Entity count unavailable"
            : `${capabilities.readiness.source.total_entities} entities`}
        </span>
      </div>
      <div class="signal-grid">
        <article>
          <span>Sources</span>
          <strong>{label(capabilities.readiness.source.state)}</strong>
          {#if capabilities.readiness.source.source_count !== undefined}
            <small
              >{capabilities.readiness.source.source_count} configured sources</small
            >
          {/if}
          {#if capabilities.readiness.source.timestamp}
            <small
              >Status snapshot {capabilities.readiness.source.timestamp}</small
            >
          {/if}
        </article>
        <article>
          <span>Structural index</span>
          <strong>{label(capabilities.readiness.structural_index.state)}</strong
          >
          {#if capabilities.readiness.structural_index.lag !== undefined}
            <small
              >Revision lag {capabilities.readiness.structural_index.lag}</small
            >
          {/if}
          {#if capabilities.readiness.structural_index.last_synced}
            <small
              >Last index progress {capabilities.readiness.structural_index
                .last_synced}</small
            >
          {/if}
        </article>
        <article>
          <span>Semantic index</span>
          <strong>{label(capabilities.readiness.semantic_index.state)}</strong>
        </article>
      </div>
    </section>

    <section class="sources" aria-labelledby="sources-heading">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Configured knowledge</p>
          <h2 id="sources-heading">Sources</h2>
        </div>
        {#if manifest}
          <span>{manifest.sources.length} configured</span>
        {/if}
      </div>
      {#if inventory?.status === "ready"}
        <ul>
          {#each inventory.data.sources as source (`${source.type}:${sourceLocation(source)}`)}
            <li>
              <div>
                <strong>{label(source.type)}</strong>
                <span>{sourceLocation(source)}</span>
              </div>
              <span class="source-mode"
                >{source.watch ? "Watching" : "Watch disabled"}</span
              >
            </li>
          {/each}
        </ul>
      {:else if inventory?.status === "empty"}
        <p class="empty">No sources are currently advertised.</p>
      {:else if inventory?.status === "loading"}
        <div class="inventory-state" data-state="loading" role="status">
          <p>Loading source inventory…</p>
        </div>
      {:else if inventory}
        <div class="inventory-state" data-state={inventory.status}>
          <strong>{stateLabel(inventory.status)}</strong>
          <p>{inventory.message}</p>
          {#if (inventory.status === "error" || inventory.status === "not_ready") && inventory.retryable}
            <button type="button" onclick={onRetry}>Retry inventory</button>
          {/if}
        </div>
      {/if}
      {#if manifest}
        <p class="freshness">Inventory updated {manifest.timestamp}</p>
      {/if}
    </section>

    <section class="sources" aria-labelledby="summary-heading">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Materialized project view</p>
          <h2 id="summary-heading">Project summary</h2>
        </div>
        {#if summary}<span>{summary.total_entities} entities</span>{/if}
      </div>
      {#if projectSummary?.status === "loading"}
        <div class="inventory-state" role="status">
          <p>Loading project summary…</p>
        </div>
      {:else if projectSummary?.status === "ready" || projectSummary?.status === "empty"}
        <div class="signal-grid">
          <article>
            <span>Phase</span><strong>{label(projectSummary.data.phase)}</strong
            >
          </article>
          <article>
            <span>Domains</span><strong
              >{projectSummary.data.domains.length}</strong
            >
          </article>
          <article>
            <span>Entity ID format</span><strong
              >{projectSummary.data.entity_id_format}</strong
            >
          </article>
        </div>
        {#if projectSummary.data.domains.length > 0}
          <ul aria-label="Project domains">
            {#each projectSummary.data.domains as domain (domain.domain)}
              <li>
                <div>
                  <strong>{label(domain.domain)}</strong>
                  <span
                    >{domain.types
                      .map((type) => `${type.type} (${type.count})`)
                      .join(", ") || "No entity types advertised"}</span
                  >
                  <small>{domain.sources.length} contributing sources</small>
                </div>
                <span class="source-mode">{domain.entity_count} entities</span>
              </li>
            {/each}
          </ul>
        {/if}
        {#if projectSummary.status === "empty"}<p class="empty">
            The project summary is ready and contains no entities.
          </p>{/if}
        <p class="freshness">
          Summary generated {projectSummary.data.timestamp}
        </p>
      {:else if projectSummary}
        <div class="inventory-state" data-state={projectSummary.status}>
          <strong>{stateLabel(projectSummary.status)}</strong>
          <p>{projectSummary.message}</p>
          {#if (projectSummary.status === "error" || projectSummary.status === "not_ready") && projectSummary.retryable}
            <button type="button" onclick={onRetry}
              >Retry project summary</button
            >
          {/if}
        </div>
      {/if}
    </section>

    <section class="capability-grid" aria-label="Workbench capabilities">
      {#if capabilities.project_views}<CapabilityCard
          title="Project views"
          capability={capabilities.project_views}
        />{/if}
      {#if capabilities.actions.okf_export}<CapabilityCard
          title="OKF export"
          capability={capabilities.actions.okf_export}
        />{/if}
    </section>

    {#if capabilities.queries.graph_projection}
      <GraphPanel
        capability={capabilities.queries.graph_projection}
        errorContract={capabilities.contracts.fusion_http_error}
        graphContract={capabilities.contracts.fusion_graph_projection}
        query={graphQuery}
        rendererFactory={graphRendererFactory}
      />
    {/if}

    <SearchPanel
      capability={capabilities.queries.code_search}
      errorContract={capabilities.contracts.fusion_http_error}
      {search}
    />
  {/if}
</main>

<style>
  main {
    width: min(1180px, calc(100% - 2rem));
    margin: 0 auto;
    padding: 2rem 0 4rem;
  }

  header {
    display: flex;
    gap: 2rem;
    align-items: flex-end;
    justify-content: space-between;
    padding: 1rem 0 2rem;
  }

  h1,
  h2,
  p {
    margin-top: 0;
  }

  h1 {
    margin-bottom: 0.35rem;
    font-size: clamp(2.1rem, 6vw, 4.6rem);
    line-height: 0.95;
    letter-spacing: -0.045em;
  }

  .eyebrow {
    margin-bottom: 0.55rem;
    color: var(--accent);
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.13em;
    text-transform: uppercase;
  }

  .subtitle,
  .empty,
  .freshness {
    color: var(--muted);
  }

  .freshness {
    margin: 0.9rem 0 0;
    font-size: 0.8rem;
  }

  .overall {
    display: grid;
    gap: 0.25rem;
    min-width: 12rem;
    padding: 0.85rem 1rem;
    border: 1px solid var(--border);
    border-radius: 0.8rem;
    background: var(--surface);
  }

  .overall span,
  article span,
  article small {
    color: var(--muted);
    font-size: 0.78rem;
  }

  .overall[data-state="partial"] strong {
    color: var(--warning);
  }

  .overall[data-state="ready"] strong {
    color: var(--accent);
  }

  .readiness,
  .sources,
  .notice {
    margin-bottom: 1rem;
    padding: 1.35rem;
    border: 1px solid var(--border);
    border-radius: 1rem;
    background: rgb(16 28 47 / 92%);
    backdrop-filter: blur(12px);
  }

  .notice.error {
    border-color: color-mix(in srgb, var(--danger) 55%, var(--border));
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

  button:focus-visible {
    outline: 3px solid var(--accent);
    outline-offset: 3px;
  }

  .inventory-state {
    padding: 0.9rem 1rem;
    border-radius: 0.75rem;
    color: var(--muted);
    background: var(--surface-raised);
  }

  .inventory-state p {
    margin-bottom: 0;
  }

  .inventory-state button {
    margin-top: 0.8rem;
  }

  .section-heading {
    display: flex;
    gap: 1rem;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 1rem;
  }

  .section-heading h2 {
    margin-bottom: 0;
    font-size: 1.45rem;
  }

  .section-heading > span {
    color: var(--muted);
    font-size: 0.85rem;
  }

  .signal-grid,
  .capability-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.8rem;
  }

  article {
    display: grid;
    min-width: 0;
    gap: 0.35rem;
    padding: 1rem;
    border-radius: 0.75rem;
    background: var(--surface-raised);
  }

  article strong {
    overflow-wrap: anywhere;
  }

  ul {
    display: grid;
    gap: 0.65rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li {
    display: flex;
    gap: 1rem;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1rem;
    border-radius: 0.75rem;
    background: var(--surface-raised);
  }

  li div {
    display: grid;
    gap: 0.2rem;
    min-width: 0;
  }

  li div span {
    overflow: hidden;
    color: var(--muted);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .source-mode {
    color: var(--accent);
    font-size: 0.78rem;
  }

  @media (max-width: 760px) {
    main {
      width: min(100% - 1rem, 42rem);
      padding-top: 0.8rem;
    }

    header {
      align-items: stretch;
      flex-direction: column;
      gap: 1rem;
    }

    .overall {
      min-width: 0;
    }

    .signal-grid,
    .capability-grid {
      grid-template-columns: 1fr;
    }

    li {
      align-items: flex-start;
      flex-direction: column;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    * {
      scroll-behavior: auto !important;
    }
  }
</style>
