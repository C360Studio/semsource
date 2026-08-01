## Why

SemSource composes `graph-clustering` but sets only two of its knobs —
`detection_interval` and `enable_llm` (`cmd/semsource/run.go:905`). Everything else runs on
substrate defaults, including virtual-edge synthesis:

| Default | Effect |
| --- | --- |
| `include_system_peers: true`, `max_system_peers: 15`, weight 0.3 | Edges between entities sharing the `system` segment |
| `include_siblings: true`, `max_siblings: 10`, weight 0.7 | Edges between entities sharing the 5-part type prefix |

**In a SemSource deployment the `system` segment is effectively constant.** Measured: a compose
deployment yields `system = workspace` for every entity; a `repo` source yields
`system = github-com-opensensorhub-osh-core` for every entity. So system-peer synthesis links each
entity to 15 arbitrary same-system peers across the whole graph, and label propagation collapses.

Measured on osh-core `sensorhub-core` (6,685 entities), changing that one default:

| | `include_system_peers: true` (default) | `false` |
| --- | --- | --- |
| Largest community | **5,487 — 82% of the graph** | **93** |
| Other communities | — | 7, 7, 7, 7 |
| Community identity | one undifferentiated blob | topical: `ModuleEvent.java`, `Event.java`, `RangeFilter.java` |

A community holding 82% of the graph makes "community-backed" answers meaningless, which is the
headline finding in [`docs/testing/tier-baselines.md`](../../../docs/testing/tier-baselines.md).

We cannot currently change this without editing Go: the values are hardcoded in `run.go`.

## What Changes

- **Expose `graph-clustering`'s `entity_id_edges` block through `semsource.json`**, passed through
  to the component. The substrate's config is deliberately tri-state — `nil` means "use the
  substrate default", and only an explicit `true`/`false` overrides — so passthrough must preserve
  that rather than flattening unset to `false`.
- **Choose SemSource's default from measurement, not from the substrate's.** The substrate default
  suits graphs with many distinct systems; SemSource's dominant shape has one. The default this
  change ships is set by the multi-repo measurement below, not assumed.
- **Document which deployment shape each setting suits**, in `configs/tiers/README.md` and the tier
  baselines doc, so an operator can tell whether their graph is single- or multi-system before
  choosing.
- **Measure the multi-repo shape before fixing the default.** The A/B corpus this feeds
  (osh-core + Meshtastic + Connected Systems API) is deliberately multi-repo, where `system` varies
  and system-peer edges are no longer degenerate — they group by repo. Whether that produces useful
  communities or merely one blob per repo is unknown and must be measured, not assumed. This is the
  gating task, not a follow-up.

## Non-goals

- Changing `graph-clustering` itself. Edge synthesis, LPA, and the defaults are substrate; this
  change only decides what SemSource passes and what it documents.
- Fixing community **summary** quality. That is a separate defect with a separate cause — the
  summarizer is never given entity content, so it summarizes the ID taxonomy
  ([semstreams#829](https://github.com/C360Studio/semstreams/issues/829)). Confirmed independent:
  after communities became small and topical, summary quality did not improve.
- Tuning `min_community_size`, `max_iterations`, or the structural/anomaly knobs. Worth revisiting,
  but each needs its own measurement and none is implicated in the collapse.
- Any change to tier 0/1, which do not run clustering at all.

## Consumers

Anyone running tier 2. Today that is the tier-2 compose overlay and the planned model A/B; the
capability also reaches agents through `graph_search`'s community-backed rung, so the blast radius
is the honesty of GraphRAG answers rather than an API shape.

## Capabilities

### Modified Capabilities

- `runtime-configuration`: the clustering edge-synthesis knobs become part of the documented,
  validated configuration surface instead of hardcoded values.
- `semstreams-governance`: SemSource's composition of a substrate component must state which
  defaults it accepts and which it overrides, with the reason recorded.

## Impact

- `config/config.go` — new optional config block under `graph`, tri-state to match the substrate.
- `cmd/semsource/run.go:905` — pass it into the `graph-clustering` config map; omit the block
  entirely when unset so substrate defaults still apply.
- `configs/tiers/tier2-compose-dev.json` — carries the measured default.
- `configs/tiers/README.md` and `docs/testing/tier-baselines.md` — document the single-system vs
  multi-system distinction and the measured numbers.
- No change to tier 0/1 configs, the MCP surface, or any query contract.
