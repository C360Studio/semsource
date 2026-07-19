import { describe, expect, it } from "vitest";
import {
  parseSourceManifest,
  sourceKey,
  type ManifestSource,
} from "./sourceManifest";

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

describe("sourceKey", () => {
  const base: ManifestSource = {
    type: "git",
    path: "/workspace/semsource",
    watch: true,
  };

  it("differentiates two sources of one repo by branch alone", () => {
    const main: ManifestSource = { ...base, branch: "main" };
    const feature: ManifestSource = { ...base, branch: "feature/dup" };
    expect(sourceKey(main)).not.toBe(sourceKey(feature));
  });

  it("differentiates two sources of one repo by language alone", () => {
    const go: ManifestSource = { ...base, language: "go" };
    const ts: ManifestSource = { ...base, language: "typescript" };
    expect(sourceKey(go)).not.toBe(sourceKey(ts));
  });

  it("renders missing branch and language as stable empty segments", () => {
    expect(sourceKey(base)).toBe("git:/workspace/semsource::");
  });

  it("is stable and unique across the full source shape, including multi-path/url sources", () => {
    const first: ManifestSource = {
      type: "url",
      urls: ["https://example.test/a", "https://example.test/b"],
      watch: false,
    };
    const second: ManifestSource = {
      type: "url",
      urls: ["https://example.test/a"],
      watch: false,
    };
    expect(sourceKey(first)).not.toBe(sourceKey(second));
    expect(sourceKey(first)).toBe(sourceKey(structuredClone(first)));
  });
});
