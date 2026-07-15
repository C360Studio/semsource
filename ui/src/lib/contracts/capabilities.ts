export type Availability = "ready" | "not_ready" | "unsupported";
import { isRFC3339 } from "./validation";
export type OverallReadiness = "ready" | "partial";
export type SourceReadinessState = "ready" | "seeding" | "degraded" | "unknown";
export type IndexReadinessState = "ready" | "building" | "degraded" | "unknown";

export class CapabilityContractError extends Error {
  constructor(
    message: string,
    readonly kind: "invalid_payload" | "incompatible_version",
  ) {
    super(message);
    this.name = "CapabilityContractError";
  }
}

export interface CapabilityReason {
  code: string;
  message: string;
  retryable: boolean;
}
export interface Capability {
  availability: Availability;
  method?: string;
  href?: string;
  readiness?: string[];
  reason?: CapabilityReason;
}
export interface ReadinessSignal {
  available: boolean;
  ready: boolean;
  state: SourceReadinessState | IndexReadinessState;
  source_count?: number;
  total_entities?: number;
  timestamp?: string;
  indexed_revision?: number;
  target_revision?: number;
  lag?: number;
  revision?: string;
  last_synced?: string;
  reason?: CapabilityReason;
}
export interface WorkbenchCapabilities {
  contract_version: 1;
  product: { key: string; name: string };
  project: { key: string; identity_kind: "deployment_namespace" };
  readiness: {
    overall: OverallReadiness;
    source: ReadinessSignal;
    structural_index: ReadinessSignal;
    semantic_index: ReadinessSignal;
  };
  queries: Record<string, Capability>;
  actions: Record<string, Capability>;
  project_views: Capability;
  contracts: Record<string, string>;
}

function fail(message: string): never {
  throw new CapabilityContractError(
    `Invalid workbench capability document: ${message}`,
    "invalid_payload",
  );
}
function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}
function reason(
  value: unknown,
  context = "capability reason",
): CapabilityReason | undefined {
  if (value === undefined) return undefined;
  const item = record(value);
  if (
    !item ||
    typeof item.code !== "string" ||
    typeof item.message !== "string" ||
    typeof item.retryable !== "boolean"
  )
    fail(`malformed ${context}`);
  return {
    code: item.code as string,
    message: item.message as string,
    retryable: item.retryable as boolean,
  };
}
function capability(value: unknown): Capability {
  const item = record(value);
  if (
    !item ||
    !["ready", "not_ready", "unsupported"].includes(String(item.availability))
  )
    fail("malformed capability");
  if (item.method !== undefined && typeof item.method !== "string")
    fail("malformed capability method");
  if (item.href !== undefined && typeof item.href !== "string")
    fail("malformed capability href");
  if (
    item.readiness !== undefined &&
    (!Array.isArray(item.readiness) ||
      !item.readiness.every((v) => typeof v === "string"))
  )
    fail("malformed readiness dependencies");
  return {
    availability: item.availability as Availability,
    method: item.method as string | undefined,
    href: item.href as string | undefined,
    readiness: item.readiness as string[] | undefined,
    reason: reason(item.reason),
  };
}
function capabilityMap(value: unknown): Record<string, Capability> {
  const map = record(value);
  if (!map) fail("malformed capability map");
  return Object.fromEntries(
    Object.entries(map).map(([key, value]) => [key, capability(value)]),
  );
}
function integer(
  item: Record<string, unknown>,
  field: string,
): number | undefined {
  const value = item[field];
  if (
    value !== undefined &&
    (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0)
  )
    fail(`malformed ${field}`);
  return value as number | undefined;
}
function string(
  item: Record<string, unknown>,
  field: string,
): string | undefined {
  const value = item[field];
  if (value !== undefined && typeof value !== "string")
    fail(`malformed ${field}`);
  return value as string | undefined;
}
function readinessSignal(
  value: unknown,
  kind: "source" | "index",
): ReadinessSignal {
  const item = record(value);
  const states =
    kind === "source"
      ? ["ready", "seeding", "degraded", "unknown"]
      : ["ready", "building", "degraded", "unknown"];
  if (
    !item ||
    typeof item.available !== "boolean" ||
    typeof item.ready !== "boolean" ||
    !states.includes(String(item.state))
  )
    fail("malformed readiness signal");
  const state = item.state as ReadinessSignal["state"];
  if (item.ready !== (state === "ready")) fail("inconsistent readiness state");
  if (item.ready && !item.available)
    fail("ready readiness signal must be available");
  const result: ReadinessSignal = {
    available: item.available as boolean,
    ready: item.ready as boolean,
    state,
    source_count: kind === "source" ? integer(item, "source_count") : undefined,
    total_entities:
      kind === "source" ? integer(item, "total_entities") : undefined,
    indexed_revision:
      kind === "index" ? integer(item, "indexed_revision") : undefined,
    target_revision:
      kind === "index" ? integer(item, "target_revision") : undefined,
    lag: kind === "index" ? integer(item, "lag") : undefined,
    timestamp: kind === "source" ? string(item, "timestamp") : undefined,
    revision: kind === "index" ? string(item, "revision") : undefined,
    last_synced: kind === "index" ? string(item, "last_synced") : undefined,
    reason: reason(item.reason, "readiness reason"),
  };
  if (
    kind === "source" &&
    result.timestamp !== undefined &&
    !isRFC3339(result.timestamp)
  )
    fail("malformed source timestamp");
  if (
    kind === "source" &&
    [
      "indexed_revision",
      "target_revision",
      "lag",
      "revision",
      "last_synced",
    ].some((field) => item[field] !== undefined)
  )
    fail("index evidence on source readiness");
  if (
    kind === "index" &&
    ["source_count", "total_entities", "timestamp"].some(
      (field) => item[field] !== undefined,
    )
  )
    fail("source evidence on index readiness");
  if (result.ready && result.lag !== undefined && result.lag !== 0)
    fail("ready index cannot have revision lag");
  if (
    result.indexed_revision !== undefined &&
    result.target_revision !== undefined
  ) {
    if (result.indexed_revision > result.target_revision)
      fail("indexed revision exceeds target revision");
    if (result.ready && result.indexed_revision < result.target_revision)
      fail("ready index is not caught up");
    if (
      result.lag !== undefined &&
      result.lag !==
        Math.max(result.target_revision - result.indexed_revision, 0)
    )
      fail("revision lag does not match revisions");
  }
  if (
    result.indexed_revision !== undefined &&
    result.revision !== undefined &&
    /^\d+$/.test(result.revision) &&
    BigInt(result.revision) !== BigInt(result.indexed_revision)
  )
    fail("numeric revision does not match indexed revision");
  return result;
}

