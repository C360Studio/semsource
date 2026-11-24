# Refuse media sources that would share an entity-ID system

Approved-by: Coby Leuschke (2026-11-24) sha256:338bb96242ad

Track: standard (extends one Requirement in `entity-identity-safety` to three more component kinds; message wording
otherwise).

## Why

image-, audio- and video-source publish with reconcile on (`processor/image-source/component.go:157`,
`processor/audio-source/component.go:157`, `processor/video-source/component.go:163`). Each mints under system
`entityid.SystemSlug(root)` (`handler/image/handler.go:194`, `handler/audio/handler.go:197`,
`handler/video/handler.go:197`) with a content-hash instance, and the component instance comes from `paths[0]` alone
(`internal/sourcespawn/components.go:375`). The #349 check (`internal/sourcespawn/collisions.go:38`, `idSpace`) covers
only ast-, doc- and objectstore-source, so two media sources of one kind that reach one root, or roots whose basenames
slug to one system, write the same IDs unchecked. While their configs match nothing visible breaks; when they differ
(one stores binaries and carries a storage ref, the other does not) each reconcile strips the other's predicates (#366).

Ruling: one ID space **per media kind**, not one shared space. The kinds mint distinct entity types (`image`, `audio`,
`video` and `keyframe`: `handler/image/handler.go:195`, `handler/audio/handler.go:211`,
`handler/video/handler.go:211,283`), so an image and an audio source over one root cannot collide and stay admitted
(owner, 2026-11-24, comment on #366).

## What Changes

- **`idSpace` gains image-, audio- and video-source,** each its own space. Their roots are their `paths`, under
  `SystemSlug(path)` (they have no `project`), so the existing file, boot and add checks refuse them, and the
  same-instance takeover check now covers their `paths[0]`-derived instances.
- **`CheckSourceCollisions` plans `image`, `audio` and `video` entries** alongside `ast`, `docs` and `s3`.
- **Refusal wording, no behaviour change:**
  - an add that rebuilds a registered instance from other roots (narrowing or moving it) says to remove the source,
    then add it, or else to give each source a distinct project; a new source's own shared-system refusal keeps the
    distinct-project remedy;
  - a root that two file entries of one instance both list counts once in the system check, so the refusal no longer
    names one root twice (a repeat with no other shared-system root is left to the same-instance message); the check
    order is unchanged, as the scenarios name the shared system;
  - "no discovered root is kept" says a restart reconciles the discovery record.

## Out of scope

cfgfile (reconcile off until #348), #350, #354.
