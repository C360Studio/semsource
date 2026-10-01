# Additive source-removal replay qualification

This suite extends the preserved #215 pre-fix evidence without changing the original SETUP 03A corpus,
removal probe or ledgers. It uses a compiled binary, public RPCs and read-only inspection of a private
broker. The versioned `removal-replay-v1` workload adds documents A/B, a document sibling, and the
original AST source sharing the target system with a different taxonomy.

## Final recorded outcome

The final run used clean production commit `ce241a5c6e07f69e5450bb60885b537e89ea27a9`, binary SHA-256
`09170e24f60af0ead2b209ce58bf9a4c4446316cc77e9c5fbe6f8a2da63b9421`. The combined command exited **1**
after **424.136 seconds**. [The final ledger](final-results.json) records all six completed reports,
exact inputs, commands and artifact hashes; the blocked assertions remain failures.

| Process case | Passed / total | Observed result |
| --- | --- | --- |
| Unchanged original removal probe | 10 / 10 | Retained parent/passage markers now appear after restart |
| Plain sibling restart control | 8 / 8 | Strict sibling facts/content preserved, with AST metadata rule below |
| Original-source retirement | 16 / 16 | Crash after durable intent; retained markers repaired without resend |
| Runtime-added source retirement | 20 / 20 | Same repair after desired add and restart admission |
| Re-add before first restart | 27 / 27 | Two generation replacements; zero transient old-removal markers |
| Selective reactivation | 24 / 26 | A-only proof captured; completion and A freshness remain blocked |

Retirement cases prove intent is durable while the old producer remains admitted and unmarked. After
observed SIGKILL and same-broker restart, the producer is absent and all expected retained parents and
passages carry `source_removed`. A legacy sweep preserves those markers. Sibling IDs, facts and exact
content bytes remain unchanged under the narrowly qualified AST timestamp rule.

The rapid re-add case proves superseded removal never marks the target, including transient writes:
an authoritative KV watcher observes current target revisions before shutdown. Its fresh entities
were never marked stale; this is not evidence that the implementation can clear a removal marker.

Selective reactivation first marks A/B, stops the process with a checked exit, deletes B offline,
then re-adds the same handle in a new generation. The new boot seals exactly A's parent and passage
with matching current-epoch acknowledged receipts. Two intended acceptance assertions remain red:

- `selective_current_epoch_A_only_manifest`: publication proof passes, but the composite assertion
  requires completed reactivation; the journal remains `pending` / `conditional_reconcile_unavailable`.
- `current_A_parent_and_passage_fresh`: both current A entities still carry `source_removed`.

B remains retained and stale, and siblings remain unchanged. Additional passing pending/no-clear
observations document the safety boundary; they do not replace either failed positive assertion.
This case uses a checked stop before offline deletion. It does not prove the separate crash point
between A+B receipt persistence and projection, or continuing withdrawal eligibility.

## Preserved earlier evidence and oracle scope

[The additive pre-fix run](additive-before-results.json) used binary
`7847df902bef611c777df4a19366dac896a9e86cc80cd475623d40d679c0a198` and exited 1 with 7/8 observations
passing. Its missing lifecycle journal caused `durable_pending_before_projection` to fail. Initial
entities, sibling capture, removal receipt and old producer admission passed. The unchanged original
probe's separate 9/10 failure remains in [before-results.json](before-results.json).

[The intermediate ledger](implementation-attempts.json) preserves the first implementation's progress
count failures, strict sibling comparison failure, and an HTTP bind failure before source assertions.
A plain restart of the preserved pre-fix binary reproduced the sibling difference as only
`dc.terms.created` on AST entities: the producer emits that predicate from per-ingest `IndexedAt`.
Independent review approved excluding only that one valid timestamp on known AST sibling IDs while
recording both values in `*-ast-reseed-metadata.json`. Every other fact, lifecycle marker, ID and exact
content byte stays strict; document timestamps are not excluded. Behavioral negative tests enforce
those limits. The corrected pre-fix restart control passed 8/8; its original 7/8 evidence is retained.

The seed oracle reads typed journal values and observed opaque key prefixes without implementing their
codec. It verifies exact manifest IDs, checksum and acknowledged current-epoch receipts. Same-handle
admission, path existence and timestamps are not substitutes for publication evidence.

## Unqualified boundaries

[#1444](https://github.com/C360Studio/semstreams/issues/1444) prevents terminal all-input removal proof.
A complete current retained pass stays `pending` / `applied_tail_unproven`, and periodic repair remains
active. Each relevant case separately records `automatic_terminal_removal_completion` as
`qualified: false` in `qualification-gates.json`; that gate is not counted as a passing assertion.

[#1445](https://github.com/C360Studio/semstreams/issues/1445) prevents safe selective clearing through
the pinned typed mutation API. Current-epoch receipts and fingerprints do not supply a caller-fenced
revision or prove continuing publication/withdrawal eligibility. Positive freshness stays red.

[#1446](https://github.com/C360Studio/semstreams/issues/1446) leaves unknown remote mutation recovery
unavailable. Deterministic tests prove durable effect fences, exact terminal evidence and refusal of
unsafe source generation changes. A graph readback, timeout or restart cannot resolve an unknown
remote commit. The normal process cases do not inject that uncertainty.

This follow-up uses BM25 only. It does not rerun semantic/semembed workloads, prove numeric equality
through the pinned graph decoder, or admit SemEngine extraction or mainline cutover.

## Reproduction and cleanup

The final execution selected both the unchanged and additive tests. Use a fresh output directory for
any reproduction; preserve failures and identify the executable independently of the test checkout.

```bash
setup03a_nats_digest=sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66
SETUP03A_BINARY=/absolute/path/to/reviewed-semsource \
SETUP03A_OUT=/absolute/path/to/fresh-removal-replay-evidence \
SETUP03A_REMOVAL_ACTIVATION=restart \
SETUP03A_IDENTITY=governed \
SETUP03A_PROFILE=bm25 \
SETUP03A_NATS_IMAGE="nats:2.14.4-alpine@$setup03a_nats_digest" \
go test -tags=qualification,removal -count=1 -v \
  -run '^Test(SourceRemovalLifecycle|RemovalReplay)' ./test/setup03a
```

All six exact broker container IDs were confirmed absent after the final run. Raw
`final-container-inspect` output sits beside each report, with combined `final-process-cleanup.json`.
Additive cases check application exit status; `TestReplayCheckedStop` rejects exit code 23 and accepts
observed zero under race detection. Intentional SIGKILL is separately observed. The unchanged original
probe's legacy `stop(false)` does not assert the application wait error, so its result adds no graceful
exit proof. Raw evidence under `/tmp/semsource-removal215-evidence` is temporary session material;
versioned ledgers preserve its compact outcomes and hashes, not durable copies of every raw artifact.
