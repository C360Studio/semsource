import { describe, expect, it } from "vitest";
import type { CanonicalGraphNode } from "./model";
import { groupEntities, unresolvedMarker } from "./drilldown";

function node(overrides: Partial<CanonicalGraphNode>): CanonicalGraphNode {
  return {
    handle: "handle",
    resolved: true,
    facts: [],
    factsTruncated: false,
    factsDropped: 0,
    ...overrides,
  };
}

describe("unresolvedMarker", () => {
  it("labels a builtin: handle by its marker kind", () => {
    expect(unresolvedMarker("builtin:len")).toEqual({
      kind: "builtin",
      label: "Builtin: len",
      description: "Builtin reference",
    });
  });

  it("labels an external: handle by its marker kind", () => {
    expect(unresolvedMarker("external:fmt.Println")).toEqual({
      kind: "external",
      label: "External: fmt.Println",
      description: "External reference",
    });
  });

  it("falls back to a verbatim unresolved label for an unhydrated in-graph handle", () => {
    expect(unresolvedMarker("org.platform.golang.repo.function.Foo")).toEqual({
      kind: "unhydrated",
      label: "Unresolved: org.platform.golang.repo.function.Foo",
      description: "Unresolved endpoint",
    });
  });
});

describe("groupEntities", () => {
  it("separates resolved from unresolved, preserving relative order", () => {
    const nodes = [
      node({ handle: "a", resolved: true, name: "Alpha" }),
      node({ handle: "u1", resolved: false }),
      node({ handle: "b", resolved: true, name: "Beta" }),
      node({ handle: "u2", resolved: false }),
    ];
    const groups = groupEntities(nodes, "");
    expect(groups.resolved.map((n) => n.handle)).toEqual(["a", "b"]);
    expect(groups.unresolved.map((n) => n.handle)).toEqual(["u1", "u2"]);
  });

  it("moves the queried symbol to the front of the resolved group and selects it", () => {
    const nodes = [
      node({ handle: "a", resolved: true, name: "Alpha" }),
      node({ handle: "b", resolved: true, name: "Beta" }),
      node({ handle: "c", resolved: true, name: "Gamma" }),
    ];
    const groups = groupEntities(nodes, "Beta");
    expect(groups.resolved.map((n) => n.handle)).toEqual(["b", "a", "c"]);
    expect(groups.selectedHandle).toBe("b");
  });

  it("matches the queried symbol case-insensitively and ignores surrounding whitespace", () => {
    const nodes = [node({ handle: "a", resolved: true, name: "Alpha" })];
    expect(groupEntities(nodes, "  alpha  ").selectedHandle).toBe("a");
  });

  it("falls back to the first resolved node when the query matches nothing", () => {
    const nodes = [
      node({ handle: "a", resolved: true, name: "Alpha" }),
      node({ handle: "b", resolved: true, name: "Beta" }),
    ];
    const groups = groupEntities(nodes, "nonexistent symbol");
    expect(groups.resolved.map((n) => n.handle)).toEqual(["a", "b"]);
    expect(groups.selectedHandle).toBe("a");
  });

  it("never selects an unresolved endpoint, even when no resolved node exists", () => {
    const nodes = [
      node({ handle: "builtin:len", resolved: false }),
      node({ handle: "external:fmt.Println", resolved: false }),
    ];
    const groups = groupEntities(nodes, "len");
    expect(groups.selectedHandle).toBeNull();
  });
});
