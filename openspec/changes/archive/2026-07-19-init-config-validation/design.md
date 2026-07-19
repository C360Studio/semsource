# Design: Init and Config Validation

## Context

Audit-confirmed silent-kill defaults on the first-run path: the wizard writes git URL `"."`
(`cli/wizard.go:115`) which slugs to an empty system segment (every git entity rejected
post-publish, status healthy); namespace/org accepts any string (`config/config.go:182`) so a
dotted org passes `semsource validate` and produces a permanently empty graph at runtime.

## Goals / Non-Goals

- Goals: `init` defaults that actually ingest; `validate` as a real pre-flight (validate-pass ⇒
  no later ID-shape rejection); actionable errors at the earliest surface.
- Non-goals: new wizard features; changing the 6-part scheme; sanitizing user config values
  silently (we reject with guidance, not mutate — config is user intent).

## Decisions

- **D1 — derive real git identity**: wizard resolves, in order: `origin` remote URL → repo-slug
  (existing remote-slug rules); no remote → absolute repo path with the path-derived slug used by
  local sources; never `"."`. Quick-init same. A produced config's git source must yield a
  non-empty, valid system segment by construction.
- **D2 — reject, don't mutate**: config load validates every ID-destined value (namespace/org, and
  any explicit source identity overrides) against the segment contract via the same substrate
  validator adopted in no-silent-entity-loss (semstreams `ValidateEntityID` on a composed probe
  ID). Errors name the field, the offending value, and the allowed alphabet. Silent sanitization
  rejected: an org the user typed as `acme.io` becoming `acme-io` invisibly would violate
  least-surprise and provenance.
- **D3 — validate-pass property**: `semsource validate` composes a representative entity ID per
  configured source (using each handler's identity rules) and validates it — a property test pins
  "validate ok ⇒ publish gate ok" for generated configs.
- **D4 — same gate at run startup**: `semsource run` re-runs config validation before spawning
  components, so a hand-edited bad config fails fast with the same actionable message instead of
  publishing rejected entities.

## Risks / Trade-offs

- [Existing configs with invalid orgs now fail startup] — they were producing empty graphs;
  failing loudly is the fix. Release note with the exact error text and remedy.
- [Wizard remote parsing edge cases] covered by existing remote-slug rules + tests; fallback is
  the path slug, never `"."`.

## Migration Plan

Ship after (or alongside) no-silent-entity-loss to share the validator. Rollback = revert.

## Open Questions

- none blocking.
