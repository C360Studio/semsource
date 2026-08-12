#!/usr/bin/env bash
# Shared wall-clock helper for every scorecard arm. Sourced, not executed —
# the same shared-by-construction contract as grade.sh.
#
# One code path on purpose: bash 3.2 (macOS default) has no EPOCHREALTIME and
# BSD date has no %N, so portable millisecond time means spawning something.
# Perl ships on every machine this harness runs on (M3 Pro and the Intel dev
# mini, both macOS; Linux CI), and using it unconditionally keeps the spawn
# overhead IDENTICAL across arms and machines instead of varying by which
# shell happened to run the script. The overhead lands inside the measured
# interval once (~5-10 ms); latency figures are medians/p95s over calls that
# take tens to thousands of ms, and the comparability rule already restricts
# them to same-machine runs (README: Comparability).
now_ms() {
	perl -MTime::HiRes=time -e 'printf("%d", time()*1000)'
}

# host_json emits the header fragment recording where latency was measured —
# figures only compare across runs on the same machine class (the
# two-machines rule: M3 Pro vs Intel dev mini numbers never mix).
host_json() {
	jq -nc --arg arch "$(uname -m)" --arg os "$(uname -s)" '{arch:$arch, os:$os}'
}
