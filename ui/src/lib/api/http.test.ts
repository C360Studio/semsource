import { describe, expect, it, vi } from "vitest";
import { fetchJSON, WorkbenchClientError } from "./http";

describe("fetchJSON", () => {
  it("classifies a disconnected backend without exposing transport detail", async () => {
    const fetcher = vi
      .fn()
      .mockRejectedValue(new TypeError("connect ECONNREFUSED 127.0.0.1"));
    await expect(fetchJSON(fetcher, "/capabilities")).rejects.toMatchObject({
      kind: "disconnected",
      message: "Could not reach SemSource",
      retryable: true,
    } satisfies Partial<WorkbenchClientError>);
  });

  it("does not expose a raw backend response body", async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response("internal upstream detail", {
        status: 503,
        statusText: "Unavailable",
      }),
    );
    await expect(fetchJSON(fetcher, "/capabilities")).rejects.toMatchObject({
      kind: "http",
      message: "SemSource request failed with HTTP 503",
    } satisfies Partial<WorkbenchClientError>);
  });

  it("classifies invalid JSON", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValue(new Response("not-json", { status: 200 }));
    await expect(fetchJSON(fetcher, "/capabilities")).rejects.toMatchObject({
      kind: "invalid_payload",
      message: "SemSource returned an invalid JSON response",
    } satisfies Partial<WorkbenchClientError>);
  });
});
