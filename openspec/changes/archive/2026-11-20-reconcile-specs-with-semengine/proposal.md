# Reconcile the current-truth specs with the SemEngine substrate

Approved-by: Coby Leuschke (2026-11-20) sha256:a92592e07504

Track: standard (renames, modifies, and removes Requirements; adds none).

## Why

#236 asks that nothing agent-facing teach a dead dependency. Two current-truth specs still do, and
they are normative inputs, not history (codex review of #318):

- `semstreams-governance` "Current SemStreams target is explicit" requires `go.mod` to pin
  `github.com/c360studio/semstreams v1.0.0-beta.160`. `go.mod:36` requires
  `semengine v0.1.0-alpha.6` and no SemStreams module (`TestNoSemStreamsImports`). The toolchain
  scenario names Go `1.26.3`; `go.mod:3` is `go 1.26.11`. Ten more Requirements name SemStreams as
  the substrate whose lanes, profiles, catalog, and subjects SemSource conforms to.
- `consumer-migration-baseline` "SemStreams remains the shipped substrate" says mainline stays on
  SemStreams until neural qualification. Main cut over at Tier 0 (#222, ADR-0013), and
  `tier0-consumer-qualification` already states the cutover rule.

## What Changes

- `semstreams-governance`: rename "Current SemStreams target is explicit" to "Current SemEngine
  target is explicit" and restate it for a released SemEngine tag with no `replace`. Modify the
  toolchain gate to follow the `go` directive in `go.mod`. In ten Requirements, name SemEngine as the
  substrate. The guarantees are unchanged. "Incompatible beta graph state is rebuilt from source" now
  names the SemEngine storage break instead of beta.160 adoption. One scenario changes: "Embedding
  component declares no output ports" named `graph-embedding`, which Tier 0 no longer composes, so it
  becomes any composed component the framework declares output-less.
- `consumer-migration-baseline`: remove "SemStreams remains the shipped substrate", because
  `tier0-consumer-qualification` "Mainline cutover qualifies Tier 0 and BM25 while deferring neural
  04C" replaces it. The frozen SETUP 03A baseline on SemStreams stays recorded in the Purpose and in
  "Migration comparisons use immutable inputs".
- Purpose text of `semstreams-governance` updated in place at archive.

## Non-goals

- Renaming the `semstreams-governance` capability. Fourteen Go tests cite it by path, and the name
  is history.
- Changing code or tests. Every cited requirement name except the renamed one is kept, and no test
  cites the renamed one.
- "The disabled-envelope workaround remains retained": it mentions SemStreams#1443 as history and is
  still true.
