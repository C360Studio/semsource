#!/usr/bin/env sh
# Verify the agent configuration is internally consistent and honestly related to
# semstreams — the scripted form of the manual parity procedure in .agents/README.md.
#
# Two failure modes it exists to catch, both of which are silent otherwise:
#
#   1. A vendored decision skill drifting from the framework truth it copies. Ours
#      had claimed NATS KV history was an audit trail long after upstream corrected
#      it; nothing failed, and every agent reading it designed against a false fact.
#   2. A platform adapter that stops pointing at its canonical contract — or, as
#      shipped before this script existed, an agent file with no frontmatter at all,
#      which is advertised in CLAUDE.md and silently undiscoverable.
#
# Read-only. Exits non-zero with a named reason on the first class of failure found.
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"

manifest=.agents/upstream.manifest
failures=0

fail() {
	printf 'FAIL  %s\n' "$1" >&2
	failures=$((failures + 1))
}
ok() { printf 'ok    %s\n' "$1"; }

# ── Resolve the semstreams source we actually depend on ─────────────────────
pinned=$(go list -m -f '{{.Version}}' github.com/c360studio/semstreams)
recorded=$(awk -F'\t' '$1 == "upstream_version" { print $2 }' "$manifest")

if [ "$pinned" != "$recorded" ]; then
	fail "manifest records upstream $recorded but go.mod pins $pinned.
      Re-reconcile the shared skills against the pinned version and update
      $manifest — a stale manifest checks nothing."
	printf '\n%d check(s) failed.\n' "$failures" >&2
	exit 1
fi

upstream=$(go list -m -f '{{.Dir}}' github.com/c360studio/semstreams)
if [ -z "$upstream" ] || [ ! -d "$upstream/.agents/skills" ]; then
	fail "cannot read semstreams $pinned sources (run: go mod download github.com/c360studio/semstreams)"
	printf '\n%d check(s) failed.\n' "$failures" >&2
	exit 1
fi
ok "semstreams $pinned resolved"

# ── Shared decision skills: vendored must match, forked must be re-read on change ──
while IFS="$(printf '\t')" read -r name mode sha; do
	case "$name" in \#*|'') continue ;; esac
	[ "$name" = upstream_version ] && continue

	ours=".agents/skills/$name/SKILL.md"
	theirs="$upstream/.agents/skills/$name/SKILL.md"

	[ -f "$ours" ] || { fail "$name: $ours is missing"; continue; }
	[ -f "$theirs" ] || { fail "$name: upstream no longer ships this skill — decide whether we still want it"; continue; }

	current=$(shasum -a 256 < "$theirs" | cut -d' ' -f1)

	case "$mode" in
	vendored)
		if cmp -s "$ours" "$theirs"; then
			ok "$name: vendored, matches upstream"
		else
			fail "$name: VENDORED COPY HAS DRIFTED from semstreams $pinned.
      semstreams owns this truth. Refresh it and update the digest:
        cp '$theirs' $ours"
		fi
		;;
	forked)
		if [ "$current" = "$sha" ]; then
			ok "$name: forked by design, upstream unchanged"
		else
			fail "$name: UPSTREAM CHANGED since we last reconciled this deliberate fork.
      Read upstream's version, decide what carries over, then record the new digest:
        $theirs
      expected $sha
      actual   $current"
		fi
		;;
	*) fail "$name: unknown mode '$mode' in $manifest (expected vendored or forked)" ;;
	esac
done < "$manifest"

# ── An upstream skill we track under neither mode is one nobody decided about ──
for theirs in "$upstream"/.agents/skills/*/SKILL.md; do
	name=$(basename "$(dirname "$theirs")")
	if ! awk -F'\t' -v n="$name" '$1 == n { found = 1 } END { exit !found }' "$manifest"; then
		fail "$name: semstreams $pinned ships this shared decision skill and we do not track it.
      Vendor it, or record a deliberate fork, in $manifest."
	fi
done

# ── Adapters stay thin and name exactly their canonical contract ─────────────
for canonical in .agents/contracts/*.md; do
	role=$(basename "$canonical" .md)

	claude=".claude/agents/$role.md"
	if [ ! -f "$claude" ]; then
		fail "$role: no Claude adapter at $claude"
	else
		head -1 "$claude" | grep -qx -- '---' ||
			fail "$role: $claude has no YAML frontmatter, so Claude cannot discover it"
		grep -q "^name: $role$" "$claude" || fail "$role: $claude frontmatter does not declare name: $role"
		grep -q '^description: ' "$claude" || fail "$role: $claude declares no description"
		grep -Fq "$canonical" "$claude" || fail "$role: $claude does not name its canonical contract $canonical"
		grep -qE '^tools:.*(Edit|Write|NotebookEdit)' "$claude" &&
			fail "$role: $claude grants a write tool; these reviewers are read-only"
	fi

	codex=".codex/agents/$role.toml"
	if [ ! -f "$codex" ]; then
		fail "$role: no Codex adapter at $codex"
	else
		grep -Fq "$canonical" "$codex" || fail "$role: $codex does not name its canonical contract $canonical"
		grep -q '^sandbox_mode = "read-only"$' "$codex" || fail "$role: $codex is not read-only"
	fi

	[ "$failures" -eq 0 ] && ok "$role: canonical contract with thin Claude and Codex adapters"
done

# ── Skill adapters point at the canonical body and carry none of it ──────────
for canonical in .agents/skills/*/SKILL.md; do
	name=$(basename "$(dirname "$canonical")")
	adapter=".claude/skills/$name/SKILL.md"

	[ -f "$adapter" ] || { fail "$name: no Claude adapter at $adapter"; continue; }
	grep -Fq "$canonical" "$adapter" || fail "$name: $adapter does not name its canonical body $canonical"

	lines=$(wc -l < "$adapter" | tr -d ' ')
	[ "$lines" -le 12 ] || fail "$name: $adapter is $lines lines — an adapter that carries the body is a second copy to drift"
done

# ── AGENTS.md and CLAUDE.md must route the same roles, on the same terms ────
# Two entry points that name different agents is how a Codex session and a Claude
# session end up working to different contracts in the same repository. The
# spawning rule is checked too: it is the difference between a reviewer that runs
# and one that waits to be asked, and it is one deletion away from silently
# reverting to the latter.
for canonical in .agents/contracts/*.md; do
	role=$(basename "$canonical" .md)
	for entry in AGENTS.md CLAUDE.md; do
		grep -Fq "$role" "$entry" || fail "$role: $entry does not route this role"
	done
done

for entry in AGENTS.md CLAUDE.md .agents/README.md; do
	grep -Fq 'no user permission needed' "$entry" ||
		fail "$entry no longer states that spawning a matching role agent is the default and needs no permission"
done

if [ "$failures" -ne 0 ]; then
	printf '\n%d check(s) failed.\n' "$failures" >&2
	exit 1
fi
printf '\nAgent configuration is consistent with semstreams %s.\n' "$pinned"
