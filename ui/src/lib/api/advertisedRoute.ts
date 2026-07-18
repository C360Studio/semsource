import type { Capability } from "$lib/contracts/capabilities";

export class AdvertisedRouteError extends Error {
  constructor(
    message: string,
    readonly reason: "missing" | "not_ready" | "unsupported" | "invalid",
  ) {
    super(message);
    this.name = "AdvertisedRouteError";
  }
}

export function resolveAdvertisedRoute(
  capability: Capability | undefined,
  method: "GET" | "POST",
): string {
  if (!capability)
    throw new AdvertisedRouteError("Capability was not advertised", "missing");
  if (capability.availability !== "ready")
    throw new AdvertisedRouteError(
      capability.reason?.message ?? `Capability is ${capability.availability}`,
      capability.availability === "not_ready" ? "not_ready" : "unsupported",
    );
  const href = capability.href;
  if (
    capability.method !== method ||
    !href ||
    !href.startsWith("/") ||
    href.startsWith("//") ||
    href.includes("\\") ||
    href.includes("#")
  ) {
    throw new AdvertisedRouteError(
      `Invalid advertised ${method} route`,
      "invalid",
    );
  }
  let parsed: URL;
  try {
    parsed = new URL(href, "https://semsource.invalid");
  } catch {
    throw new AdvertisedRouteError(
      `Invalid advertised ${method} route`,
      "invalid",
    );
  }
  if (
    parsed.origin !== "https://semsource.invalid" ||
    parsed.username ||
    parsed.password ||
    !parsed.pathname.startsWith("/") ||
    parsed.pathname.startsWith("//")
  ) {
    throw new AdvertisedRouteError(
      `Invalid advertised ${method} route`,
      "invalid",
    );
  }
  return href;
}
