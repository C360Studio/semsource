# Independent process-test and CI review

Disposition: **APPROVED for the changed-contract test and CI wiring**, with no remaining findings.
Final process qualification of the common release binary is a separate pending gate at this review.
This is an independent agent code review, not GitHub human approval or a claim that deferred graph
removal/reactivation is qualified.

Reviewer: `/root/removal_publisher`; author: `/root/removal_projection`; CI author: `/root`.
The reviewer did not edit either reviewed file. Worker A production implementation is excluded from
this independent review because the reviewer authored that slice and other agents reviewed it.

## Scope and exact reviewed files

- `test/setup03a/desired_lifecycle_correction_test.go`
  SHA256 `99e6dd3c167243e1794ac7d5804b878fb04647ad4a222e55c4aa087106c44707`.
- `.github/workflows/ci.yml`
  SHA256 `74a46c7b29fe749e38a439a6491aae30fa2a53e2c30b4d08ca2ed29bae89de53`.

Read `CLAUDE.md`, the context in `openspec/config.yaml`, both `.agents/contracts` reviewer charters,
the approved `remove-private-source-recovery` contract, actual refusal implementations, and the
unchanged qualification/retirement helpers used by the new test.

## Findings resolved before approval

1. The HTTP 410 assertion previously accepted a code substring anywhere in the body. The author now
   decodes the response and requires a non-nil error object with exactly
   `SOURCE_LIFECYCLE_UNAVAILABLE`, as well as status 410. Invalid JSON, nil error, transport failure,
   or an unrelated field containing the code cannot pass.
2. The retained-envelope assertion previously checked only name, enabled, and nonempty config.
   The author now captures the complete initial envelope, preserves numeric JSON values with
   `UseNumber`, and compares canonical JSON after excluding only the explicitly changed `enabled`
   field. Every other field/value must remain identical across disable, re-enable, and replacement.
   It also requires the exact factory, processor type, and requested enabled state; the raw complete
   envelope and authoritative revision are retained as evidence.

These were bounded test-oracle gaps, not findings of a production defect. Both fixes stay in the
new process-test file; historical fixtures, tests, and recorded results remain unchanged.

## Checked behavior and evidence boundaries

- Both original and runtime-added document sources use public add/remove requests. Receipts must
  report desired-only changes, restart required, and projection unavailable, without retired
  generation or phase fields. Repeated remove requires the real `NOT_FOUND` error outcome.
- Exact retained config is read from the existing configuration authority. Graph entities are
  read through the public query route; exact referenced document bodies are read from the existing
  content object store. There are no direct KV repairs, entity mutation requests, journal records,
  fabricated guards, or private recovery calls in the new workload.
- Old lifecycle RPC and HTTP routes must refuse, and target revisions must not change. The legacy
  RPC correctly checks its actual header-classified error envelope rather than treating a timeout
  as refusal or assuming the source RPC's JSON shape.
- Admission/transport readiness is only used to establish the existing boot boundary. It is not
  presented as freshness, durable application, retirement, or reactivation proof.
- The new workload records a distinct profile and explicit changed-contract metadata. It does not
  invoke or rewrite historical positive source-removal/freshness assertions. The earlier broker
  restart failures and upstream limitations are not changed into passes by this test.
- Exact target and sibling facts and content are retained across the desired-state changes. The
  unchanged helper's previously reviewed AST reseed timestamp exception remains confined to AST
  siblings, records before/after metadata, and does not apply to document targets or freshness.
- Fresh evidence directories, copied versioned fixtures, the binary SHA, effective authority,
  configuration, and exact broker inspect output are recorded. CI pins the broker image by digest;
  this workload uses the frozen BM25 configuration and no embedding provider.
- Each subtest owns a fresh broker. Checked graceful replacement rejects nonzero/escalated exits;
  final cleanup uses the same checked child exit. The helper captures broker logs and removes the
  exact owned container under an independent cleanup context. Failures remain test failures.
  Preliminary cleanup evidence independently records all three earlier owned containers absent.
- The new code does not start a watcher or detached worker. HTTP response bodies close, per-request
  contexts cancel, NATS closes, and registered cleanup precedes cancellation of the parent context.
- CI runs only the anchored new test with both required tags, builds the selected checkout's binary,
  preserves failure artifacts, and retains the existing local E2Es. Its integration list replaces
  the deleted private lifecycle package with the root package containing real-broker guard tests.

## Independent validation and limits

- `go test -tags=qualification,removal -run '^$' ./test/setup03a`: PASS, 0.245 seconds.
- `go vet -tags=qualification,removal ./test/setup03a`: PASS.
- `git diff --check`: PASS.
- Both file hashes above were captured after those checks.

The first compilation attempt was blocked only by sandbox access to the shared Go build cache;
the authorized retry succeeded. No worktree file was modified by the review.

No duplicate process/Docker run was started by this reviewer. The preserved `process-before.log`
is a failing negative control. `process-corrected.log` is also a failing preliminary run: its
legacy-RPC envelope oracle had not yet been corrected. Those failures remain intact. Approval of
the final oracle does not relabel either run, and final binary process results must be recorded
separately after execution. This test covers checked graceful application replacement against the
same broker, not crash recovery, ambiguous commit, broker restart, authentication enforcement,
positive source-removal projection, or positive freshness restoration.
