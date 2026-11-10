const { test, expect } = require("@playwright/test");

const pollTimeoutMs = Number(process.env.UI_PROFILE_TIMEOUT_MS || 240_000);
const pollIntervalMs = Number(process.env.UI_PROFILE_POLL_MS || 1_000);

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function bodySnippet(body) {
  return body.replace(/\s+/g, " ").trim().slice(0, 500);
}

function contentType(response) {
  return response.headers()["content-type"] || "";
}

async function expectBackendResponse(response, label) {
  const body = await response.text();
  expect(
    contentType(response),
    `${label} fell through to HTML: ${bodySnippet(body)}`,
  ).not.toContain("text/html");
  return body;
}

async function pollCapabilities(request) {
  const deadline = Date.now() + pollTimeoutMs;
  let last = { status: "none", body: "no response" };
  while (Date.now() < deadline) {
    try {
      const response = await request.get("/source-manifest/capabilities", {
        failOnStatusCode: false,
      });
      const body = await response.text();
      last = { status: response.status(), body: bodySnippet(body) };
      const json = JSON.parse(body);
      if (
        response.ok() &&
        json.product?.key === "semsource" &&
        json.readiness?.source?.state === "ready" &&
        json.queries?.source_inventory?.availability === "ready" &&
        json.queries?.code_search?.availability === "ready" &&
        json.queries?.graph_projection?.availability === "ready" &&
        json.contracts?.fusion_graph_projection === "1"
      ) {
        return json;
      }
    } catch (error) {
      last = {
        status: "request-error",
        body: error instanceof Error ? error.message : String(error),
      };
    }
    await sleep(pollIntervalMs);
  }
  throw new Error(
    `capabilities did not become source/search ready within ${pollTimeoutMs}ms; ` +
      `last response: HTTP ${last.status} ${last.body}`,
  );
}

async function assertAdvertisedGetRoutes(request, capabilities) {
  const routes = Object.values(capabilities.queries).filter(
    (capability) =>
      capability.availability === "ready" && capability.method === "GET",
  );
  expect(routes.length).toBeGreaterThanOrEqual(4);
  for (const capability of routes) {
    expect(capability.href).toMatch(/^\/[a-z0-9/_-]+$/);
    const response = await request.get(capability.href, {
      failOnStatusCode: false,
    });
    const body = await expectBackendResponse(
      response,
      `advertised GET ${capability.href}`,
    );
    expect(response.ok(), bodySnippet(body)).toBe(true);
  }
}

async function postGraphQL(request, label, query, variables) {
  const response = await request.post("/graphql", {
    data: variables ? { query, variables } : { query },
    failOnStatusCode: false,
  });
  const body = await expectBackendResponse(response, `graphql ${label}`);
  expect(
    response.ok(),
    `graphql ${label} HTTP ${response.status()}: ${bodySnippet(body)}`,
  ).toBe(true);
  const parsed = JSON.parse(body);
  expect(
    parsed.errors,
    `graphql ${label} returned errors: ${bodySnippet(body)}`,
  ).toBeUndefined();
  expect(parsed.data, `graphql ${label} missing data: ${bodySnippet(body)}`).toBeTruthy();
  return parsed.data;
}

function expectTriple(triples, predicate, expectedObject) {
  expect(Array.isArray(triples)).toBe(true);
  const match = triples.find(
    (triple) =>
      triple?.predicate === predicate &&
      (expectedObject === undefined || String(triple?.object) === String(expectedObject)),
  );
  expect(
    match,
    `missing triple (${predicate}${expectedObject !== undefined ? ` = ${expectedObject}` : ""}) in ${JSON.stringify(triples)}`,
  ).toBeTruthy();
}

