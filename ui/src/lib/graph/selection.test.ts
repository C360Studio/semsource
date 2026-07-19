import { describe, expect, it } from "vitest";
import { createSelectionReducers, type SelectionGraph } from "./selection";

const graph: SelectionGraph = {
  hasNode: (handle) => ["a", "b", "c"].includes(handle),
  neighbors: (handle) => (handle === "a" ? ["b"] : []),
  extremities: (edge) => (edge === "ab" ? ["a", "b"] : ["b", "c"]),
};

describe("createSelectionReducers", () => {
  it("emphasizes a selected node and its neighbors while dimming unrelated items", () => {
    const reducers = createSelectionReducers(graph, "a", new Set());
    expect(reducers.nodeReducer("a")).toMatchObject({
      highlighted: true,
      zIndex: 3,
    });
    expect(reducers.nodeReducer("b")).toMatchObject({ zIndex: 2 });
    expect(reducers.nodeReducer("c")).toMatchObject({ label: null, zIndex: 0 });
    expect(reducers.edgeReducer("ab")).toMatchObject({ zIndex: 2 });
    expect(reducers.edgeReducer("bc")).toMatchObject({ zIndex: 0 });
  });

  it("emphasizes all focused results independently of singular selection", () => {
    const reducers = createSelectionReducers(graph, null, new Set(["a", "b"]));
    expect(reducers.nodeReducer("a")).toMatchObject({ highlighted: true });
    expect(reducers.nodeReducer("c")).toMatchObject({ label: null });
    expect(reducers.edgeReducer("ab")).toEqual({});
    expect(reducers.edgeReducer("bc")).toMatchObject({ zIndex: 0 });
  });
});
