# Design: Compose Packaging Hardening

## Context

The cold-start audit graded the happy path good and the edges broken, with each break living in
a different packaging layer:

- **Config resolution.** Compose mounts `./configs` → `/etc/semsource` and runs
  `--config /etc/semsource/${SEMSOURCE_CONFIG:-mvp.json}`. Tier files live under
  `configs/tiers/`, so the README's `SEMSOURCE_CONFIG=tier0-statistical.json` resolves a
  nonexistent path; the tier configs themselves point sources at `/mnt/workspace/myrepo`, not
  the `/workspace` the stack actually mounts, and `configs/tiers/README.md` asserts the paths
  "match what docker compose mounts" — false on both counts.
- **Liveness.** The semsource healthcheck runs `semsource version` — a fresh process that exits
  0 regardless of whether the long-running service answers anything. NATS and semembed already
  do HTTP healthchecks; semsource is the outlier, and Caddy gates on its theater.
- **Durability.** The nats service has JetStream on with no volume (`-js` defaults its store to
  the container filesystem) and is the only service with no restart policy. Recreate the
  container, lose the graph, stack still reports healthy.
- **Default coverage.** `configs/mvp.json` declares `language: go`. The CLI `SourceEntry` carries
  a single `Language` string, while the ast-source component (`watch_paths[].languages`) and the
  parser registry (go, typescript, javascript, java, python, svelte) are already plural —
  the restriction is an artifact of the CLI schema, not the engine.
- **Identity/pins.** semembed rides mutable `:latest`; the Dockerfile takes `ARG VERSION=dev`
  but plain `docker compose up` never passes it, so every compose build answers `semsource
  version` with `dev`. `.dockerignore` excludes `.git`, so the build can't derive a version
  itself today.

Constraint: this is pure product packaging — no substrate involvement — and the one-command
happy path (`docker compose up`, ready in minutes) must not get slower or more demanding.

## Goals / Non-Goals

**Goals:**

- Every file in `configs/` and `configs/tiers/` boots under the shipped compose stack exactly
  as its documentation states.
- `healthy` means "answers HTTP", state survives container recreation, and the stack comes back
  after a daemon restart.
- The one-command install indexes every language the AST source supports.
- Shipped images are immutably pinned; a compose-built `semsource version` identifies its build.
- The compose smoke fails if any of the above regresses.

**Non-Goals:**

- Auto-detecting workspace languages by scanning (explicit list is deterministic and
  self-documenting; detection can layer on later if the explicit default proves costly).
- TLS/reverse-proxy/multi-host hardening (MVP remains same-LAN; unchanged).
- semembed/seminstruct image *contents* — pinning only; their release flow is another repo.
- The ui profile's image pin discipline (already digest-enforced by `SEMSOURCE_UI_IMAGE`).

## Decisions

### D1: Tier configs become compose-true; the README names the real subpath

Tier configs change their source paths to the compose mount (`/workspace`, matching
`mvp.json`), and every doc instruction uses the path-relative form the command line already
supports: `SEMSOURCE_CONFIG=tiers/tier0-statistical.json` (the command builds
`/etc/semsource/<value>`, and the mount ships `configs/` recursively — no compose change
needed). `configs/tiers/README.md` is corrected to say native runs must edit paths, compose
runs point `SEMSOURCE_TARGET` at the directory to index.

*Alternative considered:* flattening tier files into `configs/` so the README's current
instruction becomes true. Rejected — loses the tiers/ grouping and its README anchor; the
instruction is the cheap side to fix.

### D2: Healthcheck = HTTP status endpoint via busybox wget