// Exercises the live GraphQL query operations served by graph-gateway
// (semsource#251 / semengine#567):
//   - entitiesByPredicate(predicate[, value])
//   - entity(id)
//   - entitiesByPrefix(prefix)
//   - pathSearch(start, depth, direction[, predicates])
async function assertGraphQLSeededOperations(request, projectKey) {
  const alphaSuffix =
    ".workspace.golang.function.src-workbench_fixture-go-UiProfileFixtureAlpha";

  // 1a. entitiesByPredicate(predicate, value) — returns { entities: [String] }.
  // Discover the minted authority (<org>.<platform>) from the alpha entity ID.
  const byTitleData = await postGraphQL(
    request,
    "entitiesByPredicate(predicate, value)",
    `query($predicate: String!, $value: String!) {
      entitiesByPredicate(predicate: $predicate, value: $value)
    }`,
    {
      predicate: "dc.terms.title",
      value: "UiProfileFixtureAlpha",
    },
  );
  const titleMatches = byTitleData.entitiesByPredicate?.entities;
  expect(Array.isArray(titleMatches)).toBe(true);
  const alphaCandidates = titleMatches.filter(
    (id) =>
      typeof id === "string" &&
      id.startsWith(`${projectKey}.`) &&
      id.endsWith(alphaSuffix),
  );
  expect(alphaCandidates).toHaveLength(1);
  const alphaID = alphaCandidates[0];
  const authority = alphaID.slice(0, alphaID.length - alphaSuffix.length);
  const betaID = `${authority}.workspace.golang.function.src-workbench_fixture-go-UiProfileFixtureBeta`;
  const fileID = `${authority}.workspace.golang.file.src-workbench_fixture-go`;

  // 1b. entitiesByPredicate(predicate) without value filter
  const byTypeData = await postGraphQL(
    request,
    "entitiesByPredicate(predicate)",
    `query($predicate: String!) {
      entitiesByPredicate(predicate: $predicate)
    }`,
    {
      predicate: "code.artifact.type",
    },
  );
  expect(byTypeData.entitiesByPredicate?.entities).toEqual(
    expect.arrayContaining([alphaID, betaID, fileID]),
  );

  // 2. entity(id) — pins ExactEntity { entity { id triples } kvRevision }
  const entityData = await postGraphQL(
    request,
    "entity(id)",
    `query($id: String!) {
      entity(id: $id) {
        entity {
          id
          triples { predicate object }
        }
        kvRevision
      }
    }`,
    { id: alphaID },
  );
  expect(entityData.entity?.kvRevision).toEqual(expect.any(Number));
  const alphaEntity = entityData.entity?.entity;
  expect(alphaEntity?.id).toBe(alphaID);
  expectTriple(alphaEntity?.triples, "dc.terms.title", "UiProfileFixtureAlpha");
  expectTriple(alphaEntity?.triples, "code.artifact.type", "function");
  expectTriple(
    alphaEntity?.triples,
    "code.artifact.path",
    "src/workbench_fixture.go",
  );
  expectTriple(alphaEntity?.triples, "code.structure.belongs", fileID);

  // 3. entitiesByPrefix(prefix) — pins EntityPage { entities { id triples } next_cursor }
  const prefix = `${authority}.workspace.golang.function`;
  const prefixData = await postGraphQL(
    request,
    "entitiesByPrefix(prefix)",
    `query($prefix: String!) {
      entitiesByPrefix(prefix: $prefix) {
        entities {
          id
          triples { predicate object }
        }
        next_cursor
      }
    }`,
    { prefix },
  );
  expect(Array.isArray(prefixData.entitiesByPrefix)).toBe(false);
  const prefixEntities = prefixData.entitiesByPrefix?.entities;
  expect(Array.isArray(prefixEntities)).toBe(true);
  const byId = new Map(prefixEntities.map((entity) => [entity.id, entity]));
  expect(byId.has(alphaID)).toBe(true);
  expect(byId.has(betaID)).toBe(true);
  expectTriple(
    byId.get(betaID)?.triples,
    "dc.terms.title",
    "UiProfileFixtureBeta",
  );
  expectTriple(byId.get(betaID)?.triples, "code.relationship.calls", alphaID);

  // 4. pathSearch(start, depth, direction[, predicates])
  // Routes to graph.query.pathSearch and returns PathSearchResult { entities, paths }.
  const pathSearchUnfiltered = await postGraphQL(
    request,
    "pathSearch(start, depth, direction)",
    `query($start: String!, $depth: Int!, $direction: String!) {
      pathSearch(start: $start, depth: $depth, direction: $direction) {
        entities { id }
        paths { from predicate to }
      }
    }`,
    {
      start: betaID,
      depth: 1,
      direction: "OUTGOING",
    },
  );
  const unfilteredIds = (pathSearchUnfiltered.pathSearch?.entities || []).map(
    (entity) => entity.id,
  );
  expect(unfilteredIds).toEqual(
    expect.arrayContaining([betaID, alphaID, fileID]),
  );

  const pathSearchCallsOnly = await postGraphQL(
    request,
    "pathSearch(start, depth, direction, predicates)",
    `query($start: String!, $depth: Int!, $direction: String!, $predicates: [String]) {
      pathSearch(start: $start, depth: $depth, direction: $direction, predicates: $predicates) {
        entities { id }
        paths { from predicate to }
      }
    }`,
    {
      start: betaID,
      depth: 1,
      direction: "OUTGOING",
      predicates: ["code.relationship.calls"],
    },
  );
  const callsOnlyIds = (pathSearchCallsOnly.pathSearch?.entities || []).map(
    (entity) => entity.id,
  );
  expect(callsOnlyIds).toEqual(expect.arrayContaining([betaID, alphaID]));
  expect(callsOnlyIds).not.toContain(fileID);
  const flatSteps = (pathSearchCallsOnly.pathSearch?.paths || []).flat();
  expect(flatSteps).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        from: betaID,
        predicate: "code.relationship.calls",
        to: alphaID,
      }),
    ]),
  );
}

