export interface ManifestSource {
  type: string;
  path?: string;
  paths?: string[];
  url?: string;
  urls?: string[];
  language?: string;
  branch?: string;
  watch: boolean;
  poll_interval?: string;
  index_interval?: string;
}
import { isRFC3339 } from "./validation";
export interface SourceManifest {
  namespace: string;
  sources: ManifestSource[];
  timestamp: string;
}
function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}
export function parseSourceManifest(value: unknown): SourceManifest {
  const item = record(value);
  if (
    !item ||
    typeof item.namespace !== "string" ||
    typeof item.timestamp !== "string" ||
    (item.sources !== null && !Array.isArray(item.sources))
  )
    throw new Error(
      "Invalid source manifest: missing namespace, timestamp, or sources",
    );
  if (!isRFC3339(item.timestamp as string))
    throw new Error("Invalid source manifest: malformed timestamp");
  const sources = (item.sources ?? []).map((value: unknown) => {
    const entry = record(value);
    if (!entry || typeof entry.type !== "string")
      throw new Error("Invalid source manifest: malformed source");
    for (const field of [
      "path",
      "url",
      "language",
      "branch",
      "poll_interval",
      "index_interval",
    ])
      if (entry[field] !== undefined && typeof entry[field] !== "string")
        throw new Error(`Invalid source manifest: malformed ${field}`);
    for (const field of ["paths", "urls"])
      if (
        entry[field] !== undefined &&
        (!Array.isArray(entry[field]) ||
          !(entry[field] as unknown[]).every((v) => typeof v === "string"))
      )
        throw new Error(`Invalid source manifest: malformed ${field}`);
    if (typeof entry.watch !== "boolean")
      throw new Error("Invalid source manifest: malformed watch");
    return {
      type: entry.type,
      path: entry.path,
      paths: entry.paths,
      url: entry.url,
      urls: entry.urls,
      language: entry.language,
      branch: entry.branch,
      watch: entry.watch,
      poll_interval: entry.poll_interval,
      index_interval: entry.index_interval,
    } as ManifestSource;
  });
  return { namespace: item.namespace, timestamp: item.timestamp, sources };
}
