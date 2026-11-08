# Source-removal replay contract

## Status and scope

Architect contract approved for implementation, subject to the named proof gates below. This approval
is not implementation or merge approval. Independent Go and graph/event reviewers approved this
initial contract for implementation on 2026-09-30. The pinned tail-evidence limitation and narrowed
retained-state pass below are a subsequent architect ruling; independent reviewers must approve that
implementation before integration. Code, runtime evidence, and qualification remain unapproved.
Issue #215 extends migration PR #213; SemStreams remains
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` at
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. All frozen SETUP 03A ledgers remain unchanged.

The qualified deployment has one SemSource process per effective authority and configuration store.
A supervisor or the process harness must observe the old process exit before admitting its replacement.
Absence from a new boot snapshot is a composition fence inside this boundary, not a distributed proof
that another machine stopped publishing. No framework exclusive lease or source-generation fencing
contract exists in the inspected pin. Do not claim active/active correctness or substitute a TTL lease
for publisher fencing. A detected overlapping owner or unprovable retirement leaves work pending with
an explicit retirement error.

## Pinned facts inspected

- ADR-094 and `config.Manager.PutComponentToKV`: desired writes commit KV before memory, do not alter
  the running composition, and return no component-entry revision to the caller.
- ADR-095: exact Start-owned resources retain cleanup authority; stop/cancel/join ordering is required.
- ADR-102/104: use retained `org.platform`, source system in segment three, taxonomy in segment four.
- `graph.PrefixQueryRequest/Response`: limit is at most 1000 and may be byte-limited; pass `NextCursor`
  unchanged until empty. A short page is not completion. Keyset pages are not a concurrent snapshot.
- `pkg/projection.MutationClient.Reconcile`: authoritative read followed by one CAS; it does not accept
  the caller's previously observed revision. All product lifecycle writers therefore stay serialized
  by supersession's `runMu`, and source-fact checks happen inside that owner before mutation.
- `natsclient.BucketSpec`, `EnsureFrameworkBucket`, `KVStore.Create/Update/Get/KeysByPrefix`: available
  retained operational storage and revision CAS. No raw writes to framework-owned graph/config keys.
- `natsclient.GetStream`, `Stream.Consumer`, `Consumer.Info`: read-only current backlog observation is
  available. Do not take lifecycle authority over graph-ingest's consumer.
- Graph-ingest `readiness.go`: `NumPending + NumAckPending` reports outstanding work; bootstrap is a
  latch, MaxDeliver-parked work leaves BOTH pending counters, and cached `Ready` is not an
  applied-publication fence. `graph/index_status.go:276-284` explicitly rejects AckFloor as proof.
- `component/port_jetstream.go:130-149` defaults MaxDeliver to three; zero inherits the default and
  negative declarations are refused by `component/port_codec.go:382`. There is no qualified declared
  unlimited-delivery option on this pin. Even unlimited retries alone would not account for terminal
  rejected input.
- `MAX_DELIVERY_EVENTS` is a bounded seven-day, 64 MiB DiscardOld occurrence ledger. Its observer is
  internal framework boot plumbing, not a current parked set or completeness API. Missing advisories
  cannot prove absence of parked work before provisioning or beyond retention.
- `GRAPH_INGEST_APPLIED_SEQ` is a cataloged operational redelivery guard. Its key construction and
  eight-byte value decoding are private to graph-ingest. It stores only the last applied sequence for
  an entity/stream, not an input census, and lacks stream-incarnation identity. The catalog reader's
  availability does not turn private guard serialization into an adopted completion contract.

## Storage choice and ownership

The `kv-or-stream` skill distinguishes facts from queued execution. Here the durable record is the
current desired/projection state of one source handle, including the fence needed when intent changes.
An ordinary work stream alone is insufficient because this pin requires finite stream `MaxAge`, while
an operator may defer application restart indefinitely. The operational KV record has no TTL or
binding MaxBytes. It is not a KV watch used as an acknowledged queue: the owner performs a full startup
scan and bounded periodic full-snapshot repair, retries failed effects, and exposes degradation.

The `orchestration-check` skill puts this operational execution artifact in component-owned storage,
not in `ENTITY_STATES`. Source-manifest owns desired source operations and generation; supersession
executes the single graph projection operation. There is no rule chain or second lifecycle authority.

Use a product bucket named `SEMSOURCE_SOURCE_LIFECYCLE`, file backed, history one, owner-only,
`RetentionNoLifecycleStrict`, with the deployment's declared replica policy. Provision and verify via
the pinned bucket mechanism before source-management handlers can acknowledge changes. The bucket
name is product-owned, not added to the upstream graph catalog. The pinned Ensure mechanism reconciles
retention/history but does NOT establish storage/replica correctness on an existing bucket. After
acquisition, inspect authoritative backing-stream configuration and require FileStorage and exactly the
declared replica policy; also reject a positive GLOBAL MaxMsgs ceiling that could evict current keys.
Per-subject history one is intentional; global message-count eviction is not. Reject memory storage,
replica mismatch, count eviction, or unreadable policy before serving
successful writes. Do not silently claim Ensure verified these properties. Key grammar is
`source.<encoded-authority>.<encoded-handle>` and
`receipt.<authority>.<handle>.<generation>.<seed-epoch>.<batch-id>.<entity-id>` using the upstream opaque
KV token codec for every nonliteral token. Seed-manifest keys carry the same binding and batch identity.
The record also carries and validates the complete authority, namespace, and handle;
key encoding is not an authorization decision. Do not log credentials from the retained config.

## Journal and receipts

A version-one journal value contains:

- schema version, effective authority, namespace, exact source handle, monotonic generation;
- operation (`remove` or `reactivate`), phase (`prepared`, `pending`, `complete`, `superseded`), and
  typed blocker/error plus retry count and last successful progress;
- exact component envelope/config fingerprint and source selectors captured before disable;
- relevant graph input stream/consumer identities, creation identities and qualified policies captured in
  the old admitted boot/removal preparation, before desired acknowledgment;
- committed desired-state evidence, creation/update times for diagnostics only, completed entity count;
- replacement generation when an older operation was superseded;
- active initial-seed epoch and its sealed current-ID manifest count/digest for reactivation.

Generation is product operational intent, never an entity-ID segment or a framework loop token.
Every replacement and completion uses the KV revision read with that value. Failed CAS means reread;
never overwrite a newer generation. Keep a durable completed record for observability and duplicate
repair. Unsupported schema/ownership or corrupt records fail closed and remain visible.

A current publication receipt contains authority, handle, generation, initial-seed epoch, entity ID,
batch identity and canonical fingerprint
of the source-owned triples/content-reference/profile that were actually published, and delivery
outcome. Lifecycle triples and unrelated framework enrichment are excluded from the comparison.
Receipts do not put raw content or credentials in the journal. Canonicalization must preserve typed
objects and compare the producer's source facts rather than `EntityState.UpdatedAt`.

Expose a typed read of current journal status through the source-management surface; the private-broker
process harness may also read exact journal keys. A removal response reports desired persistence and
`restart_required`, plus its generation/pending projection state. It never claims retained projection
completed solely because the desired config write succeeded.

## Desired-state transaction without a cross-store transaction

Replace the private source-manifest `desiredMu` with one injected coordinator gate that serializes
Add, Remove, and one replay unit. Gate acquisition honors caller cancellation; a plain blocking
`Mutex.Lock` is insufficient for request/Stop deadlines. Network work is bounded; do not hold the gate
across an entire unbounded corpus. Lock order is coordinator gate then supersession `runMu`, never the
inverse. Supersession gate acquisition must also honor cancellation: replay holding the coordinator
gate must not wait beyond its deadline for a long legacy sweep. Replace blocking runMu acquisition
with a context-aware equivalent while preserving that single serialization owner. Do not hold the
gate while waiting for a seed, publisher drain, or observer callback. Receipt
storage writes are generation/epoch-keyed and do not acquire the desired-operation gate.
Before deriving query scopes or constructing factories, startup repairs journal/manifest facts against
the successfully loaded config. The immutable boot component snapshot is then copied from that
repaired desired state and passed by value to this owner. Mutable `GetConfig()` is desired state only,
never proof of runtime retirement.

Removal ordering:

1. Resolve the currently enabled component and exact selectors. Unknown handles retain `NOT_FOUND`.
2. CAS a new `prepared` removal generation with the original component envelope before changing config.
3. Persist the complete disabled envelope, preserving the reviewed upstream #1443 workaround.
4. Repair the desired source-manifest entry and expected count, then promote journal to `pending`.
5. Return success only when all required desired writes and durable intent evidence are confirmed.
   A config Put error is an ambiguous result: durable KV may have committed before its in-memory
   application failed. Verify via the config catalog reader or retry the exact desired write through
   ConfigManager; do not infer non-commit from the error. Direct product writes to config keys remain
   prohibited. The reader derives bucket identity with `config.BucketName(org, declaredStem)` and uses
   `graph.OpenCatalogReader`; it must not use the minted platform as the bucket stem.
   On partial failure return the committed desired-change facts and a typed retryable error.

Crash/failure recovery is deterministic. Prepared plus the same enabled envelope means disable did not
commit; a retry may finish the request, but background replay must not remove an enabled source.
Prepared plus the exact disabled envelope means removal committed; startup repairs manifest/pending
state and retains the work. Different or enabled replacement config supersedes the old operation;
missing/ambiguous config is an explicit blocker, never permission to broaden graph scope. A duplicate
request for a known incomplete operation resumes that generation; an unrelated unknown handle does
not become a successful idempotent deletion.

Re-add commits its enabled component envelope under the same desired-operation lock before replacing
or superseding removal journal state. This ordering is deliberate: if the journal update fails, the
enabled desired config already blocks every old-removal attempt. Report that partial failure and retry
the journal transition; restart repair observes it too. A new removal creates a newer journal generation
before disabling again. Each attempt rereads both generation and desired envelope; stale completions
cannot win a CAS. Same-handle re-add before the first restart invalidates old removal without marking
entities that remained live. Every re-add with prior removal history records a `reactivate` generation,
including partially applied or pending removal. Do not infer marker absence from the latest phase: a
remove/add/remove/add sequence may retain markers from an earlier generation.

## Retirement and graph-tail barrier

Start-owned reconciliation starts after the fixed boot configuration is known. For removal it first
requires that the exact producer handle is absent/disabled in this immutable boot snapshot and the
current desired envelope still matches the removal generation. The requesting boot, where that producer
was admitted, must leave projection pending even after desired removal succeeds.

Within the single-process replacement boundary, inspect current graph-ingest consumers for every
relevant declared entity input stream/filter. Derive the durable identities from the pinned admitted
port configuration, not a broad wildcard or fabricated constant. Read stream identity and current
consumer information, requiring no pending and no unacknowledged delivery. Inspect the observed
consumer config as well: require AckExplicit, the exact admitted filter/stream, and the qualified
DeliverAll policy. AckNone can drain counters before effects finish; AckAll can acknowledge lower
in-flight lanes. Those policies are unsupported at this barrier and fail visibly. Capture and compare
consumer creation identity as well: delete/recreate with DeliverNew cannot fabricate an empty fence.
Preserve the observed
stream generation/creation identity already captured durably in the removal intent while the old boot
was admitted, and refuse changes across boots. Capturing it first in the replacement cannot detect a
reset between processes and is insufficient. A missing consumer, unreadable
state, negative/unknown counter, broker outage, or nonzero backlog defers projection with a specific
reason. No arbitrary sleep, cached readiness boolean, stream PubAck, or publisher counter proves this
barrier. A present poison/parked/degraded signal blocks stronger claims, but the absence of such a
signal proves nothing about historical parked or terminally rejected input.

### Pinned completion limitation and safe useful boundary

**Automatic terminal removal completion is blocked on this frozen pin.** No supported API establishes
that every accepted source input was applied or reports a complete current unresolved-input census.
An audit of the retained GRAPH stream cannot recover expired, evicted, or memory-lost input; its normal
one-hour retention makes a negative scan especially insufficient. Do not decode private applied-guard
values, import the internal advisory observer, infer completion from AckFloor, or add a new product
outbox to conceal this framework gap. The contract request is
[SemStreams #1444](https://github.com/C360Studio/semstreams/issues/1444), for an authoritative,
stream-incarnation-aware applied/unresolved-input proof. This is a verified code-contract finding,
not a newly induced runtime failure. Changing this migration pin is a separate baseline decision.

The permitted production result is narrower and explicit:

1. Proven old-process retirement plus unchanged qualified stream/consumer identity and policy and
   zero CURRENT backlog admits a retained-state pass. A fresh authoritative prefix query must work.
2. Fully enumerate the currently retained exact source scope and converge source_removed through the
   existing mutation owner. This repairs the known missing-marker symptom for retained parents and
   passages. Preserve all normal scope, error, cancellation and generation guards.
3. Persist observed counts and progress, but keep the removal journal `pending` with the typed blocker
   `applied_tail_unproven`. A successful retained pass is not terminal intent completion, even if its
   currently retained set is empty. Periodic full repair remains active so a later source-scoped
   authority arrival is marked while the removed producer remains absent.
4. Any mutation/page failure remains a stronger immediate blocker with honest partial counts. Once it
   recovers, a successful pass returns to `applied_tail_unproven`, not `complete`.
5. Selective reactivation additionally requires an admitted mutation conditioned on the exact revision
   whose source facts were checked. The frozen public client cannot express that condition; matching
   source_removed entities remain pending with `conditional_reconcile_unavailable` (SemStreams #1445).
   Current-epoch manifests and receipts remain useful evidence, not permission to clear at this pin.
   Superseding removal generations and retained history safeguards remain.

`ProjectionResult.Complete` means the explicitly requested retained set was traversed and all requested
mutations verified; it does not authorize `Record.Phase=Complete` for removal. Qualification must assert
that distinction. No production adapter in this frozen-pin change may fabricate stronger proof.

Index/embedding readiness is not required to mutate retained authority, but later query acceptance
must wait for the respective view's revision/normal qualification condition. The original full #215
terminal-completion acceptance remains open behind the upstream proof gap; known-answer marker success
must not be reported as full issue closure or migration qualification.

The same contract applies to a dirty process death: the replacement follows observed process exit,
retained JetStream work drains, then replay proceeds. Total loss of NATS persistence is outside this
contract. The existing broker stream-generation defect remains a separate blocker, not repaired here.

## Exact source scope and graph execution

Factor source selectors from the same persisted identity inputs used by the producer: effective
`org.platform`, exact source system(s), exact taxonomy, and where needed the source's canonical artifact
identity. Reuse producer identity helpers, not the stale manifest scope helper or a component-name slug.
Capture removed config before disabling. Expanded-repo children are instance-scoped: removing docs must
not mark AST, git, or config siblings sharing the source system.

Compare the proposed scope against enabled boot/desired siblings. Source predicates lack an instance
stamp: two docs roots sharing Project can emit indistinguishable relative paths and IDs. Such overlap
must produce `ambiguous_source_scope` and leave pending work visible; do not guess ownership or mark
shared live entities. URL host and object-store project alone are not exclusive scopes either. Support
only selectors whose exclusivity is proven by code/tests; an unsupported source shape fails visibly.

Use narrow prefixes and follow every opaque cursor. Prefer page-at-a-time processing with bounded
memory; never accept the old max-entity truncation boolean as success. Detect repeated/cyclic cursors,
out-of-scope entities, malformed IDs, decoding errors, and any page error. Retry from the beginning
unless a persisted cursor is proven safe; idempotent mutations make repeated successful pages cheap.
Retained-pass completion requires exhausted cursors for every scope and zero unresolved mutation
failures. Terminal removal intent completion additionally needs the unavailable applied-tail proof.

Route durable replay through the synchronous injected projector with exact selectors, generation,
intent identity and effect-fence capability. The old product lifecycle RPC refuses mutations. Keep
legacy filesystem/absent-path requests separate and reject `source_removed` as a legacy reason. In those legacy passes, `source_removed` is sticky: filesystem presence, an empty Absent
set, or passage-count logic must not clear or replace it. Only admitted removal/receipt-reactivation
may change that reason. Preserve existing behavior for file_deleted, path_missing and passage_removed;
The restriction prevents a later path sweep from bypassing current-entity publication proof.
Supersession retains sole ownership of lifecycle mutations and its `runMu` serialization.
On removal reconcile `entity.lifecycle.stale` to exactly `source_removed`, including when another stale
reason was present. Preserve source facts, content references, IDs, and all retained entities. Return
partial counts with an error; logging and continuing cannot become a successful completion response.
Unknown/ambiguous mutation commit retains a durable unresolved effect fence and blocks generation
changes. It cannot be retried or released from entity readback; #1446 is the recovery dependency.

## Re-add publication proof and selective freshness

Inject an owner-bound publication observer through factory construction before producer Start. The
binding contains the exact handle, journal generation, and initial-seed epoch, copied from boot facts.
Durably advance the eligible seed epoch before producer Start, even when the re-add generation is
unchanged across restart or its previous phase was Complete. Every matching enabled source with
Reactivate history starts a new epoch in Pending with no initial seal; completion of a prior batch does
not erase retained reactivation history. Prior-process receipts are never eligible in the new epoch.
Extend the common `internal/entitypub` seam once; source components forward the observer without a
global registry or stored context. Sources with no reactivation history incur no receipt-store writes.

For reactivation, each successfully acknowledged publication records its exact current-entity receipt.
Freeze the payload evidence before enqueue/publication so later caller mutation cannot rewrite it.
After PubAck succeeds, receipt persistence retries ONLY the receipt; never republish the entity because
the observer failed. Track delivery and receipt failure separately: transport-acknowledged data is not
transport loss, but unresolved receipt work degrades source/replay completion and failed shutdown.
Do not issue a receipt before transport PubAck and do not classify receipt-write failure as successful
source completion. Retain the failed receipt as owned retry work while the process is live; after crash,
the source's next full seed republishes current entities under a NEW seed epoch, invalidating every
prior epoch's receipt eligibility before it starts.
Tests must cover crash after PubAck but before receipt persistence, bounded retry/shutdown, and honest
source degradation when persistence cannot complete. Receipt persistence must settle before publisher
work is reported complete; existing source loss/readiness counters must not hide this failure.

A producer must close its initial enumeration explicitly with success or failure. Build a bounded
current-ID manifest from the exact expected publications in that seed, stored as per-entity records
plus a terminal count/digest. Seal it only after enumeration/validation succeed and every expected
publication and receipt settles. A short, failed, lossy, or crashed enumeration cannot seal success.
Do not interpret existing `watching` status or `ingestOnce` returning nil as this proof: current source
implementations can log individual errors and return nil before the publisher drains. The implementation
must propagate that seed failure to the observer contract.

The common publisher exposes an explicit seed/batch tracker, not a `Pending()==0` heuristic. Bind one
initial seed before the first Send. Track every accepted payload's immutable ID/fingerprint plus
its terminal PubAck-and-receipt outcome, including items already removed from the buffer into a local
batch. The producer closes enumeration explicitly with its aggregate error. Only that close plus
settlement of the exact accepted set can call `SeedFinished`; an empty successful seed still needs
an explicit close and publishes the canonical empty manifest. A rejected Send or producer validation/
listing/parsing error makes the seed unsuccessful even if other messages committed. Shutdown cannot
invent a close or ignore in-flight observer work. A watch/reseed uses a separately identified explicit
batch, carried in observer calls and every receipt/manifest storage key; its records cannot amend or
complete an earlier failed seed. The initial seed manifest is stable before watcher traffic can amend
a later producer pass; later live events cannot retrospectively fill a failed seed.

Intended selective freshness requires the CURRENT boot epoch's successfully sealed initial manifest,
matching per-ID acknowledged receipts, continuing entity eligibility, and authoritative graph
source-fact matches. It may clear only an existing `source_removed` marker, using an admitted mutation
conditioned on the SAME exact KV revision whose facts and marker were checked. Supersession's serialized
lifecycle owner is necessary but does not serialize independent graph-ingest source writes. The frozen
pin cannot provide the required public mutation capability, so production performs no such clear.
A newer file-deleted, path-missing, or passage-removed reason must remain unchanged. The current producer
cannot grant freshness to entities it did not emit: a removed
file, old symbol, or passage beyond a shortened document remains retained stale. Pair passages with
exact parent identity (`DocChunkOf`), never a first parent found by shared logical path.

If complete current source publication cannot be established, status remains pending/degraded. Do not
claim reactivation complete from a raw entity count, filesystem stat, `UpdatedAt`, or same-handle
admission alone. A source seed with explicit loss or an unresolved receipt error cannot satisfy it.

## Conditional reactivation is an upstream gate

[SemStreams #1445](https://github.com/C360Studio/semstreams/issues/1445) records the frozen-pin gap.
`projection.ReconcileMutation` has no caller-supplied ExpectedRevision. `MutationClient.Reconcile`
performs a fresh authoritative read internally and uses that new revision, after SemSource's earlier
source-fingerprint check. Shared lifecycle serialization and checking a returned receipt after the
mutation cannot prevent a wrong clear against changed source facts.

The public graph DTO's ExpectedRevision is not an admitted application escape hatch. The operation
resolver and wire client live in `internal/graphmutation`, whose contract directs applications to narrow
typed clients. The projection capability spec excludes raw subjects and raw KV. Do not copy that client,
issue a literal mutation subject, bypass the contract-bound client, or modify the frozen pin locally.
A supported upstream conditional reconcile capability must accept the caller's nonzero exact revision,
preserve contract/group validation and typed outcomes, and perform no hidden reread or automatic retry.

At this pin, validated receipt/fingerprint matches may be recorded as observed progress. An entity
still marked source_removed remains unchanged and pending with `conditional_reconcile_unavailable`.
Positive freshness scenarios remain intended acceptance, explicitly blocked; proof that the adapter
refuses to clear is not proof of successful reactivation. Already-fresh observations do not prove that a
clear occurred. #1444 continues to govern terminal removal independently of this mutation gap.

## Current-epoch batch inventory and continuous repair

`Journal.ListSeeds(ctx, binding)` inventories only immutable terminal seals and their exact entries for
one current binding/epoch. It delegates each result's integrity to LoadSeed; absent, corrupt, foreign,
partial or contradictory records cannot become a successful empty inventory. The coordinator validates
receipt agreement for each selected batch independently. Enumeration order is not publication or
withdrawal order, and storage completion is not continuing source eligibility.

Every boot rebinds matching enabled Reactivate history, including a previous Complete record, to a new
epoch and Pending initial proof. Periodic repair also scans Reactivate records after a batch completed;
new work or failures must become visible rather than disappearing behind a terminal phase. A successful
initial seed from that SAME epoch is a prerequisite for considering any separately sealed live batch.
A live batch cannot amend, complete, or substitute for an incomplete/failed initial seed.

The typed reactivation request keeps Manifest/Receipts as the exact target batch. An initial target
proves its own prerequisite. A non-initial target additionally carries InitialManifest and
InitialReceipts: the independent successful initial seal and exact acknowledged receipt set from the
same binding/epoch. The projector validates both sets, including identities, digests and receipts; it
must not merely drop its Initial check or trust a boolean supplied by the coordinator. This extension
admits integrity checks and authoritative reads only at the frozen pin, not lifecycle clears.

A-only initial completion followed by B recreation is intended to become repairable from a separately
sealed eligible B publication. That positive behavior is not qualified here. Older receipts must not
revive B after a newer deletion, failed current enumeration, or withdrawn publication. In particular,
a legacy delete can leave a sticky source_removed marker and its revision unchanged. Historical receipt
matching alone therefore cannot prove continued eligibility. Publication/withdrawal eligibility needs
its own specified and tested contract before production live-batch freshness is admitted; until then,
preserve pending state. No claimed latest ordering is inferred from BatchID spelling or KV list order.

## Retry, lifecycle, and observation

The owner performs a bootstrap scan and periodic bounded reconciliation regardless of KV watch delivery.
One transient failure does not require another operator request or restart to recover. Each storage,
query, and mutation call receives a caller context and a finite deadline. Cancellation leaves durable
pending work. Stop drains source-management request handles, prevents new replay admission, waits for
accepted work, then cancels and joins its Start-owned workers; no detached goroutine escapes cleanup.

Expose pending count, generation, current blocker/error, last progress and retry count in typed status
and component health. Log failure/recovery transitions, not one warning per entity. Completed means
all required source scope was traversed and every effect verified, including required applied-tail
evidence for removal and conditional revision/continuing eligibility proof for reactivation. The frozen
production adapters cannot supply these stronger proofs. Partial
progress remains partial.
No automatic TTL, eviction, or physical graph purge is introduced. Receipt/history compaction requires
an explicit later ownership proof and is not an implicit completion side effect.

## Shared Go seams

Use `internal/sourceintent/contract.go` for shared value types/interfaces and
`internal/sourcelifecycle` for the product coordinator and storage adapter. The journal developer owns
the contract file first; the receipt developer consumes it without concurrent edits. Dependencies
remain narrow interfaces to permit deterministic failure tests. The agreed exported value types are:

| Type | Required fields / meaning |
| --- | --- |
| `Binding` | Authority, Namespace, Handle, Generation, BootEpoch, Factory, ConfigDigest |
| `Publication` | Binding, BatchID, Initial, immutable cloned EntityPayload after PubAck |
| `ManifestEntry` | EntityID, canonical source fingerprint |
| `SeedManifest` | Binding, BatchID, Initial, successful enumeration, exact Entries, digest |
| `Receipt` | Binding, BatchID, EntityID, source fingerprint, transport acknowledged |
| `Record` | Version, Binding, Operation, Phase, retired config/scope/tail identities, status |
| `ProjectionResult` | Enumerated, Marked, Cleared, retained-pass complete flag and failed details |
| `ReactivationProjection` | Binding, target Manifest/Receipts, optional InitialManifest/InitialReceipts prerequisite |
| `TailProof` | `current_retained` or `applied_complete`; zero/unknown values fail closed |
| `TailEvidence` | Qualified current observations plus explicit Proof; diagnostics never imply proof |

`SeedManifest.Entries` is an in-process slice; the journal writes bounded per-entity records and a
terminal count/digest rather than putting an unbounded JSON value in one KV entry. `LoadSeed` reads
that exact binding/batch and verifies the terminal seal against its entries before returning it; it
is the restart read seam, not permission to accept an incomplete manifest. Error responses
remain typed results/errors, not strings that callers parse.

```go
type Journal interface {
    Get(ctx context.Context, handle string) (Record, uint64, error)
    Create(ctx context.Context, record Record) (uint64, error)
    Update(ctx context.Context, record Record, revision uint64) (uint64, error)
    List(ctx context.Context) ([]Record, error)
    PutReceipt(ctx context.Context, receipt Receipt) error
    ListReceipts(ctx context.Context, binding Binding, batchID string) ([]Receipt, error)
    SealSeed(ctx context.Context, manifest SeedManifest) error
    LoadSeed(ctx context.Context, binding Binding, batchID string) (SeedManifest, error)
    ListSeeds(ctx context.Context, binding Binding) ([]SeedManifest, error)
}

