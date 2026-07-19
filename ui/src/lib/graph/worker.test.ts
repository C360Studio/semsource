import { afterEach, describe, expect, it, vi } from "vitest";
import { createBoundedLayout, type ForceAtlasSupervisor } from "./worker";

afterEach(() => vi.useRealTimers());

describe("createBoundedLayout", () => {
  it("starts, stops at the bound, restarts, and kills the ForceAtlas worker", () => {
    vi.useFakeTimers();
    let running = false;
    const supervisor: ForceAtlasSupervisor = {
      isRunning: () => running,
      start: vi.fn(() => (running = true)),
      stop: vi.fn(() => (running = false)),
      kill: vi.fn(),
    };
    const layout = createBoundedLayout(supervisor, 50);
    layout.start();
    expect(supervisor.start).toHaveBeenCalledOnce();
    vi.advanceTimersByTime(50);
    expect(supervisor.stop).toHaveBeenCalledOnce();
    layout.restart();
    expect(supervisor.start).toHaveBeenCalledTimes(2);
    layout.kill();
    expect(supervisor.stop).toHaveBeenCalledTimes(2);
    expect(supervisor.kill).toHaveBeenCalledOnce();
    layout.start();
    expect(supervisor.start).toHaveBeenCalledTimes(2);
  });
});
