# Proposal: Compose Packaging Hardening

**Priority: P1** — install is the first thing a Graphify-comparing prospect touches

## Why

The cold-start audit found the happy path genuinely good (one command, ready in under four
minutes) and the edges genuinely bad — several documented paths are broken or silently fragile:

1. **README's tier0 instruction crash-loops**: `SEMSOURCE_CONFIG=tier0-statistical.json` points at
   a nonexistent container path (README.md:302; tier files live under `configs/tiers/` but the
   compose mount + command resolve `/etc/semsource/<name>`); `configs/tiers/README.md` claims the
   paths "match what docker compose mounts" — false (5 of the 10 bad doc claims in the audit are
   in this file).
2. **The semsource healthcheck proves nothing**: `semsource version` (docker-compose.yml:62) exits
   0 while the service could be wedged; compose "healthy" gates (Caddy depends_on) are theater.
3. **NATS has no data volume** (docker-compose.yml:145): recreating the container silently wipes
   all graph state under a still-healthy stack; it is also the only service with no restart policy.
4. **The default config indexes Go only** (configs/mvp.json:4): a polyglot repo gets a silently
   code-free (or Go-only) graph from the advertised one-command install.
5. Supply-chain/support hygiene: semembed pinned to mutable `:latest`; the compose-built
   `semsource` image reports version `dev` (no version/commit for support triage); stale local
   images are silently reused by `docker compose up` (bit the audit itself); port-4222 collisions
   produce a raw bind error with the remedy documented far from the Quick Start.

## What Changes

- Tier configs work under compose exactly as documented: mount + `SEMSOURCE_CONFIG` resolve every
  file shipped in `configs/` and `configs/tiers/`; `configs/tiers/README.md` corrected to match.
- Healthcheck hits a real liveness surface (HTTP status endpoint) instead of running the binary.
- NATS gets a named data volume and a restart policy consistent with the other services.
- Default `mvp.json` indexes all languages the AST source supports for the mounted workspace (or
  auto-detects), so polyglot repos work from the one-command install; the choice and its cost are
  documented.
- semembed pinned by digest; the compose build stamps version/commit via ldflags so
  `semsource version` identifies the running build; Quick Start gains the two-line "port already
  allocated" and "stale image — use --build/--pull" remedies next to the command.

## Capabilities

### New Capabilities
- `compose-deployment`: the shipped compose stack is durable (state survives container recreation),
  honestly health-checked, reproducible (pinned images, identifiable builds), and every documented
  config path boots.

### Modified Capabilities
- `advertised-surface-coverage`: "Default Docker Compose profile has a core smoke" extends to the
  tier0 path (boot with `SEMSOURCE_CONFIG=tier0-statistical.json`) and a healthcheck-tells-truth
  scenario.

## Impact

- `docker-compose.yml`, `docker-compose.ui-dev.yml`, `Dockerfile` (ldflags), `configs/mvp.json`,
  `configs/tiers/*`, README.md, `configs/tiers/README.md`, smoke scripts in `scripts/`.
- Consumers: every new install; semdev harness reproducibility.
- Boundary check: pure packaging/product; no substrate involvement.
