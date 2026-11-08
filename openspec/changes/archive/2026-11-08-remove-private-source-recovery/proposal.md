# Remove private source recovery authority

## Why

The owner approved a corrective PR after reassessing merged #213 against SemStreams #1147:
settlement, stable replay and existing storage precede additional persistent recovery authority.
The coordinator, journal, publication receipts, generation/epoch machinery and global effect fences
were not justified by a named failure proving those simpler choices insufficient. Their successful
implementation tests do not establish architectural necessity. Remove them rather than repackage them.

SemSource owns source desired configuration, ingestion and domain staleness. SemStreams owns graph
mutation and settlement contracts. This change supplies no new substrate or upstream fix.

## What changes

- Delete the private source recovery machinery and its publisher/factory/handler wiring.
- Keep `Enabled:false`, desired manifest repair, partial-write truthfulness and restart-only admission.
- Make automatic source-removal projection and positive source reactivation explicitly unavailable.
  Do not create another replay worker, queue, command stream, receipt store or terminal resolver.
- Retain ordinary file/passage staleness, exact identities/content, publisher settlement and owned joins.
- Before boot writes or producer construction, reject nonempty or unreadable legacy lifecycle storage
  through a bounded read-only compatibility check; never parse, reset or delete its records.
- Preserve frozen corpus/results. Record new changed-contract evidence separately.

## Impact

Consumers are SemSource's HTTP/NATS source-management clients, including SemTeams and downstream
SemSpec/SemDragon/SemOps integrations. Source desired changes remain available without graph readiness.
Removal replies no longer advertise generation or pending projection; they state projection unavailable.
The old lifecycle status route returns HTTP 410 with an explicit unavailable error. Any retained message
in the account-wide legacy bucket blocks upgrade, including terminal/foreign/tombstone records.

The SemStreams pin, configuration profiles, source fixtures, baseline failures and SemEngine cutover
rules remain unchanged. SemEngine #18–#20 remain separate contract decisions, not prerequisites for
inventing replacement recovery here. #215's positive projection/reactivation acceptance is deferred.

## Non-goals

No SemStreams changes, SemEngine port, durable source command ingress, replacement reconciler,
automatic legacy-state migration, terminal-outcome lookup, graph retraction or new freshness guarantee.
No data deletion, interpretation of unknown outcomes, changed frozen expectations or test skip waiver.
