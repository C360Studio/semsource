<script lang="ts">
  import type { Capability } from "$lib/contracts/capabilities";

  let { title, capability }: { title: string; capability?: Capability } =
    $props();

  const availability = $derived(capability?.availability ?? "unsupported");
  const available = $derived(availability === "ready");
  const badge = $derived(
    availability === "ready"
      ? "Available"
      : availability === "not_ready"
        ? "Not ready"
        : "Unsupported",
  );
  const message = $derived(
    capability?.reason?.message ??
      (available ? "Available" : "Not available in this SemSource version"),
  );
</script>

<section
  class:available
  class:not-ready={availability === "not_ready"}
  aria-label={title}
>
  <div class="card-heading">
    <h2>{title}</h2>
    <span class="badge">{badge}</span>
  </div>
  <p>{message}</p>
</section>

<style>
  section {
    min-height: 9rem;
    padding: 1.2rem;
    border: 1px solid var(--border);
    border-radius: 1rem;
    background: var(--surface);
  }

  section.available {
    border-color: color-mix(in srgb, var(--accent) 55%, var(--border));
  }

  .card-heading {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    justify-content: space-between;
  }

  h2 {
    margin: 0;
    font-size: 1rem;
  }

  p {
    color: var(--muted);
    line-height: 1.5;
  }

  .badge {
    padding: 0.25rem 0.55rem;
    border-radius: 999px;
    color: var(--warning);
    background: rgb(244 198 106 / 12%);
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .available .badge {
    color: var(--accent);
    background: rgb(99 214 199 / 12%);
  }

  .not-ready .badge {
    color: var(--warning);
  }
</style>
