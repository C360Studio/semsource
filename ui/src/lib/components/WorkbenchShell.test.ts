import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { InventoryState, ProjectSummaryState } from "$lib/api/workbench";
import type { WorkbenchCapabilities } from "$lib/contracts/capabilities";
import WorkbenchShell from "./WorkbenchShell.svelte";

const capabilities: WorkbenchCapabilities = {
  contract_version: 1,
  product: { key: "semsource", name: "SemSource" },
  project: { key: "acme", identity_kind: "deployment_namespace" },
  readiness: {
    overall: "partial",
    source: {
      available: true,
      ready: true,
      state: "ready",
      source_count: 1,
      total_entities: 42,
    },
    structural_index: {
      available: true,
      ready: false,
      state: "building",
      lag: 8,
    },
    semantic_index: { available: false, ready: false, state: "unknown" },
  },
  queries: {
    source_inventory: {
      availability: "ready",
      method: "GET",
      href: "/source-manifest/sources",
    },
    graph_projection: {
      availability: "unsupported",
      reason: {
        code: "upstream_contract_pending",
        message: "Governed graph projection is not available",
        retryable: false,
      },
    },
  },
  actions: {
    okf_export: {
      availability: "unsupported",
      reason: {
        code: "not_implemented",
        message: "OKF export is not available",
        retryable: false,
      },
    },
  },
  project_views: {
    availability: "unsupported",
    reason: {
      code: "not_implemented",
      message: "Project views are not available",
      retryable: false,
    },
  },
  contracts: { fusion_http_error: "1" },
};

const inventory: InventoryState = {
  status: "ready",
  data: {
    namespace: "acme",
    timestamp: "2026-07-15T12:00:00Z",
    sources: [{ type: "ast", path: "/workspace", language: "go", watch: true }],
  },
};

const projectSummary: ProjectSummaryState = {
  status: "ready",
  data: {
    namespace: "acme",
    phase: "ready",
    entity_id_format: "org.platform.domain.entity_type.source_id.entity_id",
    total_entities: 42,
    domains: [
      {
        domain: "code",
        entity_count: 42,
        types: [{ type: "code.symbol", count: 42 }],
        sources: ["ast-source-repo"],
      },
    ],
    predicates: [],
    timestamp: "2026-07-15T12:01:00Z",
  },
};

describe("WorkbenchShell", () => {
  it("leads with project, readiness, and source inventory", () => {
    render(WorkbenchShell, {
      capabilities,
      inventory,
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(
      screen.getByRole("heading", { level: 1, name: "SemSource" }),
    ).toBeInTheDocument();
    expect(screen.getByText("acme")).toBeInTheDocument();
    expect(
      screen.getByRole("status", { name: /overall readiness/i }),
    ).toHaveTextContent("Partial");
    expect(
      screen.getByRole("heading", { name: /sources/i }),
    ).toBeInTheDocument();
    expect(screen.getByText("/workspace")).toBeInTheDocument();
    expect(screen.getByText("1 configured sources")).toBeInTheDocument();
    expect(screen.getByText("code.symbol (42)")).toBeInTheDocument();
    expect(screen.getByText("1 contributing sources")).toBeInTheDocument();
    expect(
      screen.getByText("org.platform.domain.entity_type.source_id.entity_id"),
    ).toBeInTheDocument();
  });

  it("shows unsupported graph and future actions without probing them", () => {
    render(WorkbenchShell, {
      capabilities,
      inventory,
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(
      screen.getByRole("region", { name: /graph drill-down/i }),
    ).toHaveTextContent("Governed graph projection is not available");
    expect(
      screen.getByRole("region", { name: /okf export/i }),
    ).toHaveTextContent("Unsupported");
  });

  it("renders a recoverable disconnected state", () => {
    render(WorkbenchShell, {
      capabilities: null,
      inventory: null,
      projectSummary: null,
      loading: false,
      error: "Could not reach SemSource",
      onRetry: vi.fn(),
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Could not reach SemSource",
    );
  });

  it("distinguishes an incompatible contract and does not offer a futile retry", () => {
    render(WorkbenchShell, {
      capabilities: null,
      inventory: null,
      projectSummary: null,
      loading: false,
      error: {
        kind: "incompatible_version",
        message: "This contract is incompatible",
      },
      onRetry: vi.fn(),
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Incompatible workbench contract",
    );
    expect(
      screen.queryByRole("button", { name: /retry/i }),
    ).not.toBeInTheDocument();
  });

  it("distinguishes an inventory that is not ready from a true empty inventory", () => {
    const { rerender } = render(WorkbenchShell, {
      capabilities,
      inventory: {
        status: "not_ready",
        message: "Sources are still loading",
        retryable: true,
      },
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(screen.getByText("Sources are still loading")).toBeInTheDocument();
    expect(screen.queryByText(/0 configured/i)).not.toBeInTheDocument();

    void rerender({
      capabilities,
      inventory: {
        status: "empty",
        data: { namespace: "acme", timestamp: "now", sources: [] },
      },
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(
      screen.getByText("No sources are currently advertised."),
    ).toBeInTheDocument();
  });

  it("does not invent an entity count when evidence is absent", () => {
    const withoutCount = structuredClone(capabilities);
    delete withoutCount.readiness.source.total_entities;
    render(WorkbenchShell, {
      capabilities: withoutCount,
      inventory,
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(screen.getByText("Entity count unavailable")).toBeInTheDocument();
  });

  it("lets the operator retry a disconnected workbench", async () => {
    const onRetry = vi.fn();
    render(WorkbenchShell, {
      capabilities: null,
      inventory: null,
      projectSummary: null,
      loading: false,
      error: "Could not reach SemSource",
      onRetry,
    });
    await userEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("labels not-ready capabilities separately from unsupported ones", () => {
    const partial = structuredClone(capabilities);
    partial.project_views = {
      availability: "not_ready",
      reason: {
        code: "building",
        message: "Views are building",
        retryable: true,
      },
    };
    render(WorkbenchShell, {
      capabilities: partial,
      inventory,
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(
      screen.getByRole("region", { name: /project views/i }),
    ).toHaveTextContent("Not ready");
    expect(
      screen.getByRole("region", { name: /graph drill-down/i }),
    ).toHaveTextContent("Unsupported");
  });

  it("renders independently loading resources and exact freshness labels", () => {
    render(WorkbenchShell, {
      capabilities,
      inventory: { status: "loading" },
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(screen.getByText(/loading source inventory/i)).toBeInTheDocument();
    expect(screen.getByText(/summary generated/i)).toHaveTextContent(
      "2026-07-15T12:01:00Z",
    );
    expect(screen.queryByText(/^Snapshot$/)).not.toBeInTheDocument();
  });

  it("does not manufacture an unsupported card for an absent capability", () => {
    const without = structuredClone(capabilities);
    delete without.actions.okf_export;
    render(WorkbenchShell, {
      capabilities: without,
      inventory,
      projectSummary,
      loading: false,
      error: null,
      onRetry: vi.fn(),
    });
    expect(
      screen.queryByRole("region", { name: /okf export/i }),
    ).not.toBeInTheDocument();
  });
});
