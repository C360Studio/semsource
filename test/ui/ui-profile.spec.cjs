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

async function assertProxyAllowlist(request) {
  const health = await request.get("/health", { failOnStatusCode: false });
  const healthBody = await expectBackendResponse(health, "health");
  expect(health.ok(), bodySnippet(healthBody)).toBe(true);
  expect(JSON.parse(healthBody)).toMatchObject({
    component: "semsource",
    healthy: true,
  });

  const graphQL = await request.post("/graphql", {
    data: { query: "query UiProfileSmoke { __typename }" },
    failOnStatusCode: false,
  });
  const graphQLBody = await expectBackendResponse(graphQL, "graphql");
  expect(graphQL.status()).toBeLessThan(500);
  expect(JSON.parse(graphQLBody)).toEqual(
    expect.objectContaining({
      [graphQL.ok() ? "data" : "errors"]: expect.anything(),
    }),
  );

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
        const url = `${scheme}//${window.location.host}/graph`;
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
    url: expect.stringMatching(/^ws(s)?:\/\/[^/]+\/graph$/),
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
  await assertProxyAllowlist(request);
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
      view_revision: {
        start: expect.any(Number),
        end: expect.any(Number),
        coherent: true,
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
