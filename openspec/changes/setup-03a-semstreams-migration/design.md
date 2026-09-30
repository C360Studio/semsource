# SETUP 03A design

## Boundary and order

Run the known-answer workload before the dependency changes. Freeze it for comparison; invalid fixture
fixes require a new corpus version and both reruns. Selected upstream SHA:
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. The exact Go module version is
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. Neither later main nor a later tag silently moves it.
SemSource stays wholly on SemStreams for this migration.

## Qualification profile crosswalk

| SemEngine slice | SemSource comparison configuration | Admitted evidence |
| --- | --- | --- |
| 0: graph foundation | `configs/tiers/tier0-statistical.json`, structural paths | Ingest, identity, exact queries, provenance, bodies, mutations, restart |
| 1: lexical | `configs/tiers/tier0-statistical.json` | Slice 0 plus BM25, scope-before-limit, stale/update/rebuild behavior |
| 2: neural | `configs/tiers/tier1-semantic.json` and shipped semembed composition | Lower suites plus semembed, paraphrase, cold/warm and provider recovery |
| Separate generation | `tier2-semantic-instruct.json` or dev overlay only if admitted | Separate model/output contract, not implied by slice 2 |

The current root always composes graph-embedding. Structural checks under BM25 do not prove a
no-embedder composition. That composition belongs to later SemEngine qualification. Do not rename
SemSource files or equate tier numbers. The actual JSON governs: numbered tier2-semantic-instruct
leaves clustering off, while the dev overlay enables it.

## Fixture and public-interface contract

Version stable relative paths, known Go symbols/relationships, Markdown bodies, fixed provenance
revision labels, and exact expected identities using `entityid.*`. Include duplicate names in separate
scopes, mixed code/docs, exact body bytes, an obsolete relationship removed by edit, a deleted/recreated
source, and unknown entities/terms. Normalize only incidental assigned ports and runtime roots.

Exercise SemSource HTTP/MCP tools and retained `graph.query.*` requests. KV supports diagnostics but
cannot replace public semantic assertions. Poll bounded authoritative state and exact expected answers;
startup, stable counts, log silence, and `ready` alone are insufficient. Gate each query on applicable
index/embedding readiness. Use deterministic structural expectations and stable retrieval acceptance
criteria instead of invented timing/ranking guarantees.

Sequence ingest/read/edit/reread/delete/visibility/recreate/reread. Current deletion retains stale graph
history: measure visibility and lifecycle metadata rather than demand physical removal. Recreation
must converge to intended identity and bytes with no duplicate current entity. Check provenance
source/revision and body retrieval independently of rank.

## Lifecycle and durability

Keep Start authority live while owners stop admission/drain. Pass the exact finite caller Stop context;
never store production contexts, substitute deadlines, or call timeout a completed join. Follow
ADR-095 one-shot running shutdown and separate failed-Start cleanup authority. Restart means a fresh
process, not reusing a stopped generation.

Application and broker restart are separate scenarios. Use isolated persistent NATS storage retained
inside each revision's run, never across revision comparisons. Inspect GRAPH and KV storage. A GRAPH
publish acknowledgement proves acceptance, not survival of memory storage. Demonstrate durable effects
or explicitly prove recovery via replay/re-ingestion. A pending-work probe may pause graph processing,
publish a valid fixture envelope, observe server-confirmed acceptance and unfinished effect, restart
broker from the same store, then resume/reboot and assert the declared durable-or-reingested outcome.
Do not call a direct KV write evidence for GRAPH's publish/index durability path.

## Configuration compatibility

ADR-094 seals component composition at boot. Source-management config writes remain durable desired
state, with explicit restart-required results and no claim that the running composition changed.
Branch lifecycle writes must not claim materialization merely because desired configuration persisted.
Existing per-component source-file watches remain live; this is distinct from replacing components.
The migration tests must distinguish configured sources from effective running source status.

## Evidence and holds

Every result identifies consumer snapshot, exact substrate version/SHA, corpus/hash, config/hash,
broker build/storage, provider/model artifacts, dimensions, and preprocessing. Preserve expected and
observed outcomes. Intended differences require a pinned contract reference; others need reproduction
and an open defect/blocker. Do not rewrite baseline results to erase changes.

GRAPH #178 needs its known OSH corpus/tail or a measured equivalent load; tiny corpus success is not
resolution. RPC #1143 needs its stream-subject collision mechanism, not route reachability. Label
reduced probes. Use `go list -deps -test` plus production measurement with equal build-tag policy.

Missing evidence blocks the affected claim, not independent implementation. Independent code approval
is distinct from SETUP 03A baseline acceptance. Keep the change unarchived and PR draft when mandatory
qualification remains incomplete.
