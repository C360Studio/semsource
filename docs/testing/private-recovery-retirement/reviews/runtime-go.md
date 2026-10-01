# Independent runtime review — corrective private recovery retirement

Reviewer: removal_projection, independent of Worker A runtime implementation.
Reviewed against `.agents/contracts/go-component-reviewer.md` and
`.agents/contracts/graph-event-reviewer.md`, approved OpenSpec
`remove-private-source-recovery` at 0221635 plus final cause-preservation correction.

APPROVED for the corrective runtime scope. No blocking findings.

Scope: cmd/semsource root composition, legacy compatibility guard and desired read adapter;
processor/source-manifest desired request serialization, readback and HTTP refusal;
internal/sourcespawn partial config outcome reporting; processor/supersession retirement
and owned refusal subscriptions/local gate; MCP desired-only descriptions and partial receipt forwarding.
This does not approve the reviewer's own publisher restoration or additive process test.

Evidence and conclusions:

- setupNATS checks legacy storage after connect and before stream provisioning or ConfigManager startup.
  Existing-only lookup plus authoritative bucket Status.Values is conservative: at pinned nats.go this
  returns stream retained message count, including tombstones. Missing/zero admits; nonzero, unknown,
  canceled or unreadable refuses. It neither interprets nor deletes old recovery records.
- ConfigManager remains desired authority. Request-local snapshots only read its retained envelopes;
  local cancellation-aware gates retain no work or outcomes. Successful/partial Add/Remove report
  desired changes with runtime_changed=false, restart_required=true and projection_status=unavailable.
  Enabled:false, deterministic handles, manifest repair and repeated-remove NOT_FOUND remain.
- Failed writes only claim exact returned committed envelopes after authoritative retained-config read.
  Errors remain errors; final KV_WRITE_FAILED fix keeps joined readback cause visible. These reads do
  not infer graph mutation completion and introduce no graph recovery state.
- Whole-source graph projection and positive reactivation paths are removed. Retired direct/RPC requests
  refuse without enumeration/mutation; delayed canceled callers cannot later dispatch through that route.
  The owned stateless subscription remains drained/joined during Stop.
- Existing ordinary file/passage staleness, retained source_removed stickiness, exact passage parent pairing,
  truncation/error behavior and governed publisher contracts remain. No new recovery bucket, journal,
  generation, receipt, seedproof, replay worker or effect resolver was introduced.
- MCP preserves partial desired-change receipts and describes next-boot activation, without live-stop claims.

Independent targeted validation: review-runtime-tests.log, command exit 0, Go race enabled and integration
build tag, cmd/semsource + source-manifest + supersession. Covers absent/empty/nonempty/foreign/corrupt/
terminal/tombstone guard, pre-provision refusal, canceled/unreadable probes, desired partial commit/error,
request cancellation, stateless and late RPC refusal/Stop ownership, sticky staleness/exact parent/truncation.
Scoped git diff --check clean.

Limitations: this approves removing unsupported private recovery, not full source-removal/freshness
qualification or resolution of historical unknown upstream effects. Nonempty legacy storage remains a
startup incompatibility and must be preserved with its former writers stopped. Separate fresh deployment
storage is the supported boundary. Process qualification is recorded separately; no crash or broker
restart guarantee is inferred from the additive graceful application replacement workload.
