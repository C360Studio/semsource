## 1. Config paths boot as documented (D1)

- [x] 1.1 Point tier config sources at the compose mount (`/workspace`, matching mvp.json) and
      correct `configs/tiers/README.md` (compose = `SEMSOURCE_TARGET`; native = edit paths).
      Proves: config-validate test that loads every `configs/**/*.json` through
      `config.LoadConfig` successfully (paths well-formed, no schema drift).
- [x] 1.2 Fix every doc instruction to the resolvable form
      (`SEMSOURCE_CONFIG=tiers/tier0-statistical.json`) — README.md and configs/tiers/README.md.
      Proves: doc-claims test greps the docs for `SEMSOURCE_CONFIG=` values and asserts each
      referenced file exists under `configs/`.

## 2. Honest liveness + durable state (D2 + D3)

- [x] 2.1 Replace the semsource healthcheck with the HTTP status probe (busybox wget,
      start_period 30s); keep interval/timeout/retries shape. Proves: smoke assertion that the
      rendered `docker compose config` healthcheck targets `/source-manifest/status` (task 5.2).
- [x] 2.2 NATS: add `-sd /data`, named volume `nats-data:/data`, `restart: unless-stopped`.
      Proves: smoke durability round-trip (task 5.3).

## 3. Polyglot default install (D4)

- [x] 3.1 Add `Languages []string` to `config.SourceEntry` (additive; singular `Language` still
      honored, plural wins; both-set consistency validated). Proves:
      `TestSourceEntry_LanguagesPrecedence` in config.
- [x] 3.2 `astComponentConfig` passes the list through to `watch_paths[].languages`;
      `ExpandRepoSources` propagates `Languages` so add_source repo entries get parity. Proves:
      `TestASTComponentConfig_MultiLanguage` + repo-expansion propagation test.
- [x] 3.3 `configs/mvp.json` declares all six registered languages explicitly. Proves: test
      asserting mvp.json's ast entry languages == the parser registry's registered names (drift
      gate).

## 4. Pinned + identifiable images (D5 + D6)

- [x] 4.1 Remove `.git` from `.dockerignore`; Dockerfile derives
      `git describe --tags --always --dirty` when `VERSION` is left at `dev` (explicit arg still
      wins; no `.git` → `dev`). Proves: smoke asserts `semsource version` in the built container
      does not report bare `dev` (task 5.1 boot logs).
- [x] 4.2 Pin semembed by immutable digest (multi-arch index digest, refresh command in a
      comment beside the pin). Proves: smoke/test asserting no `:latest`-without-digest image
      refs in the rendered core-profile compose config.
- [x] 4.3 Quick Start gains the port-collision (`NATS_HOST_PORT`) and stale-image
      (`--build` / `--pull always`) remedies beside the up command.

## 5. Smoke coverage (D7)

- [x] 5.1 Extend `scripts/core-profile-smoke.sh`: tier0 path boots
      (`SEMSOURCE_CONFIG=tiers/tier0-statistical.json` → healthy + status answers, no crash
      loop) and the built image identifies itself (non-`dev` version in `semsource version`).
- [x] 5.2 Smoke asserts the rendered semsource healthcheck targets the HTTP status endpoint;
      best-effort behavioral check (kill serving process → not perpetually healthy) with a
      generous window, skip-with-notice if runtime semantics prevent it.
- [x] 5.3 Smoke durability round-trip: ingest → `docker compose rm -sf nats` → `up -d` →
      previously ingested entities still queryable.
- [x] 5.4 Run the full extended smoke green locally (isolated project name + high ports per the
      harness rule) and record the evidence in this change.

**Extended smoke evidence (2026-07-19, isolated project `semsource-cph-smoke` on
28080/24222/28222, exit 0):** core ready (5 entities); lifecycle round-trip add →
observable removal → unknown NOT_FOUND (`doc-source-docs`); built image identifies itself
(`semsource v1.0.0-beta.5-3-g5ae1296-dirty` — git-describe, not `dev`); healthcheck targets
the serving surface and discriminates serving from non-serving; no `:latest`-without-digest
images; graph state survived `docker compose rm -sf nats` + re-up (`code_context` answered
post-recreate); `SEMSOURCE_CONFIG=tiers/tier0-statistical.json` booted to ready (BM25, no
crash loop). Bonus finds fixed en route: the MCP `add_source` tool never mapped its flat
`path` onto `Paths` for docs/config/media (the advertised call was an unconditional
VALIDATION_FAILED — the #81 smoke round-trip had never actually been runnable), and the
smoke's SSE content assertions missed escaped quotes.
