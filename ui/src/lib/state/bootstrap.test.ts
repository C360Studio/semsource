import { describe, expect, it, vi } from "vitest";
import { createBootstrapController } from "./bootstrap";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => (resolve = done));
  return { promise, resolve };
}

describe("bootstrap controller", () => {
  it("publishes capabilities before concurrent optional resources", async () => {
    const inventory = deferred<string>();
    const summary = deferred<string>();
    const events: string[] = [];
    const controller = createBootstrapController({
      loadCapabilities: async () => "caps",
      loadInventory: () => inventory.promise,
      loadProjectSummary: () => summary.promise,
      onStart: () => events.push("start"),
      onCapabilities: () => events.push("capabilities"),
      onInventory: () => events.push("inventory"),
      onProjectSummary: () => events.push("summary"),
      onError: vi.fn(),
    });
    const run = controller.refresh();
    await vi.waitFor(() => expect(events).toEqual(["start", "capabilities"]));
    inventory.resolve("inventory");
    await vi.waitFor(() => expect(events).toContain("inventory"));
    expect(events).not.toContain("summary");
    summary.resolve("summary");
    await run;
  });

  it("aborts predecessors and suppresses stale completions monotonically", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const seen: string[] = [];
    let call = 0;
    const controller = createBootstrapController({
      loadCapabilities: () => (++call === 1 ? first.promise : second.promise),
      loadInventory: async () => "inventory",
      loadProjectSummary: async () => "summary",
      onStart: vi.fn(),
      onCapabilities: (value) => seen.push(value),
      onInventory: vi.fn(),
      onProjectSummary: vi.fn(),
      onError: vi.fn(),
    });
    const oldRun = controller.refresh();
    const newRun = controller.refresh();
    second.resolve("new");
    await newRun;
    first.resolve("old");
    await oldRun;
    expect(seen).toEqual(["new"]);
  });

  it("resets optional state at every generation start", async () => {
    const first = deferred<string>();
    const starts = vi.fn();
    const controller = createBootstrapController({
      loadCapabilities: () => first.promise,
      loadInventory: async () => "inventory",
      loadProjectSummary: async () => "summary",
      onStart: starts,
      onCapabilities: vi.fn(),
      onInventory: vi.fn(),
      onProjectSummary: vi.fn(),
      onError: vi.fn(),
    });
    void controller.refresh();
    void controller.refresh();
    expect(starts).toHaveBeenCalledTimes(2);
    controller.cancel();
  });
});
