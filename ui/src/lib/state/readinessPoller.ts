/**
 * Drives a self-healing refresh while the workbench is not ready: starts a
 * repeating timer the moment `sync(true)` is observed, stops it the moment
 * `sync(false)` is observed, and starts a fresh one if readiness regresses
 * (D4). Framework-agnostic and directly testable, matching the
 * `state/bootstrap.ts` controller style.
 */
export interface ReadinessPoller {
  /** Reconcile the timer against the current not-ready verdict. */
  sync(notReady: boolean): void;
  /** Stop any active timer; safe to call repeatedly. */
  stop(): void;
}

export function createReadinessPoller(
  refresh: () => void,
  intervalMs = 10_000,
): ReadinessPoller {
  let timer: ReturnType<typeof setInterval> | null = null;
  return {
    sync(notReady: boolean): void {
      if (notReady && timer === null) {
        timer = setInterval(refresh, intervalMs);
      } else if (!notReady && timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    },
    stop(): void {
      if (timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    },
  };
}
