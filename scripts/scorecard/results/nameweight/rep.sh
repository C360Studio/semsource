#!/usr/bin/env bash
# NameWeight sweep (semengine#620): one native semsource per weight, fresh NATS per run.
set -uo pipefail
cd "$1"   # the semsource checkout whose scorecard runs
cfg=scripts/scorecard/results/alpha5-tier0/tier0.json
for spec in $RUNS; do corpus=${spec%%:*}; w=${spec##*:};
	src=/tmp/base230/corpus-$corpus
	q=scripts/scorecard/questions.json; [ $corpus = osh ] && q=scripts/scorecard/questions-osh.json
	jq --arg p "$src" 'walk(if . == "/workspace" then $p else . end)' "$cfg" > /tmp/nw/cfg-$corpus.json
	{
		label=nw-w$w-$corpus-rep
		docker rm -f nw-nats >/dev/null 2>&1
		docker run -d --name nw-nats -p 127.0.0.1:25222:4222 nats:2.14-alpine -js >/dev/null
		until nc -z 127.0.0.1 25222; do sleep 1; done
		NATS_URL=nats://127.0.0.1:25222 SEMSOURCE_HTTP_PORT=28080 SEMSOURCE_WS_BIND=127.0.0.1:27890 \
			/tmp/nw/semsource-w$w run --config /tmp/nw/cfg-$corpus.json > /tmp/nw/$label.semsource.log 2>&1 &
		pid=$!
		SEMSOURCE_HTTP_PORT=28080 SCORECARD_READY_TIMEOUT=5400 SCORECARD_QUESTIONS=$q \
			scripts/scorecard/run.sh $label > /tmp/nw/$label.run.log 2>&1
		echo "$label exit=$? $(grep '^=== ' /tmp/nw/$label.run.log)"
		kill $pid; wait $pid 2>/dev/null
		docker rm -f nw-nats >/dev/null
	}
done
echo SWEEP-DONE
