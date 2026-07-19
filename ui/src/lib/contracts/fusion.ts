export type FusionProvenance = "deterministic" | "embedding" | "llm";
import { parseGraphProjection, type GraphProjection } from "./graph";
export type FusionIndexState =
  "building" | "ready" | "degraded" | "reset_required";

export interface FusionIndexStatus {
  ready: boolean;
  state: FusionIndexState;
  indexed_revision?: number;
  target_revision?: number;
  lag?: number;
  phase?: string;
  revision?: string;
  last_synced?: string;
}

export interface FusionRef {
  name: string;
  path?: string;
  fragment?: string;
  line?: number;
}

export interface FusionNode {
  name: string;
  kind?: string;
  path?: string;
  fragment?: string;
  lines?: [number, number];
  body?: string;
  relations?: Record<string, FusionRef[]>;
  class?: string;
  handle?: string;
}

export interface FusionMiss {
  query: string;
  did_you_mean?: string[];
}

export interface FusionPath {
  names: string[];
  truncated?: boolean;
}

export interface FusionImpact {
  nodes: number;
  files: number;
  truncated: boolean;
}

export interface FusionResponse {
  contract_version: "1";
  index: FusionIndexStatus;
  provenance: FusionProvenance;
  nodes: FusionNode[];
  misses: FusionMiss[];
  paths?: FusionPath[];
  impact?: FusionImpact;
  graph?: GraphProjection;
  truncated: boolean;
}

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function optionalString(
  item: Record<string, unknown>,
  field: string,
  context: string,
): string | undefined {
  const value = item[field];
  if (value !== undefined && typeof value !== "string") {
    throw new Error(`Invalid fusion response: malformed ${context} ${field}`);
  }
  return value as string | undefined;
}

function optionalInteger(
  item: Record<string, unknown>,
  field: string,
  context: string,
): number | undefined {
  const value = item[field];
  if (
    value !== undefined &&
    (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0)
  ) {
    throw new Error(`Invalid fusion response: malformed ${context} ${field}`);
  }
  return value as number | undefined;
}

function parseIndex(value: unknown): FusionIndexStatus {
  const item = record(value);
  if (
    !item ||
    typeof item.ready !== "boolean" ||
    !["building", "ready", "degraded", "reset_required"].includes(
      String(item.state),
    )
  ) {
    throw new Error("Invalid fusion response: malformed index");
  }
  const result: FusionIndexStatus = {
    ready: item.ready,
    state: item.state as FusionIndexState,
    indexed_revision: optionalInteger(item, "indexed_revision", "index"),
    target_revision: optionalInteger(item, "target_revision", "index"),
    lag: optionalInteger(item, "lag", "index"),
    phase: optionalString(item, "phase", "index"),
    revision: optionalString(item, "revision", "index"),
    last_synced: optionalString(item, "last_synced", "index"),
  };
  if (result.ready !== (result.state === "ready")) {
    throw new Error("Invalid fusion response: inconsistent index readiness");
  }
  if (result.ready && result.lag !== undefined && result.lag !== 0) {
    throw new Error("Invalid fusion response: ready index must have zero lag");
  }
  if (
    result.indexed_revision !== undefined &&
    result.target_revision !== undefined
  ) {
    if (result.indexed_revision > result.target_revision) {
      throw new Error(
        "Invalid fusion response: indexed revision exceeds target",
      );
    }
    if (result.ready && result.indexed_revision < result.target_revision) {
      throw new Error("Invalid fusion response: ready index is not caught up");
    }
    if (
      result.lag !== undefined &&
      result.lag !==
        Math.max(result.target_revision - result.indexed_revision, 0)
    ) {
      throw new Error(
        "Invalid fusion response: index lag does not match revisions",
      );
    }
  }
  if (
    result.indexed_revision !== undefined &&
    result.revision !== undefined &&
    /^\d+$/.test(result.revision) &&
    BigInt(result.revision) !== BigInt(result.indexed_revision)
  ) {
    throw new Error(
      "Invalid fusion response: numeric revision does not match indexed revision",
    );
  }
  return result;
}

function parseRef(value: unknown): FusionRef {
  const item = record(value);
  if (!item || typeof item.name !== "string") {
    throw new Error("Invalid fusion response: malformed relation reference");
  }
  return {
    name: item.name,
    path: optionalString(item, "path", "relation reference"),
    fragment: optionalString(item, "fragment", "relation reference"),
    line: optionalInteger(item, "line", "relation reference"),
  };
}

