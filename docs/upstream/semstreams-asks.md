# SemStreams consumer asks

SemStreams owns framework repairs. SemSource reports evidence here and does not commit to that repository.
The SETUP 03A pin remains frozen; SemEngine qualification and repair-before-port decisions are separate.

| Issue | Consumer evidence | Migration disposition |
| --- | --- | --- |
| [#1442: applied-sequence guard after stream recreation](https://github.com/C360Studio/semstreams/issues/1442) | beta.161 and pinned BM25/neural runs retain an old applied sequence after memory GRAPH recreation and reject changed-source re-ingestion | Blocks broker recovery and SemEngine admission; retain the failing corpus assertions |
| [#1443: deleted file component returns at restart](https://github.com/C360Studio/semstreams/issues/1443) | Desired deletion removes the KV key but pinned bootstrap overlays remaining keys onto the original file components | Consumer removal must persist an explicit disabled envelope; regression and re-add qualification required |
| [#1143: RPC stream collision](https://github.com/C360Studio/semstreams/issues/1143) | Isolated wildcard `graph.ingest.>` probe receives a JetStream PubAck for an RPC request at both pins | Keep explicit publish subjects in SemSource; retained framework path needs repair/admission review |

The exact revisions, provider pins, profile crosswalk, and before/after results live in
[SETUP 03A compatibility](../testing/setup-03a/compatibility.md). The separate
[SemSource GRAPH capacity regression #178](https://github.com/C360Studio/semsource/issues/178)
is measured with the versioned OSH transport probe; the existing parser guard limits what that run proves.

## Applied and unresolved input proof: #1444

[SemStreams #1444](https://github.com/C360Studio/semstreams/issues/1444) requests a supported,
stream-incarnation-aware proof that accepted source input was applied, or a complete current census
of unresolved input. On the frozen `8b99efe9c66a` pin, MaxDeliver-parked work can disappear from both
pending counters. Zero current backlog, AckFloor, cached readiness, and absence of a visible advisory
do not prove historical application. A scan of retained GRAPH input cannot account for expired,
evicted, or memory-lost publications. This is a verified code-contract limitation, not a newly
induced runtime failure; the migration pin remains unchanged.

SemSource may converge markers over the currently retained exact source set after observed producer
retirement, unchanged qualified stream/consumer identities and policies, zero current backlog, and
successful authoritative enumeration. The removal record must remain `pending` with the typed blocker
`applied_tail_unproven`, preserving periodic repair. Mutation/page errors remain stronger immediate
blockers; recovery returns to this honest pending state, never fabricated terminal completion.

Selective reactivation requires separate current-publication proof and an admitted mutation bound to
the exact graph revision observed. The frozen client cannot provide the latter (#1445, below).
Neither proof establishes that all historical publications materialized. [Source-removal #215][removal215] and the migration's
terminal-removal admission gate remain open even when the original missing-marker probe turns green.

[removal215]: https://github.com/C360Studio/semsource/issues/215

## Reconcile at the caller-observed revision: #1445

[SemStreams #1445](https://github.com/C360Studio/semstreams/issues/1445) requests a supported public,
contract-bound reconcile operation that accepts the caller's observed entity revision. The frozen
`projection.ReconcileMutation` has no expected-revision field; its client rereads the entity before
sending the mutation. A concurrent graph-ingest write can therefore replace the source facts after
SemSource verifies their fingerprint, while the client silently reconciles against the newer revision.
The shared lifecycle gate does not serialize graph-ingest source writes.

SemSource now retains matching reactivation work as `pending` with
`conditional_reconcile_unavailable` and sends no clear mutation. It does not copy the internal client,
send raw mutation RPCs, or treat a post-mutation fingerprint check as a safe substitute. The positive
freshness assertions remain required and unqualified. After an upstream API is admitted, continuing
publication and withdrawal eligibility also needs proof: an old acknowledged receipt must never make
a later-deleted entity fresh. The migration pin remains unchanged.

## Terminal mutation outcomes and backend commit ambiguity: #1446

[SemStreams #1446](https://github.com/C360Studio/semstreams/issues/1446) requests supported terminal
operation resolution or fencing and correct commit classification through backend write ambiguity.
The public client has no terminal outcome lookup; a timeout cannot prove an accepted effect will not
commit later. The canonical server can also classify a generic KV update error as internal while the
client calls classified errors not committed. This is code-contract evidence, not an induced live
backend lost-ack reproduction.

SemSource uses synchronous local projection and records an exact durable effect attempt before remote
mutation. Unknown, internal, malformed, crash and uncertain-clear outcomes keep source generation
changes blocked before config writes. Only confirmed commit or explicit safe before-effect rejection
can clear the exact attempt. A graph read, timeout margin, or process restart cannot substitute for
terminal proof. The frozen pin and independent #1444/#1445 holds remain unchanged.
