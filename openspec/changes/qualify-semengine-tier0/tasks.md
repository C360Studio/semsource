# SemEngine Tier 0 consumer tasks

## 1. Planning claim and prerequisites

- [x] Record merged reference #213 at `3604a9ce8d5aec717253d7a905389f7910efd25d` and open consumer issue #221.
- [x] Inspect live SemEngine #8/#9 and PR #21; record 03B contract work as a dependency, not an engine API.
- [x] Inventory concrete composition/provider and corpus dependencies; architect accepts the bounded
  consumer inventory for planning only. Record current seams, required changes and code references in
  `design.md`; no engine API, runtime change or available foundation config is implied.
- [x] Obtain independent review of the bounded consumer design and its prerequisite/acceptance mapping.
  Architect and independent reviewer approve planning only; strict OpenSpec and formatting checks pass.
  Implementation still requires the separate engine admission gate below.
- [ ] Record approved 03B contracts, package scope, relevant #15–#20 repair/decision dispositions and
  the exact admitted usable SemEngine Tier 0 commit before implementation begins.

## 2. Future implementation after admission

- [ ] Migrate the separate qualification branch to SemEngine exclusively; prove no SemStreams runtime
  or mixed-substrate binary, and measure production/test closure against the admitted Tier 0 boundary.
- [ ] Implement a true no-embedder composition and capability-appropriate readiness/public interfaces
  using admitted contracts, with failing-first tests for changed consumer behavior.
- [ ] Add an explicit graph-foundation corpus selection without weakening existing known answers or
  incorrectly requiring lexical/neural APIs; retain the original BM25/neural reference workload.
- [ ] Qualify an admitted public exact document/passage body path; the current corpus selects document
  bodies through NL retrieval. Preserve exact bytes and bodyless-parent semantics without a private-store shortcut.
- [ ] Prove structural readiness and authoritative absence with no embedder present; expose unsupported
  retrieval capabilities honestly and test that no model service is required.
- [ ] Preserve the Enabled:false envelope until the admitted SemEngine #17 contract passes its
  removal/restart/re-add proof; record #18–#20 decisions and their consumer consequences honestly.

## 3. Qualification and handoff

- [ ] Pass graph ingestion, identity, metadata/provenance, exact queries and bodies, updates,
  deletion/recreation, application restart and broker re-ingestion through admitted public interfaces.
- [ ] Require `broker_restart_reingested_exact_relationship` and
  `broker_restart_reingested_exact_content` to pass; no known-at-pin waiver applies to SemEngine.
- [ ] Qualify #16's admitted reserved-RPC/startup-refusal contract using new engine assertions; preserve
  the historical PubAck-collision reproduction without counting hazard reproduction as an engine pass.
- [ ] Record exact consumer/engine commits, binary/corpus/configuration hashes, commands, results and
  production/tagged-test closures. Reject unadmitted dependencies and distinguish captured build
  provenance from embedded binary revision metadata.
- [ ] Assert graceful application exit zero, intentional crash signals and exact owned broker cleanup;
  reuse the checked-stop behavior rather than inheriting the original probe's unasserted wait result.
- [ ] Perform the separately admitted SemConnect reference and engine dogfood workloads; code/docs
  corpus results do not establish those distinct obligations.
- [ ] Obtain independent Go/component and graph/event sign-off on implementation and exact qualification.
- [ ] Retain Tier 0 as a lower-profile regression obligation for 04B/04C; mainline stays on SemStreams
  until the complete semembed-backed workload and retained lower-profile gates pass.

Current stop point: planning only. No implementation task is admitted by creating this claim.
