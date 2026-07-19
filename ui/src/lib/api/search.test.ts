import { describe, expect, it, vi } from "vitest";
import { FusionSearchError, searchCode } from "./search";

const ok = {
  contract_version: "1",
  index: { ready: true, state: "ready" },
  provenance: "embedding",
  nodes: [],
  truncated: false,
};

describe("searchCode", () => {
  it("posts only the query to the advertised href", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify(ok), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await searchCode(fetcher, "/code-context/search", "retry logic", "1");
    expect(fetcher).toHaveBeenCalledWith(
      "/code-context/search",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ query: "retry logic" }),
      }),
    );
    expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
      query: "retry logic",
    });
  });

  it.each([
    [400, "invalid_request", "invalid", false],
    [405, "method_not_allowed", "invalid", false],
    [413, "payload_too_large", "invalid", false],
    [500, "internal_error", "fatal", false],
    [502, "upstream_unavailable", "transient", true],
    [503, "dependency_unavailable", "transient", true],
    [504, "upstream_timeout", "transient", true],
  ])(
    "validates an advertised fusion error envelope for HTTP %i",
    async (status, code, errorClass, retryable) => {
      const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: {
              contract_version: "1",
              code,
              class: errorClass,
              message: "Safe server message",
              retryable,
            },
          }),
          { status },
        ),
      );
      await expect(
        searchCode(fetcher, "/code-context/search", "query", "1"),
      ).rejects.toMatchObject({
        status,
        code,
        message: "Safe server message",
        retryable,
      });
    },
  );

  it("falls back to a generic status error when the envelope fails to parse, for any status", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response("not json", { status: 500 }));
    await expect(
      searchCode(fetcher, "/code-context/search", "query", "1"),
    ).rejects.toMatchObject({ status: 500, code: "http_error" });
  });

  it("does not trust an error body when the contract was not advertised", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: {
            contract_version: "1",
            code: "leak",
            class: "fatal",
            message: "/private/path/backend failed",
            retryable: false,
          },
        }),
        { status: 503 },
      ),
    );
    await expect(
      searchCode(fetcher, "/code-context/search", "query", undefined),
    ).rejects.toEqual(
      expect.objectContaining({
        message: "Code search failed with HTTP 503",
        code: "http_error",
      }),
    );
  });

  it("sanitizes malformed advertised error bodies", async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValue(
        new Response('{"error":{"message":"/private/leak"}}', { status: 504 }),
      );
    let error: unknown;
    try {
      await searchCode(fetcher, "/code-context/search", "query", "1");
    } catch (cause) {
      error = cause;
    }
    expect(error).toBeInstanceOf(FusionSearchError);
    expect(error).toMatchObject({
      message: "Code search failed with HTTP 504",
      code: "http_error",
      retryable: true,
    });
  });

  it("rejects unsafe or non-relative advertised hrefs without fetching", async () => {
    const fetcher = vi.fn<typeof fetch>();
    await expect(
      searchCode(fetcher, "https://other.example/search", "query", "1"),
    ).rejects.toThrow("unavailable");
    await expect(
      searchCode(fetcher, "//other.example/search", "query", "1"),
    ).rejects.toThrow("unavailable");
    expect(fetcher).not.toHaveBeenCalled();
  });
});