async function assertProxyAllowlist(request, projectKey) {
  const health = await request.get("/health", { failOnStatusCode: false });
  const healthBody = await expectBackendResponse(health, "health");
  expect(health.ok(), bodySnippet(healthBody)).toBe(true);
  expect(JSON.parse(healthBody)).toMatchObject({
    component: "semsource",
    healthy: true,
  });

  await assertGraphQLSeededOperations(request, projectKey);

  const metrics = await request.get("/metrics", { failOnStatusCode: false });
  const metricsBody = await expectBackendResponse(metrics, "metrics");
  expect(metrics.ok(), bodySnippet(metricsBody)).toBe(true);

  for (const [path, data] of [
    ["/code-context/context", {}],
    ["/code-context/impact", {}],
    ["/doc-context/context", {}],
  ]) {
    const response = await request.post(path, {
      data,
      failOnStatusCode: false,
    });
    const body = await expectBackendResponse(response, path);
    expect(response.status(), bodySnippet(body)).not.toBe(404);
    expect(response.status(), bodySnippet(body)).toBeLessThan(500);
  }

  const mcp = await request.post("/mcp-gateway/mcp", {
    headers: { Accept: "application/json, text/event-stream" },
    data: {},
    failOnStatusCode: false,
  });
  await expectBackendResponse(mcp, "mcp");
  expect(mcp.status()).not.toBe(404);
  expect(mcp.status()).toBeLessThan(500);
}

async function assertGraphWebSocket(page) {
  const outcome = await page.evaluate(
    ({ timeoutMs }) =>
      new Promise((resolve, reject) => {
        let opened = false;
        const scheme = window.location.protocol === "https:" ? "wss:" : "ws:";
        const url = `${scheme}//${window.location.host}/ws`;
        const socket = new WebSocket(url);
        const timer = window.setTimeout(() => {
          socket.close();
          reject(
            new Error(`WebSocket did not open and close within ${timeoutMs}ms`),
          );
        }, timeoutMs);

        socket.addEventListener(
          "open",
          () => {
            opened = true;
            socket.close(1000, "ui profile smoke complete");
          },
          { once: true },
        );
        socket.addEventListener(
          "error",
          () => {
            window.clearTimeout(timer);
            reject(new Error("WebSocket upgrade failed before an open event"));
          },
          { once: true },
        );
        socket.addEventListener(
          "close",
          (event) => {
            window.clearTimeout(timer);
            if (!opened) {
              reject(
                new Error(
                  `WebSocket closed before opening (code ${event.code})`,
                ),
              );
              return;
            }
            resolve({ opened, url });
          },
          { once: true },
        );
      }),
    { timeoutMs: 10_000 },
  );

  // The browser emits `open` only after the server accepts the HTTP Upgrade
  // handshake (101 Switching Protocols). Reaching `close` proves cleanup too.
  expect(outcome).toEqual({
    opened: true,
    url: expect.stringMatching(/^ws(s)?:\/\/[^/]+\/ws$/),
  });
}

test("advertised API routes stay behind the Caddy allowlist", async ({
  page,
  request,
}) => {
  const capabilities = await pollCapabilities(request);
  expect(capabilities).toMatchObject({
    contract_version: 1,
    product: { key: "semsource", name: "SemSource" },
    readiness: {
      source: { available: true, ready: true, state: "ready" },
    },
  });
  await assertAdvertisedGetRoutes(request, capabilities);
  await assertProxyAllowlist(request, capabilities.project.key);
  const response = await page.goto("/", { waitUntil: "domcontentloaded" });
  expect(response?.ok()).toBe(true);
  if (process.env.UI_PROFILE_FORCE_FAILURE === "1") {
    expect(
      false,
      "controlled UI profile failure requested for artifact validation",
    ).toBe(true);
  }
  await assertGraphWebSocket(page);
});

test("retired and unknown routes return terminal JSON 404", async ({
  request,
}) => {
  for (const path of [
    "/semsource/health",
    "/flowbuilder/flows",
    "/trajectory/runs",
    "/api/status",
    "/okf/export",
    "/project-view/summary",
    // Retired raw-stream path: the contract moved to /ws when semstreams
    // beta.160 pinned the websocket output's serving path (#147). A clean
    // break, so the old path must 404 — never resurrect as an SPA fallback.
    "/graph",
    "/definitely-not-a-route",
  ]) {
    const response = await request.get(path, { failOnStatusCode: false });
    expect(response.status(), path).toBe(404);
    expect(contentType(response), path).toContain("application/json");
    expect(await response.json()).toEqual({ error: "not_found" });
  }
});