export function parseCapabilities(value: unknown): WorkbenchCapabilities {
  const root = record(value);
  if (!root) fail("expected an object");
  if (root.contract_version !== 1)
    throw new CapabilityContractError(
      `Unsupported capability contract version: ${String(root.contract_version)}`,
      "incompatible_version",
    );
  const product = record(root.product),
    project = record(root.project),
    readiness = record(root.readiness),
    contracts = record(root.contracts);
  if (
    !product ||
    typeof product.key !== "string" ||
    typeof product.name !== "string" ||
    !project ||
    typeof project.key !== "string" ||
    project.identity_kind !== "deployment_namespace" ||
    !readiness ||
    !["ready", "partial"].includes(String(readiness.overall)) ||
    !contracts ||
    !Object.values(contracts).every((v) => typeof v === "string")
  )
    fail("missing required identity or readiness");
  const source = readinessSignal(readiness.source, "source");
  const structuralIndex = readinessSignal(readiness.structural_index, "index");
  const semanticIndex = readinessSignal(readiness.semantic_index, "index");
  if (
    readiness.overall === "ready" &&
    (!source.ready || !structuralIndex.ready)
  )
    fail("overall ready requires source and structural index readiness");
  return {
    contract_version: 1,
    product: { key: product.key as string, name: product.name as string },
    project: {
      key: project.key as string,
      identity_kind: "deployment_namespace",
    },
    readiness: {
      overall: readiness.overall as OverallReadiness,
      source,
      structural_index: structuralIndex,
      semantic_index: semanticIndex,
    },
    queries: capabilityMap(root.queries),
    actions: capabilityMap(root.actions),
    project_views: capability(root.project_views),
    contracts: contracts as Record<string, string>,
  };
}
