# Skip unchanged folder and repo republishes on the periodic reindex

Approved-by: Coby Leuschke (2026-11-26) sha256:f497a487db8d

Track: design (changes what the reindex publishes; durability trade-off named below).

## Why

Dogfood findings F1–F3 (#401). Each periodic reindex (`index_interval`, 60 s by default with `watch: true`) already
skips files whose hash is unchanged (`processor/ast-source/component.go:741`), but it republishes **every** folder
and repo entity: `reindexPath` calls `publishContainers(..., all=true, ...)` (`component.go:733`). Each container carries
`dc.terms.created` = `time.Now()` (`source/ast/hierarchy.go:43`, `BuildHierarchyFromFiles`, `source/ast/entities.go:205`), so every pass is a
real `ENTITY_STATES` write even when nothing changed. On the mini, six repos made ~390 writes/min idle. That is ~3.5
cores per instance (semsource 142% + NATS 235% vs 21% + 3% with the reindex off), and `embedding.lag` never falls
because `target_revision` keeps moving (F3).

## What Changes

- The ast-source remembers, per container ID, a fingerprint of the statements the publisher last **admitted** from
  this process, leaving out `dc.terms.created`. A reindex skips a folder or repo publish whose fingerprint matches.
- A refused publish records nothing, so the next pass retries it (the existing `pendingContainers` path is unchanged).
- Two live folders that sanitize to one ID alternate content, so their fingerprints differ and both still publish
  on a full pass, as today.
- Watch-event container publishes are unchanged.
- Tests first: a second reindex of an unchanged tree publishes zero containers (fails on main); a changed child
  republishes its ancestors and the repo; a refused publish is retried on the next pass.

## Decisions you are making by approving

- **Only folder and repo entities get the gate.** File and symbol entities are already gated by the file hash; the
  other sources are untouched.
- **"Admitted" is the bar, as for the file hash gate today.** If an admitted container publish is later lost
  downstream, this process does not resend it until that container changes or the process restarts. That is the same
  property file entities have now. The alternative (a periodic forced full republish) brings back part of the churn.
- **A local fingerprint helper in ast-source**, a third copy of the cfgfile/url-source one (`processor/cfgfile-source/reconcile.go:46`),
  marked `// shortcut:`. Extracting a shared package is a follow-up, not this change.
- **`dc.terms.created` on a container stays the time of its last real write.** It no longer moves every minute.

## Out of scope

- Reparse CPU: the reindex still parses every file to compute hashes. #407 measures what remains.
- The warm-restart index plateau (#403) and the 30 s stop deadline (#406).
