# Design: graph-search-match-properties

## Context

`deriveMatches` (processor/mcp-gateway/graph_matches.go) renders three substrate response
shapes: digests (id/type/label/relevance/tags — no triples), full entities (id + all
triples — the common path below the summarize threshold, per semstreams#823), and bare
entity IDs. Today the entities path reads exactly one predicate (`dc.terms.title`) and
discards the rest. The standing anti-bloat decision (in `graphSearch`'s comment: 197 KB of
raw triples for a 38-hit query, 11x code_search) rules out rendering triples wholesale.

## Goals / Non-Goals

- Goal: value-shaped facts (config versions, kinds, scopes; artifact version) answerable
  in one `graph_search` call, at a measured, capped context cost.
- Non-goal: general triple projection, relationship rendering, or hydration of
  triple-less paths. Non-goal: any substrate change (rendering only).

## Decisions

1. **Allowlist, not denylist.** A fixed, package-level allowlist of value-bearing
   predicates. Starting set (from `source/vocabulary/predicates.go` constants — resolve
   exact names at implementation):
   - Config family values: dep name, dep version, dep kind, dep scope, dep configuration,
     dep indirect; module path, module go-version; package version; project
     group/artifact/version/packaging/build; image name; file path.
   - Code family: `code.artifact.version` (the one value-shaped code fact an agent asks
     for by value).
   Relationship predicates (requires/depends/contains), bodies, signatures, comments, and
   timestamps (`dc.terms.created`/`modified`) are structurally excluded by not being
   listed.
2. **Caps**: 8 properties per match, 160 bytes per value (post-truncation marker-free —
   hard cut; values here are versions/slugs/paths, so 160 B is generous). Deterministic
   order: allowlist declaration order, not map iteration (determinism gate #127 applies
   to ingest, but the same doctrine holds — no map-range output).
3. **Multi-valued predicates**: first value wins (matches `entityTitle`'s existing
   first-match semantics); config value predicates are single-valued by construction.
4. **JSON shape**: `"properties": {"<predicate>": "<value>", ...}` keyed by full
   predicate string — self-describing for agents, no invented short names.
5. **Object decoding**: `substrateEntity.Triples[].Object` stays `string`-typed. All
   semsource producers emit string objects; a non-string object would already break the
   current label rendering (whole-body unmarshal → disclosure-only fallback). Not
   changing that behavior in this change; if it ever fires, the existing fallback path
   reports honestly.
6. **Scorecard proof**: `questions-osh.json` v2 adds a config band — one name-level
   question (answerable since #157) and value-level questions hitting
   `ConfigDepVersion`/`ConfigDepConfiguration`. Gate: value questions FAIL against the
   pre-change build and PASS post-change (probe-becomes-test). Both checkers re-run
   against the OSH pin; README config-band paragraph updated. Context-bytes delta for
   arm B recorded on both corpora (dogfood set is v4-frozen — no dogfood question
   changes; its arm B context delta still measures the rendering cost on code/doc
   corpora, expected ≈ 0 because code/doc entities carry few allowlisted predicates —
   only `code.artifact.version` and doc file paths… verify empirically, do not assume).

## Amendment (FAIL-before diagnosis, 2026-08-13)

The FAIL-before run falsified an assumption: the gateway sent
`summarize_threshold: 1`, so any graphrag query with >1 hits returned the compact
digest shape — no triples, and dependency digest labels are the ID's hash instance
(upstream naming gap; every dependency match on the OSH corpus rendered as a bare
hash, so even #157's name-level facts never showed on this rung). G01 (the
name-level control) missed for exactly this reason. Decision: send
`summarize_threshold: 0` (explicitly disabled; absent means default 50) so the
graphrag strategy returns full EntityStates — the shape the entities path renders
labels and properties from. Community summaries are unaffected
(`include_summaries` defaults true upstream, independent of auto-summarize).
Agent-side output stays bounded by maxGraphMatches; the full shape's transfer cost
is gateway-internal (upstream caps candidate loading at
MaxTotalEntitiesInSearch). The digest-label gap is filed upstream (asks-file
entry) since digests remain reachable via the semantic strategy's own shaping.

## Amendment 2 (PASS-after run 1 failure, 2026-08-13)

The first PASS-after run returned `matches: null` on every config question: with the
full-entity shape enabled, the response carries triples with NUMERIC objects
(`code.metric.start-line`, `code.metric.end-line`, `code.metric.lines`,
`source.doc.chunk-index` — named by a direct NATS probe of the live stack), and
`substrateEntity`'s string-typed `Object` failed the WHOLE unmarshal — every match
silently collapsed into the disclosure-only fallback. This falsifies design decision
5 ("all producers emit string objects"). Fix: `Object json.RawMessage` + a scalar
converter (strings unquote; numbers/bools render compactly; composites render as
absent). Regression test carries the substrate shape verbatim. Measured transfer for
the full shape at count=100: 513,859 B gateway-internal per call.

Also recorded: the dogfood arm-B context delta required by task 2.3 is structurally
zero — no dogfood question invokes `graph_search`, and both edits are
`graph_search`-only; the 10 shared OSH questions moved −0.3 % (run noise), which is
the empirical form of the same statement.

## Risks / Trade-offs

- Context growth on config-heavy corpora: bounded by 25 matches × 8 props × ~200 B ≈
  40 KB worst case; realistic OSH growth is far smaller and measured before merge.
- Allowlist drift: a new value predicate someone adds later won't render until listed —
  acceptable; the failure mode is the status quo (absence), never bloat.
- The v2 OSH set forks the comparability domain; SUMMARY-rc-beta6 numbers remain v1 and
  are not re-stated against v2.
