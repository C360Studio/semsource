# Design

## D1 Identity
- `newGoDependencyEntity`: `contentHashShort(depPath)`; npm: `contentHashShort(name)`; Maven: `contentHashShort(key)`
  (key keeps type/classifier); Gradle: `contentHashShort(group + ":" + name)`. All keep `declared(instance, rel)`.
- `newDockerImageEntity`: `imageName(ref)` strips `@digest`, then a `:tag` only when the last colon follows the last
  slash (`registry:5000/foo` keeps its port). Instance: `slugify(name) + "-" + contentHashShort(name)`.
- Per-manifest folding (`foldByID` and the per-file builders from #375) merges the version, or the image reference,
  into a sorted multi-valued fact on the surviving entity, like scopes and configurations today.
- `requires` edges list each ID once (the existing #375 dedupe).

## D2 Staleness
- cfgfile-source appends `graph.LifecycleOwnerTriple(state.ID, owner, state.UpdatedAt)` to every state before publishing,
  with `owner = graph.LifecycleOwner(instance, root)`, as doc-source does (`processor/doc-source/component.go:475`).
- The dependency and image builders emit `source.config.file-path` with the declaring manifest's path. The Dockerfile
  image already does.
- cfgfile-source triggers `graph.PublishLifecycleTrigger` with `RootPath: root, Owner: owner, Systems: [system]`:
  once per root after the seed settles, and on a watch delete event. It uses the same degraded-condition logging as
  doc-source.
- `supersession.pathOf` adds `source.ConfigFilePath`. `livenessOracle`'s stat function uses the path as given when
  `filepath.IsAbs`, else joins `RootPath`.

## D3 Why the clear works
A re-created manifest is republished whole, and the pass's clear path removes the marker from entities whose path
stats present (`lifecycle.go`, `toClear`). No new code is needed beyond D2.
