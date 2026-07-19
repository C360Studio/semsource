export interface SelectionGraph {
  hasNode(handle: string): boolean;
  neighbors(handle: string): string[];
  extremities(edge: string): [string, string];
}

export interface NodeVisual {
  color?: string;
  label?: string | null;
  highlighted?: boolean;
  zIndex?: number;
}

export interface EdgeVisual {
  color?: string;
  hidden?: boolean;
  zIndex?: number;
}

export interface SelectionReducers {
  nodeReducer(handle: string): NodeVisual;
  edgeReducer(edge: string): EdgeVisual;
}

const dimNode = "#38485d";
const dimEdge = "#26364a";

export function createSelectionReducers(
  graph: SelectionGraph,
  selectedHandle: string | null,
  focusedHandles: ReadonlySet<string>,
): SelectionReducers {
  const neighbors =
    selectedHandle && graph.hasNode(selectedHandle)
      ? new Set(graph.neighbors(selectedHandle))
      : new Set<string>();

  return {
    nodeReducer(handle) {
      if (selectedHandle) {
        if (handle === selectedHandle) return { highlighted: true, zIndex: 3 };
        if (neighbors.has(handle)) return { zIndex: 2 };
        return { color: dimNode, label: null, zIndex: 0 };
      }
      if (focusedHandles.size > 0 && !focusedHandles.has(handle))
        return { color: dimNode, label: null, zIndex: 0 };
      if (focusedHandles.has(handle)) return { highlighted: true, zIndex: 2 };
      return {};
    },
    edgeReducer(edge) {
      const [source, target] = graph.extremities(edge);
      if (selectedHandle) {
        if (source === selectedHandle || target === selectedHandle)
          return { zIndex: 2 };
        return { color: dimEdge, zIndex: 0 };
      }
      if (
        focusedHandles.size > 0 &&
        !(focusedHandles.has(source) && focusedHandles.has(target))
      )
        return { color: dimEdge, zIndex: 0 };
      return {};
    },
  };
}
