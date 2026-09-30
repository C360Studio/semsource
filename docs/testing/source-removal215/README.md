# Source removal replay: issue #215

[Issue #215](https://github.com/C360Studio/semsource/issues/215) extends
[draft migration PR #213](https://github.com/C360Studio/semsource/pull/213). Before implementation,
the unchanged removal probe reproduced the missing `source_removed` markers: **9 of 10 checks pass;
the process exits 1**. This records the defect, not an acceptance pass. No retry was used to obtain green.

[The compact pre-fix record](before-results.json) contains exact commands, observations, artifact hashes,
and cleanup evidence. The frozen [SETUP 03A pins](../setup-03a/pins.json), comparison ledgers, and versioned
fixtures remain unchanged. Later #215 acceptance evidence must be recorded separately.

## Exact pre-fix inputs

| Input | Recorded value |
| --- | --- |
| SemSource commit | `fa5c25082b0de7f2d56d7e6281d1e2c348fdee4b` |
| Binary SHA-256 | `7847df902bef611c777df4a19366dac896a9e86cc80cd475623d40d679c0a198` |
| Corpus v1 SHA-256 | `9724050f1ebdc0623460557bd4bb5c73c9b2591b4032737b79f9e7db821b0258` |
| SemStreams commit | `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` |
| SemStreams module | `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` |
| Go environment | `go1.26.4`, `darwin/arm64`, `CGO_ENABLED=1` |
| NATS release | `2.14.4-alpine` |
| NATS digest | `sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66` |

The source checkout was clean before and after the build. The binary matches the prior final migration
executable; intervening fixture, test, and documentation changes did not alter it. The probe selects
BM25 (`tier0-statistical.json`), covering the structural/lexical slices. No embedding provider was
started or used, and no provider or dependency pin changed.

## Recorded commands

These commands ran from the migration repository at the commit above. Preserve the recorded output;
choose a fresh evidence directory for any later reproduction.

```bash
go build -o /tmp/semsource-removal215-evidence/semsource-before ./cmd/semsource

setup03a_nats_digest=sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66
SETUP03A_BINARY=/tmp/semsource-removal215-evidence/semsource-before \
SETUP03A_OUT=/tmp/semsource-removal215-evidence/before \
SETUP03A_REMOVAL_ACTIVATION=restart \
SETUP03A_IDENTITY=governed \
SETUP03A_PROFILE=bm25 \
SETUP03A_NATS_IMAGE="nats:2.14.4-alpine@$setup03a_nats_digest" \
go test -tags=qualification,removal -count=1 -v \
  -run '^TestSourceRemovalLifecycle$' ./test/setup03a
```

The provider environment variable was unset. The run started September 30, 2026 at 23:34:52 UTC and
completed in 52.395 seconds with exit code 1. The [unchanged harness][harness] never sends its own
`graph.lifecycle.run` trigger to repair the missing behavior.

[harness]: ../../../test/setup03a/removal_test.go

## Observed failure and successful surrounding behavior

Initial sources settle without loss, and the persisted authority is verified. Removing the original
JSON document source returns the expected desired-change/restart receipt. It remains live until
restart and is absent afterward. Both its document parent and passage remain exactly addressable,
but neither contains `entity.lifecycle.stale=source_removed`. Their observed marker is absent
(`null` in the compact record), not a different accepted value. This is the sole failed check.

The automatic `graph.lifecycle.run` request capture is empty during the probe. The test subsequently
re-adds the same disabled handle, verifies runtime deferral, and confirms admission on the next restart.
Those receipt and activation checks pass; they do not prove graph-marker clearing, since no removal
marker was present in this pre-fix run.

## Ownership and evidence limits

Docker inventory was empty before the run. The probe owned one uniquely identified NATS container,
with dynamic loopback ports, one CPU, and 512 MiB memory. It preserved application/broker logs and
removed that exact container. Post-run inspection reports no such object and `docker ps -a` is empty.
Raw inspection and cleanup results are retained with their hashes in the compact record.

The existing harness's `stop(false)` consumes but does not assert the application wait error. These
semantic restart checks therefore do **not** newly certify graceful process exit. Container absence
is checked separately and must not be confused with application exit status.

Raw files are retained under `/tmp/semsource-removal215-evidence`, including `before/report.json`,
three application logs, broker log and inspection, both source-change receipts, the empty automatic
request capture, build/environment metadata, run outcome, cleanup, and the evidence hash inventory.
They are supporting session artifacts; the compact record versions their identities and outcomes.
No durable-replay, paging, duplicate-delivery, fault-recovery, sibling-isolation, or SemEngine admission
claim is established by this pre-fix reproduction.
