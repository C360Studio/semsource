import { describe, expect, it } from "vitest";
import { parseSourceManifest } from "./sourceManifest";

describe("parseSourceManifest", () => {
  it("accepts an empty manifest without inventing sources", () => {
    expect(
      parseSourceManifest({
        namespace: "acme",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [],
      }),
    ).toMatchObject({ namespace: "acme", sources: [] });
  });

  it("accepts live null sources as empty and all live source fields", () => {
    expect(
      parseSourceManifest({
        namespace: "acme",
        timestamp: "2026-07-15T12:00:00Z",
        sources: null,
      }).sources,
    ).toEqual([]);
    expect(
      parseSourceManifest({
        namespace: "acme",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [
          {
            type: "url",
            urls: ["https://example.test/a", "https://example.test/b"],
            watch: false,
            poll_interval: "5m",
            index_interval: "1h",
          },
        ],
      }).sources[0],
    ).toMatchObject({ poll_interval: "5m", index_interval: "1h" });
  });

  it("requires the live watch boolean", () => {
    expect(() =>
      parseSourceManifest({
        namespace: "acme",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [{ type: "ast", path: "/workspace" }],
      }),
    ).toThrow(/watch/i);
  });

  it("rejects malformed optional source fields", () => {
    expect(() =>
      parseSourceManifest({
        namespace: "acme",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [{ type: "ast", paths: ["/workspace", 42] }],
      }),
    ).toThrow(/paths/i);
  });

  it("requires an RFC3339 timestamp", () => {
    expect(() =>
      parseSourceManifest({ namespace: "acme", timestamp: "now", sources: [] }),
    ).toThrow(/timestamp/i);
  });

  it("rejects impossible calendar dates and accepts Go offsets and nanoseconds", () => {
    expect(() =>
      parseSourceManifest({
        namespace: "acme",
        timestamp: "2026-02-31T12:00:00Z",
        sources: [],
      }),
    ).toThrow(/timestamp/i);
    expect(
      parseSourceManifest({
        namespace: "acme",
        timestamp: "2026-07-15T12:00:00.123456789-05:00",
        sources: [],
      }).timestamp,
    ).toBe("2026-07-15T12:00:00.123456789-05:00");
  });
});
