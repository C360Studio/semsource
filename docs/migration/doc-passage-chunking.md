# Migration: document passage chunking

Documents are now ingested as a navigational **parent** entity plus one **passage**
entity per structural section, instead of a single entity carrying the whole file.

**A reindex is not sufficient. Rebuild the graph from empty.**

## Why an in-place reindex leaves the graph wrong

Re-ingesting into an existing graph converges most of the change, but not all of it,
because of one substrate property: `StorageRef` can be swapped but **not cleared**.
A republished entity carrying a nil `StorageRef` preserves the stored one
(`graph/mutation_requests.go:82-88`, `graph-ingest/component.go:2470-2472`; upstream
gh#260).

Parent document entities used to carry a whole-file body. After this change they
carry none — but a reindex cannot remove the ref that is already there. The result is
a graph where every parent still points at its old whole-file blob and still produces
the diluted, truncated whole-file vector that passages exist to replace, competing
with the passages for the same queries. The corpus looks migrated and behaves as if
it were not.

Passage entities and triples migrate correctly on their own; it is only the parent's
stored body reference that cannot be undone in place.

## How to migrate

Stop the stack, delete the graph state, and start it again. Ingestion re-seeds from
source on startup — seeding is the first pass of the normal event loop, not a separate
mode — so there is nothing to export or replay.

```bash
docker compose down -v      # -v removes the NATS volume holding graph state
docker compose up
```

Wait for `graph.query.status` to report `phase: "ready"` before querying. Readiness
means every source has been seeded, so the corpus is complete when it flips.

## What changes for consumers

- **Doc bodies now hang off passages, not files.** Anything resolving
  `source.doc.body-store` / `source.doc.body-key` from a file-level entity finds
  nothing. Follow `code.structure.belongs` from a passage, or query `doc_context`,
  which returns passage-scoped evidence and needs no change.
- **A `doc_context` answer may return several passages of one document** where it
  previously returned one node per file. Consumers that assumed one evidence node per
  file should group by `source.doc.file-path`.
- **`source.doc.summary` is gone.** It duplicated `dc.terms.title` and its only reader
  was an unreachable fallback. Read `dc.terms.title`.
- **New entity type segment `chunk`**, with IDs shaped
  `{org}.semsource.web.{system}.chunk.{path-slug}-{ordinal}`.

SemSpec and SemDragon read documents through `doc_context` and the governed graph, and
neither resolves body handles directly, so neither requires a code change.

## Operational requirement

The verbatim body store is now **required**. If it cannot be created, `doc-source`
fails startup with an explicit error rather than continuing without it. Previously an
unavailable store logged a warning and carried on, which left every document without a
retrievable body and without an embedding while the component reported healthy.

Deployments already running the standard Compose profile need no change — the store
rides on the NATS JetStream object store that is already present.
