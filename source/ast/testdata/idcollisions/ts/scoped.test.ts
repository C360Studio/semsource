// Each group below is distinct declarations that today share one entity ID.

describe("first", () => {
  const fetcher = () => 1;
  function inner() { return fetcher(); }
  inner();
});

describe("second", () => {
  const fetcher = () => 2;
  function inner() { return fetcher(); }
  inner();
});

export class Box {
  private n = 0;
  get size(): number { return this.n; }
  set size(v: number) { this.n = v; }
  static make(): Box { return new Box(); }
  make(): Box { return this; }
}
