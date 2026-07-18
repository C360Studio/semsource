import { describe, expect, it, vi } from "vitest";
import {
  loadCapabilities,
  loadInventory,
  loadProjectSummary,
  type ResourceState,
} from "./workbench";

function capabilityDocument() {
  return {
    contract_version: 1,
    product: { key: "semsource", name: "SemSource" },
    project: { key: "acme", identity_kind: "deployment_namespace" },
    readiness: {
      overall: "partial",
      source: { available: true, ready: true, state: "ready" },
      structural_index: { available: true, ready: false, state: "building" },
      semantic_index: { available: false, ready: false, state: "unknown" },
    },
    queries: {
      source_inventory: {
        availability: "ready",
        method: "GET",
        href: "/sources",
      },
      project_summary: {
        availability: "ready",
        method: "GET",
        href: "/summary",
      },
    },
    actions: {},
    project_views: { availability: "unsupported" },
    contracts: {},
  };
}

describe("workbench resources", () => {
  it("maps incompatible capability versions distinctly", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ ...capabilityDocument(), contract_version: 2 }),
        ),
      );
    await expect(loadCapabilities(fetcher)).rejects.toMatchObject({
      kind: "incompatible_version",
      retryable: false,
    });
  });

  it("loads inventory and summary independently from advertised routes", async () => {
    const capabilities = await loadCapabilities(
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(new Response(JSON.stringify(capabilityDocument()))),
    );
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            namespace: "acme",
            sources: null,
            timestamp: "2026-07-15T12:00:00Z",
          }),
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            namespace: "acme",
            phase: "ready",
            entity_id_format: "a.b.c.d.e.f",
            total_entities: 0,
            domains: null,
            predicates: null,
            timestamp: "2026-07-15T12:00:00Z",
          }),
        ),
      );
    expect(await loadInventory(fetcher, capabilities)).toMatchObject({
      status: "empty",
    });
    expect(await loadProjectSummary(fetcher, capabilities)).toMatchObject({
      status: "empty",
    });
  });

  it("rejects namespace mismatches and never fetches unsafe routes", async () => {
    const capabilities = await loadCapabilities(
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(new Response(JSON.stringify(capabilityDocument()))),
    );
    const mismatch = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          namespace: "other",
          sources: [],
          timestamp: "2026-07-15T12:00:00Z",
        }),
      ),
    );
    expect(await loadInventory(mismatch, capabilities)).toMatchObject({
      status: "error",
    });

    capabilities.queries.project_summary.href = "https://evil.test/summary";
    const unsafe = vi.fn<typeof fetch>();
    const state: ResourceState<unknown> = await loadProjectSummary(
      unsafe,
      capabilities,
    );
    expect(state).toMatchObject({ status: "error" });
    expect(unsafe).not.toHaveBeenCalled();
  });

  it("does not fabricate unsupported when a capability is absent", async () => {
    const capabilities = await loadCapabilities(
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(new Response(JSON.stringify(capabilityDocument()))),
    );
    delete capabilities.queries.project_summary;
    expect(await loadProjectSummary(vi.fn(), capabilities)).toMatchObject({
      status: "error",
    });
  });

  it("keeps an invalid inventory independent from a valid project summary", async () => {
    const capabilities = await loadCapabilities(
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(new Response(JSON.stringify(capabilityDocument()))),
    );
    const fetcher = vi.fn<typeof fetch>().mockImplementation((href) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            href === "/sources"
              ? {
                  namespace: "other",
                  timestamp: "2026-07-15T12:00:00Z",
                  sources: [],
                }
              : {
                  namespace: "acme",
                  phase: "ready",
                  entity_id_format: "a.b.c.d.e.f",
                  total_entities: 1,
                  domains: [],
                  predicates: [],
                  timestamp: "2026-07-15T12:00:00Z",
                },
          ),
        ),
      ),
    );
    const [inventory, summary] = await Promise.all([
      loadInventory(fetcher, capabilities),
      loadProjectSummary(fetcher, capabilities),
    ]);
    expect(inventory).toMatchObject({
      status: "error",
      kind: "invalid_payload",
    });
    expect(summary).toMatchObject({ status: "ready" });
  });
});
