# Proposal: Init and Config Validation

**Priority: P0** (audit 2026-07-19, findings: high ×2 — both silent-kill defaults on the first-run path)

## Why

The two first-touch surfaces — `semsource init` and config validation — can each produce a
configuration that looks valid, passes `semsource validate`, boots cleanly, and yields a silently
broken graph:

1. **Default `semsource init` ingests zero git history**: the wizard writes git URL `"."`
   (`cli/wizard.go:115`), which slugs to an empty system segment; every commit/author/branch entity
   is rejected at the publish gate while status reports healthy. The default install's git story is
   dead on arrival and nobody is told.
2. **Namespace/org is accepted with zero format validation** (`config/config.go:182`): a dotted or
   spaced org (e.g. `acme.io`) passes the wizard and `semsource validate`, then every published
   entity fails ID validation at runtime — a permanently empty graph with a green config check.

Ease-of-install is a headline rubric item for the first public sem* product; both defects sit on
theexact path a new user walks in their first five minutes.

## What Changes

- The wizard derives a valid git source identity for the current repo (real remote URL or a
  path-derived slug — never `"."`), and the resulting default config ingests actual git history.
- Namespace/org (and any other config value that becomes an ID segment) is validated at config
  load against the same segment contract the graph enforces (`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`);
  invalid values fail `semsource init`, `semsource validate`, AND `semsource run` startup with an
  actionable message naming the field and the allowed alphabet.
- `semsource validate` SHALL be a real pre-flight: a config that passes validate SHALL NOT be
  rejectable later purely for ID-shape reasons.
- Wizard/validate error text tells the user exactly what to change (field, value, why).

## Capabilities

### New Capabilities
- `cli-onboarding`: `semsource init` (and `init --quick`) produce configurations whose every
  source actually ingests on first run; validation failures on the onboarding path are actionable.

### Modified Capabilities
- `runtime-configuration`: "SemSource has one runtime configuration" gains ID-shape validation
  requirements — config values that become ID segments are validated at load with the graph's own
  segment contract (ADDED requirements; existing single-runtime behavior unchanged).

## Impact

- `cli/wizard.go`, `cli/` quick-init path, `config/config.go` (Validate), `cmd/semsource/run.go`
  startup validation ordering.
- Depends on: `no-silent-entity-loss` centralizing the segment contract in `entityid` (this change
  reuses that validator; sequence after it or share the helper in this change).
- Consumers: none directly (CLI-local); downstream products benefit from defaults that ingest.
- Boundary check: pure product-side; the segment regex mirrors semstreams' published contract,
  imported/duplicated as a constant with a pinning test — no substrate changes.
