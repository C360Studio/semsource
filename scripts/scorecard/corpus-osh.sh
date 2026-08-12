#!/usr/bin/env bash
# Build the Open Sensor Hub scorecard corpus — the second scale point.
#
#   scripts/scorecard/corpus-osh.sh <target-dir>
#
# OSH core is the early adopter's shape (Java + Gradle/Maven-family config,
# `master` branch) and roughly 3x the dogfood corpus — the first
# Java/Gradle-family system this project has ever measured. The corpus is its
# OWN comparability domain: questions-osh.json scores never merge with the
# dogfood set's (README: Comparability), and compare.sh's questions_version
# guard enforces that mechanically because the sets carry different versions.
#
# The checkout is PINNED, not tracking master: a moving corpus silently
# invalidates every question authored against it — the same rot class the
# checkers gate. questions-osh.json is authored against exactly this SHA;
# bumping the pin means re-verifying the question set (both checkers) and
# bumping that file's version.
#
# Extraction is `git archive`, never a worktree or a plain clone: a .git dir
# in the corpus would be walked by handlers and grep alike, and archive gives
# byte-identical trees on any machine (the lesson recorded from the
# default-path ingest bugs).
set -euo pipefail

OSH_REPO="https://github.com/opensensorhub/osh-core.git"
# master as of 2026-08-12 — the pin questions-osh.json v1 is authored against.
OSH_PIN="235c0eabf24b6d6137b499b4402943d2794b70e6"

target="${1:?usage: corpus-osh.sh <target-dir>}"

if [ -e "$target" ] && [ -n "$(ls -A "$target" 2>/dev/null)" ]; then
	echo "refusing: $target exists and is not empty — a mixed corpus measures nothing" >&2
	exit 1
fi
mkdir -p "$target"
target="$(cd "$target" && pwd)"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Fetch exactly the pinned commit (GitHub serves arbitrary-SHA fetches), so
# the build is reproducible even after master moves on.
git -C "$tmp" init -q
git -C "$tmp" remote add origin "$OSH_REPO"
git -C "$tmp" fetch -q --depth 1 origin "$OSH_PIN"
git -C "$tmp" archive "$OSH_PIN" | tar -x -C "$target"

# Scorecard exclusions, same rule as the dogfood corpus: the harness must
# never be able to read its own answer key. OSH has no scripts/scorecard
# today; the rm guards against the corpus ever growing one.
rm -rf "$target/scripts/scorecard"

echo "OSH corpus built: $target"
echo "  repo: $OSH_REPO"
echo "  pin:  $OSH_PIN"
du -sh "$target" | awk '{print "  size: " $1}'
find "$target" -name '*.java' | wc -l | awk '{print "  java files: " $1}'