`test: wget --quiet --tries=1 --spider http://localhost:8080/source-manifest/status`, with a
`start_period` covering ServiceManager boot (30s) and the existing interval/timeout/retries
shape. The runtime image is alpine, so busybox wget is present (the nats service already relies
on exactly this). Liveness is "the serving surface answers", not readiness — a seeding stack is
healthy-but-not-ready by design (readiness stays the status payload's job, ADR-066).

### D3: NATS gets an explicit store dir on a named volume + restart policy

`command` gains `-sd /data`, a named volume `nats-data` mounts at `/data`, and the service gets
`restart: unless-stopped` like every other service. Explicit `-sd` rather than guessing the
image default: the volume target and the server's store dir are then coupled in one visible
line. This is container-lifecycle durability only — graph *retention* semantics stay ADR-0008's
(no NATS TTL/MaxBytes lifecycle, unchanged here).

### D4: Polyglot default via a plural `languages` on the CLI source entry

`config.SourceEntry` gains `Languages []string` (additive; singular `Language` remains honored,
`languages` wins when both set — validated mutually consistent). `astComponentConfig` passes
the list through to the component's `watch_paths[].languages` (already plural), and repo
expansion (`ExpandRepoSources`) propagates it, so the ADR-040 curator path (`add_source` for a
polyglot repo) gets the same fix as the local mount. `configs/mvp.json` then declares all six
registered languages explicitly. Startup already logs per-source config; the explicit list in
the config file is the "stated restriction" the spec demands, with nothing silent.

*Alternative considered:* extension-scan auto-detect. Rejected for the default path —
nondeterministic against the spec's "stated explicitly", and the explicit list costs nothing
(parsers no-op on absent extensions).

### D5: Version identity — `.git` enters the build context, Dockerfile self-describes

Remove `.git` from `.dockerignore`; the builder stage (git is already installed) derives
`VERSION=$(git describe --tags --always --dirty)` when the `VERSION` arg is left at its `dev`
default, and CI keeps passing the release tag explicitly (unchanged precedence: explicit arg >
git describe > `dev` when neither exists, e.g. a tarball build). Cost: the build context grows
by `.git` and the `COPY . .` layer invalidates per commit — that layer already invalidates on
any source change, and the mod-download cache layers are unaffected.

*Alternative considered:* compose `build.args: VERSION: ${SEMSOURCE_VERSION:-dev}`. Rejected as
the primary fix — the advertised one-command path would still stamp `dev`; kept as an override
hook since the arg plumbing exists anyway.

### D6: semembed pinned by digest, refresh documented

`ghcr.io/c360studio/semembed:latest` → `ghcr.io/c360studio/semembed:<tag>@sha256:<digest>`
(resolved at implementation time via `docker manifest inspect`; the multi-arch *index* digest,
not a platform manifest). A comment beside the pin documents the refresh command so bumping is
mechanical. The ui profile already enforces this discipline via `SEMSOURCE_UI_IMAGE`.

### D7: Smoke extends to the new guarantees inside the existing script

`scripts/core-profile-smoke.sh` (already isolated: per-PID project name, disposable workspace)
gains three assertions rather than a new script:

1. **tier0 boots**: re-up with `SEMSOURCE_CONFIG=tiers/tier0-statistical.json`, assert the
   container reaches healthy and status answers (BM25 mode, no crash loop).
2. **healthcheck truth**: assert the rendered compose config's semsource healthcheck targets
   the HTTP status endpoint (config-level proof), and that a stopped serving process flips
   health — `docker exec` kill of the semsource process, observe `unhealthy` (or restart)
   rather than perpetual `healthy`.
3. **durability**: after the existing ingest assertions, `docker compose rm -sf nats && up -d`,
   then assert previously ingested entities are still queryable.

Quick Start gains the two-line remedies next to the command: port 4222 collision →
`NATS_HOST_PORT`, stale image → `docker compose up --build` / `--pull always`.

## Risks / Trade-offs

- [Healthcheck start_period too short on slow machines → restart loops] → 30s start_period with
  the existing retries; the smoke exercises a cold boot, so a wrong bound fails CI, not users.
- [`.git` in build context slows first build slightly] → bounded (repo history is small); mod
  cache layers unaffected; explicit `VERSION` arg skips describe entirely in CI.
- [Digest pin goes stale as semembed releases] → deliberate: stale-but-working beats
  silently-moved; refresh command documented at the pin site.
- [Six-language default parses more files on big polyglot repos] → parsers only walk matching
  extensions; the audit's four-minute happy path is Go-only either way (semsource's own repo);
  tier docs note `languages` can be narrowed per source.
- [Kill-based health assertion could be flaky across Docker versions] → the config-level
  assertion (healthcheck targets HTTP) is the hard gate; the behavioral kill check is
  best-effort with a generous window, and skippable-with-notice if the runtime lacks the
  needed signal semantics.

## Migration Plan

All additive or config-file changes; no wire, ID, or CLI breaks. Existing installs pick up the
volume/healthcheck/pins on their next `docker compose up` (a fresh `nats-data` volume starts
empty — same data loss they were always one recreate away from; release notes call it out).
Rollback = revert the compose/config files.

## Open Questions

- None blocking. If semembed publishes proper release tags before implementation lands, pin
  `:<version>@sha256:...` instead of `:latest@sha256:...` for readability.
