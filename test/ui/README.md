# UI Profile Smoke

This Playwright smoke belongs to SemSource and checks the SemSource-owned
`docker compose --profile ui` contract. It intentionally asserts only the shared
operator shell and SemSource routes, not SemTeams-specific workflows.

Run the full start/test/teardown smoke:

```bash
task ui:smoke
```

The one-command smoke preflights `UI_CONTEXT`, `Dockerfile.dev`, and the local
Playwright install before starting Docker.

Or run only the Playwright assertions after the profile is already up:

```bash
docker compose --profile ui up
task ui:e2e
```

The task uses Playwright from the SemTeams UI checkout (`../semteams/ui` by
default). Override `UI_CONTEXT` or `C360_PORT` when using a different checkout or
port.
