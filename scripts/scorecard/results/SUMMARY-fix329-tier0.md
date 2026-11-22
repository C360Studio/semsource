# Tier 0 scorecard on the #337 branch (fix-ast-id-collisions, task 5.3)

The beta.10 tier 0 run ([`SUMMARY-beta10-tier0.md`](SUMMARY-beta10-tier0.md)), repeated on a reseeded store
built from the branch. This is the acceptance run for OpenSpec change `fix-ast-id-collisions` task 5.3, and the R1
evidence for #324.

## Setup

| | |
|---|---|
| SemSource | built locally from `08227da` (branch `claude/329-ast-id-collisions`) by the base compose file's `build`; rerun at `5518379` and again at `d13b191` after the review fixes; the committed JSON is from the `d13b191` rerun |
| Config | [`fix329-tier0/tier0.json`](fix329-tier0/tier0.json), byte-identical to beta.10's; `tier0-reversed.json` is `jq '.sources \|= reverse'` of it |
| Corpora | dogfood `b4c2cfd9026b20b5`, OSH `9b1e77bc0bf0291f`: the beta.10 trees, hashes rechecked |
| Questions | dogfood: `questions.json` from `86626c0`, as in beta.10; OSH: `questions-osh.json`, unchanged since `9537f59` |
| Harness | `run.sh`, `rank-diff.sh`, `grade.sh` and `docker-compose.yml` unchanged since `9537f59` |
| Stack | [`fix329-tier0/compose.fix329.yml`](fix329-tier0/compose.fix329.yml), project `fix329`, fresh NATS storage per corpus |

## Result

| | beta.10 (released image) | this branch |
|---|---|---|
| dogfood, arm B | 32/36 · 559 KB · 8,192 entities | **32/36 · 569 KB · 7,828 entities** |
| dogfood misses | X02, P03, L02, L07 | X02, P03, L02, L07 |
| dogfood loose band | 5/7 | **5/7** |
| OSH, arm B | 12/13 · 175 KB · 29,422 entities | **13/13 · 195 KB · 28,461 entities** |
| OSH misses | C03 | none |
| R1 (restart, sources reversed) | 35/36, RANK-UNSTABLE (L03 ranks 18/19) | **36/36 identical, RANK-STABLE** |

- Every dogfood verdict matches beta.10 question by question. Context bytes are 1.8% higher, spread over ten
  questions (most +1–2 KB), with no verdict change.
- OSH C03 (`code_search`) now grades correct. That is a lexical-ranking shift on a smaller entity set, not a targeted
  fix, so it is recorded and not claimed.
- R1: [`fix329-tier0/r1-evidence/`](fix329-tier0/r1-evidence/) shows the container recreated with
  `tier0-reversed.json` and all three initial ingests logged before leg 2.

## P01, found and fixed during this run

The first OSH leg, at `396123d`, scored 12/13 with **P01 a miss**. `code_impact` on
`ModulePermissions.cloneAsTemplatePermission` had lost all three callers (`ConSysApiSecurity`, `SOSSecurity`,
`SPSSecurity`).

The calls go through `rootPerm`, a field inherited from `ModuleSecurity` in another module. The field's type is
resolved as written in `ModuleSecurity.java`, but `sourceRootPrefix` stripped the *parsed* file's package from that
file's directory. The strip failed, no source root was found, and the call was emitted as `external:`. Main hid this
with a source-root memo taken once per `ParseFile` from the parsed file; this branch keys that memo per referrer
layout (`TestForeignAncestorDoesNotHideOtherModules`), which exposed it.

`08227da` resolves the referrer's own package (`packageFor`), pinned by `TestInheritedFieldTypedInAnotherModuleLinks`.
Every number above is from the rerun at `08227da`, and P01's impact is back to beta.10's 6 nodes in 6 files.

## Rerun after the review fixes

At `5518379` (TS block locals and namespace scoping, the ast-source fold fixes) every figure above reproduces
exactly. Dogfood is 32/36 with 569,361 bytes and 7,828 entities in both legs, R1 is 36/36 RANK-STABLE, and OSH is
13/13 with 195,263 bytes and 28,461 entities. Neither corpus contains the module-level block or namespace shapes,
which is why the 5.1 probe already showed zero collisions there.

At `d13b191` (the `declare global` and merged-namespace fixes) every figure reproduces again.

## Not covered

- Arms A and C, Tier 1 and the setup03a lane were not rerun; this change touches only what SemSource ingests.
- R1 on OSH was not run, as in the baseline.