function parseRelations(
  value: unknown,
): Record<string, FusionRef[]> | undefined {
  if (value === undefined) return undefined;
  const relations = record(value);
  if (!relations) {
    throw new Error("Invalid fusion response: malformed node relations");
  }
  return Object.fromEntries(
    Object.entries(relations).map(([role, refs]) => {
      if (!Array.isArray(refs)) {
        throw new Error("Invalid fusion response: malformed node relations");
      }
      return [role, refs.map(parseRef)];
    }),
  );
}

function parseNode(value: unknown): FusionNode {
  const item = record(value);
  if (!item || typeof item.name !== "string") {
    throw new Error("Invalid fusion response: malformed node");
  }
  let lines: [number, number] | undefined;
  if (item.lines !== undefined) {
    if (
      !Array.isArray(item.lines) ||
      item.lines.length !== 2 ||
      !item.lines.every(
        (line) =>
          typeof line === "number" && Number.isSafeInteger(line) && line >= 0,
      )
    ) {
      throw new Error("Invalid fusion response: malformed node lines");
    }
    lines = [item.lines[0] as number, item.lines[1] as number];
  }
  return {
    name: item.name,
    kind: optionalString(item, "kind", "node"),
    path: optionalString(item, "path", "node"),
    fragment: optionalString(item, "fragment", "node"),
    lines,
    body: optionalString(item, "body", "node"),
    relations: parseRelations(item.relations),
    class: optionalString(item, "class", "node"),
    handle: optionalString(item, "handle", "node"),
  };
}

function parseMiss(value: unknown): FusionMiss {
  const item = record(value);
  if (!item || typeof item.query !== "string") {
    throw new Error("Invalid fusion response: malformed miss");
  }
  if (
    item.did_you_mean !== undefined &&
    (!Array.isArray(item.did_you_mean) ||
      !item.did_you_mean.every((suggestion) => typeof suggestion === "string"))
  ) {
    throw new Error("Invalid fusion response: malformed miss suggestions");
  }
  return {
    query: item.query,
    did_you_mean: item.did_you_mean as string[] | undefined,
  };
}

function parsePath(value: unknown): FusionPath {
  const item = record(value);
  if (
    !item ||
    !Array.isArray(item.names) ||
    !item.names.every((name) => typeof name === "string") ||
    (item.truncated !== undefined && typeof item.truncated !== "boolean")
  ) {
    throw new Error("Invalid fusion response: malformed path");
  }
  return {
    names: item.names,
    truncated: item.truncated as boolean | undefined,
  };
}

function parseImpact(value: unknown): FusionImpact | undefined {
  if (value === undefined) return undefined;
  const item = record(value);
  if (
    !item ||
    typeof item.nodes !== "number" ||
    !Number.isSafeInteger(item.nodes) ||
    item.nodes < 0 ||
    typeof item.files !== "number" ||
    !Number.isSafeInteger(item.files) ||
    item.files < 0 ||
    typeof item.truncated !== "boolean"
  ) {
    throw new Error("Invalid fusion response: malformed impact");
  }
  return {
    nodes: item.nodes,
    files: item.files,
    truncated: item.truncated,
  };
}

function optionalArray<T>(
  value: unknown,
  field: string,
  parser: (entry: unknown) => T,
): T[] {
  if (value === undefined) return [];
  if (!Array.isArray(value)) {
    throw new Error(`Invalid fusion response: malformed ${field}`);
  }
  return value.map(parser);
}

export function parseFusionResponse(value: unknown): FusionResponse {
  const item = record(value);
  if (!item) throw new Error("Invalid fusion response: expected an object");
  if (item.contract_version !== "1") {
    throw new Error(
      `Invalid fusion response: unsupported contract version ${String(item.contract_version)}`,
    );
  }
  if (
    !["deterministic", "embedding", "llm"].includes(String(item.provenance))
  ) {
    throw new Error("Invalid fusion response: malformed provenance");
  }
  if (typeof item.truncated !== "boolean") {
    throw new Error("Invalid fusion response: malformed truncated flag");
  }
  return {
    contract_version: "1",
    index: parseIndex(item.index),
    provenance: item.provenance as FusionProvenance,
    nodes: optionalArray(item.nodes, "nodes", parseNode),
    misses: optionalArray(item.misses, "misses", parseMiss),
    paths:
      item.paths === undefined
        ? undefined
        : optionalArray(item.paths, "paths", parsePath),
    impact: parseImpact(item.impact),
    graph:
      item.graph === undefined ? undefined : parseGraphProjection(item.graph),
    truncated: item.truncated,
  };
}
