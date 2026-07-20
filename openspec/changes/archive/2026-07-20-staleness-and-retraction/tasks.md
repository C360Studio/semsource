## 1. Marker vocabulary (D1)

- [x] 1.1 Register `entity.lifecycle.stale` in source vocabulary with `WithWeight(-3.0)`
      (below superseded_by's −2.0), reason values `file_deleted`/`source_removed`/
      `path_missing`. Proves: vocabulary unit test pinning weight ordering.

## 2. Lifecycle pass (D2 + D4)

- [x] 2.1 Lifecycle pass in the supersession component: enumerate a source's entities by
      prefix, group by path predicate, stat under the source root; missing → marker write
      (update lane AddTriples), present+marked → clear (RemoveTriples). NATS trigger +
      optional interval; idempotent/resumable. Proves: unit tests on the diff/converge logic.
- [x] 2.2 `remove_source` triggers a scoped lifecycle pass stamping `source_removed` markers.
      Proves: integration test — remove → markers observable within one pass.
- [x] 2.3 Skip non-watched sources entirely (D5 coherence). Proves: unit test — frozen
      source's vanished path yields no marker.

## 3. Watch-path fast paths (D6)

- [x] 3.1 fsnotify OpDelete/OpRename arms publish targeted marker writes (debounced past the
      coalesce window) instead of being discarded. Proves: watcher unit test + integration
      test (delete file → marker within one interval; recreate → marker cleared).

## 4. Doc identity (D3)

- [x] 4.1 `handler/doc` instance = path slug; content hash moves to a predicate; edit
      re-ingests the same entity (in-place replace). BREAKING doc IDs; migration = reindex,
      release-noted. Proves: doc handler unit tests (stable ID across edits, one live entity)
      + integration test (edit → single entity, updated content).

## 5. True-freeze snapshots (D5)

- [x] 5.1 `watch:false` disables periodic reindex unless `index_interval` explicitly set;
      docs/tool descriptions updated to match. Proves: config/spawn unit test + doc-claims
      check.

## 6. End-to-end + acceptance

- [x] 6.1 Integration test over the real stack: delete a watched file → entities marked and
      demoted below live entities in ranking; recreate → marker cleared; removed source →
      `source_removed` provenance. Proves: `TestIntegration_StalenessLifecycle`.
- [x] 6.2 Update ADR-0008 (the deferred exception now references this change's spec);
      `openspec validate --all` green.
