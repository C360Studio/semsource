import { describe, expect, it } from "vitest";
import { resolveAdvertisedRoute } from "./advertisedRoute";

describe("resolveAdvertisedRoute", () => {
  it("accepts only a ready capability with the exact method and safe local route", () => {
    expect(
      resolveAdvertisedRoute(
        { availability: "ready", method: "GET", href: "/summary?view=all" },
        "GET",
      ),
    ).toBe("/summary?view=all");
  });

  it.each([
    ["https://example.test/summary"],
    ["//example.test/summary"],
    ["/\\evil"],
    ["/summary#private"],
    ["/summary\\private"],
    ["///summary"],
  ])("rejects unsafe href %s", (href) => {
    expect(() =>
      resolveAdvertisedRoute(
        { availability: "ready", method: "GET", href },
        "GET",
      ),
    ).toThrow(/route/i);
  });

  it("rejects missing, unready, and wrong-method capabilities", () => {
    expect(() => resolveAdvertisedRoute(undefined, "GET")).toThrow();
    expect(() =>
      resolveAdvertisedRoute({ availability: "not_ready" }, "GET"),
    ).toThrow();
    expect(() =>
      resolveAdvertisedRoute(
        { availability: "ready", method: "POST", href: "/summary" },
        "GET",
      ),
    ).toThrow();
  });
});
