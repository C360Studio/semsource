import { describe, expect, it } from "vitest";
import { parseCapabilities } from "./capabilities";

const ready = {
  contract_version: 1,
  product: { key: "semsource", name: "SemSource" },
  project: { key: "acme", identity_kind: "deployment_namespace" },
  readiness: {
    overall: "ready",
    source: { available: true, ready: true, state: "ready", source_count: 1 },
    structural_index: { available: true, ready: true, state: "ready" },
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

describe("parseCapabilities", () => {
  it("accepts ready and unsupported capabilities while ignoring additive fields", () => {
    const parsed = parseCapabilities({
      ...ready,
      future_field: { enabled: true },
    });
    expect(parsed.product.name).toBe("SemSource");
    expect(parsed.queries.graph_projection.availability).toBe("unsupported");
  });

  it("rejects an incompatible contract version", () => {
    expect(() => parseCapabilities({ ...ready, contract_version: 2 })).toThrow(
      /unsupported capability contract version/i,
    );
  });

  it("rejects malformed required identity", () => {
    expect(() => parseCapabilities({ ...ready, project: { key: 42 } })).toThrow(
      /invalid workbench capability document/i,
    );
  });

  it("rejects malformed optional readiness evidence", () => {
    const malformed = structuredClone(ready);
    Object.assign(malformed.readiness.source, { total_entities: "forty-two" });
    expect(() => parseCapabilities(malformed)).toThrow(/total_entities/i);
  });

  it("rejects a malformed readiness reason", () => {
    const malformed = structuredClone(ready);
    Object.assign(malformed.readiness.semantic_index, {
      reason: { code: "status_unavailable", message: 42, retryable: true },
    });
    expect(() => parseCapabilities(malformed)).toThrow(/readiness reason/i);
  });

  it.each([
    [
      "overall",
      () => ({
        ...ready,
        readiness: { ...ready.readiness, overall: "unknown" },
      }),
    ],
    [
      "source state",
      () => {
        const value = structuredClone(ready);
        value.readiness.source.state = "building";
        return value;
      },
    ],
    [
      "index state",
      () => {
        const value = structuredClone(ready);
        value.readiness.structural_index.state = "seeding";
        return value;
      },
    ],
    [
      "safe integer",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.source, { total_entities: 1.5 });
        return value;
      },
    ],
    [
      "ready invariant",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.structural_index, {
          ready: true,
          state: "building",
        });
        return value;
      },
    ],
    [
      "lag invariant",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.structural_index, {
          ready: true,
          lag: 1,
        });
        return value;
      },
    ],
    [
      "overall ready dependency invariant",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.structural_index, {
          ready: false,
          state: "building",
        });
        return value;
      },
    ],
    [
      "exact revision lag",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.structural_index, {
          ready: false,
          state: "building",
          indexed_revision: 5,
          target_revision: 8,
          lag: 2,
        });
        value.readiness.overall = "partial";
        return value;
      },
    ],
    [
      "ready revision caught-up invariant",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.structural_index, {
          indexed_revision: 5,
          target_revision: 6,
        });
        return value;
      },
    ],
    [
      "source RFC3339 timestamp",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.source, { timestamp: "yesterday" });
        return value;
      },
    ],
    [
      "indexed revision beyond target",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.structural_index, {
          indexed_revision: 9,
          target_revision: 8,
          lag: 0,
        });
        return value;
      },
    ],
    [
      "numeric revision disagreement",
      () => {
        const value = structuredClone(ready);
        Object.assign(value.readiness.structural_index, {
          indexed_revision: 8,
          revision: "7",
        });
        return value;
      },
    ],
  ])("rejects invalid %s", (_label, build) => {
    expect(() => parseCapabilities(build())).toThrow();
  });

  it("accepts reset_required as a structural index state", () => {
    const value = structuredClone(ready);
    Object.assign(value.readiness.structural_index, {
      ready: false,
      state: "reset_required",
    });
    value.readiness.overall = "partial";
    expect(parseCapabilities(value).readiness.structural_index.state).toBe(
      "reset_required",
    );
  });

  it("accepts reset_required as a semantic index state without failing parse", () => {
    const value = structuredClone(ready);
    Object.assign(value.readiness.semantic_index, {
      available: true,
      ready: false,
      state: "reset_required",
    });
    expect(parseCapabilities(value).readiness.semantic_index.state).toBe(
      "reset_required",
    );
  });

  it("accepts matching unsigned decimal revision evidence", () => {
    const value = structuredClone(ready);
    Object.assign(value.readiness.structural_index, {
      indexed_revision: 8,
      revision: "8",
    });
    expect(parseCapabilities(value).readiness.structural_index.revision).toBe(
      "8",
    );
  });

  it("rejects impossible source dates while accepting live Go timestamps", () => {
    const invalid = structuredClone(ready);
    Object.assign(invalid.readiness.source, {
      timestamp: "2026-02-31T12:00:00Z",
    });
    expect(() => parseCapabilities(invalid)).toThrow(/timestamp/i);
    const valid = structuredClone(ready);
    Object.assign(valid.readiness.source, {
      timestamp: "2026-07-15T12:00:00.123456789+02:30",
    });
    expect(parseCapabilities(valid).readiness.source.timestamp).toContain(
      "+02:30",
    );
  });
});
