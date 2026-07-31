## 1. Freeze the migration contract

- [x] 1.1 Record the observed starting pin and commit separately from the migration target: SemSource
  at `v1.0.0-beta.158`, target `v1.0.0-beta.159`, with the SemStreams commit for the target tag.
  - Test: the evidence envelope's dependency-transition row names both tags and both commits, and
    `go list -m github.com/c360studio/semstreams` matches the recorded starting pin before any edit.
- [x] 1.2 Derive the beta.159 framework-owned bucket set from the SemStreams KV catalog at the target
  tag and diff it against the inventory literal in `test/e2e/beta148_cutover_test.go`.
  - Test: the diff reports `EMBEDDINGS_CACHE` as removed and names every added bucket, and the
    existing parity assertion fails against the pre-migration literal for exactly those reasons.

## 2. Adopt beta.159 and conform the composition

- [x] 2.1 Pin `github.com/c360studio/semstreams v1.0.0-beta.159`, keep the module free of any
  `replace` directive, and run `go mod tidy`.
  - Test: `go list -m github.com/c360studio/semstreams` reports beta.159, `go.mod` has no matching
    `replace`, and a second `go mod tidy` is a no-op.
- [x] 2.2 **BREAKING** Delete the graph-embedding `ports.outputs` block from `cmd/semsource/run.go`,
  leaving the `entity_watch` input port untouched.
  - Test: a focused composition test builds the graph-embedding component config and asserts creation
    succeeds; the pre-fix config fails with `graph-embedding declares no output ports`.
- [x] 2.3 Assert every KV port subject SemSource declares resolves to a beta.159 catalog bucket,
  covering the graph-ingest, graph-index, and graph-clustering blocks.
  - Test: a table test over the rendered component composition resolves each declared KV subject
    against the framework catalog and fails on any unresolvable subject.
- [x] 2.4 **BREAKING** Update the cutover rehearsal inventory in `test/e2e/beta148_cutover_test.go` to
  the beta.159 catalog, keeping the parity assertion first so a future catalog change still stops the
  rehearsal.
  - Test: `go test -tags=e2e -run Beta148 ./test/e2e/` passes, and mutating the literal by one bucket
    makes the parity assertion fail.
- [x] 2.5 Confirm the build baseline is unchanged by the pin, and record explicitly that this proves
  compilation only, not conformance.
  - Test: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, and
    `go vet -tags=e2e ./...` are all clean.
- [x] 2.6 **BREAKING** Declare the `GRAPH` ingest-transport stream's `discard` policy (`new`) alongside
  its existing `max_age`/`max_bytes`, and expose a `streams.GRAPH.discard` operator override.
  beta.159 requires all three fields on an ordinary stream; the pre-migration audit missed this
  because the first two were already declared. See design D7 for why `new` and not `old`.
  - Test: `go test ./internal/sourcespawn/` passes — before the fix, component writes fail config
    validation with `ordinary stream bounds are not declared: stream "GRAPH" ... missing max_age,
    max_bytes, discard`.
- [x] 2.7 Give the `internal/sourcespawn` fixture the same `GRAPH` stream declaration production
  builds, so it validates a config shape that can actually exist at runtime.
  - Test: the sourcespawn suite passes for every flat source type (ast, docs, config, url, image, git),
    each of which declares a `graph.ingest` output port.

- [x] 2.8 **BREAKING** Read index readiness from the `GRAPH_STATUS` KV bucket instead of the removed
  `graph.index.query.status` subject, via a shared `internal/graphstatus` reader that binds
  must-exist through the catalog seam and treats an absent key as unknown rather than not-ready.
  Covers both production call sites (`source-manifest`, `supersession`). See design D9.
  - Test: against a live stack, `/source-manifest/status` reports
    `index: {available:true, ready:true}`; before the fix the same stack reported
    `{available:false, state:"unknown", reason:"status_unavailable"}` while `GRAPH_STATUS/graph-index`
    carried `ready:true`.
- [x] 2.9 Prove the acquisition seam classifies a missing owner as retryable-not-ready naming the
  owner, and an off-catalog bucket as permanently invalid.
  - Test: `go test -tags=integration ./internal/graphstatus/` — the first yields `framework bucket
    "GRAPH_STATUS" is not ready: its owner (graph-index/graph-embedding ...) has not provisioned it`,
    the second yields a classified invalid error naming the off-catalog name.

- [x] 2.10 **BREAKING** Add `workspace.IsPathReady` and use it in `doc-source`/`cfgfile-source`, so a
  source that legitimately names a FILE (`README.md`, `go.mod`) can finish `Start`. beta.159's
  component-start barrier turns a component that never completes `Start` into a total boot failure;
  both sources gated ingest on the directory-only `IsRepoReady` and retried forever. Pre-existing
  SemSource bug, made fatal by the migration. See design D10.
  - Test: `go test ./workspace/ -run TestIsPathReady` covers file/directory/in-progress-clone/missing;
    `go test -tags=e2e -run TestE2E_NativeQuickStart ./test/e2e/` passes (it failed on this branch and
    passed at `main`, which is how the regression was isolated).

