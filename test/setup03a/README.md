# SETUP 03A known-answer corpus

This is a versioned, black-box migration workload. It calls the compiled binary's public NATS graph queries and
HTTP code/doc fusion routes; it does not link SemSource or SemStreams implementation packages. The literal expected
IDs in `fixtures/v1/manifest.json` are frozen beta.161 answers, not a second production ID builder.

## Baseline disposition and SemEngine handoff

The [current 03A merge ruling](../../docs/testing/setup-03a/merge-readiness.md) accepts the measured
SemStreams baseline. Historical JSON results and failed assertions remain unchanged. In particular,
`broker_restart_reingested_exact_relationship` and `broker_restart_reingested_exact_content` remain
known-at-pin failures tracked by SemStreams #1442 / SemEngine #15. Classification does not skip either
assertion, turn it green, or change a failing process exit.

This directory is the acceptance corpus for later SemEngine qualification. Next consumer work is
SETUP 04A on a separate branch built wholly on SemEngine after 03B approval. It must add and qualify a
true no-embedder composition; the existing BM25 structural checks are its comparison baseline, not
proof that SemSource already supplies a graph-only runtime. Keep the profile crosswalk below explicit.
The admitted 04A graph-foundation slice must pass both named broker-reingestion assertions above;
known-at-pin classification does not carry into engine qualification. Lexical retrieval is 04B and
neural retrieval is 04C, so the 04A gate does not imply running or qualifying the full neural workload.
Mainline remains on SemStreams until the full semembed-backed 04C workload and promised lower-profile
regressions pass. This handoff changes no fixture, test behavior, provider input or module pin.

## Run

Use a separately built binary for each revision and a fresh evidence directory. Docker is required. Neural selection
requires a real semembed endpoint and fails if it is missing; it is never skipped or substituted with a fake embedder.

```bash
SETUP03A_BINARY=/absolute/path/semsource-beta161 \
SETUP03A_OUT=/absolute/path/evidence/beta161-bm25 \
SETUP03A_PROFILE=bm25 \
go test -tags=qualification -count=1 -v ./test/setup03a

SETUP03A_BINARY=/absolute/path/semsource-beta161 \
SETUP03A_OUT=/absolute/path/evidence/beta161-neural \
SETUP03A_PROFILE=neural \
SETUP03A_PROVIDER=http://127.0.0.1:8081/v1 \
go test -tags=qualification -count=1 -v ./test/setup03a
```

Add `SETUP03A_IDENTITY=governed` for the pinned revision. The adapter checks ADR-102's reordered segments against the
same fixture and checks the only variable segment against ADR-104's persisted `platform_identity` record in
`semstreams_config_setup03a_semsource`. Every application/broker restart reuses those exact expected IDs and authority.
A fresh store per revision is intentional: this does not claim an in-place legacy graph conversion.

`SETUP03A_NATS_IMAGE` defaults to `nats:2.14.4-alpine`. Use the same immutable digest for paired runs. The harness
records the actual Docker image/container inspection, final config, binary SHA-256, corpus SHA-256, raw application
logs, broker logs, and a machine-readable `report.json`. Failed semantic assertions remain failures in that report and
the process exit code. The report is flushed after every check, including before later failures.

## Profile crosswalk and frozen inputs

| Harness profile | SemSource selected config | SemStreams capability | SemEngine qualification slice |
| --- | --- | --- | --- |
| `bm25`, structural checks | `configs/tiers/tier0-statistical.json` | Structural slice under BM25 | 0 |
| `bm25`, retrieval checks | `configs/tiers/tier0-statistical.json` | BM25 | 1 |
| `neural` | `configs/tiers/tier1-semantic.json` | Real HTTP embeddings | 2, retaining structural checks |

The harness loads the selected config and replaces only source paths, fixture namespace/project/revision, per-run
ports, and the provider URL. It retains model name, query prefix, index-worker count, and coalescing settings.
Record the provider image/build, served model artifact/version, and dimensions alongside the report before starting;
the config's requested model is not proof of the served model. The same real provider must serve both revisions.
Neural runs record provider request counters before ingestion and after cold/warm queries to prove actual use, and
exercise a paraphrase whose expected answer is the north-depot document passage.
`TestProviderFailureAndRecovery` adds a separate run through an owned HTTP proxy: real initial retrieval,
503 unavailability, a stalled request reaching the caller deadline, then real recovery. It records evidence under
`provider-faults/`, requires an injected provider request, and rejects an outage returned as confident absence.
The proxy never modifies the shared provider container or its served model.

## Expected behavior

The corpus contains four functions across two packages, two named `Greet`, plus one Markdown passage. The initial
`Run` calls alpha's `Greet`; editing it replaces that relationship with `Farewell` and changes the exact body. The
fixture's project, registered revision, source path, exact entity identity, and nonzero authority revision must agree.
The unrelated `AbsentQualificationSymbol` must not resolve. Duplicate-name lookup must return both distinct anchors.

Code/doc natural-language queries run cold and warm with a one-node cap and must remain in their requested domain.
A document metadata parent has no body by design. The separate document content assertion retrieves the passage with
a wider cap and compares exact bytes, including the terminal newline; it never treats a metadata parent as a missing
passage body. Code content is exactly the parser's function declaration slice, excluding its leading comment.

Deleting the watched Go file retains addressable history with `entity.lifecycle.stale`; it does not require physical
purge. Recreation must clear staleness under the same identity, and exact current relationships must not accumulate
obsolete calls. An explicit public `graph.lifecycle.run` waits for completion of the declared source scope, avoiding
an arbitrary wait for the watch-triggered background lifecycle pass.

An application restart must preserve the edited answers. For a separate persistent-broker restart, the harness stops
all application workers with `SIGSTOP`, captures an authentic ingestion envelope, acknowledges its replay while
proving authority revision has not advanced, edits the source, and kills the application. The same Docker container
restarts with the same `/data`; its dynamically assigned port is rediscovered. Before restarting the app, the test
checks retained authority revision and content. Default memory `GRAPH` transport is expected to disappear: a publish
ack is **not** a promise of broker durability. The required recovery is source re-ingestion of the changed file.
`pending-authentic-envelope.json` and `restart-guard.json` retain transport and applied-sequence evidence.

The final isolated RPC probe widens only this owned broker's `GRAPH` subject to `graph.ingest.>` after stopping the
application. An exact-query RPC then receives a JetStream PubAck, reproducing upstream #1143's overlapping-subject
hazard. The normal semantic workload has already proved the explicit-subject composition routes actual queries.
This probe does not mutate an external broker or fix the substrate.

## Resource ownership and limitations

Each run owns one uniquely named/labeled NATS container, capped at one CPU and 512 MiB, with dynamic loopback ports.
It preserves logs before removing only its own exact container ID. Application stop/join and broker commands have
finite deadlines. Readiness uses bounded authoritative polling; source delivery must settle with zero loss, and
known-answer checks and index status establish semantic readiness. No fixed sleeps substitute for state checks.

This small corpus does not establish the OSH scale/capacity regression, physical purge, generation, clustering,
SemEngine exclusive-module admission, or full repository dogfooding. Those evidence rows must remain separate. A
passing suite does not waive upstream defects or the independent Go review gate.