type Projector interface {
    ApplyRemoval(ctx context.Context, request RemovalProjection) (ProjectionResult, error)
    ApplyReactivation(ctx context.Context, request ReactivationProjection) (ProjectionResult, error)
}

type TailObserver interface {
    // nil error with Proof=current_retained permits only a retained-state pass.
    // The production adapter at this frozen pin never emits Proof=applied_complete.
    Settled(ctx context.Context, scope SourceScope) (TailEvidence, error)
}

// Bound before producer Start; metadata includes handle/generation/current seed epoch.
// Expected IDs and a terminal seed marker are separate from transport acknowledgment.
type PublicationObserver interface {
    Published(ctx context.Context, publication Publication) error // immutable binding/batch/payload
    SeedFinished(ctx context.Context, manifest SeedManifest) error // all effects settled
}

// Common publisher owns this tracker, including payloads currently in its local drain batch.
// Producer binds it before enumeration and explicitly ends that enumeration, even if empty.
type SeedBatch interface {
    Send(payload *graph.EntityPayload) error
    Finish(ctx context.Context, enumerationErr error) error
}

// Implemented by every eligible source component; called by the root factory decorator.
type BindablePublisher interface {
    BindSourceLifecycle(binding Binding, observer PublicationObserver) error
}
```

Add `CodeAppliedTailUnproven = "applied_tail_unproven"` to the typed blocker vocabulary and `Proof`
to TailEvidence. `TailProofCurrentRetained` permits the narrow pass described above;
`TailProofAppliedComplete` represents the stronger contract for a future supported adapter and tests.
No current production path may produce that latter value. The coordinator validates proof explicitly
and cannot promote a removal record merely because the projector's retained-pass Complete is true.

Add `CodeConditionalReconcileUnavailable = "conditional_reconcile_unavailable"` for #1445. Extend
ReactivationProjection with `InitialManifest *SeedManifest` and `InitialReceipts []Receipt` for a
non-initial target. These fields carry actual prerequisite evidence; they do not assert withdrawal
eligibility or authorize the unavailable conditional mutation. Keep failed live validation fail-closed.

The coordinator exports `Acquire(ctx) (release func(), err error)`, `PrepareRemoval`,
`RepairDesired`, `RecordReadd`, `BindBoot`, `PublisherObserver`, `ReconcileOnce`, and `Status`.
`BindBoot` runs in `cmd/semsource/run.go` immediately after `ConfigManager.Start` and journal desired
repair, BEFORE `sourceScopeSystems`, `registerComponentFactories`, or `createServiceManager`; it
preflights the binding map and advances every matching enabled Reactivate history's epoch durably,
including records whose previous phase was Complete. The publisher exports
`BindSourceLifecycle` (before Start only) and `BeginSeed(batchID string, initial bool) (SeedBatch, error)`;
source initial-seed methods use the returned `Send`/`Finish` pair. They explicitly finish unsuccessful
or empty enumeration as well. Watch/reseed work uses separately named batches after initial closure.
Source-manifest calls desired-operation methods under the SAME coordinator gate as Add/Remove;
it does not create a second mutex pretending to fence replay. The root owns
construction, pre-admission repair, immutable boot snapshot, and factory dependency injection.
Source-manifest owns the Start/Stop lifetime of periodic replay. The root-created journal and receipt
writer remain usable until ALL producer publishers have drained, even if source-manifest stopped its
replay worker earlier. They retain no stored context. Root storage/transport cleanup follows producer
Stop completion; a manifest Stop must not close the shared receipt writer.

A factory decorator implementing the existing `RegisterWithConfig` seam can wrap each source factory,
construct normally, and inject its bound observer through an explicit optional pre-Start interface.
Registry factories receive only raw component config plus dependencies; no instance map key is in
Dependencies at this pin. Before creating the decorator, root preflights every source envelope so its
`instance_name` is present and equals its immutable Components map key; handles are unique. Bind via
that root-owned map of handle, factory name, canonical envelope digest, generation and current epoch,
and verify the factory's raw config digest matches. Never trust a free raw `instance_name` to select
another source's receipt binding. Missing, duplicate, mismatched, or changed bindings fail boot.
Preserve the factory's port declarer, schema, authority injection, and error paths. Every producer with
a reactivation binding must implement that interface; missing implementation fails boot visibly.
Do not change framework registration or use a process-global publisher observer. The producer owns
explicit enumeration success and expected current IDs. The publisher owns
its in-flight observer retry, receipt/manifest agreement, and drain, and reports observer failure in
existing delivery/health accounting. The coordinator owns persisted receipt reconciliation after publisher termination.

## Implementation ownership and handoff

- Journal/replay developer: new product lifecycle package, source-manifest protocol, exact scope,
  supersession pagination/mutation honesty, typed status, unit and broker integration tests.
- Producer receipt developer: common publisher observer/retry contract, pre-Start factory bindings,
  source forwarding, current seed manifest/epoch, receipt tests and seed/readiness accounting.
  Coordinate shared interfaces first.
- Qualification developer: additive process tests and result artifacts; never rewrite frozen probes.
- Independent Go and graph/event reviewers: concurrency/lifecycle, source scope, reactivation safety,
  exact completion evidence, and changed contracts before integration.

All implementations must pass the task matrix. If the pinned APIs cannot prove an admitted fence,
record the reproduced limitation and leave the operation pending rather than weakening the contract.

## Generation authorization and uncertain effects

A caller timeout cannot cancel a queued NATS lifecycle request. The old mutation-capable product RPC
must permanently refuse requests; it cannot be an alternate path around current journal authorization.
The composition root binds a synchronous, process-local `sourceintent.Projector` exactly once to the
supersession owner. The coordinator holds its desired-state gate through the direct invocation;
projection readiness remains an ordinary retryable condition until the component has started.

Each removal request carries a non-serialized `EffectFence` capability. Before every possible remote
mutation, `Begin(ctx, entityID)` verifies the current binding and disabled desired envelope, then
persists an exact binding/entity/attempt fence. Its hooks run under the already-held coordinator gate
and never reacquire it. A failed or ambiguous journal write authorizes no mutation. `Resolve` stores
terminal resolution
only for the same attempt via CAS, based on verified commit or an explicit proven before-effect rejection.
It retains the exact attempt and terminal evidence instead of deleting the record. A lost journal reply
does not manufacture remote uncertainty: later authoritative journal inspection may establish that a
known terminal outcome was durably recorded; graph readback may not resolve an unknown remote effect.
The coordinator reloads the current journal revision after effect writes before storing progress.

Unknown remote replies, internal errors, malformed outcomes, crashes before terminal proof, or a
journal without authoritative terminal resolution retain that durable obligation. They conservatively block all source generation changes before desired-config
writes, including Add/refresh/re-add, and block unsafe enabled boot admission. Existing non-overlapping
boot sources may continue. Readback, elapsed time, queue counters, a later matching marker, or process
restart cannot clear an unresolved attempt. This recovery limitation is tracked in
[SemStreams #1446](https://github.com/C360Studio/semstreams/issues/1446).

The pinned public mutation receipt must be preserved. Its `CommitNotCommitted` classification alone
is insufficient for generic internal failures: the canonical server can convert a backend KV update
error into a classified internal rejection. The consumer therefore uses an explicit safe rejection
allowlist and retains the fence for internal/unclassified/unknown outcomes. No private client is copied.
