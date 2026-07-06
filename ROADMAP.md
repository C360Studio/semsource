# SemSource Roadmap

SemSource is in **public beta** (`v1.0.0-beta.1`). This page is an honest snapshot
of what works today, what's intentionally limited, and where we're headed — so
early adopters know what to build on now and what's coming.

It's a direction, not a dated commitment. Items are grouped by how far along they
are, not by release date. The "why" behind most of these lives in
[`docs/adr/`](docs/adr/); non-trivial work is specced first under
[`openspec/`](openspec/).

## In the beta today

- **Graph-first ingestion** of code (Go, TypeScript/JavaScript, Java, Python,
  Svelte), Markdown/docs, config files (go.mod, package.json, pom.xml, Dockerfile),
  URLs, and media (metadata by reference).
- **Deterministic fusion gateway for agents** — `code_context`, `code_search`,
  `code_impact`, `doc_context`, and `code_changes` — over **MCP, NATS, and HTTP**.
  An agent queries the graph instead of grepping the filesystem.
- **Domain-scoped retrieval** so a small doc set isn't drowned by a large code
  corpus (and vice versa).
- **Ontology-aware ranking** — results are ranked by BFO/CCO class specificity and
  predicate salience (public API boosted, superseded versions demoted), not by
  lexical match alone (ADR-0005).
- **Versioned source, retained and related** — every version of a symbol is kept
  and linked by supersession lineage; the current version ranks above historical
  ones; `code_changes` tells you what changed between two versions (added / removed
  / changed symbols with before/after bodies).
- **Governed graph** — deterministic 6-part entity IDs, ownership bootstrap, and
  semantic envelopes on every write, on the SemStreams substrate.
- **One-command bringup** — `docker compose up` starts SemSource + NATS + tier-1
  semantic search, indexing the current directory, with the MCP gateway on `:8080`.

## Known limitations (by design, for now)

- **Same-LAN deployment focus.** No built-in TLS / reverse-proxy hardening yet;
  run it behind your own gateway for anything exposed.
- **Media payloads are on the local filestore, not object storage.** Image
  originals + thumbnails, video keyframes, and audio are stored to a local
  filestore when a store root is configured; there's no shared ObjectStore backing
  for media bytes yet (code/doc bodies already use ObjectStore).
- **Version diffs don't detect renames.** A renamed or moved symbol shows up as a
  removal plus an addition, not a single "renamed" entry.
- **The graph is retention-first — nothing is deleted by policy.** Safe,
  reference-complete deletion for genuine mistakes/churn is a deliberate future
  step (see below), not a gap we paper over.
- **GraphQL** is reachable via the `ui` compose profile (Caddy on `:3000`), not
  host-published in the core profile.

## On the roadmap

### Next — code & version intelligence

- **Commit-level changesets** — which commit / PR touched which symbols, linking a
  change entity to the symbols it edited (ADR-0008). Complements today's
  version-to-version `code_changes`.
- **Rename / move detection** in version diffs — correspond a renamed symbol across
  versions instead of reporting remove + add (uses embedding similarity).
- **Dependency-version lifecycle** — refresh-on-bump (index the new version, add
  supersession edges) with bounded retention, so upgrading a dependency updates the
  graph additively.

### Next — retrieval & ranking

- **Tier-2 semantic + instruct** — community/GraphRAG summaries and
  `local`/`global`/`summary` search (wired today via `graph.enable_clustering`;
  being validated and made first-class).

### Later — operations & scale

- **Operator dashboard** — health, graph visualization, and source search over the
  existing status/query APIs.
- **Multi-instance federation validation** — multiple SemSource instances producing
  identical `public.*` IDs that merge cleanly across deployments.
- **Sidecar branch lifecycle** — dynamic repo registration and branch-aware
  cleanup for tools that index many short-lived branches (ADR-0007).
- **ObjectStore backing for media** — move image/video/audio payloads from the
  local filestore to a shared ObjectStore (as code/doc bodies already are), so
  media is servable location-independently.

### Exploring — lifecycle & deployment

- **Safe retraction / deletion** — referentially-complete removal for mistakes and
  churn, off the critical path. This depends on upstream SemStreams primitives
  (index cleanup on delete + referential cascade); until those land, the graph
  stays retention-first (ADR-0008).
- **Semantic change summaries** — natural-language "this bump changed retry
  semantics" over a version diff (tier-2 LLM). Far out; the deterministic diff is
  the durable core.
- **Beyond same-LAN** — TLS and reverse-proxy hardening for exposed deployments.

## Tell us what matters

Early-adopter feedback reorders this list. If a capability here (or one that isn't)
would unblock you, open an issue on
[GitHub](https://github.com/C360Studio/semsource/issues) — real consumer pressure
is how we prioritize.
