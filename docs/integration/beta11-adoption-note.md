# SemSource beta.11 — consumer adoption note

Audience: SemTeams, SemDragon, and any deployment moving from `v1.0.0-beta.10` to `v1.0.0-beta.11` (SemEngine
`v0.1.0-alpha.8`). This is the delta since beta.10; the tag is tracked on #389. Coming from beta.9 or earlier, read
the [SemEngine adoption note](semengine-adoption-note.md) first.

## What you must do

**Provision fresh storage and re-ingest.** Entity IDs changed and a beta.10 `GRAPH` stream refuses boot. There is no
alias, rewrite, or migration. For Compose:

```bash
docker compose down -v   # removes NATS volumes
docker compose up -d     # fresh provision + reseed from source
```

Wait for `/source-manifest/status` → `phase: "ready"` before querying. Three reasons an in-place upgrade is not enough:

1. Code and config-file entity IDs changed (next section). IDs are never aliased or rewritten, so beta.10's entities
   would stay in `ENTITY_STATES` beside the new ones.
2. A server still holding the beta.10 `GRAPH` stream refuses boot ([GRAPH stream](#graph-stream)).
3. A hybrid entity from beta.10 does not heal on restart: the first pass after a start never reconciles
   ([why](../migration/ast-entity-identity.md#how-to-migrate)).

`nats stream rm GRAPH` clears only the second. It leaves the old IDs, so it is not a substitute for the reseed.

## Entity-ID changes since beta.10

Anything that stores or hard-codes an entity ID must re-resolve it after the reseed.

- **Code: one ID per declaration** (#329/#337, #338/#356). Java methods and constructors gain a signature
  discriminator; TS/JS/Svelte accessors, statics and namespace members get distinct IDs; Python setters and deleters
  get `-setter`/`-deleter`. Locals, Go `init` and blank declarations, and vendored twins of minified files are no
  longer entities. Details: [migration note](../migration/ast-entity-identity.md).
- **TS/Svelte `declare module "<specifier>"` members** (#340/#379) are scoped by a segment built from the raw
  specifier (`module-m-4c89ba36-0` for `"m"`), so `"foo.bar"`, `"foo/bar"` and `namespace foo.bar` stay apart.
- **Config files are keyed by their declaring file** (#348/#374). Every config-file entity ID (go.mod, package.json,
  pom.xml, Gradle, Dockerfile) gains an 8-hex hash of the manifest's path relative to the source root, so two
  manifests naming one coordinate or image write two entities. IDs no longer depend on the checkout location.
- **One config entity per artifact per manifest** (#375/#383). Maven scopes and Gradle configurations become sorted
  multi-valued `source.config.dependency-scope` / `dependency-configuration` facts; npm prod wins over dev; Go
  `indirect` is the AND. A Maven `<type>` other than `jar` and a `<classifier>` enter the instance. Image IDs hash
  the raw reference after the slug, so `foo/bar:1` and `foo-bar:1` stay apart.
- **POM `<module>` IDs** (#384/#392) hash the raw name after the slug, so `my_module` and `my-module` stay apart.

Checked and **not** identity-breaking, so they need no reseed on their own:

- Java calls inside anonymous and local class bodies no longer bind to the enclosing class (#339/#342; edges only).
- A git source with a configured repo URL keeps it when it re-ingests after a HEAD change, so commits, authors and
  branches stop landing under a second system (#350/#370; no format change).
- A git HEAD change whose re-ingest fails is retried on the next poll instead of lost (#355/#359).
- AST folder `contains` sets stay complete under watch edits (#347/#358).

## Config refusals

A config whose sources would mint IDs under one org and system now fails before anything publishes. Beta.10 booted
all of these. Each pair silently replaced the other's entities, except that config-file roots were partly kept apart
by a checkout-path hash that #348 removed. `semsource validate` and `semsource run` name both sources and roots, then
print `would share entity-ID system "<system>" in org "<org>"` and the remedy for the kind:

- **`ast`, `docs`, `s3`** (#349/#365): same `project` and `version`; with no `project`, the same root directory name
  (the bucket for `s3`). Remedy: give each source a distinct `project`, `bucket` or `version`.
- **`image`, `audio`, `video`**, each kind alone (#366/#367, #369/#382): same root directory name, since media
  sources have no `project`. Remedy: distinct, non-overlapping paths whose directory names differ.
- **`git`** (#354/#371): same repo URL slug, else same repo directory name. Remedy: a distinct repo URL, or a repo
  directory with a distinct name.
- **`config`** (#348/#374): same `project`, or the same root directory name with case and dots folded. Remedy: a
  distinct `project`.

Rules that apply to every kind:

- `docs` and `s3` share one ID space. Two entries that build one component instance from different roots are
  refused whatever their systems.
- A runtime `add_source` that would collide is refused with `validation_failed` and nothing is persisted.
- The one admitted overlap is a standalone source at a submodule pin's `project` and `version`, when its checkout is
  at the pinned commit with the pin's parse options (the multi-repo [quickstart](../QUICKSTART.md) dedup).
- Boot also checks persisted `add_source` entries and discovered branches. On a collision it exits with
  `refusing to boot: ...`. Remove a file source from the config file. Remove a persisted `add_source` entry with the
  new `semsource remove --manifest <instance>`, run while the service is stopped (needs NATS; #365).
- `project` and `version` are part of the system segment. Change them before the reseed, not after.

## Substrate

- **SemEngine `v0.1.0-alpha.8`** (#378), Tier 0 by default. Tier 1 stays opt-in (`graph.semantic_search`, see
  [`configs/tiers/README.md`](../../configs/tiers/README.md)); it was qualified on alpha.7 (#327) and the shipped
  config now declares `dimensions: 384` so a restart keeps stored vectors (#378; measurement on SemSource is post-tag,
  #387). At Tier 1 only, `graph_search` relevance is a rank score of at most 2/61 with `word_rank`/`vector_rank` and
  a `merge` block, so a client that thresholds `relevance` on the word scale breaks, and `embedding.ready` also waits
  for graph-embedding (#327).
- **NATS `max_pending` 512 MiB.** If you run your own NATS instead of the bundled Compose service, set
  `max_pending: 536870912` ([`configs/nats.conf`](../../configs/nats.conf), #351/#357). alpha.8 requires it: a warm
  restart replays the whole `ENTITY_STATES` bucket to several readers over one connection. Without it NATS logs
  `Slow Consumer Detected: MaxPending of 67108864 Exceeded` and SemSource fails to start. Compose mounts the file.

## GRAPH stream

`GRAPH` now uses workqueue retention (#160/#394): a message is removed once graph-ingest has applied it. It holds
transport only, never graph state; that lives in `ENTITY_STATES`.

- **Boot refuses over a beta.10 `GRAPH`.** The error contains `reconcile stream "GRAPH" failed: existing stream
  diverges in a field that cannot be reconciled in place` and `refuses a retention change to or from "workqueue"`.
  The stream is left untouched. `docker compose down -v` (already required) or `nats stream rm GRAPH` clears it.
- **Why:** on `main` before #394, a SemSource-only restart within the stream's one-hour age bound republished the
  corpus into a stream still holding the first seed. On the OSH corpus (about 28k entities) it hit the 256 MiB
  ceiling, dropped entities, and never returned to `ready`. After #394 the same restart reached `ready` in about
  150 s with no refusals (#388, one measurement on a loaded laptop).
- **External JetStream consumers on `GRAPH` are unsupported.** A consumer's acknowledgements delete messages for
  every reader. Read `ENTITY_STATES`, graph query, or `/source-manifest/status`; gate readiness on the status
  request or HTTP poll, not a `GRAPH` subscription
  ([consumer guide](m5-consumer-integration.md#option-a-do-not-consume-the-graph-stream)).

## Behavior you can observe

- **Re-ingest removes predicates a source stopped emitting** (#336/#346; config files #384/#392). Opted in: ast, doc,
  url, image, audio, video, object-store, config. Not git. It reconciles only when the process knows the content
  changed (a watch edit, a reindex whose hash differs, a url or config republish whose statements differ from the
  last sent), never on the first pass after a start, so an edit made while SemSource was down persists until the
  file's next edit (semengine#730). Lineage edges, stale markers, `entity.ontology.class` and other sources'
  statements are untouched, and entities are not retracted: dropping a `go.mod` requirement removes the module's
  `source.config.requires` edge, not the dependency entity.
- **Statement timestamps** on doc, url, image, audio, video, object-store and config entities are graph-ingest's
  envelope time, not the handler's index time.
- **Natural-language resolves without a search index** (#291/#326): with `graph.lexical_search: false`, every nl
  resolve (`code_search`, and `doc_context`, which resolves in nl mode) answers `not_enabled`, HTTP `501`,
  `retryable: false`. MCP fusion tools name the code in the tool error. Symbol and prefix resolves still work.
- **Status** gains per-source `id_collisions` (#337), the count of duplicate IDs the AST source dropped; strict
  decoders must accept it. At Tier 0 the readiness contract is unchanged: gate on `phase: "ready"`.
- **Config:** ast sources accept `index_vendored_js` (#337), which indexes an unminified twin of a minified file.
- **Known limit:** `graph_search` shows only the first value of a multi-valued dependency fact (#385).

Questions: file a SemSource issue that references #386.
