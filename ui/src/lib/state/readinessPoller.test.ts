import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createReadinessPoller } from "./readinessPoller";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("createReadinessPoller", () => {
  it("does nothing while ready", () => {
    const refresh = vi.fn();
    const poller = createReadinessPoller(refresh, 10_000);
    poller.sync(false);
    vi.advanceTimersByTime(30_000);
    expect(refresh).not.toHaveBeenCalled();
  });

  it("polls on the interval while not ready", () => {
    const refresh = vi.fn();
    const poller = createReadinessPoller(refresh, 10_000);
    poller.sync(true);
    vi.advanceTimersByTime(10_000);
    expect(refresh).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(20_000);
    expect(refresh).toHaveBeenCalledTimes(3);
  });

  it("stops polling once ready and does not double-schedule on repeat syncs", () => {
    const refresh = vi.fn();
    const poller = createReadinessPoller(refresh, 10_000);
    poller.sync(true);
    poller.sync(true);
    poller.sync(true);
    vi.advanceTimersByTime(10_000);
    expect(refresh).toHaveBeenCalledTimes(1);
    poller.sync(false);
    vi.advanceTimersByTime(30_000);
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("restarts polling when readiness regresses", () => {
    const refresh = vi.fn();
    const poller = createReadinessPoller(refresh, 10_000);
    poller.sync(true);
    vi.advanceTimersByTime(10_000);
    expect(refresh).toHaveBeenCalledTimes(1);
    poller.sync(false);
    poller.sync(true);
    vi.advanceTimersByTime(10_000);
    expect(refresh).toHaveBeenCalledTimes(2);
  });

  it("stop() clears an active timer and is safe to call repeatedly", () => {
    const refresh = vi.fn();
    const poller = createReadinessPoller(refresh, 10_000);
    poller.sync(true);
    poller.stop();
    poller.stop();
    vi.advanceTimersByTime(30_000);
    expect(refresh).not.toHaveBeenCalled();
  });
});
