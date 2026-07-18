import {
  parseFusionResponse,
  type FusionResponse,
} from "$lib/contracts/fusion";
import { resolveAdvertisedRoute } from "./advertisedRoute";
import type { Capability } from "$lib/contracts/capabilities";

export type CodeSearch = (
  href: string,
  query: string,
  errorContract: string | undefined,
  signal: AbortSignal,
) => Promise<FusionResponse>;

export class FusionSearchError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly retryable: boolean,
    readonly status?: number,
  ) {
    super(message);
    this.name = "FusionSearchError";
  }
}

interface FusionErrorBody {
  contract_version: "1";
  code: string;
  class: "invalid" | "transient" | "fatal";
  message: string;
  retryable: boolean;
}

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function parseFusionError(value: unknown): FusionErrorBody | null {
  const envelope = record(value);
  const error = record(envelope?.error);
  if (
    !error ||
    error.contract_version !== "1" ||
    typeof error.code !== "string" ||
    !["invalid", "transient", "fatal"].includes(String(error.class)) ||
    typeof error.message !== "string" ||
    typeof error.retryable !== "boolean"
  ) {
    return null;
  }
  return error as unknown as FusionErrorBody;
}

export function isSameOriginRelativeHref(
  href: string | undefined,
): href is string {
  try {
    resolveAdvertisedRoute(
      { availability: "ready", method: "POST", href } as Capability,
      "POST",
    );
    return true;
  } catch {
    return false;
  }
}

function statusError(status: number): FusionSearchError {
  return new FusionSearchError(
    `Code search failed with HTTP ${status}`,
    "http_error",
    status === 503 || status === 504,
    status,
  );
}

export async function searchCode(
  fetcher: typeof fetch,
  href: string,
  query: string,
  errorContract: string | undefined,
  signal?: AbortSignal,
): Promise<FusionResponse> {
  try {
    resolveAdvertisedRoute(
      { availability: "ready", method: "POST", href },
      "POST",
    );
  } catch {
    throw new FusionSearchError(
      "Code search is unavailable because its advertised route is invalid",
      "invalid_capability",
      false,
    );
  }

  let response: Response;
  try {
    response = await fetcher(href, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ query }),
      signal,
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError")
      throw cause;
    throw new FusionSearchError(
      "Could not reach SemSource code search",
      "disconnected",
      true,
    );
  }

  const body = await response.text();
  if (!response.ok) {
    if (errorContract === "1" && [400, 503, 504].includes(response.status)) {
      try {
        const error = parseFusionError(JSON.parse(body) as unknown);
        if (error) {
          throw new FusionSearchError(
            error.message,
            error.code,
            error.retryable,
            response.status,
          );
        }
      } catch (cause) {
        if (cause instanceof FusionSearchError) throw cause;
      }
    }
    throw statusError(response.status);
  }

  try {
    return parseFusionResponse(JSON.parse(body) as unknown);
  } catch (cause) {
    if (
      cause instanceof Error &&
      cause.message.startsWith("Invalid fusion response")
    ) {
      throw new FusionSearchError(
        "SemSource code search returned an invalid response",
        "invalid_payload",
        false,
      );
    }
    throw new FusionSearchError(
      "SemSource code search returned invalid JSON",
      "invalid_payload",
      false,
    );
  }
}
