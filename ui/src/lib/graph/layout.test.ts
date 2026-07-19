import { describe, expect, it } from "vitest";
import { computeLayout, initialPosition } from "./layout";

describe("computeLayout", () => {
  it("places the same handles deterministically regardless of input order", () => {
    const forward = computeLayout(["c", "a", "b"], 720, 420);
    const reverse = computeLayout(["b", "c", "a"], 720, 420);
    expect(forward).toEqual(reverse);
    expect(forward.map((point) => point.handle)).toEqual(["a", "b", "c"]);
  });

  it("derives finite initial coordinates from the opaque handle", () => {
    expect(computeLayout([], 720, 420)).toEqual([]);
    const first = initialPosition("opaque-handle");
    expect(first).toEqual(initialPosition("opaque-handle"));
    expect(Number.isFinite(first.x)).toBe(true);
    expect(Number.isFinite(first.y)).toBe(true);
  });
});
