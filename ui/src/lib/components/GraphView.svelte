<script lang="ts">
  import { onDestroy } from "svelte";
  import type { GraphEvidence } from "$lib/contracts/graph";
  import type { CanonicalGraph, CanonicalGraphNode } from "$lib/graph/model";
  import {
    createSigmaRenderer,
    type GraphRendererFactory,
    type GraphRendererSession,
  } from "$lib/graph/renderer";

  let {
    graph,
    focusedHandles = [],
    selectedHandle = $bindable<string | null>(null),
    rendererFactory = createSigmaRenderer,
  }: {
    graph: CanonicalGraph;
    focusedHandles?: string[];
    selectedHandle?: string | null;
    rendererFactory?: GraphRendererFactory;
  } = $props();

  let container = $state<HTMLElement>();
  let renderer: GraphRendererSession | null = null;
  let initializing = false;
  let destroyed = false;
  let pendingGraph = $state<CanonicalGraph>({
    nodes: [],
    edges: [],
    revision: null,
  });
  let renderError = $state<string | null>(null);
  const selectedNode = $derived(
    graph.nodes.find((node) => node.handle === selectedHandle) ?? null,
  );

  function nodeLabel(node: CanonicalGraphNode): string {
    return (
      node.name ?? (node.resolved ? node.handle : `Unresolved: ${node.handle}`)
    );
  }

  function choose(handle: string): void {
    selectedHandle = handle;
  }

  function rendererFailure(cause: unknown): string {
    const detail = cause instanceof Error ? ` ${cause.message}` : "";
    return `Graph visualization unavailable; use the entity navigator below.${detail}`;
  }

  async function ensureRenderer(): Promise<void> {
    if (!container || renderer || initializing || destroyed) return;
    initializing = true;
    renderError = null;
    try {
      const created = await rendererFactory(container, pendingGraph, choose);
      if (destroyed) {
        created.kill();
        return;
      }
      renderer = created;
      renderer.setGraph(pendingGraph);
      renderer.setSelection(selectedHandle, new Set(focusedHandles));
    } catch (cause) {
      renderError = rendererFailure(cause);
    } finally {
      initializing = false;
    }
  }

  $effect(() => {
    pendingGraph = graph;
    if (renderer) {
      try {
        renderer.setGraph(graph);
      } catch (cause) {
        renderer.kill();
        renderer = null;
        renderError = rendererFailure(cause);
      }
    } else {
      void ensureRenderer();
    }
  });

  $effect(() => {
    const focused = new Set(focusedHandles);
    try {
      renderer?.setSelection(selectedHandle, focused);
    } catch (cause) {
      renderer?.kill();
      renderer = null;
      renderError = rendererFailure(cause);
    }
  });

  onDestroy(() => {
    destroyed = true;
    renderer?.kill();
    renderer = null;
  });

  function evidenceParts(evidence: GraphEvidence): string[] {
    return [
      evidence.source === undefined ? null : `Source ${evidence.source}`,
      evidence.timestamp === undefined
        ? null
        : `Timestamp ${evidence.timestamp}`,
      evidence.confidence === undefined
        ? null
        : `Confidence ${evidence.confidence}`,
      evidence.context === undefined ? null : `Context ${evidence.context}`,
    ].filter((part): part is string => part !== null);
  }
</script>

