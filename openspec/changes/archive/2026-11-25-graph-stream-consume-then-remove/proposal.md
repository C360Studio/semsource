# GRAPH ingest stream removes what graph-ingest has consumed

Approved-by: Coby Leuschke (2026-11-25) sha256:aa3ca23c50c9

Track: design (durability/ack behaviour and the boot path; one Requirement in `entity-publish-integrity`).

## Why

The GRAPH stream (`cmd/semsource/run.go:966-1020`) keeps every message for its 1 h age bound, even after graph-ingest
has applied and acknowledged it. Its 256 MiB cap therefore counts bytes published per hour, not unconsumed backlog. One
OSH seed fills 82% of it (#160 comment, 2026-11-22). A semsource-only restart within the hour re-publishes everything,
the stream refuses with 10077, 657 entities are dropped after bounded backpressure, and ast-source stays `degraded`
(#388; #160 comment 2026-11-25). A full NATS + semsource restart passes only because memory storage starts empty.

The values date from 32c0fa0 (2026-03-08), when `/ws` replayed GRAPH on reconnect. `/ws` now watches ENTITY_STATES
(#262), and graph-ingest is the only consumer in this repo. Owner ruling 2026-11-25: fix before beta.11 with
consume-then-remove retention, keeping the refuse-don't-evict rationale (`run.go:978-993`).

## What Changes

- **GRAPH removes a message once graph-ingest acknowledges it.** graph-ingest acknowledges only after the ENTITY_STATES
  write and its redelivery stamp, so removal never runs ahead of the graph.
- **The refuse-at-the-ceiling posture stays.** The byte and age bounds and refusing new writes are unchanged; the cap
  now measures real backlog.
- **The retention is fixed, not configurable.** A `streams.GRAPH` override cannot change it.
- **Tests:** an e2e warm-restart repro that fails on main, and the two e2e tests that read GRAPH directly move to
  reading ENTITY_STATES.
- **Docs:** one adoption-note line (#386) and the consumer guide's "subscribe to GRAPH" readiness option.

Consumers: SemTeams, SemDragon and semspec read through graph query, MCP, HTTP status and GraphQL. Those surfaces do not
change.

## Decisions you are making by approving

- **When graph-ingest has applied an entity, its message leaves the stream, and nothing published before graph-ingest
  is listening is lost.** Restarting semsource within the hour no longer wedges. The cost: nothing else may read entity
  messages off this stream; such a reader is refused at startup, loudly. Recommended over a mode that silently drops
  messages published before the first reader exists, and over only raising the limits.
- **The stream stays in memory for beta.11.** Restarting NATS alone still loses facts that were in flight. Making it
  survive NATS restarts is the follow-up's choice, and it will cost one more stream reset when it lands.
- **Operators cannot change this behaviour through configuration.** A named durability setting, as #160's title asks,
  is a follow-up, not beta.11.
- **The one-hour age bound and the size cap keep today's values.** After this change an expired message is always one
  that graph-ingest never applied, so the bound now only matters if graph-ingest stalls for an hour.
- **Upgrading needs a fresh stream.** A NATS server still holding the old GRAPH stream refuses semsource's boot with a
  message naming the fix. beta.11 already needs `docker compose down -v` and a reseed, so the note gains one line.

## Non-goals

- File storage, the named durability posture, and removing the raw stream overrides (#160 follow-up).
- Taking `graph.ingest.manifest/status/predicates/batch` out of the stream or moving them to core publish.
- Any SemEngine change. If an interest-retention variant is later wanted, the consumer-before-producer gap is an
  upstream issue (rule 2), not a local workaround.
- The OSH proof itself (#388 gates beta.11).
