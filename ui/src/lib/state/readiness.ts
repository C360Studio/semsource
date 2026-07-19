import type {
  ReadinessSignal,
  WorkbenchCapabilities,
} from "$lib/contracts/capabilities";

type ReadinessKey = "source" | "structural_index" | "semantic_index";

const READINESS_LABELS: Record<ReadinessKey, string> = {
  source: "Sources",
  structural_index: "Structural index",
  semantic_index: "Semantic index",
};

export interface ReadinessCoverage {
  /** True only when every advertised (available) signal is ready. */
  ready: boolean;
  /** Labels of the signals this computation actually covers. */
  covered: string[];
  /** Labels of covered signals that are not yet ready. */
  building: string[];
}

/**
 * Derives an overall-readiness verdict from ALL THREE readiness signals, not
 * just the backend's own `overall` field (which only gates on source and
 * structural index) — the missing gate is exactly how "Ready" could show
 * while the semantic index is still building (D4). A signal that is not
 * advertised (`available: false`, e.g. a tier that isn't deployed) does not
 * block readiness; one that is advertised and not yet ready does.
 */
export function deriveReadinessCoverage(
  readiness: WorkbenchCapabilities["readiness"],
): ReadinessCoverage {
  const signals: [ReadinessKey, ReadinessSignal][] = [
    ["source", readiness.source],
    ["structural_index", readiness.structural_index],
    ["semantic_index", readiness.semantic_index],
  ];
  const tracked = signals.filter(([, signal]) => signal.available);
  const building = tracked
    .filter(([, signal]) => !signal.ready)
    .map(([key]) => READINESS_LABELS[key]);
  return {
    ready: building.length === 0,
    covered: tracked.map(([key]) => READINESS_LABELS[key]),
    building,
  };
}

/** Whether the workbench, as a whole, needs no further self-healing poll. */
export function isFullyReady(
  capabilities: WorkbenchCapabilities | null,
): boolean {
  return (
    capabilities !== null &&
    deriveReadinessCoverage(capabilities.readiness).ready
  );
}
