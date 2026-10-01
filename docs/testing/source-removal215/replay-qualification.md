# Additive source-removal replay qualification

This suite extends the preserved #215 pre-fix evidence; it does not change the original SETUP 03A
corpus, removal probe, or ledgers. It uses only a compiled binary, public RPCs, and read-only inspection
of its private broker. The versioned `removal-replay-v1` workload adds target documents A/B, a separate
document sibling, and the original AST source sharing the target's system but a different taxonomy.

## Current evidence

[The additive pre-fix run](additive-before-results.json) ran the original-source retirement case against
preserved binary SHA `7847df902bef611c777df4a19366dac896a9e86cc80cd475623d40d679c0a198`.
It exits 1 with seven of eight observations passing. The sole failure is
`durable_pending_before_projection`: the pre-fix binary has no lifecycle journal bucket. Initial exact
A/B entities, sibling graph/content capture, removal receipt, and admitted old producer all pass.
The added shutdown oracle observes application exit code zero. The exact owned NATS container is gone;
raw logs, report, command, identities, and cleanup hashes are recorded in that JSON.

`TestReplayCheckedStop` passes with the race detector and rejects exit code 23 while accepting observed
exit code zero. Tagged `go vet` passes. Post-fix process execution remains pending a runnable binary;
compiling the remaining cases is not runtime qualification.

## Process cases and retained limits

- `TestRemovalReplayRetirement`: original and runtime-added sources; durable intent before observed
  SIGKILL; same-broker restart; exact retained parent/passage markers; unchanged sibling graph and bodies.
- `TestRemovalReplayReaddBeforeRestart`: two remove/add cycles; generation replacement; current boot
  seed/receipt proof; an authoritative KV watcher rejects even a transient old-removal marker. Before
  stopping, the watcher must observe each target's current authoritative revision.
- `TestRemovalReplaySelectiveReactivation`: observed process exit before offline deletion of B;
  current-epoch exact A-only manifest/receipts; A becomes fresh while B remains retained `source_removed`.
  This does not replace the deterministic integration test for crash after A+B receipts but before
  receipt persistence/projection, or injected partial mutation and paging failures.

The seed oracle reads typed journal values, uses observed opaque key prefixes without implementing
the codec, and verifies the exact manifest ID set, checksum, and acknowledged current-epoch receipts.
Same-handle admission, path existence, or entity timestamps cannot substitute for that evidence.

[Upstream #1444](https://github.com/C360Studio/semstreams/issues/1444) prevents automatic terminal
removal completion on the frozen pin. A successful retained marker pass must remain journal `pending`
with `applied_tail_unproven`; periodic repair remains active. Each such run writes the separately named
`automatic_terminal_removal_completion` gate with `qualified: false` to `qualification-gates.json`.
This explicitly unqualified gate is not counted as a successful assertion or silently skipped.
Selective reactivation has an independent current-publication proof and does not close that gate.

## Commands for the eventual reviewed binary

Select a fresh output directory for each execution. Keep the recorded NATS digest and BM25 profile;
these tests do not start or use an embedding provider. All processes and containers are privately owned.

```bash
setup03a_nats_digest=sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66
SETUP03A_BINARY=/absolute/path/to/reviewed-semsource \
SETUP03A_OUT=/absolute/path/to/fresh-removal-replay-evidence \
SETUP03A_IDENTITY=governed \
SETUP03A_PROFILE=bm25 \
SETUP03A_NATS_IMAGE="nats:2.14.4-alpine@$setup03a_nats_digest" \
go test -tags=qualification,removal -count=1 -v \
  -run '^TestRemovalReplay' ./test/setup03a
```

To repeat only the captured pre-fix oracle, select
`-run '^TestRemovalReplayRetirement$/^original$'` with the preserved binary and another fresh directory.
Do not relabel that expected failure as a passing baseline or use retries to conceal it.
