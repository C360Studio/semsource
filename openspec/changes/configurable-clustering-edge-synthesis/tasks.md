# Tasks

## 1. Config surface

- [ ] 1.1 Add an optional `entity_id_edges` block under `graph` in `config/config.go`, mirroring
      semstreams' `EntityIDEdgesConfig`: `include_siblings` and `include_system_peers` as `*bool`
      (tri-state), plus `sibling_weight`, `max_siblings`, `system_peer_weight`, `max_system_peers`.
      Pointers and zero-means-unset are load-bearing — see 1.3.
- [ ] 1.2 Document each field's meaning in the struct tags, including which graph shape it suits.
      An operator reading only the config must be able to tell that `include_system_peers` is about
      the `system` ID segment.
- [ ] 1.3 Pass it through in `cmd/semsource/run.go:905`. **Omit the key entirely when unset** — do
      not send an empty object, and do not marshal `false` for a nil `*bool`. Omission must be a
      true no-op that leaves substrate defaults intact.
- [ ] 1.4 Validation: accept the block on tiers without clustering (inert, not an error), and
      reject nonsensical numerics (negative weights, negative caps).

## 2. Prove omission changes nothing

- [ ] 2.1 Unit test: with no `entity_id_edges` in config, the composed `graph-clustering` config map
      contains no synthesis keys at all.
- [ ] 2.2 Unit test: with only one field set, exactly that key is emitted and no sibling keys are
      manufactured. This is the tri-state guarantee — a nil `*bool` must not become `false`.
- [ ] 2.3 Unit test: the block on a tier with clustering disabled validates and composes no
      clustering component.

## 3. Measure the multi-repo shape — gating, not follow-up

- [ ] 3.1 Build the multi-repo corpus: OSH Core plus Meshtastic plus the Connected Systems API
      source (see design — Open Questions for which CS API repo). Record entity counts per repo.
- [ ] 3.2 Record the parser-coverage skew alongside the corpus: Meshtastic firmware is C++ and
      MAVLink is C, and we parse neither, so those repos contribute file/doc/config entities rather
      than code symbols. The measurement is "multi-system, one dominant language" and must be
      labelled as such.
- [ ] 3.3 Measure community-size distribution with system peers ON (substrate default) — the
      question is whether multi-system yields useful grouping or one blob per repo.
- [ ] 3.4 Measure with system peers OFF. One variable per run; the single-repo baseline
      (largest 5,487 → 93) is the comparison.
- [ ] 3.5 If neither extreme is good, measure `max_system_peers` / `system_peer_weight` before
      concluding. "Off" is the blunt instrument, not automatically the answer.

## 4. Ship the default

- [ ] 4.1 Set `tier2-compose-dev.json`'s edge-synthesis block from 3.3–3.5. Do not set it from the
      single-repo result alone.
- [ ] 4.2 Leave tier 0/1 configs untouched — they run no clustering.

## 5. Document both shapes

- [ ] 5.1 `docs/testing/tier-baselines.md` — add the single-repo numbers (82% → 93) and the
      multi-repo numbers, each labelled with the graph shape and the parser-coverage caveat.
- [ ] 5.2 `configs/tiers/README.md` — explain the single-system vs multi-system distinction and
      when to override, so an operator can tell which case they are in.
- [ ] 5.3 State plainly that community *summary* quality is a separate, unfixed defect
      (semstreams#829), so better clustering is not mistaken for better answers.

## 6. Gates

- [ ] 6.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`, and
      `go test -tags=integration ./...` green.
- [ ] 6.2 `openspec validate configurable-clustering-edge-synthesis --strict` green.
- [ ] 6.3 Boot a real tier-2 stack and confirm the composed clustering config carries the intended
      settings — the collapse was only ever visible at runtime, and a unit test on the config map
      does not prove the substrate honored it.

## 7. Not this change

- [ ] 7.1 **C / C++ / Rust parsers.** The A/B prompts ("write a Meshtastic driver using OSH's CS
      API", "write a MAVLink driver for OSH using CS API") need the C/C++ side as code, not as file
      entities. This is a product gap on the A/B's critical path and is arguably higher priority
      than the default chosen here. It needs its own change.
- [ ] 7.2 Community summary content (semstreams#829) — upstream, already filed.
