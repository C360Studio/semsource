import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function horizontalOverflowReport(page: Page) {
  return page.evaluate(() => {
    const viewportWidth = window.innerWidth;
    const documentWidth = document.documentElement.scrollWidth;
    const offenders = Array.from(document.querySelectorAll<HTMLElement>("*"))
      .map((element) => {
        const bounds = element.getBoundingClientRect();
        return {
          element: `${element.tagName.toLowerCase()}${element.id ? `#${element.id}` : ""}${Array.from(
            element.classList,
          )
            .map((name) => `.${name}`)
            .join("")}`,
          left: Math.round(bounds.left * 100) / 100,
          right: Math.round(bounds.right * 100) / 100,
          width: Math.round(bounds.width * 100) / 100,
          clientWidth: element.clientWidth,
          scrollWidth: element.scrollWidth,
        };
      })
      .filter(({ left, right }) => left < -0.5 || right > viewportWidth + 0.5)
      .sort((left, right) => right.right - left.right)
      .slice(0, 12);
    return { viewportWidth, documentWidth, offenders };
  });
}

const capabilities = {
  contract_version: 1,
  product: { key: "semsource", name: "SemSource" },
  project: { key: "acme-workbench", identity_kind: "deployment_namespace" },
  readiness: {
    overall: "partial",
    source: {
      available: true,
      ready: true,
      state: "ready",
      source_count: 1,
      total_entities: 42,
      timestamp: "2026-07-15T11:59:00Z",
    },
    structural_index: {
      available: true,
      ready: true,
      state: "ready",
      lag: 0,
      last_synced: "2026-07-15T11:58:00Z",
    },
    semantic_index: {
      available: true,
      ready: true,
      state: "ready",
    },
  },
  queries: {
    source_inventory: {
      availability: "ready",
      method: "GET",
      href: "/source-manifest/sources",
    },
    project_summary: {
      availability: "ready",
      method: "GET",
      href: "/source-manifest/summary",
    },
    code_search: {
      availability: "ready",
      method: "POST",
      href: "/code-context/search",
    },
    graph_projection: {
      availability: "unsupported",
      reason: {
        code: "upstream_contract_pending",
        message: "Governed graph projection is not available",
        retryable: false,
      },
    },
  },
  actions: {
    okf_export: {
      availability: "unsupported",
      reason: {
        code: "not_implemented",
        message: "OKF export is not available",
        retryable: false,
      },
    },
  },
  project_views: {
    availability: "unsupported",
    reason: {
      code: "not_implemented",
      message: "Project views are not available",
      retryable: false,
    },
  },
  contracts: { fusion_http_error: "1" },
};

const graphResponse = {
  contract_version: "1",
  index: {
    ready: true,
    state: "ready",
    indexed_revision: 153,
    target_revision: 153,
    lag: 0,
  },
  provenance: "deterministic",
  nodes: [
    {
      name: "Alpha",
      kind: "function",
      path: "alpha.go",
      handle: "opaque-alpha",
    },
    { name: "Beta", kind: "function", path: "beta.go", handle: "opaque-beta" },
  ],
  graph: {
    nodes: [
      {
        handle: "opaque-alpha",
        facts: [
          { predicate: "source.line", value: 12, datatype: "xsd:integer" },
        ],
      },
      { handle: "opaque-beta" },
    ],
    edges: [
      {
        id: "opaque-alpha|calls|opaque-beta",
        source: "opaque-alpha",
        target: "opaque-beta",
        predicate: "calls",
        direction: "outgoing",
        evidence: [{ source: "ast" }],
      },
    ],
    view_revision: { start: 153, end: 153, coherent: true },
    truncated: false,
  },
  truncated: false,
};

function graphCapabilities() {
  const ready = structuredClone(capabilities);
  ready.queries.graph_projection = {
    availability: "ready",
    method: "POST",
    href: "/code-context/context",
  };
  ready.contracts = {
    ...ready.contracts,
    fusion_graph_projection: "1",
  };
  return ready;
}

async function routeWorkbenchBootstrap(page: Page, document = capabilities) {
  await page.route("**/source-manifest/capabilities", (route) =>
    route.fulfill({ json: document }),
  );
  await page.route("**/source-manifest/sources", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [],
      },
    }),
  );
  await page.route("**/source-manifest/summary", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        phase: "ready",
        entity_id_format: "org.platform.domain.entity_type.source_id.entity_id",
        total_entities: 2,
        domains: [],
        predicates: [],
        timestamp: "2026-07-15T12:01:00Z",
      },
    }),
  );
}

test("renders the owned SemSource shell truthfully", async ({ page }) => {
  let searchBody: unknown;
  // D6: the favicon and every other fix in this change must leave the page
  // free of console/page errors.
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  page.on("console", (message) => {
    if (message.type() === "error") pageErrors.push(message.text());
  });
  await page.route("**/source-manifest/capabilities", (route) =>
    route.fulfill({ json: capabilities }),
  );
  await page.route("**/source-manifest/sources", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [
          {
            type: "ast",
            path: "/workspace/semsource",
            language: "go",
            watch: true,
          },
        ],
      },
    }),
  );
  await page.route("**/source-manifest/summary", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        phase: "ready",
        entity_id_format: "org.platform.domain.entity_type.source_id.entity_id",
        total_entities: 42,
        domains: [
          {
            domain: "code",
            entity_count: 42,
            types: [{ type: "code.symbol", count: 42 }],
            sources: ["ast-source-repo"],
          },
        ],
        predicates: [],
        timestamp: "2026-07-15T12:01:00Z",
      },
    }),
  );
  await page.route("**/code-context/search", async (route) => {
    searchBody = route.request().postDataJSON();
    await route.fulfill({
      json: {
        contract_version: "1",
        index: { ready: true, state: "ready", lag: 0 },
        provenance: "embedding",
        nodes: [
          {
            name: "loadWorkbench",
            kind: "function",
            path: "ui/src/lib/api/workbench.ts",
            lines: [24, 76],
            body: "export async function loadWorkbench() {}",
            handle: "opaque-not-an-address",
          },
          {
            name: "refresh",
            kind: "function",
            path: "ui/src/routes/+page.svelte",
            body: "function refresh(): void {}",
          },
        ],
        truncated: false,
      },
    });
  });

  await page.goto("/");

  await expect(page.getByRole("heading", { level: 1 })).toHaveText("SemSource");
  await expect(page.getByText("acme-workbench")).toBeVisible();
  // All three readiness signals in this fixture are actually ready; the
  // banner now derives truth from them (D4) rather than trusting the
  // fixture's stale `readiness.overall: "partial"` field verbatim.
  const overallReadiness = page.getByRole("status", {
    name: /overall readiness/i,
  });
  await expect(overallReadiness).toContainText("Ready");
  await expect(overallReadiness).toContainText(
    "Covers Sources, Structural index, Semantic index",
  );
  await expect(page.getByText("/workspace/semsource")).toBeVisible();
  await expect(page.getByText(/summary generated/i)).toContainText(
    "2026-07-15T12:01:00Z",
  );
  await expect(page.getByText(/status snapshot/i)).toContainText(
    "2026-07-15T11:59:00Z",
  );
  await expect(page.getByText(/last index progress/i)).toContainText(
    "2026-07-15T11:58:00Z",
  );
  await expect(page.getByText(/inventory updated/i)).toContainText(
    "2026-07-15T12:00:00Z",
  );
  await expect(
    page.getByRole("region", { name: /graph drill-down/i }),
  ).toContainText("Governed graph projection is not available");
  await expect(page.getByRole("region", { name: /okf export/i })).toContainText(
    "Unsupported",
  );

  await page.getByLabel("Search code").fill("workbench bootstrap");
  await page.getByRole("button", { name: "Search" }).click();
  await expect(
    page.getByRole("button", { name: /loadWorkbench/i }),
  ).toBeVisible();
  expect(searchBody).toEqual({ query: "workbench bootstrap" });
  await page.getByRole("button", { name: /refresh/i }).click();
  await expect(
    page.getByRole("article", { name: /result detail/i }),
  ).toContainText("ui/src/routes/+page.svelte");
  await expect(page.getByText("opaque-not-an-address")).toHaveCount(0);
  expect(pageErrors).toEqual([]);
});

test("renders two sources of one repo differing only by branch without a duplicate-key crash", async ({
  page,
}) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.route("**/source-manifest/capabilities", (route) =>
    route.fulfill({ json: capabilities }),
  );
  await page.route("**/source-manifest/sources", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [
          {
            type: "git",
            path: "/workspace/semsource",
            branch: "main",
            watch: true,
          },
          {
            type: "git",
            path: "/workspace/semsource",
            branch: "feature/dup",
            watch: true,
          },
        ],
      },
    }),
  );
  await page.route("**/source-manifest/summary", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        phase: "ready",
        entity_id_format: "org.platform.domain.entity_type.source_id.entity_id",
        total_entities: 0,
        domains: [],
        predicates: [],
        timestamp: "2026-07-15T12:01:00Z",
      },
    }),
  );
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("SemSource");
  await expect(
    page.getByRole("region", { name: "Sources" }).locator("li"),
  ).toHaveCount(2);
  expect(pageErrors).toEqual([]);
});

test("stays alive and shows a distinct not-ready state when the index reports reset_required", async ({
  page,
}) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  const resetRequired = structuredClone(capabilities);
  resetRequired.readiness.structural_index = {
    available: true,
    ready: false,
    state: "reset_required",
  };
  resetRequired.readiness.overall = "partial";
  await routeWorkbenchBootstrap(page, resetRequired);
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("SemSource");
  await expect(page.getByText(/reset required/i)).toBeVisible();
  expect(pageErrors).toEqual([]);
});

test("refreshes a not-ready panel every 10 seconds without a manual reload", async ({
  page,
}) => {
  await page.clock.install();
  let capabilitiesRequests = 0;
  const buildingSemantic = structuredClone(capabilities);
  buildingSemantic.readiness = {
    overall: "ready",
    source: { available: true, ready: true, state: "ready" },
    structural_index: { available: true, ready: true, state: "ready" },
    semantic_index: { available: true, ready: false, state: "building" },
  };
  const readySemantic = structuredClone(capabilities);
  readySemantic.readiness = {
    overall: "ready",
    source: { available: true, ready: true, state: "ready" },
    structural_index: { available: true, ready: true, state: "ready" },
    semantic_index: { available: true, ready: true, state: "ready" },
  };
  await page.route("**/source-manifest/capabilities", async (route) => {
    capabilitiesRequests += 1;
    await route.fulfill({
      json: capabilitiesRequests === 1 ? buildingSemantic : readySemantic,
    });
  });
  await page.route("**/source-manifest/sources", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [],
      },
    }),
  );
  await page.route("**/source-manifest/summary", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        phase: "ready",
        entity_id_format: "org.platform.domain.entity_type.source_id.entity_id",
        total_entities: 0,
        domains: [],
        predicates: [],
        timestamp: "2026-07-15T12:01:00Z",
      },
    }),
  );
  await page.goto("/");
  const semanticIndexCard = page.locator("article", {
    hasText: "Semantic index",
  });
  await expect(semanticIndexCard).toContainText("Building");
  await page.clock.fastForward("00:11");
  await expect(semanticIndexCard).toContainText("Ready");
  expect(capabilitiesRequests).toBeGreaterThanOrEqual(2);
});

test("@a11y has no axe violations and follows keyboard result focus", async ({
  page,
}) => {
  await page.route("**/source-manifest/capabilities", (route) =>
    route.fulfill({ json: capabilities }),
  );
  await page.route("**/source-manifest/sources", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        timestamp: "2026-07-15T12:00:00Z",
        sources: [],
      },
    }),
  );
  await page.route("**/source-manifest/summary", (route) =>
    route.fulfill({
      json: {
        namespace: "acme-workbench",
        phase: "ready",
        entity_id_format: "org.platform.domain.entity_type.source_id.entity_id",
        total_entities: 0,
        domains: null,
        predicates: null,
        timestamp: "2026-07-15T12:01:00Z",
      },
    }),
  );
  let releaseCancelledSearch!: () => void;
  const cancelledSearch = new Promise<void>((resolve) => {
    releaseCancelledSearch = resolve;
  });
  let searchRequests = 0;
  await page.route("**/code-context/search", async (route) => {
    searchRequests += 1;
    if (searchRequests === 1) {
      await cancelledSearch;
      await route.abort("aborted");
      return;
    }
    await route.fulfill({
      json: {
        contract_version: "1",
        index: { ready: true, state: "ready" },
        provenance: "embedding",
        nodes: [
          { name: "First result", path: "first.go" },
          { name: "Second result", path: "second.go" },
        ],
        misses: [],
        truncated: false,
      },
    });
  });
  await page.goto("/");

  const input = page.getByLabel("Search code");
  await expect(input).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(input).toBeFocused();
  await page.keyboard.type("evidence");
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Search" })).toBeFocused();
  await page.keyboard.press("Enter");
  const cancel = page.getByRole("button", { name: "Cancel search" });
  await expect(cancel).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(cancel).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(input).toBeFocused();
  releaseCancelledSearch();
  await page.keyboard.press("Tab");
  await expect(page.getByRole("button", { name: "Search" })).toBeFocused();
  await page.keyboard.press("Enter");
  const first = page.getByRole("button", { name: /First result/i });
  await expect(first).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(first).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(
    page.getByRole("button", { name: /Second result/i }),
  ).toBeFocused();
  const violations = await new AxeBuilder({ page }).analyze();
  expect(violations.violations).toEqual([]);
});

test("@a11y keeps degraded truth states reachable at narrow width", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  const degraded = structuredClone(capabilities);
  degraded.readiness.overall = "partial";
  degraded.readiness.source = {
    available: true,
    ready: false,
    state: "degraded",
    reason: {
      code: "source_errors_present",
      message: "One or more sources reported errors",
      retryable: true,
    },
  };
  degraded.queries.source_inventory = {
    availability: "not_ready",
    method: "GET",
    href: "/source-manifest/sources",
    reason: {
      code: "source_not_ready",
      message: "Inventory is still loading",
      retryable: true,
    },
  };
  degraded.queries.project_summary = {
    availability: "not_ready",
    method: "GET",
    href: "/source-manifest/summary",
    reason: {
      code: "source_not_ready",
      message: "Summary is still loading",
      retryable: true,
    },
  };
  await page.route("**/source-manifest/capabilities", (route) =>
    route.fulfill({ json: degraded }),
  );
  await page.goto("/");
  await expect
    .poll(() =>
      page.evaluate(
        () => getComputedStyle(document.documentElement).scrollBehavior,
      ),
    )
    .toBe("smooth");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect
    .poll(() =>
      page.evaluate(
        () => getComputedStyle(document.documentElement).scrollBehavior,
      ),
    )
    .toBe("auto");
  await expect(page.getByText("Inventory is still loading")).toBeVisible();
  await expect(page.getByText("Summary is still loading")).toBeVisible();
  await expect(
    page.getByRole("region", { name: /graph drill-down/i }),
  ).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test("@a11y renders the ready graph route with keyboard-synchronized detail", async ({
  page,
}) => {
  const ready = graphCapabilities();
  await routeWorkbenchBootstrap(page, ready);
  let graphBody: unknown;
  await page.route("**/code-context/context", async (route) => {
    graphBody = route.request().postDataJSON();
    await route.fulfill({ json: graphResponse });
  });
  await page.goto("/");
  await page.getByLabel("Graph query").fill("Alpha");
  await page.getByRole("button", { name: "Investigate graph" }).click();

  const entities = page.getByRole("list", { name: "Graph entities" });
  await expect(entities).toBeVisible();
  await expect(entities.getByRole("button")).toHaveCount(2);
  expect(graphBody).toEqual({ query: "Alpha", want: ["graph"] });
  await expect(
    page.getByTestId("sigma-graph").locator("canvas"),
  ).not.toHaveCount(0);
  await expect(
    page.getByRole("alert", { name: "Graph visualization" }),
  ).toHaveCount(0);

  const beta = entities.getByRole("button", { name: /^Beta/ });
  await beta.focus();
  await page.keyboard.press("Enter");
  await expect(beta).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("region", { name: "Selected entity details" }),
  ).toContainText("Beta");
  await expect(page.getByLabel("Graph query")).toBeVisible();
  const overflow = await horizontalOverflowReport(page);
  expect(
    overflow.documentWidth,
    `horizontal overflow: ${JSON.stringify(overflow)}`,
  ).toBeLessThanOrEqual(overflow.viewportWidth);
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test("keeps graph not-ready separate and does not call its route", async ({
  page,
}) => {
  const notReady = graphCapabilities();
  notReady.queries.graph_projection = {
    availability: "not_ready",
    method: "POST",
    href: "/code-context/context",
    reason: {
      code: "structural_index_not_ready",
      message: "Graph projection is still building",
      retryable: true,
    },
  };
  await routeWorkbenchBootstrap(page, notReady);
  let requests = 0;
  await page.route("**/code-context/context", (route) => {
    requests += 1;
    return route.abort();
  });
  await page.goto("/");
  await expect(
    page.getByRole("region", { name: "Graph drill-down" }),
  ).toContainText("Graph projection is still building");
  expect(requests).toBe(0);
});

test("cancels an in-flight graph request without publishing stale state", async ({
  page,
}) => {
  await routeWorkbenchBootstrap(page, graphCapabilities());
  let release!: () => void;
  const held = new Promise<void>((resolve) => (release = resolve));
  let requests = 0;
  await page.route("**/code-context/context", async (route) => {
    requests += 1;
    if (requests === 1) {
      await held;
      await route.abort("aborted");
      return;
    }
    await route.fulfill({ json: graphResponse });
  });
  await page.goto("/");
  await page.getByLabel("Graph query").fill("Alpha");
  await page.getByRole("button", { name: "Investigate graph" }).click();
  await page.getByRole("button", { name: "Cancel" }).click();
  release();
  await expect(page.getByRole("list", { name: "Graph entities" })).toHaveCount(
    0,
  );
  await page.getByRole("button", { name: "Investigate graph" }).click();
  await expect(
    page.getByRole("list", { name: "Graph entities" }),
  ).toBeVisible();
});

test("falls back to the accessible graph surface when WebGL initialization fails", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (type, ...options) {
      if (String(type).includes("webgl")) return null;
      return original.call(this, type, ...options);
    } as typeof HTMLCanvasElement.prototype.getContext;
  });
  await routeWorkbenchBootstrap(page, graphCapabilities());
  await page.route("**/code-context/context", (route) =>
    route.fulfill({ json: graphResponse }),
  );
  await page.goto("/");
  await page.getByLabel("Graph query").fill("Alpha");
  await page.getByRole("button", { name: "Investigate graph" }).click();
  await expect(
    page.getByRole("alert", { name: "Graph visualization" }),
  ).toContainText("visualization unavailable");
  await expect(
    page.getByRole("list", { name: "Graph entities" }),
  ).toBeVisible();
});
