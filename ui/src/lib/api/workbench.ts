import {
  resolveAdvertisedRoute,
  AdvertisedRouteError,
} from "./advertisedRoute";
import { fetchJSON, WorkbenchClientError } from "./http";
import {
  CapabilityContractError,
  parseCapabilities,
  type Capability,
  type WorkbenchCapabilities,
} from "$lib/contracts/capabilities";
import {
  parseSourceManifest,
  type SourceManifest,
} from "$lib/contracts/sourceManifest";
import {
  parseProjectSummary,
  type ProjectSummary,
} from "$lib/contracts/projectSummary";

export type ResourceState<T> =
  | { status: "loading" }
  | { status: "ready"; data: T }
  | { status: "empty"; data: T }
  | { status: "not_ready"; message: string; retryable: boolean }
  | { status: "unsupported"; message: string }
  | { status: "error"; message: string; retryable: boolean; kind: string };
export type InventoryState = ResourceState<SourceManifest>;
export type ProjectSummaryState = ResourceState<ProjectSummary>;

export async function loadCapabilities(
  fetcher: typeof fetch,
  signal?: AbortSignal,
): Promise<WorkbenchCapabilities> {
  const value = await fetchJSON(
    fetcher,
    "/source-manifest/capabilities",
    signal,
  );
  try {
    return parseCapabilities(value);
  } catch (cause) {
    if (cause instanceof CapabilityContractError) {
      throw new WorkbenchClientError(
        cause.kind === "incompatible_version"
          ? "This SemSource capability contract is not compatible with the workbench"
          : "SemSource returned an invalid capability document",
        cause.kind,
        false,
      );
    }
    throw cause;
  }
}

function advertisedState<T>(
  capability: Capability | undefined,
): ResourceState<T> | null {
  if (!capability)
    return {
      status: "error",
      kind: "invalid_payload",
      retryable: false,
      message: "This resource was not advertised by SemSource",
    };
  if (capability.availability === "not_ready")
    return {
      status: "not_ready",
      message: capability.reason?.message ?? "This resource is not ready",
      retryable: capability.reason?.retryable ?? true,
    };
  if (capability.availability === "unsupported")
    return {
      status: "unsupported",
      message: capability.reason?.message ?? "This resource is unsupported",
    };
  return null;
}
function resourceError<T>(cause: unknown, label: string): ResourceState<T> {
  if (cause instanceof AdvertisedRouteError)
    return {
      status: "error",
      kind: "invalid_payload",
      retryable: false,
      message: `${label} has an invalid advertised route`,
    };
  if (cause instanceof WorkbenchClientError)
    return {
      status: "error",
      kind: cause.kind,
      retryable: cause.retryable,
      message: cause.message,
    };
  return {
    status: "error",
    kind: "invalid_payload",
    retryable: false,
    message: `${label} returned an invalid payload`,
  };
}

export async function loadInventory(
  fetcher: typeof fetch,
  capabilities: WorkbenchCapabilities,
  signal?: AbortSignal,
): Promise<InventoryState> {
  const capability = capabilities.queries.source_inventory;
  const state = advertisedState<SourceManifest>(capability);
  if (state) return state;
  try {
    const href = resolveAdvertisedRoute(capability, "GET");
    const data = parseSourceManifest(await fetchJSON(fetcher, href, signal));
    if (data.namespace !== capabilities.project.key)
      throw new Error("namespace mismatch");
    return { status: data.sources.length === 0 ? "empty" : "ready", data };
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError")
      throw cause;
    return resourceError(cause, "Source inventory");
  }
}

export async function loadProjectSummary(
  fetcher: typeof fetch,
  capabilities: WorkbenchCapabilities,
  signal?: AbortSignal,
): Promise<ProjectSummaryState> {
  const capability = capabilities.queries.project_summary;
  const state = advertisedState<ProjectSummary>(capability);
  if (state) return state;
  try {
    const href = resolveAdvertisedRoute(capability, "GET");
    const data = parseProjectSummary(await fetchJSON(fetcher, href, signal));
    if (data.namespace !== capabilities.project.key)
      throw new Error("namespace mismatch");
    return {
      status:
        data.total_entities === 0 &&
        data.domains.length === 0 &&
        data.predicates.length === 0
          ? "empty"
          : "ready",
      data,
    };
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError")
      throw cause;
    return resourceError(cause, "Project summary");
  }
}
