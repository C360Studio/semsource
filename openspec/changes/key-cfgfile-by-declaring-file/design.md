# Design: key cfgfile entities by their declaring file

## Premises (verified at main aec1f7f)

- `handler/cfgfile/entities.go` builds every ID as `authority.Build(system, "config", <type>, instance)`, with one
  system per source (`handler.go:626`). Instances: go/npm/maven/gradle dependency = `hash8(coordinate@version)`
  (`:98`, `:200`, `:368`, `:517`); image = `slugify(image)` (`:244`); npm package = `slugify(name)`, else
  `hash8(filePath)` (`:146-148`); Maven project = `slugify(group:artifact)`, else `hash8(filePath)` (`:300-302`); POM
  module = `slugify(name)` (`:420`); Gradle project = `slugify(dirBase)`, else `hash8(filePath)` (`:464-466`, `:663`);
  Go module = `slugify(modulePath)` (`:46`).
- `filePath` is the `filepath.Walk` path under the configured root (`handler.go:189`) or the watch event path
  (`:305`). It is absolute whenever the root is.
- Per-declaration facts on shared IDs: `dependency-scope` (`:388`), `dependency-configuration` (`:538`),
  `dependency-indirect` (`:120`), `dependency-kind` (`:121`, `:222`, `:386`, `:534`), `file-path` and
  `image-exposed-ports` (image, `:256-270`).
- Readers: `processor/mcp-gateway/graph_matches.go:41-44` lists the dependency predicates as match fields;
  `processor/source-manifest/status.go:354` maps `config.requires` to the relationship role. Neither depends on one
  node per coordinate. The OSH scorecard G band asserts dependency name and version by value (`questions-osh.json`).

## Options

**A. Declaring file in identity (recommended).** Append `hash8(rel)` to every instance (`rel` = path relative to the
source root). One function change per constructor plus one `rel` computed per file.
- Cost: small, one package. Blast radius: every cfgfile ID (clean break). Facts per node come from one file, so the
  #336 reconcile and the `idSpace` refusal become straightforward later.
- Forecloses: a single node per artifact. A cross-manifest view is a query (`dependency-name` = X).

**B. Shared artifact plus declaration entity.** The artifact node keeps intrinsic facts only (name, version, ecosystem
with Maven and Gradle unified as `maven` coordinates). A new `config.declaration` entity per manifest×dependency
carries scope/configuration/indirect/dev, with edges manifest → declaration → artifact. Same split for Dockerfile →
image.
- Cost: new entity type, vocabulary, about 2× the entity count on dependency-heavy repos (OSH), and MCP match fields
  and the scorecard G band change. The shared node still has many writers, so the #336 reconcile needs per-predicate
  ownership rules before cfgfile can opt in.
- Keeps the one-node join that no current reader uses.

**C. Facts as manifest predicates** (for example `requires-test`). Rejected: loses version per scope and multiplies
the vocabulary.

## Ruling needed

A vs B, and whether the rule covers every cfgfile type (recommended, same defect) or only the three in #348.
