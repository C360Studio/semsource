const { test, expect } = require("@playwright/test");

const pollTimeoutMs = Number(process.env.UI_PROFILE_TIMEOUT_MS || 30_000);
const pollIntervalMs = Number(process.env.UI_PROFILE_POLL_MS || 500);

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function bodySnippet(body) {
  return body.replace(/\s+/g, " ").trim().slice(0, 500);
}

function isSemSourceHealth(json) {
  return (
    json &&
    json.component === "semsource" &&
    json.healthy === true &&
    typeof json.status === "string" &&
    typeof json.message === "string" &&
    typeof json.namespace === "string" &&
    json.namespace.length > 0 &&
    typeof json.phase === "string" &&
    json.phase.length > 0 &&
    typeof json.total_entities === "number" &&
    json.total_entities >= 0 &&
    (!Object.prototype.hasOwnProperty.call(json, "sub_statuses") ||
      Array.isArray(json.sub_statuses))
  );
}

async function pollJSON(request, path, label, predicate) {
  const deadline = Date.now() + pollTimeoutMs;
  let last = { status: "none", body: "no response" };

  while (Date.now() < deadline) {
    try {
      const response = await request.get(path, { failOnStatusCode: false });
      const body = await response.text();
      last = { status: response.status(), body: bodySnippet(body) };

      let json;
      try {
        json = JSON.parse(body);
      } catch {
        json = undefined;
      }

      if (response.ok() && json && predicate(json)) {
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
    `${label} did not reach expected state at ${path} within ${pollTimeoutMs}ms. ` +
      `Last response: HTTP ${last.status} ${last.body}`,
  );
}

async function pollGraphQL(request) {
  const deadline = Date.now() + pollTimeoutMs;
  let last = { status: "none", body: "no response" };

  while (Date.now() < deadline) {
    try {
      const response = await request.post("/graphql", {
        data: { query: "query UiProfileSmoke { __typename }" },
        failOnStatusCode: false,
      });
      const body = await response.text();
      last = { status: response.status(), body: bodySnippet(body) };

      let json;
      try {
        json = JSON.parse(body);
      } catch {
        json = undefined;
      }

      if (
        response.status() >= 200 &&
        response.status() < 500 &&
        json &&
        (Object.prototype.hasOwnProperty.call(json, "data") || Array.isArray(json.errors))
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
    `GraphQL route did not return a GraphQL-shaped response within ${pollTimeoutMs}ms. ` +
      `Last response: HTTP ${last.status} ${last.body}`,
  );
}

test("ui profile serves the SemSource workbench and advertised routes", async ({
  page,
  request,
}) => {
  const health = await pollJSON(
    request,
    "/health",
    "profile health",
    isSemSourceHealth,
  );
  expect(health).toMatchObject({ component: "semsource", healthy: true });
  expect(health.namespace.length).toBeGreaterThan(0);
  expect(health.phase.length).toBeGreaterThan(0);
  expect(health.total_entities).toBeGreaterThanOrEqual(0);
  expect(health.message).toContain("SemSource");

  const status = await pollJSON(
    request,
    "/source-manifest/status",
    "source manifest status",
    (json) => typeof json.namespace === "string" && typeof json.phase === "string",
  );
  expect(status.namespace.length).toBeGreaterThan(0);
  expect(status.phase.length).toBeGreaterThan(0);

  const graphQL = await pollGraphQL(request);
  expect(graphQL.data || graphQL.errors).toBeTruthy();

  const response = await page.goto("/", { waitUntil: "domcontentloaded" });
  expect(response, "UI root did not return a response").toBeTruthy();
  expect(response.ok(), `UI root returned HTTP ${response.status()}`).toBe(true);
  await expect(
    page.getByRole("heading", { level: 1, name: "SemSource" }),
  ).toBeVisible();
  await expect(
    page.getByRole("status", { name: /overall readiness/i }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { level: 2, name: "Sources" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { level: 2, name: "Project summary" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { level: 2, name: "Code search" }),
  ).toBeVisible();
  await expect(
    page.getByRole("region", { name: /graph drill-down/i }),
  ).toContainText(/governed.*projection.*not available/i);
});
