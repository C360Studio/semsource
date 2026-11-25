# Design: fold same-coordinate declarations within one cfgfile manifest

## Context (verified at main 61edfd7)

- Every cfgfile instance is `declared(instance, rel)` = `<instance>-hash8(rel)` (`handler/cfgfile/entities.go:699`), so
  IDs are already per manifest (#348). Within one manifest the instance is the coordinate alone: Go
  `hash8(path@version)` (`:101`), npm `hash8(name@version)` (`:203`), Maven `hash8(g:a@v)` (`:371`), Gradle
  `hash8(g:n@v)` (`:520`), image `slugify(image)` (`:247`).
- Per-declaration facts on those IDs: `dependency-indirect` (Go), `dependency-kind` `npm-prod`/`npm-dev` (`:204-207`),
  `dependency-scope` (Maven, `:391-399`), `dependency-configuration` (Gradle). `source/vocabulary/config.go:41-53`.
- The per-file builders emit one state per declaration and append each ID to the parent's `DepIDs` (`:594`, `:614`,
  `:648`, `:678`), so a repeated coordinate yields two states and a duplicate `requires` edge in one pass.
- Parsers: npm appends `dependencies` then `devDependencies` (`parsers.go:142-147`); `pomDepXML` has no `type` or
  `classifier` field (`parsers.go:247`); Gradle matches five string-notation configurations (`parsers.go:320`);
  Dockerfile dedups the raw `FROM` string, not its slug (`parsers.go:176`).
- Multi-valued literal facts already exist on one cfgfile entity: `image-exposed-ports`, one triple per port
  (`entities.go:266-275`).
- `entity-identity-safety` "One entity ID per declaration within a parse pass" allows folding same-symbol
  declarations into one entity, and requires a discriminator to apply to every declaration of its kind.

## Goals / Non-Goals

Goals: no manifest emits two states under one ID or a duplicate edge; every usage fact survives; IDs stay intrinsic.
Non-goals: the #336 reconcile opt-in; Gradle map notation or version catalogs; cross-manifest artifact nodes.

## Decisions

### D1. Repeated usage: fold into one entity (recommended) or mint one entity per usage

- **Fold.** Builders group by ID within the file and merge usage facts. Gradle configurations and Maven scopes become
  one triple per distinct value; npm kind is `npm-prod` if any declaration is prod; Go indirect is the AND. Dependency
  IDs are unchanged. Cost: a small merge step in four builders, one package. Forecloses: per-line provenance (which
  line said `test`), which nothing reads.
- **Per usage.** Put configuration/scope/dev into the instance. Cost: one string per constructor, but every Maven,
  Gradle and npm dependency ID changes, and "does this manifest use X" returns several entities. Forecloses nothing
  new but doubles dependency nodes for common Gradle `implementation` + `testImplementation` pairs.
- Adopter seam: a reader of `dependency-scope` or `dependency-configuration` must accept more than one value per
  entity. The one in-repo reader, `processor/mcp-gateway/graph_matches.go:265-292`, renders the first value per
  predicate, so graph search shows one of the folded usages (sorted first) until multi-value rendering lands (review
  finding on PR #383; #385). The full facts stay in the graph. The OSH scorecard G band asserts name and
  version, not scope. Doing nothing leaves last-writer-wins facts and blocks the reconcile.

### D2. Maven `<type>`/`<classifier>`: artifact identity (recommended) or ignore

- **Identity.** Parse both; when `type` is set and not `jar`, or `classifier` is set, hash them into the instance and
  emit them as facts. A plain jar keeps today's ID. This applies to every declaration that sets them, never only to
  colliding ones. Cost: two parser fields, two predicates, one constructor.
- **Ignore and fold.** Folds a `test-jar` into the main jar; loses that they are different files.

### D3. Image instance: slug plus raw-reference hash (recommended) or slug only

- **Slug + `hash8(image)`.** Every image ID changes once; images that slugify alike stay apart. Rides the beta.11
  reseed. **Slug only** with fold: wrong, since `foo/bar:1` and `foo-bar:1` are different images.

## Risks / Trade-offs

- Multi-valued `dependency-scope` is new for consumers; documented in the adoption note for beta.11.
- Folding order: merged values are sorted so a state does not depend on declaration order.

## Migration Plan

Clean storage break plus reseed, already required for beta.11 (#348, #340). No aliasing of old image IDs.

## Open Questions

- **Owner ruling:** D1 fold (recommended) vs one entity per usage. D2 and D3 follow the identity rule and are
  recommended as written; approving the proposal accepts them.
