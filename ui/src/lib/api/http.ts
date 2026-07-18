export type WorkbenchErrorKind =
  "disconnected" | "http" | "invalid_payload" | "incompatible_version";

export class WorkbenchClientError extends Error {
  constructor(
    message: string,
    readonly kind: WorkbenchErrorKind,
    readonly retryable: boolean,
    readonly status?: number,
  ) {
    super(message);
    this.name = "WorkbenchClientError";
  }
}

export async function fetchJSON(
  fetcher: typeof fetch,
  href: string,
  signal?: AbortSignal,
): Promise<unknown> {
  let response: Response;
  try {
    response = await fetcher(href, {
      headers: { Accept: "application/json" },
      signal,
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw cause;
    }
    throw new WorkbenchClientError(
      "Could not reach SemSource",
      "disconnected",
      true,
    );
  }
  const body = await response.text();
  if (!response.ok) {
    throw new WorkbenchClientError(
      `SemSource request failed with HTTP ${response.status}`,
      "http",
      response.status >= 500,
      response.status,
    );
  }
  try {
    return JSON.parse(body) as unknown;
  } catch {
    throw new WorkbenchClientError(
      "SemSource returned an invalid JSON response",
      "invalid_payload",
      false,
    );
  }
}
