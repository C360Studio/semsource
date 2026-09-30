# SemStreams consumer asks

SemStreams owns framework repairs. SemSource reports evidence here and does not commit to that repository.
The SETUP 03A pin remains frozen; SemEngine qualification and repair-before-port decisions are separate.

| Issue | Consumer evidence | Migration disposition |
| --- | --- | --- |
| [#1442: applied-sequence guard after stream recreation](https://github.com/C360Studio/semstreams/issues/1442) | beta.161 and pinned BM25/neural runs retain an old applied sequence after memory GRAPH recreation and reject changed-source re-ingestion | Blocks broker recovery and SemEngine admission; retain the failing corpus assertions |
| [#1143: RPC stream collision](https://github.com/C360Studio/semstreams/issues/1143) | Isolated wildcard `graph.ingest.>` probe receives a JetStream PubAck for an RPC request at both pins | Keep explicit publish subjects in SemSource; retained framework path needs repair/admission review |

The exact revisions, provider pins, profile crosswalk, and before/after results live in
[SETUP 03A compatibility](../testing/setup-03a/compatibility.md). The separate
[SemSource GRAPH capacity regression #178](https://github.com/C360Studio/semsource/issues/178)
is measured with the versioned OSH transport probe; the existing parser guard limits what that run proves.
