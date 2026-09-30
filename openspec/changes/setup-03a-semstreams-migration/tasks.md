# SETUP 03A tasks

## 1. Architecture and immutable evidence inputs

- [x] 1.1 Read repository guidance and SemEngine SETUP 03A; record consumer/upstream SHAs and issue #212.
- [x] 1.2 Create proposal, design, tasks, and capability delta before implementation.
- [x] 1.3 Inventory ADRs 094–107 at selected SHA; sign off fixture/API contracts and profile crosswalk.

## 2. Additive baseline fixture before dependency migration

- [ ] 2.1 Version a small Go/docs corpus with fixed expectations and explicit absent-answer cases.
- [ ] 2.2 Add bounded public-interface real-process qualification for ingestion, exact/scoped queries,
  provenance, exact bodies, updates, deletion/recreation, and restarts.
- [ ] 2.3 Prove application and persistent broker restart separately, including unfinished indexing;
  state acknowledged-write durability/re-ingestion boundaries.
- [ ] 2.4 Freeze provider image/build, actual model artifact/version, dimensions, and preprocessing.
- [ ] 2.5 Run beta.161 structural/BM25 and full semembed baselines; record snapshots, commands, per-case
  outcomes, observed defects, and unavailable evidence without silently skipping gates.

## 3. Breaking dependency and API migration

- [ ] 3.1 Update exact Go module pin only after baseline run evidence is retained.
- [ ] 3.2 Adapt retained contracts from compile and ADR/implementation inventory; behavior-test changed
  bootstrap, context, NATS ownership, identity, and desired-versus-running configuration semantics.
- [ ] 3.3 Rerun identical workload/profile matrix with identical providers at selected pin.
- [ ] 3.4 Attribute every difference to a pinned upstream contract or reproduced defect.
- [ ] 3.5 Reconcile shared agents/skills and run `task agents:check`.

## 4. Defect probes and dependency measurement

- [ ] 4.1 Reproduce GRAPH capacity (#178); distinguish small probes from the known OSH corpus tail.
- [ ] 4.2 Probe RPC stream collision (SemStreams #1143) at both pins and assess retained request paths.
- [ ] 4.3 Measure production/test-inclusive closures with commands and raw non-test line counts;
  compare direct-import/registration cut and composed port-set closure.

## 5. Review and delivery

- [ ] 5.1 Run gofmt, vet, unit tests, applicable integration/race checks, pinned revive, and OpenSpec
  validation; record unavailable gates/failures as blockers.
- [ ] 5.2 Obtain independent Go reviewer implementation and baseline-evidence sign-off.
- [ ] 5.3 Update migration/results docs with pins, changes, provider manifest, outcomes, blockers;
  open/attach migration PR.
- [ ] 5.4 Retain holds until full matrix passes; later exclusive SemEngine branch qualification is
  separate, and mainline cutover waits for the complete semembed-backed workload.
