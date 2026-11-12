export type FusionProvenance = "deterministic" | "embedding" | "llm";
import { parseGraphProjection, type GraphProjection } from "./graph";
export type FusionDeferReason = "index_not_ready" | "all_seeds_unhydrated";

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
  relations_truncated?: boolean;
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
  contract_version: "2";
  provenance: FusionProvenance;
  deferred: boolean;
  defer_reason?: FusionDeferReason;
  unreadable_records?: number;
  failed_records?: number;
  nodes: FusionNode[];
  misses: FusionMiss[];
  paths?: FusionPath[];
  paths_truncated?: boolean;
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

function optionalBooleanField(
  item: Record<string, unknown>,
  field: string,
  context: string,
): boolean | undefined {
  const value = item[field];
  if (value !== undefined && typeof value !== "boolean") {
    throw new Error(`Invalid fusion response: malformed ${context} ${field}`);
  }
  return value as boolean | undefined;
}

function parseDeferral(item: Record<string, unknown>): {
  deferred: boolean;
  defer_reason?: FusionDeferReason;
} {
  const deferred = optionalBooleanField(item, "deferred", "response") ?? false;
  const rawReason = item.defer_reason;
  if (!deferred) {
    if (rawReason !== undefined) {
      throw new Error(
        "Invalid fusion response: defer_reason present on non-deferred response",
      );
    }
    return { deferred: false };
  }
  if (rawReason !== "index_not_ready" && rawReason !== "all_seeds_unhydrated") {
    throw new Error("Invalid fusion response: malformed defer_reason");
  }
  return {
    deferred: true,
    defer_reason: rawReason,
  };
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
    relations_truncated: optionalBooleanField(
      item,
      "relations_truncated",
      "node",
    ),
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
  if (item.contract_version !== "2") {
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
  const { deferred, defer_reason } = parseDeferral(item);
  return {
    contract_version: "2",
    provenance: item.provenance as FusionProvenance,
    deferred,
    defer_reason,
    unreadable_records: optionalInteger(item, "unreadable_records", "response"),
    failed_records: optionalInteger(item, "failed_records", "response"),
    nodes: optionalArray(item.nodes, "nodes", parseNode),
    misses: optionalArray(item.misses, "misses", parseMiss),
    paths:
      item.paths === undefined
        ? undefined
        : optionalArray(item.paths, "paths", parsePath),
    paths_truncated: optionalBooleanField(item, "paths_truncated", "response"),
    impact: parseImpact(item.impact),
    graph:
      item.graph === undefined ? undefined : parseGraphProjection(item.graph),
    truncated: item.truncated,
  };
}
