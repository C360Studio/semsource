import { isRFC3339 } from "./validation";

export type JSONValue =
  null | boolean | number | string | JSONValue[] | { [key: string]: JSONValue };

export interface GraphEvidence {
  source?: string;
  timestamp?: string;
  confidence?: number;
  context?: string;
}

export interface GraphFact {
  predicate: string;
  value: JSONValue;
  datatype?: string;
  evidence: GraphEvidence[];
  truncated: boolean;
}

export interface GraphNode {
  handle: string;
  facts: GraphFact[];
  facts_truncated: boolean;
  facts_dropped: number;
}

export type GraphDirection = "outgoing" | "incoming";

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  predicate: string;
  direction: GraphDirection;
  evidence: GraphEvidence[];
  truncated: boolean;
}

export interface ViewRevision {
  start: number;
  end: number;
  coherent: boolean;
}

export interface GraphProjection {
  nodes: GraphNode[];
  edges: GraphEdge[];
  view_revision: ViewRevision;
  truncated: boolean;
}

function invalid(context: string): never {
  throw new Error(`Invalid graph projection: malformed ${context}`);
}

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function nonempty(value: unknown, context: string): string {
  if (typeof value !== "string" || value.length === 0) invalid(context);
  return value;
}

function optionalBoolean(value: unknown, context: string): boolean {
  if (value === undefined) return false;
  if (typeof value !== "boolean") invalid(context);
  return value;
}

function count(value: unknown, context: string): number {
  if (value === undefined) return 0;
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0)
    invalid(context);
  return value;
}

function jsonValue(value: unknown, context: string): JSONValue {
  if (value === null || typeof value === "string" || typeof value === "boolean")
    return value;
  if (typeof value === "number") {
    if (!Number.isFinite(value)) invalid(context);
    return value;
  }
  if (Array.isArray(value))
    return value.map((entry) => jsonValue(entry, context));
  const item = record(value);
  if (!item) invalid(context);
  return Object.fromEntries(
    Object.entries(item).map(([key, entry]) => [
      key,
      jsonValue(entry, context),
    ]),
  );
}

function parseEvidence(value: unknown): GraphEvidence {
  const item = record(value);
  if (!item) invalid("evidence");
  if (item.source !== undefined && typeof item.source !== "string")
    invalid("evidence source");
  if (
    item.timestamp !== undefined &&
    (typeof item.timestamp !== "string" || !isRFC3339(item.timestamp))
  )
    invalid("evidence timestamp");
  if (
    item.confidence !== undefined &&
    (typeof item.confidence !== "number" ||
      !Number.isFinite(item.confidence) ||
      item.confidence < 0 ||
      item.confidence > 1)
  )
    invalid("evidence confidence");
  if (item.context !== undefined && typeof item.context !== "string")
    invalid("evidence context");
  return {
    source: item.source as string | undefined,
    timestamp: item.timestamp as string | undefined,
    confidence: item.confidence as number | undefined,
    context: item.context as string | undefined,
  };
}

function array<T>(
  value: unknown,
  context: string,
  parser: (entry: unknown) => T,
): T[] {
  if (value === undefined) return [];
  if (!Array.isArray(value)) invalid(context);
  return value.map(parser);
}

function parseFact(value: unknown): GraphFact {
  const item = record(value);
  if (!item || !("value" in item)) invalid("fact");
  if (item.datatype !== undefined && typeof item.datatype !== "string")
    invalid("fact datatype");
  return {
    predicate: nonempty(item.predicate, "fact predicate"),
    value: jsonValue(item.value, "fact value"),
    datatype: item.datatype as string | undefined,
    evidence: array(item.evidence, "fact evidence", parseEvidence),
    truncated: optionalBoolean(item.truncated, "fact truncated"),
  };
}

function parseNode(value: unknown): GraphNode {
  const item = record(value);
  if (!item) invalid("node");
  const factsTruncated = optionalBoolean(
    item.facts_truncated,
    "node facts truncated",
  );
  const factsDropped = count(item.facts_dropped, "node facts dropped");
  if (factsTruncated !== factsDropped > 0) invalid("node facts truncation");
  return {
    handle: nonempty(item.handle, "node handle"),
    facts: array(item.facts, "node facts", parseFact),
    facts_truncated: factsTruncated,
    facts_dropped: factsDropped,
  };
}

function parseEdge(value: unknown): GraphEdge {
  const item = record(value);
  if (!item) invalid("edge");
  const source = nonempty(item.source, "edge source");
  const target = nonempty(item.target, "edge target");
  const predicate = nonempty(item.predicate, "edge predicate");
  const id = nonempty(item.id, "edge id");
  if (id !== `${source}|${predicate}|${target}`) invalid("edge id");
  if (!["outgoing", "incoming"].includes(String(item.direction)))
    invalid("edge direction");
  return {
    id,
    source,
    target,
    predicate,
    direction: item.direction as GraphDirection,
    evidence: array(item.evidence, "edge evidence", parseEvidence),
    truncated: optionalBoolean(item.truncated, "edge truncated"),
  };
}

function parseRevision(value: unknown): ViewRevision {
  const item = record(value);
  if (
    !item ||
    typeof item.start !== "number" ||
    !Number.isSafeInteger(item.start) ||
    item.start < 0 ||
    typeof item.end !== "number" ||
    !Number.isSafeInteger(item.end) ||
    item.end < 0 ||
    typeof item.coherent !== "boolean"
  )
    invalid("view revision");
  if (item.coherent && item.start !== item.end) invalid("view revision");
  return {
    start: item.start,
    end: item.end,
    coherent: item.coherent,
  };
}

export function parseGraphProjection(value: unknown): GraphProjection {
  const item = record(value);
  if (!item || typeof item.truncated !== "boolean") invalid("truncated flag");
  const nodes = array(item.nodes, "nodes", parseNode);
  const edges = array(item.edges, "edges", parseEdge);
  const handles = new Set<string>();
  for (const node of nodes) {
    if (handles.has(node.handle)) invalid("duplicate node handle");
    handles.add(node.handle);
  }
  const edgeIDs = new Set<string>();
  for (const edge of edges) {
    if (edgeIDs.has(edge.id)) invalid("duplicate edge id");
    edgeIDs.add(edge.id);
  }
  const childTruncated =
    nodes.some(
      (node) =>
        node.facts_truncated || node.facts.some((fact) => fact.truncated),
    ) || edges.some((edge) => edge.truncated);
  if (childTruncated && !item.truncated) invalid("truncated flag");
  return {
    nodes,
    edges,
    view_revision: parseRevision(item.view_revision),
    truncated: item.truncated,
  };
}
