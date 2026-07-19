import type { MultiDirectedGraph } from "graphology";
import type Sigma from "sigma";
import type { EdgeProgramType } from "sigma/rendering";
import type { CanonicalGraph } from "./model";
import { initialPosition } from "./layout";
import { createSelectionReducers } from "./selection";
import { createBoundedLayout, type LayoutLifecycle } from "./worker";

export interface GraphRendererSession {
  setGraph(graph: CanonicalGraph): void;
  setSelection(selected: string | null, focused: ReadonlySet<string>): void;
  kill(): void;
}

export type GraphRendererFactory = (
  container: HTMLElement,
  graph: CanonicalGraph,
  onSelect: (handle: string) => void,
) => Promise<GraphRendererSession>;

interface NodeAttributes {
  x: number;
  y: number;
  size: number;
  label: string;
  color: string;
  resolved: boolean;
}

interface EdgeAttributes {
  size: number;
  label: string;
  color: string;
  type: "arrow";
}

function nodeLabel(node: CanonicalGraph["nodes"][number]): string {
  return (
    node.name ?? (node.resolved ? node.handle : `Unresolved: ${node.handle}`)
  );
}

function syncGraphologyGraph(
  target: MultiDirectedGraph<NodeAttributes, EdgeAttributes>,
  source: CanonicalGraph,
): void {
  const incoming = new Set(source.nodes.map((node) => node.handle));
  for (const handle of target.nodes()) {
    if (!incoming.has(handle)) target.dropNode(handle);
  }
  for (const node of source.nodes) {
    const existing = target.hasNode(node.handle)
      ? target.getNodeAttributes(node.handle)
      : null;
    const position = existing ?? initialPosition(node.handle);
    const attributes: NodeAttributes = {
      x: position.x,
      y: position.y,
      size: node.resolved ? 9 : 7,
      label: nodeLabel(node),
      color: node.resolved ? "#63d6c7" : "#f4c66a",
      resolved: node.resolved,
    };
    if (existing) target.replaceNodeAttributes(node.handle, attributes);
    else target.addNode(node.handle, attributes);
  }
  target.clearEdges();
  for (const edge of source.edges) {
    target.addDirectedEdgeWithKey(edge.id, edge.source, edge.target, {
      size: 1.5,
      label: edge.predicate,
      color: "#7890aa",
      type: "arrow",
    });
  }
}

export const createSigmaRenderer: GraphRendererFactory = async (
  container,
  initialGraph,
  onSelect,
) => {
  if (typeof window === "undefined")
    throw new Error("Sigma renderer requires a browser");

  const [graphology, sigmaModule, forceAtlasModule, workerModule, rendering] =
    await Promise.all([
      import("graphology"),
      import("sigma"),
      import("graphology-layout-forceatlas2"),
      import("graphology-layout-forceatlas2/worker"),
      import("sigma/rendering"),
    ]);
  const graph = new graphology.MultiDirectedGraph<
    NodeAttributes,
    EdgeAttributes
  >();
  syncGraphologyGraph(graph, initialGraph);

  let renderer: Sigma<NodeAttributes, EdgeAttributes> | null = null;
  let layout: LayoutLifecycle | null = null;
  try {
    const createdRenderer = new sigmaModule.default<
      NodeAttributes,
      EdgeAttributes
    >(graph, container, {
      allowInvalidContainer: false,
      defaultEdgeType: "arrow",
      edgeProgramClasses: {
        arrow: rendering.EdgeArrowProgram as unknown as EdgeProgramType<
          NodeAttributes,
          EdgeAttributes
        >,
      },
      renderEdgeLabels: true,
      zIndex: true,
    });
    renderer = createdRenderer;
    const supervisor = new workerModule.default(graph, {
      settings: forceAtlasModule.inferSettings(graph),
    });
    layout = createBoundedLayout(supervisor);
    createdRenderer.on("clickNode", ({ node }) => onSelect(node));
    layout.start();
  } catch (cause) {
    layout?.kill();
    renderer?.kill();
    throw cause;
  }

  function setSelection(
    selected: string | null,
    focused: ReadonlySet<string>,
  ): void {
    if (!renderer) return;
    const reducers = createSelectionReducers(graph, selected, focused);
    renderer.setSettings({
      nodeReducer: (handle, data) => ({
        ...data,
        ...reducers.nodeReducer(handle),
      }),
      edgeReducer: (edge, data) => ({
        ...data,
        ...reducers.edgeReducer(edge),
      }),
    });
    renderer.refresh();
  }

  return {
    setGraph(next) {
      syncGraphologyGraph(graph, next);
      renderer?.refresh();
      layout?.restart();
    },
    setSelection,
    kill() {
      layout?.kill();
      layout = null;
      renderer?.kill();
      renderer = null;
    },
  };
};