<div class="graph-layout">
  <div class="visual-surface">
    {#if renderError}
      <div class="visual-error" role="alert" aria-label="Graph visualization">
        {renderError}
      </div>
    {/if}
    <div
      class:failed={renderError !== null}
      class="sigma-container"
      bind:this={container}
      aria-hidden="true"
      data-testid="sigma-graph"
    ></div>
  </div>

  <div class="accessible-surface">
    <div>
      <h3>Entities</h3>
      <ul aria-label="Graph entities">
        {#each graph.nodes as node (node.handle)}
          <li>
            <button
              type="button"
              aria-pressed={selectedHandle === node.handle}
              onclick={() => choose(node.handle)}
            >
              <strong>{nodeLabel(node)}</strong>
              <small
                >{node.resolved
                  ? (node.kind ?? "Type not supplied")
                  : "Unresolved endpoint"}</small
              >
            </button>
          </li>
        {/each}
      </ul>
    </div>

    <section
      id="graph-detail"
      class="detail"
      aria-label="Selected entity details"
      aria-live="polite"
    >
      {#if selectedNode}
        <h3>{nodeLabel(selectedNode)}</h3>
        {#if selectedNode.path}<p>{selectedNode.path}</p>{/if}
        {#if !selectedNode.resolved}
          <p>
            The explicit relationship endpoint <code>{selectedNode.handle}</code
            >
            was supplied without entity details.
          </p>
        {:else if selectedNode.facts.length === 0}
          <p>No property facts were supplied for this entity.</p>
        {:else}
          <h4>Property facts</h4>
          {#if selectedNode.factsTruncated}
            <p class="truncation" role="status">
              Property facts were truncated; {selectedNode.factsDropped} facts were
              omitted.
            </p>
          {/if}
          <ul class="facts">
            {#each selectedNode.facts as fact, index (`${fact.predicate}:${index}`)}
              <li>
                <strong>{fact.predicate}</strong>
                <code>{JSON.stringify(fact.value)}</code>
                {#if fact.datatype}<small>Datatype {fact.datatype}</small>{/if}
                {#each fact.evidence as evidence, evidenceIndex (`${fact.predicate}:${index}:${evidenceIndex}`)}
                  {@const parts = evidenceParts(evidence)}
                  {#if parts.length > 0}<small>{parts.join(" · ")}</small>{/if}
                {/each}
                {#if fact.truncated}<small class="truncation"
                    >Fact evidence was truncated.</small
                  >{/if}
              </li>
            {/each}
          </ul>
        {/if}
        {@const related = graph.edges.filter(
          (edge) =>
            edge.source === selectedNode?.handle ||
            edge.target === selectedNode?.handle,
        )}
        {#if related.length > 0}
          <h4>Directed relationships</h4>
          <ul class="facts">
            {#each related as edge (edge.id)}
              <li>
                <strong>{edge.predicate}</strong>
                <small>{edge.source} → {edge.target}</small>
                {#each edge.evidence as evidence, evidenceIndex (`${edge.id}:${evidenceIndex}`)}
                  {@const parts = evidenceParts(evidence)}
                  {#if parts.length > 0}<small>{parts.join(" · ")}</small>{/if}
                {/each}
                {#if edge.truncated}<small class="truncation"
                    >Relationship evidence was truncated.</small
                  >{/if}
              </li>
            {/each}
          </ul>
        {/if}
      {:else}
        <p>Select an entity to inspect its supplied facts and relationships.</p>
      {/if}
    </section>
  </div>
</div>

<style>
  .graph-layout {
    display: grid;
    gap: 1rem;
  }
  .visual-surface {
    position: relative;
    min-height: 22rem;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: 0.8rem;
    background: #0b1627;
  }
  .sigma-container {
    width: 100%;
    height: 22rem;
  }
  .sigma-container.failed {
    visibility: hidden;
  }
  .visual-error {
    position: absolute;
    z-index: 2;
    padding: 1rem;
    color: var(--warning);
  }
  .accessible-surface {
    display: grid;
    grid-template-columns: minmax(13rem, 0.8fr) minmax(0, 1.4fr);
    gap: 1rem;
  }
  h3,
  h4,
  p {
    margin-top: 0;
  }
  ul {
    display: grid;
    gap: 0.45rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li button {
    display: grid;
    width: 100%;
    gap: 0.15rem;
    padding: 0.7rem;
    border: 1px solid var(--border);
    border-radius: 0.55rem;
    color: var(--ink);
    background: var(--surface);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  li button[aria-pressed="true"] {
    border-color: var(--accent);
    background: rgb(99 214 199 / 18%);
  }
  small {
    color: var(--muted);
  }
  .detail {
    min-width: 0;
    padding: 1rem;
    border-radius: 0.75rem;
    background: var(--surface-raised);
  }
  .facts li {
    display: grid;
    gap: 0.25rem;
    padding: 0.65rem;
    border-radius: 0.55rem;
    background: rgb(8 17 31 / 45%);
  }
  code {
    overflow-wrap: anywhere;
    color: #dce8f8;
  }
  .truncation {
    color: var(--warning);
  }
  @media (max-width: 760px) {
    .accessible-surface {
      grid-template-columns: 1fr;
    }
  }
</style>
