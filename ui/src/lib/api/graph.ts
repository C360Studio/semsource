import { FusionSearchError, postFusionRequest } from "./search";
import type { FusionResponse } from "$lib/contracts/fusion";

export type GraphQuery = (
  href: string,
  query: string,
  errorContract: string | undefined,
  signal: AbortSignal,
) => Promise<FusionResponse>;

export async function queryGraph(
  fetcher: typeof fetch,
  href: string,
  query: string,
  errorContract: string | undefined,
  signal?: AbortSignal,
): Promise<FusionResponse> {
  const response = await postFusionRequest(
    fetcher,
    href,
    { query, want: ["graph"] },
    errorContract,
    "Graph query",
    signal,
  );
  if (
    !response.graph &&
    response.index.ready &&
    response.nodes.length > 0 &&
    response.misses.length === 0
  )
    throw new FusionSearchError(
      "SemSource graph query returned an invalid response",
      "invalid_payload",
      false,
    );
  return response;
}
