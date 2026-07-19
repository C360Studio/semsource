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
and search evidence, keyboard result/detail selection, the capability-advertised
graph state, and terminal JSON 404s for retired paths.

Released mode rejects `latest` even when digest-qualified. Before starting the
stack it proves the Compose-rendered UI image equals `SEMSOURCE_UI_IMAGE`; after
start it proves the running UI container's configured image equals the same exact
tag-plus-digest. CI additionally verifies the published tag, multi-platform
manifest, OCI version/full revision labels, and local `RepoDigest` before passing
that exact reference into `task ui:smoke`.

Failure screenshots, videos, traces, and page snapshots survive the disposable
runner under `test-results/ui-profile/<compose-project>/`. To verify the failure
diagnostics deliberately, run `UI_PROFILE_FORCE_FAILURE=1 task ui:smoke:dev`;
the command is expected to fail after loading the real shell and prints bounded
HTTP snapshots for the shell, health, and capability endpoints before cleanup.
Trusted CI failures additionally upload the preserved profile diagnostics; a
successful release-smoke run records the authoritative run URL and attempt in
its summary and evidence artifact.

## First Trusted Released-Image Evidence

Trusted `main` UI publish/smoke jobs for revision
`25b2816d14a147c1d6eb7b54e40668b51ba3574a` published and verified:

- exact image:
  `ghcr.io/c360studio/semsource-ui:sha-25b2816d14a147c1d6eb7b54e40668b51ba3574a@sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`
- platforms: `linux/amd64` and `linux/arm64`
- local `RepoDigest`:
  `ghcr.io/c360studio/semsource-ui@sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`
- Compose-rendered and running-container `Config.Image`: the same exact image
  reference
- [Actions run 29693062800](https://github.com/C360Studio/semsource/actions/runs/29693062800),
  attempt 1; all six jobs green, including `build-and-push` and
  `ui-release-smoke`, with released browser profile 6/6
- [evidence artifact 8444245976](https://github.com/C360Studio/semsource/actions/runs/29693062800/artifacts/8444245976)

This evidence records the completed successful trusted publication and
release-smoke workflow.
