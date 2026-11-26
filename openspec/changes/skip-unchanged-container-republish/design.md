# Design: skip unchanged container republishes

## Where the gate sits

`publishContainer` (`processor/ast-source/fileset.go:168`) is the one place every folder/repo publish passes through
(full pass, watch event, the emptied-folder path). The gate goes there:

1. Build the payload as today.
2. Fingerprint its triples, excluding predicate `dc.terms.created`: sorted `subject\0predicate\0object\0datatype`
   lines, sha256.
3. If `skipUnchanged` and the fingerprint equals the one stored for the ID, return admitted without publishing.
4. Otherwise publish; on success, store the fingerprint under the ID.

`skipUnchanged` is true only for the periodic reindex call (`component.go:733`). The seed (`component.go:500`) and
watch-event publishes always send, so a seed after restart still rewrites every container once, and watch behaviour
is exactly as today. They do record the fingerprint, so the first reindex after a seed is already quiet.

## Concurrency

Container publishes run under `pw.filesMu` (the caller holds it, `fileset.go:150`), so one watch path's container
publishes are serialised. The fingerprint map is per component and keyed by entity ID, which is unique per watch
path's scoped system; a mutex guards it because watch paths run concurrently.

## Why not `all=false` on the reindex

`planContainers(..., all=false)` would publish only ancestors of changed paths. It also drops the full-pass
handling of colliding IDs and of containers whose earlier publish was refused before `pendingContainers` existed. A
content gate keeps every current full-pass guarantee and removes only identical writes.

## Memory

One 32-byte sum per container ID; bounded by the folders the watch path indexes (semengine: ~1.3k).
