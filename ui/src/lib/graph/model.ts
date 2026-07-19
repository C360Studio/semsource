import type { FusionNode } from "$lib/contracts/fusion";
import type {
  GraphEdge,
  GraphFact,
  GraphProjection,
  ViewRevision,
} from "$lib/contracts/graph";

export interface CanonicalGraphNode {
  handle: string;
  resolved: boolean;
  name?: string;
  kind?: string;
  path?: string;
  facts: GraphFact[];
  factsTruncated: boolean;
  factsDropped: number;
}

export interface CanonicalGraph {
  nodes: CanonicalGraphNode[];
  edges: GraphEdge[];
  revision: ViewRevision | null;
}

export interface GraphSyncResult {
  graph: CanonicalGraph;
  mode: "complete" | "partial";
  reason?: string;
}

export function emptyGraph(): CanonicalGraph {
  return { nodes: [], edges: [], revision: null };
}

function sorted<T>(items: Iterable<T>, key: (item: T) => string): T[] {
  return [...items].sort((left, right) => key(left).localeCompare(key(right)));
}

function incomingNodes(
  projection: GraphProjection,
  responseNodes: FusionNode[],
): Map<string, CanonicalGraphNode> {
  const metadata = new Map(
    responseNodes
      .filter((node) => node.handle)
      .map((node) => [node.handle as string, node]),
  );
  const nodes = new Map<string, CanonicalGraphNode>();
  for (const node of projection.nodes) {
    const detail = metadata.get(node.handle);
    nodes.set(node.handle, {
      handle: node.handle,
      resolved: true,
      name: detail?.name,
      kind: detail?.kind,
      path: detail?.path,
      facts: node.facts,
      factsTruncated: node.facts_truncated,
      factsDropped: node.facts_dropped,
    });
  }
  for (const edge of projection.edges) {
    for (const handle of [edge.source, edge.target]) {
      if (!nodes.has(handle)) {
        nodes.set(handle, {
          handle,
          resolved: false,
          facts: [],
          factsTruncated: false,
          factsDropped: 0,
        });
      }
    }
  }
  return nodes;
}

function revisionValue(revision: ViewRevision | null): number {
  return revision ? Math.max(revision.start, revision.end) : 0;
}

function partialReason(
  previous: CanonicalGraph,
  projection: GraphProjection,
): string | undefined {
  if (projection.truncated) return "Graph projection is truncated";
  if (!projection.view_revision.coherent)
    return `Graph projection is incoherent at revisions ${projection.view_revision.start}-${projection.view_revision.end}`;
  if (
    projection.view_revision.start === 0 ||
    projection.view_revision.end === 0
  )
    return "Graph projection revision unavailable";
  if (projection.view_revision.end < revisionValue(previous.revision))
    return `Graph projection revision ${projection.view_revision.end} is stale`;
  return undefined;
}

export function syncGraph(
  previous: CanonicalGraph,
  projection: GraphProjection,
  responseNodes: FusionNode[],
): GraphSyncResult {
  const reason = partialReason(previous, projection);
  if (
    projection.view_revision.end > 0 &&
    projection.view_revision.end < revisionValue(previous.revision)
  ) {
    return {
      graph: previous,
      mode: "partial",
      reason,
    };
  }
  const nodes = reason
    ? new Map(previous.nodes.map((node) => [node.handle, node]))
    : new Map<string, CanonicalGraphNode>();
  for (const [handle, node] of incomingNodes(projection, responseNodes))
    nodes.set(handle, node);

  const edges = reason
    ? new Map(previous.edges.map((edge) => [edge.id, edge]))
    : new Map<string, GraphEdge>();
  for (const edge of projection.edges) edges.set(edge.id, edge);

  return {
    graph: {
      nodes: sorted(nodes.values(), (node) => node.handle),
      edges: sorted(edges.values(), (edge) => edge.id),
      revision:
        reason &&
        revisionValue(previous.revision) >
          revisionValue(projection.view_revision)
          ? previous.revision
          : projection.view_revision,
    },
    mode: reason ? "partial" : "complete",
    reason,
  };
}