test("workbench exposes real search evidence and keyboard detail", async ({
  page,
  request,
}) => {
  const capabilities = await pollCapabilities(request);
  const graphCapability = capabilities.queries.graph_projection;
  expect(graphCapability).toMatchObject({
    availability: "ready",
    method: "POST",
    href: "/code-context/context",
  });
  const response = await page.goto("/", { waitUntil: "domcontentloaded" });
  expect(response?.ok()).toBe(true);

  await expect(
    page.getByRole("heading", { level: 1, name: "SemSource" }),
  ).toBeVisible();
  await expect(
    page.getByRole("status", { name: /overall readiness/i }),
  ).toContainText(/ready/i);
  await expect(
    page.getByRole("heading", { level: 2, name: "Sources" }),
  ).toBeVisible();
  await expect(
    page.getByText(capabilities.project.key, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("/workspace", { exact: true }).first(),
  ).toBeVisible();

  const search = page.getByLabel(/search code/i);
  const searchButton = page.getByRole("button", {
    name: "Search",
    exact: true,
  });
  const graphInput = page.getByLabel(/graph query/i);
  const graphButton = page.getByRole("button", { name: /investigate graph/i });
  expect(await page.evaluate(() => document.activeElement?.tagName)).toBe(
    "BODY",
  );
  await page.keyboard.press("Tab");
  await expect(graphInput).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(graphButton).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(search).toBeFocused();
  await page.keyboard.type("UiProfileFixtureAlpha");
  await page.keyboard.press("Tab");
  await expect(searchButton).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(search).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(searchButton).toBeFocused();
  await page.keyboard.press("Enter");

  const results = page.getByRole("list", { name: /code search results/i });
  await expect(results).toBeVisible({ timeout: pollTimeoutMs });
  const resultButtons = results.getByRole("button");
  expect(await resultButtons.count()).toBeGreaterThan(0);
  const keyboardTarget = resultButtons.first();
  const selectedName = (
    await keyboardTarget.locator("strong").textContent()
  )?.trim();
  await page.keyboard.press("Tab");
  await expect(keyboardTarget).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(searchButton).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(keyboardTarget).toBeFocused();
  await page.keyboard.press("Enter");
  const detail = page.getByRole("article", { name: /result detail/i });
  await expect(detail).toBeVisible();
  await expect(detail.getByRole("heading", { level: 3 })).toHaveText(
    selectedName || /UiProfileFixture/,
  );
  await expect(detail).toContainText(/provenance/i);
  await expect(detail).toContainText(/workbench_fixture\.go/i);

  const graphQuery = "UiProfileFixtureAlpha";
  const graphResponsePromise = page.waitForResponse((candidate) => {
    const request = candidate.request();
    return (
      new URL(candidate.url()).pathname === graphCapability.href &&
      request.method() === graphCapability.method &&
      request.postDataJSON()?.want?.includes("graph")
    );
  });
  await graphInput.fill(graphQuery);
  await graphButton.click();

  const graphResponse = await graphResponsePromise;
  const graphRequest = graphResponse.request();
  expect(new URL(graphRequest.url()).pathname).toBe(graphCapability.href);
  expect(graphRequest.method()).toBe(graphCapability.method);
  expect(graphRequest.headers()["content-type"]).toContain("application/json");
  expect(graphRequest.postDataJSON()).toEqual({
    query: graphQuery,
    want: ["graph"],
  });

  const graphBody = await expectBackendResponse(
    graphResponse,
    `advertised graph POST ${graphCapability.href}`,
  );
  expect(graphResponse.ok(), bodySnippet(graphBody)).toBe(true);
  expect(contentType(graphResponse)).toContain("application/json");
  const graphEnvelope = JSON.parse(graphBody);
  expect(graphEnvelope).toMatchObject({
    contract_version: "1",
    index: { ready: true, state: "ready" },
    graph: {
      nodes: expect.any(Array),
      edges: expect.any(Array),
      // `coherent` left this envelope in semstreams beta.157 (ViewRevision is
      // Start/End only since); the start === end assertion below carries the
      // coherence claim in substance. See #146.
      view_revision: {
        start: expect.any(Number),
        end: expect.any(Number),
      },
      truncated: expect.any(Boolean),
    },
  });
  expect(graphEnvelope.graph.nodes.length).toBeGreaterThan(0);
  expect(graphEnvelope.graph.view_revision.start).toBeGreaterThan(0);
  expect(graphEnvelope.graph.view_revision.end).toBe(
    graphEnvelope.graph.view_revision.start,
  );

  const graphNavigator = page.getByRole("list", { name: /graph entities/i });
  await expect(graphNavigator).toBeVisible({ timeout: pollTimeoutMs });
  expect(await graphNavigator.getByRole("button").count()).toBeGreaterThan(0);
  await expect(
    page.getByRole("region", { name: /selected entity details/i }),
  ).toContainText(/UiProfileFixtureAlpha/i);
});
