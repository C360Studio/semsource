## 1. Config surface

- [ ] 1.1 Test first: `graph_stream` decodes `posture` and `max_bytes`; `streams` is rejected with an error naming it; an unknown posture or a non-positive `max_bytes` fails validation.
- [ ] 1.2 Replace `Config.Streams` / `StreamOverride` with `GraphStream`; delete the subject guard and its tests.

## 2. Stream config

- [ ] 2.1 Test first: `graphStreamConfig` gives file storage for durable and the default, memory for ephemeral, and the fixed subjects, workqueue, discard new, 1 h and 1 replica for both; `max_bytes` overrides only the byte bound.
- [ ] 2.2 Rewrite `graphStreamConfig` on the posture; move the e2e retention config to `graph_stream.max_bytes`.
- [ ] 2.3 Integration test for the durable-restart scenario: publish with graph-ingest stopped, stop and start the NATS container (same filesystem), then graph-ingest applies the messages.

## 3. Measure (D3)

- [ ] 3.1 Five seeds per posture of the retention fixture; post the median, spread and GRAPH bytes on the PR.
- [ ] 3.2 If the bar is missed, stop and take the numbers to the owner before 2.1's default lands.

## 4. Docs and release

- [ ] 4.1 Document `graph_stream` where the config reference lives; add the drift remedy to the release note on #408.
- [ ] 4.2 `task verify`; `go test -race -tags=integration ./config/... ./cmd/semsource/...`; `go test -tags=e2e -run Retention ./test/e2e/`.
- [ ] 4.3 Review via `ateam review request`; archive before ready.
