<script lang="ts">
  import { onMount } from "svelte";
  import {
    loadCapabilities,
    loadInventory,
    loadProjectSummary,
    type InventoryState,
    type ProjectSummaryState,
  } from "$lib/api/workbench";
  import { WorkbenchClientError } from "$lib/api/http";
  import { searchCode, type CodeSearch } from "$lib/api/search";
  import { createBootstrapController } from "$lib/state/bootstrap";
  import WorkbenchShell from "$lib/components/WorkbenchShell.svelte";
  import type { WorkbenchCapabilities } from "$lib/contracts/capabilities";

  let capabilities = $state<WorkbenchCapabilities | null>(null);
  let inventory = $state<InventoryState | null>(null);
  let projectSummary = $state<ProjectSummaryState | null>(null);
  let loading = $state(true);
  let error = $state<{ message: string; kind: string } | null>(null);
  const search: CodeSearch = (href, query, errorContract, signal) =>
    searchCode(fetch, href, query, errorContract, signal);

  const bootstrap = createBootstrapController({
    loadCapabilities: (signal) => loadCapabilities(fetch, signal),
    loadInventory: (value, signal) => loadInventory(fetch, value, signal),
    loadProjectSummary: (value, signal) =>
      loadProjectSummary(fetch, value, signal),
    onStart: () => {
      inventory = { status: "loading" };
      projectSummary = { status: "loading" };
    },
    onCapabilities: (value) => {
      capabilities = value;
      loading = false;
      error = null;
    },
    onInventory: (value) => (inventory = value),
    onProjectSummary: (value) => (projectSummary = value),
    onError: (cause) => {
      const clientError = cause instanceof WorkbenchClientError ? cause : null;
      error = {
        message: clientError?.message ?? "Could not reach SemSource",
        kind: clientError?.kind ?? "disconnected",
      };
      loading = false;
    },
  });

  function refresh(): void {
    if (!capabilities) loading = true;
    error = null;
    void bootstrap.refresh();
  }

  onMount(() => {
    refresh();
    return bootstrap.cancel;
  });
</script>

<WorkbenchShell
  {capabilities}
  {inventory}
  {projectSummary}
  {loading}
  {error}
  onRetry={refresh}
  {search}
/>
