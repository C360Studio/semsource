export interface BootstrapDependencies<C, I, S> {
  loadCapabilities(signal: AbortSignal): Promise<C>;
  loadInventory(capabilities: C, signal: AbortSignal): Promise<I>;
  loadProjectSummary(capabilities: C, signal: AbortSignal): Promise<S>;
  onStart(): void;
  onCapabilities(value: C): void;
  onInventory(value: I): void;
  onProjectSummary(value: S): void;
  onError(cause: unknown): void;
}

export function createBootstrapController<C, I, S>(
  dependencies: BootstrapDependencies<C, I, S>,
) {
  let active: AbortController | null = null;
  let generation = 0;
  async function refresh(): Promise<void> {
    active?.abort();
    dependencies.onStart();
    const controller = new AbortController();
    active = controller;
    const current = ++generation;
    const valid = () => !controller.signal.aborted && current === generation;
    try {
      const capabilities = await dependencies.loadCapabilities(
        controller.signal,
      );
      if (!valid()) return;
      dependencies.onCapabilities(capabilities);
      await Promise.all([
        dependencies
          .loadInventory(capabilities, controller.signal)
          .then((value) => {
            if (valid()) dependencies.onInventory(value);
          }),
        dependencies
          .loadProjectSummary(capabilities, controller.signal)
          .then((value) => {
            if (valid()) dependencies.onProjectSummary(value);
          }),
      ]);
    } catch (cause) {
      if (valid()) dependencies.onError(cause);
    } finally {
      if (active === controller) active = null;
    }
  }
  function cancel(): void {
    generation += 1;
    active?.abort();
    active = null;
  }
  return { refresh, cancel };
}
