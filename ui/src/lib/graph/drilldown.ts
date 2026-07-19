import type { CanonicalGraphNode } from "./model";

export type UnresolvedMarkerKind = "builtin" | "external" | "unhydrated";

export interface UnresolvedMarker {
  kind: UnresolvedMarkerKind;
  label: string;
  description: string;
}

const MARKER_PREFIXES: [Exclude<UnresolvedMarkerKind, "unhydrated">, string][] =
  [
    ["builtin", "builtin:"],
    ["external", "external:"],
  ];

/** Classifies an unresolved endpoint handle for grouped, marker-labeled presentation (D6). */
export function unresolvedMarker(handle: string): UnresolvedMarker {
  for (const [kind, prefix] of MARKER_PREFIXES) {
    if (handle.startsWith(prefix)) {
      const name = handle.slice(prefix.length);
      return {
        kind,
        label: `${kind === "builtin" ? "Builtin" : "External"}: ${name}`,
        description: `${kind === "builtin" ? "Builtin" : "External"} reference`,
      };
    }
  }
  return {
    kind: "unhydrated",
    label: `Unresolved: ${handle}`,
    description: "Unresolved endpoint",
  };
}

export interface EntityGroups {
  resolved: CanonicalGraphNode[];
  unresolved: CanonicalGraphNode[];
  selectedHandle: string | null;
}

/**
 * Groups drill-down entities resolved-first, with the queried symbol moved to
 * the front of the resolved group and selected by default (falling back to
 * the first resolved node); unresolved endpoints are grouped separately and
 * are never a default selection (D6).
 */
export function groupEntities(
  nodes: CanonicalGraphNode[],
  queriedName: string,
): EntityGroups {
  const resolvedAll = nodes.filter((node) => node.resolved);
  const unresolved = nodes.filter((node) => !node.resolved);
  const normalized = queriedName.trim().toLowerCase();
  const queriedIndex = normalized
    ? resolvedAll.findIndex(
        (node) => node.name?.trim().toLowerCase() === normalized,
      )
    : -1;
  const resolved =
    queriedIndex > 0
      ? [
          resolvedAll[queriedIndex],
          ...resolvedAll.slice(0, queriedIndex),
          ...resolvedAll.slice(queriedIndex + 1),
        ]
      : resolvedAll;
  const selectedHandle = resolved[0]?.handle ?? null;
  return { resolved, unresolved, selectedHandle };
}
