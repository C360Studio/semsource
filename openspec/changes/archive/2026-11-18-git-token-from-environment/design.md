## Context

The token reaches the bucket because it rides in the component configuration: SemEngine's config
manager pushes every `components.<name>` key verbatim (`config/manager.go` `encodePush`/`pushToKV` in
the pinned module) and redacts nothing. It reaches `semsource.json` because `add`/`remove` round-trip
the defaulted struct. Both come from one root: an environment value resolved into a struct that is
later serialised.

All consumers of the token run in the SemSource process: the git source's clone and pull
(`handler/git` → `workspace`), boot-time discovery's default-branch resolution and remote clones
(`internal/sourcespawn`, `cmd/semsource/discovery.go`), and runtime-spawned sources from
`add_source_repo`.

## Goals / Non-Goals

Goals: no serialised configuration holds the token; `add`/`remove` preserve the file. Non-goals are in
`proposal.md`.

## Decisions

### D1. One accessor reads the variable at the point of use

`config.GitToken()` returns `os.Getenv("SEMSOURCE_GIT_TOKEN")`. The git source calls it in
`Initialize` and hands it to the git handler; boot discovery and runtime spawning call it where they
build `sourcespawn.Options`. `config.Config.GitToken` is deleted, so no struct that is marshalled can
carry it.

Rejected: a `json:"-"` field on the component config. Component configs are decoded from the bucket
JSON, so such a field is always empty in the component; the token would have to be injected through a
side channel anyway.

### D2. The file field is rejected with a pointer to the variable

Removing the field makes strict decoding reject it as an unknown field. `LoadConfigFromReader` wraps
that one error with "set SEMSOURCE_GIT_TOKEN instead", so an upgrading operator sees the fix rather
than a bare unknown-field message. This matches the object-store credential rule (rejected by strict
decoding).

### D3. The git-source configuration drops `git_token`; generation 5 rewrites the bucket

The field leaves `processor/git-source/config.go`, and `internal/sourcespawn/components.go` stops
writing it. The git-source config is decoded non-strictly, so a retained key that still has the field
loads and the value is ignored.

Ignoring is not enough: the retained key still holds the token. The removal changes the generated
composition, so the fingerprint test forces `compositionGeneration` 4 → 5. On the next boot the file is
newer, `pushToKV` rewrites every `components.*` key from the file, and the existing
`pruneRetainedComponents` deletes keys the file lacks. The latest revision of every key is token-free
after one boot.

### D4. `add`/`remove` edit the file's own keys

A new `config.EditFile(path, func(*Config) error)`:

1. reads the file;
2. validates it through `LoadConfigFromReader`, so a broken file still fails before anything is
   written;
3. decodes it a second time into `map[string]json.RawMessage`;
4. decodes only `sources` and `config_revision` into a `Config`, runs the edit, and replaces those two
   keys in the map;
5. writes the map back.

Defaults and environment values never enter the written file because they only ever exist in the
validated copy, which is discarded.

Trade-off: the map marshals keys in sorted order, so the first `add` on a hand-written file can
reorder its top-level keys. Values and nested structure are unchanged.

## Risks / Trade-offs

- **KV history.** The config bucket keeps five revisions per key (SemEngine's descriptor, History 5).
  Generation 5 writes a new revision; up to four older ones still hold the token until later writes
  push them out. Deleting history means purging keys SemEngine owns, which this change does not do.
  Mitigation: tagged deployments start on fresh storage at beta.10; a deployment that ran main with a
  token rotates it. The release note says so.
- **Breaking file field.** A file with `git_token` stops loading. The error says exactly what to do.
- **Key reordering** by `add`/`remove` (D4).

## Migration Plan

1. Move `git_token` from `semsource.json` to `SEMSOURCE_GIT_TOKEN`.
2. Restart; generation 5 rewrites the retained component keys.
3. If the deployment ran with a token before this change, rotate the token.

## Open Questions

None.
