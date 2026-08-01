## Context

See `proposal.md` — Why for the measurement. This records the decisions, and one constraint
discovered while scoping the measurement that changes what it can prove.

`EntityIDEdgesConfig` (semstreams `processor/graph-clustering/component.go:112`) is richer than a
switch:

| Field | Default | Notes |
| --- | --- | --- |
| `include_siblings` | `true` (tri-state `*bool`) | Entities sharing the 5-part type prefix |
| `sibling_weight` | 0.7 | |
| `max_siblings` | 10 | |
| `include_system_peers` | `true` (tri-state `*bool`) | Entities sharing the `system` segment |
| `system_peer_weight` | 0.3 | |
| `max_system_peers` | 15 | |

The caps matter to the diagnosis: system peers do **not** build a complete graph — each entity gets
at most 15. Collapse still occurs because 15 arbitrary same-system neighbours per entity is far more
than enough for label propagation to flood a single connected component. So "disable it" and "tune
it" are both plausible fixes, and only measurement separates them.

## Goals / Non-Goals

**Goals:**

- Make edge synthesis configurable without changing substrate behavior when unset.
- Choose SemSource's shipped default from measurement on the shape that matters.
- Leave a record that distinguishes a deliberate override from an inherited accident.

**Non-Goals:**

- Community **summary** quality — a separate, confirmed-independent defect
  ([semstreams#829](https://github.com/C360Studio/semstreams/issues/829)): the summarizer is never
  given entity content, so it summarizes the ID taxonomy. Communities became small and topical and
  summaries did not improve, which is what isolates it.
- Adding language parsers (see Risks — this bounds the measurement but is not fixed here).
- `min_community_size`, `max_iterations`, and the structural/anomaly knobs.

## Decisions

### D1 — Pass the whole block through, tri-state, rather than adding one boolean

A single `clustering_system_peers` boolean would encode today's diagnosis into the config surface.
The caps and weights are equally plausible levers, and the multi-repo measurement may well land on
"system peers on, cap lowered" rather than "off".

The substrate's config is deliberately tri-state — `*bool` where nil means "use the default" — and
the passthrough preserves that: an omitted block sends no keys at all. This is the difference
between "SemSource has no opinion" and "SemSource says false", and conflating them would silently
change behavior for anyone who upgrades without touching config.

### D2 — Do not ship a changed default until the multi-repo shape is measured

The single-repo measurement is unambiguous: 82% → largest community 93. It is also **not** the
shape that matters most.

SemSource's `system` segment is constant per source, so a single-repo deployment has one system and
system peers are degenerate. The A/B corpus this feeds is deliberately multi-repo — OSH Core plus
Meshtastic plus the Connected Systems API — where `system` varies and system-peer edges become
meaningful: they group by repo.

Whether that is *useful* grouping or merely one blob per repo is unknown. Both are plausible, and
they imply opposite defaults. Measuring it is a task in this change, not a follow-up.

### D3 — The default is a config value, not a code constant

`tier2-compose-dev.json` carries the measured default; `run.go` passes through whatever the config
holds and hardcodes nothing. An operator who disagrees changes JSON, and the tier config remains the
single place that states what tier 2 means.

## Risks / Trade-offs

- **The multi-repo measurement is bounded by our parser coverage** → We parse Go, Java, Python,
  TypeScript/JavaScript, and Svelte (`source/ast/`). The A/B corpus is not all covered: Meshtastic
  firmware is **C++** and MAVLink is **C**, neither of which we parse; the OSH side including its
  Connected Systems API service is Java, which we do. So a multi-repo graph would carry Java code
  entities plus docs/config from the rest, and entity density would skew heavily to one repo.
  That is still a genuine multi-system graph and still answers the clustering question — but the
  result must be read as "multi-system, one dominant language", not "multi-language". State that
  alongside the numbers rather than letting a later reader assume broader coverage.
- **The parser gap is on the A/B's critical path, beyond clustering** → The A/B prompts are "write
  a Meshtastic driver using OSH's CS API" and "write a MAVLink driver for OSH using CS API". An
  agent doing that needs the C/C++ side as *code*, not as file-and-doc entities. That is a product
  gap this change does not close and should not be conflated with it; it wants its own change, and
  it is arguably higher priority for the A/B than the default chosen here.
- **A measured default can still be wrong for an unmeasured shape** → Mitigated by the spec
  requirement to state the shape a default suits, and by tri-state passthrough so any operator can
  override without a code change.
- **Two variables could be confounded if measured together** → Change one setting per run, as the
  single-repo measurement did. The recorded baseline makes each run a direct comparison.

## Migration Plan

Additive and backward compatible: an existing `semsource.json` with no edge-synthesis block behaves
exactly as today, because an omitted block sends no keys. The only behavior change ships in
`tier2-compose-dev.json`, and only after D2's measurement.

Sequencing: add the config type and passthrough → verify omission is a true no-op → run the
multi-repo measurement → set the tier-2 default from it → document both shapes.

## Open Questions

- Which Connected Systems API repository semdev intends. OSH's own `sensorhub-service-consys`
  module is Java and already inside osh-core; a separate specification or reference-implementation
  repository would add a different mix. This changes the corpus composition but not the design, and
  can be settled when the measurement is set up.
