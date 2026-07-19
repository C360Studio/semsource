export interface ForceAtlasSupervisor {
  isRunning(): boolean;
  start(): void;
  stop(): void;
  kill(): void;
}

export interface LayoutLifecycle {
  start(): void;
  stop(): void;
  restart(): void;
  kill(): void;
}

export function createBoundedLayout(
  supervisor: ForceAtlasSupervisor,
  durationMs = 1_500,
): LayoutLifecycle {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let killed = false;

  function clearTimer(): void {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function stop(): void {
    clearTimer();
    if (!killed && supervisor.isRunning()) supervisor.stop();
  }

  function start(): void {
    if (killed) return;
    stop();
    supervisor.start();
    timer = setTimeout(stop, durationMs);
  }

  function restart(): void {
    stop();
    start();
  }

  function kill(): void {
    if (killed) return;
    stop();
    killed = true;
    supervisor.kill();
  }

  return { start, stop, restart, kill };
}