## 3. Align the deployment substrate

- [x] 3.1 **BREAKING** Pin the Compose NATS service to `nats:2.12-alpine` in `docker-compose.yml`,
  matching the server line SemStreams tests against.
  - Test: `docker compose config` renders `nats:2.12-alpine` with no floating tag, and `task
    core:smoke` starts the stack and reaches source-manifest ready.
- [x] 3.2 Apply the same server pin to the untracked tier-2 development Compose overlay if it is
  retained, so no local path silently runs a different server line.
  - Test: every `docker-compose*.yml` in the repo renders an exact NATS version tag; a grep for
    `nats:2-alpine` returns nothing.

## 4. Execute the local cutover

- [x] 4.1 Stop every writer against the local deployment and capture the literal NATS account
  inventory (streams, KV buckets, object stores) before any deletion.
  - Test: the captured inventory is attached to the evidence envelope and no `semsource` process
    holds a connection at capture time.
- [x] 4.2 **BREAKING** Execute the catalog-derived deletion — `semstreams_config`, observed `GRAPH`,
  every enabled observed catalog bucket, and `PREDICATE_CATALOG` only if observed — preserving
  authoritative source inputs, source/content/media stores, and unrelated state.
  - Test: the post-deletion inventory contains no resource from the deletion sheet and every resource
    on the preservation list, with no wildcard deletion issued.
  - **Result: the deletion set was EMPTY.** The local Docker environment held no volumes and no
    containers, so no pre-beta.159 graph state existed to delete. Recorded as an observed empty
    inventory rather than a performed wipe — the procedure itself is proven by the beta148 cutover
    rehearsal, which executes it end to end against a disposable account.
- [x] 4.3 Start only migrated writers, reseed from canonical source inputs, and wait for readiness.
  - Test: the source-manifest status surface reaches `phase: ready` with every configured source
    seeded, and the first-boot log carries the expected `ENTITY_STATES` History reconcile warning.
- [x] 4.4 Prove query parity with a canonical known-answer query, then restart once with no
  intervening write and prove replay parity.
  - Test: the known-answer query returns the expected result before and after the restart, byte-equal
    on the asserted fields.

## 5. Verify the silent behavioral contracts

- [x] 5.1 Verify add-lane six-field tuple deduplication against SemSource's producers and reconcile
  any distinct-entity count movement as a finding rather than a re-baseline.
  - Test: entity/triple counts on the status surfaces are stable across a republication cycle, and
    any delta from the pre-migration baseline is explained in the evidence envelope.
- [x] 5.2 Verify that a read path against an unprovisioned bucket surfaces the framework's classified
  not-ready error naming the owner, and that SemSource branches on the error class rather than the
  code string.
  - Test: an integration test starts a reader without its owning component and asserts the classified
    `index_not_ready` error and a backoff retry, not an empty-result path.
- [x] 5.3 Verify graph-clustering under a tier-2 config: `COMMUNITY_INDEX` remains the declared output
  and becomes trigger-only, with summaries landing in `COMMUNITY_SUMMARIES`.
  - Test: with clustering enabled, both buckets are present and populated as expected, and local/global
    summary query routes still answer.

## 6. Review and release gates

- [x] 6.1 Run the full gate set and record results.
  - Test: `task check`, `task test:race`, `task test:e2e`, and `task core:smoke` all pass, with revive
    clean (warnings fail).
  - **Result:** lint PASS (revive v1.15.0), unit 41/41, race 41/41, integration 42/42, e2e PASS
    (156s), `task core:smoke` PASS — the last of which blocks on `index.ready` and was unreachable
    before task 2.8.
- [x] 6.2 Complete the evidence envelope required by the sister-repo cutover checklist — product
  identity, dependency transition, deployment identity, composition, wipe, reseed/rebuild,
  verification, and exceptions.
  - Test: every envelope field is populated from a clean tree at the final migration commit; no field
    is filled from diagnostic-only or dirty-tree evidence.
  - **Result:** `openspec/changes/migrate-semstreams-beta-159-lockstep/evidence.md`, including an
    explicit "Not claimed" section for the History reconcile that could not be observed on empty
    state.
- [ ] 6.3 Report adoption against semstreams gh#753 and file any framework problem found during
  adoption as a new semstreams issue referencing it.
  - Test: the gh#753 comment names SemSource's migration commit and evidence; any filed issue is
    linked from the envelope's exceptions row, or that row reads `none`.
- [x] 6.4 Update `CLAUDE.md`'s roadmap pin and `docs/upstream/semstreams-asks.md` if adoption produced
  new upstream asks.
  - Test: the roadmap names beta.159 as the current target and no stale beta.158 reference remains in
    the roadmap section.
