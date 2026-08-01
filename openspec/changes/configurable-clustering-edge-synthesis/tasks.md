# Tasks

## 1. Config surface

- [x] 1.1 Add an optional `entity_id_edges` block under `graph` in `config/config.go`, mirroring
      semstreams' `EntityIDEdgesConfig`: `include_siblings` and `include_system_peers` as `*bool`
      (tri-state), plus `sibling_weight`, `max_siblings`, `system_peer_weight`, `max_system_peers`.
      Pointers and zero-means-unset are load-bearing — see 1.3.
- [x] 1.2 Document each field's meaning in the struct tags, including which graph shape it suits.
      An operator reading only the config must be able to tell that `include_system_peers` is about
      the `system` ID segment.
- [x] 1.3 Pass it through in `cmd/semsource/run.go:905`. **Omit the key entirely when unset** — do
      not send an empty object, and do not marshal `false` for a nil `*bool`. Omission must be a
      true no-op that leaves substrate defaults intact.
- [x] 1.4 Validation: accept the block on tiers without clustering (inert, not an error), and
      reject nonsensical numerics (negative weights, negative caps).

## 2. Prove omission changes nothing

- [x] 2.1 Unit test: with no `entity_id_edges` in config, the composed `graph-clustering` config map
      contains no synthesis keys at all.
- [x] 2.2 Unit test: with only one field set, exactly that key is emitted and no sibling keys are
      manufactured. This is the tri-state guarantee — a nil `*bool` must not become `false`.
- [x] 2.3 Unit test: the block on a tier with clustering disabled validates and composes no
      clustering component.

## 3. Measure the multi-repo shape — gating, not follow-up

- [x] 3.1 Build the multi-repo corpus: OSH Core plus Meshtastic plus the Connected Systems API
      source (see design — Open Questions for which CS API repo). Record entity counts per repo.
      Built from `opensensorhub/osh-core` (scoped to `sensorhub-core` + README, matching the
      single-repo baseline so the added repos are the only new variable),
      `meshtastic/firmware`, and `opengeospatial/ogcapi-connected-systems`. **14,802 entities:**
      osh-core 8,725 (58.9%), ogcapi 4,998 (33.8%), meshtastic 981 (6.6%), plus 98 across 18
      stray systems — source roots that resolve to a nested directory base name, so `system`
      is not exactly `repo` even here.
- [x] 3.2 Record the parser-coverage skew alongside the corpus: Meshtastic firmware is C++ and
      MAVLink is C, and we parse neither, so those repos contribute file/doc/config entities rather
      than code symbols. The measurement is "multi-system, one dominant language" and must be
      labelled as such. Confirmed and worse than "fewer symbols": Meshtastic's **1,299 `.c/.cpp/.h`
      files produce zero entities** — it reaches the graph only through 40 `.py`, 37 `.md`, and
      config files. So the corpus is 59% Java symbols, 34% AsciiDoc passages, 7% incidental.
- [x] 3.3 Measure community-size distribution with system peers ON (substrate default) — the
      question is whether multi-system yields useful grouping or one blob per repo. **Answer: one
      blob per repo.** Level 0, 12,798 entities: 14 communities, largest **8,823 = 66.1% of placed
      members**, holding *every* osh-core entity (99% osh-core); #2 is 2,994 and is 100% ogcapi;
      the rest are meshtastic. **13 of 14 communities draw from a single system.** Multi-system does
      not rescue the default — it partitions the collapse along repo lines, which a `system` filter
      would give without running label propagation at all.
- [x] 3.4 Measure with system peers OFF. One variable per run; the single-repo baseline
      (largest 5,487 → 93) is the comparison. **Strictly better than ON on every axis, but far
      short of the single-repo result:** 19 communities (vs 14), largest **6,069 = 47.4%** (vs
      8,823 = 66.1%), and osh-core now splits across at least four communities instead of sitting
      in one. It does **not** reproduce the single-repo "largest 93" — because that run had almost
      no other edges, whereas here **sibling** synthesis (still ON, unchanged) carries the
      remaining 6,069-entity blob. Turning system peers off does not make clustering good; it
      makes it less bad.
- [x] 3.5 If neither extreme is good, measure `max_system_peers` / `system_peer_weight` before
      concluding. "Off" is the blunt instrument, not automatically the answer. Measured, and it
      changed what we know: `max_system_peers: 3` gives **22 communities, largest 6,067 (47.4%)** —
      statistically indistinguishable from OFF (19, 6,069, 47.4%) and far better than the default
      15 (14, 8,823, 66.1%). So the response to the cap is **not** monotonic in any useful sense:
      a small cap already recovers the entire benefit, and the harm comes from the *number* of
      synthesized peers rather than from the concept. `system_peer_weight` was not swept — with
      cap 3 already matching OFF there is no gap left for the weight to close.

## 4. Ship the default

- [x] 4.1 Set `tier2-compose-dev.json`'s edge-synthesis block from 3.3–3.5. Do not set it from the
      single-repo result alone. Ships `{"include_system_peers": false}` — chosen over
      `max_system_peers: 3`, which measured the same, because it is the simpler setting and the
      only one that stays correct when a deployment has a single source root (the shape where the
      collapse is worst).
- [x] 4.2 Leave tier 0/1 configs untouched — they run no clustering. Verified: the only config
      file changed is `tier2-compose-dev.json`.

## 5. Document both shapes

- [x] 5.1 `docs/testing/tier-baselines.md` — add the single-repo numbers (82% → 93) and the
      multi-repo numbers, each labelled with the graph shape and the parser-coverage caveat. Added
      "Clustering edge synthesis, measured on a multi-repo corpus", including the three-arm table,
      the reproduction command, the two COMMUNITY_INDEX reading traps, and semstreams#837.
- [x] 5.2 `configs/tiers/README.md` — explain the single-system vs multi-system distinction and
      when to override, so an operator can tell which case they are in. Added under tier 2, keyed
      to how `system` is derived (source-root base name) so an operator can classify their own
      deployment.
- [x] 5.3 State plainly that community *summary* quality is a separate, unfixed defect
      (semstreams#829), so better clustering is not mistaken for better answers. Stated in both
      docs, alongside the equally important caveat that the residual 47% community is sibling
      synthesis and remains unfixed.

## 6. Gates

- [x] 6.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`, and
      `go test -tags=integration ./...` green.
- [x] 6.2 `openspec validate configurable-clustering-edge-synthesis --strict` green.
- [x] 6.3 Boot a real tier-2 stack and confirm the composed clustering config carries the intended
      settings — the collapse was only ever visible at runtime, and a unit test on the config map
      does not prove the substrate honored it. Booted the full tier-2 stack (nats + semembed +
      **seminstruct**, `clustering_llm: true`) on the **shipped** `tier2-compose-dev.json` over the
      same corpus. That config uses one source root (`/workspace`), so it exercises the
      single-system shape where the collapse was worst — `system = workspace` for all 12,701
      clustered entities. Result: **26 communities, largest 6,055 = 47.3%**, matching the
      multi-root peers-off arm (19 / 6,069 / 47.4%) rather than the ~66-82% collapse. The substrate
      honored the setting; the distribution is the proof a config-map assertion could not give.

## 7. Not this change

- [ ] 7.1 **C / C++ / Rust parsers.** The A/B prompts ("write a Meshtastic driver using OSH's CS
      API", "write a MAVLink driver for OSH using CS API") need the C/C++ side as code, not as file
      entities. This is a product gap on the A/B's critical path and is arguably higher priority
      than the default chosen here. It needs its own change.
- [ ] 7.2 Community summary content (semstreams#829) — upstream, already filed.
