# SETUP 03A measurement tools

`measure-capacity.py` is the bounded OSH transport probe used alongside the small semantic corpus in
`test/setup03a`. It starts one private NATS container, records authoritative source delivery and broker state, and
removes only that container after preserving logs. It requires Python 3, Docker, a prebuilt SemSource binary, and
an existing corpus. It does not fetch dependencies, edit source data, or call a model provider.

Prepare the pinned OSH corpus using the existing repository script:

```bash
scripts/scorecard/corpus-osh.sh /tmp/semsource-setup03a-osh
```

That script fetches/archive-extracts OpenSensorHub commit `235c0eabf24b6d6137b499b4402943d2794b70e6`.
The corpus script excludes `scripts/scorecard`; that directory is absent in this upstream tree. The measurement
consumes the supplied `--corpus` verbatim and fingerprints all source paths/bytes, excluding only `.git` metadata
from the fingerprint. Use the same corpus directory for paired runs to hold path-derived identity inputs constant.

```bash
python3 scripts/setup03a/measure-capacity.py \
  --binary /absolute/path/semsource-beta161 \
  --label beta161 \
  --out /absolute/path/evidence/capacity-beta161 \
  --corpus /tmp/semsource-setup03a-osh \
  --identity beta161

python3 scripts/setup03a/measure-capacity.py \
  --binary /absolute/path/semsource-pinned \
  --label pinned \
  --out /absolute/path/evidence/capacity-pinned \
  --corpus /tmp/semsource-setup03a-osh \
  --identity governed
```

Use a fresh output directory for each run. Identity selection is explicit: the governed revision supplies the
`setup03a-capacity` platform stem; beta.161 uses its historical composition. Both use BM25, four index workers,
200 ms coalescing, AST's same six languages, docs, and config ingestion. The default 256 MiB memory GRAPH transport
ceiling remains unchanged. NATS is pinned by version and digest in the script, with one CPU and 1 GiB RAM.
HTTP, metrics, WebSocket, GraphQL, and broker ports are per-run dynamic loopback ports.

`observations.json` records source status and NATS `/jsz` stream/consumer/account state every two seconds, with a
compact progress line every 30 seconds. A source phase of `ready` alone does not end the run: every source must finish
seeding and reconcile `offered_total = delivered_total + lost_total`. A pass additionally requires zero loss/errors
and successful resource cleanup. No source progress for 120 seconds after startup aborts the run; the overall bound
is ten minutes. Application stop and Docker commands have independent finite deadlines. `resources.json` records
actual image/container inspection and binary hash; `result.json` records the terminal reason and final observation.

This probe measures transport delivery, not full indexing or query correctness. SemSource's existing #175 parser
guard unconditionally skips minified symbol extraction and caps symbols per file at 5,000. The same guard applies
to both binaries, reducing the payload volume relative to the historical unguarded #178 failure. Successful guarded
runs do not prove the old high-volume regression was fixed. Keep that limitation explicit alongside peak GRAPH bytes,
offered/delivered/lost counts, and the small-corpus semantic results.

## Dependency closure

`dependency-closure.py` runs `go list` against an explicit checkout at its pinned dependency version:

```bash
python3 scripts/setup03a/dependency-closure.py \
  --repo /absolute/path/checkout \
  --out /absolute/path/evidence/dependency-closure
```

The output directory must not already exist. The script records raw JSON, commands, package lists,
module identities, and counts for production, consumer tests, direct imports, the registration cut,
and the graph port set with its own tests. The default test tags are `integration,e2e,qualification`;
use the same tags for paired measurements. It reads every non-test Go file in each unique upstream
source directory, including files for inactive build tags. Synthetic test binaries and external-test
variants are not extra source directories, but their imported dependencies remain included.

Upstream packages' own tests may require dependencies absent from a consumer's normal checksums.
Resolution is confined to `closure.mod`/`closure.sum` under the evidence directory; the consumer's
module files remain untouched. Network access may be needed for uncached test dependencies.
The resulting module graph and exact command arguments are retained. This measures dependency cost,
not approval to extract any package or a claim that its tests were executed.
