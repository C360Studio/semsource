# fusion-gateway

## ADDED Requirements

### Requirement: Domain-scoped NL retrieval per lens

The fusion gateway SHALL constrain natural-language seed resolution to the
querying lens's own domain, so that in a corpus where one domain is much larger
than another, the smaller domain is not diluted by the larger co-resident one.

When a request does not carry an explicit retrieval scope, the gateway SHALL
default `fusion.Request.Scope` to the entity-ID prefixes of the lens's domain(s),
formed as `{org}.{platform}.{domain}` where `org` is the deployment's single
global org (platform identity), `platform` is `semsource`, and `domain` is each
domain the lens covers:

- the `docs` lens covers the `web` domain;
- the `code` lens covers the AST-parsed language domains (`golang`, `python`,
  `typescript`, `javascript`, `java`, `svelte`).

Scoping is a filter on NL seed resolution only; symbol and prefix resolve modes
are unaffected (per the SemStreams `fusion.Request.Scope` contract). The default
SHALL NOT override a caller-provided scope, and SHALL be omitted (no filter) when
the org is unknown, preserving prior unfiltered behavior.

#### Scenario: A doc-shaped NL query is not diluted by a larger code corpus

- **GIVEN** a corpus with many `{org}.semsource.golang.*` code entities and few
  `{org}.semsource.web.*` doc entities
- **WHEN** a natural-language query is fused through the `docs` lens without an
  explicit scope
- **THEN** the gateway defaults the scope to `{org}.semsource.web` and the
  resolved seeds are doc entities, not code entities

#### Scenario: A code NL query scopes to code-language domains

- **WHEN** a natural-language query is fused through the `code` lens without an
  explicit scope
- **THEN** the gateway defaults the scope to the code-language domain prefixes
  (`{org}.semsource.golang`, `.python`, `.typescript`, `.javascript`, `.java`,
  `.svelte`), so NL seeds resolve from code, not from `web`/`config`/`git`/`media`

#### Scenario: A caller-provided scope is respected

- **WHEN** a request already carries a non-empty `Scope`
- **THEN** the gateway uses that scope verbatim and does not apply the per-lens
  default

#### Scenario: No org identity means no scope filter

- **WHEN** the gateway has no org (empty platform identity, e.g. a standalone or
  test context) and the request carries no scope
- **THEN** no scope is applied and NL seed resolution runs unfiltered, exactly as
  before this capability existed
