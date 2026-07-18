import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

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

test("renders the owned SemSource shell truthfully", async ({ page }) => {
  let searchBody: unknown;
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
  await expect(
    page.getByRole("status", { name: /overall readiness/i }),
  ).toContainText("Partial");
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
