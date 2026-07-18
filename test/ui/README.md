# UI Profile Smoke

This Playwright smoke belongs to SemSource and checks the SemSource-owned
`docker compose --profile ui` contract. It intentionally asserts only the shared
workbench shell and SemSource routes, not consumer-product workflows.

Smoke an immutable released image without host Node or a sibling checkout:

```bash
SEMSOURCE_UI_IMAGE=ghcr.io/c360studio/semsource-ui:<version>@sha256:<digest> task ui:smoke
```

Build and smoke the local `./ui` development image explicitly:

```bash
task ui:smoke:dev
```

Or run only the Playwright assertions after the profile is already up:

```bash
SEMSOURCE_UI_IMAGE=<immutable-ref> docker compose --profile ui up
task ui:e2e
```

When the running stack uses a custom Compose project, export the same
`COMPOSE_PROJECT_NAME` before `task ui:e2e` so the runner joins that project's
network.

The task builds the locked Playwright runner in `test/ui/Dockerfile` and joins
the running Compose network. It does not require host Node or `node_modules`.
The suite exercises desktop and narrow widths, real advertised SemSource routes
and search evidence, keyboard result/detail selection, the graph-unavailable
state, and terminal JSON 404s for retired paths.

Failure screenshots, videos, traces, and page snapshots survive the disposable
runner under `test-results/ui-profile/<compose-project>/`. To verify the failure
diagnostics deliberately, run `UI_PROFILE_FORCE_FAILURE=1 task ui:smoke:dev`;
the command is expected to fail after loading the real shell and prints bounded
HTTP snapshots for the shell, health, and capability endpoints before cleanup.
