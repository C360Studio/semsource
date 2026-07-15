# UI Profile Smoke

This Playwright smoke belongs to SemSource and checks the SemSource-owned
`docker compose --profile ui` contract. It intentionally asserts only the shared
workbench shell and SemSource routes, not consumer-product workflows.

Run the full start/test/teardown smoke:

```bash
task ui:smoke
```

The one-command smoke preflights the SemSource-owned `ui/Dockerfile` and local
Playwright install before starting Docker.

Or run only the Playwright assertions after the profile is already up:

```bash
docker compose --profile ui up
task ui:e2e
```

The task uses Playwright from `ui/node_modules`. Run `npm --prefix ui ci` to
install it. Override `C360_PORT` when the profile uses a different host port.
