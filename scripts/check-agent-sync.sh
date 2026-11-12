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

# ── Resolve the semengine source we track against ───────────────────────────
module=$(awk -F'\t' '$1 == "upstream_module" { print $2 }' "$manifest")
[ -n "$module" ] || module="github.com/c360studio/semengine"
recorded=$(awk -F'\t' '$1 == "upstream_version" { print $2 }' "$manifest")
pinned=$(go list -m -f '{{.Version}}' "$module" 2>/dev/null || true)

# When go.mod pins $module directly (not an indirect transitive version), enforce version parity.
if grep -Eq "^[[:space:]]*${module}[[:space:]]+${pinned}\$" go.mod; then
	if [ "$pinned" != "$recorded" ]; then
		fail "manifest records upstream $recorded ($module) but go.mod pins $pinned.
      Re-reconcile the shared skills against the pinned version and update
      $manifest — a stale manifest checks nothing."
		printf '\n%d check(s) failed.\n' "$failures" >&2
		exit 1
	fi
fi

upstream=$(GOTOOLCHAIN=auto go mod download -json "${module}@${recorded}" 2>/dev/null | sed -n 's/^[[:space:]]*"Dir":[[:space:]]*"\(.*\)",*$/\1/p')
if [ -z "$upstream" ] || [ ! -d "$upstream/.agents/skills" ]; then
	fail "cannot read $module $recorded sources (run: GOTOOLCHAIN=auto go mod download ${module}@${recorded})"
	printf '\n%d check(s) failed.\n' "$failures" >&2
	exit 1
fi
ok "$module $recorded resolved"

# ateam-managed skills live in both repos under ateam doctor --check; do not duplicate in upstream.manifest.
is_ateam_skill() {
	case "$1" in
	design | handoff | pickup | ponytail | preflight | review-pr) return 0 ;;
	esac
	return 1
}

# ── Shared decision skills: vendored must match, forked/excluded must be re-read on change ──
while IFS="$(printf '\t')" read -r name mode sha; do
	case "$name" in \#*|'') continue ;; esac
	case "$name" in upstream_version | upstream_module) continue ;; esac

	ours=".agents/skills/$name/SKILL.md"
	theirs="$upstream/.agents/skills/$name/SKILL.md"

	[ -f "$theirs" ] || { fail "$name: upstream no longer ships this skill — decide whether we still want it"; continue; }

	current=$(shasum -a 256 < "$theirs" | cut -d' ' -f1)

	if [ "$mode" != excluded ] && [ ! -f "$ours" ]; then
		fail "$name: $ours is missing"
		continue
	fi

	case "$mode" in
	excluded)
		if [ -f "$ours" ]; then
			fail "$name: excluded upstream workflow has a local canonical copy"
		elif [ "$current" != "$sha" ]; then
			fail "$name: excluded upstream workflow changed; re-read its scope before renewing the exclusion"
		else
			ok "$name: reviewed exclusion, upstream unchanged"
		fi
		;;
	vendored)
		if cmp -s "$ours" "$theirs"; then
			ok "$name: vendored, matches upstream"
		else
			fail "$name: VENDORED COPY HAS DRIFTED from $module $recorded.
      $module owns this truth. Refresh it and update the digest:
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
	*) fail "$name: unknown mode '$mode' in $manifest (expected vendored, forked, or excluded)" ;;
	esac
done < "$manifest"

# ── An upstream skill we track under neither mode is one nobody decided about ──
for theirs in "$upstream"/.agents/skills/*/SKILL.md; do
	name=$(basename "$(dirname "$theirs")")
	is_ateam_skill "$name" && continue
	if ! awk -F'\t' -v n="$name" '$1 == n { found = 1 } END { exit !found }' "$manifest"; then
		fail "$name: $module $recorded ships this shared decision skill and we do not track it.
      Vendor it, record a deliberate fork, or document a reviewed exclusion in $manifest."
	fi
done

# ── ateam base contracts are read by skills and specialists, not spawned ────
# architect/developer/reviewer are ateam's generated bases (drift-checked by
# `ateam doctor --check`); they carry no adapter. Every specialist must open by
# pointing at its base, or a subagent reading only the specialist never sees it.
is_base() { case "$1" in architect | developer | reviewer) return 0 ;; esac; return 1; }

for canonical in .agents/contracts/*.md; do
	role=$(basename "$canonical" .md)
	is_base "$role" && continue
	sed -n '1,5p' "$canonical" | grep -Eq '\.agents/contracts/(architect|developer|reviewer)\.md' ||
		fail "$role: $canonical does not open with a pointer to its base contract"
done

# ── Adapters stay thin and name exactly their canonical contract ─────────────
for canonical in .agents/contracts/*.md; do
	role=$(basename "$canonical" .md)
	is_base "$role" && continue

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

# ── The entry point must route every role, on the same terms ──────────────────
# Two entry points that name different agents is how a Codex session and a Claude
# session end up working to different contracts in the same repository. The
# spawning rule is checked too: it is the difference between a reviewer that runs
# and one that waits to be asked, and it is one deletion away from silently
# reverting to the latter.
# CLAUDE.md is an `@AGENTS.md` import (ateam), so AGENTS.md is the one entry to check.
grep -qx '@AGENTS.md' CLAUDE.md || fail "CLAUDE.md does not import @AGENTS.md"
for canonical in .agents/contracts/*.md; do
	role=$(basename "$canonical" .md)
	grep -Fq "$role" AGENTS.md || fail "$role: AGENTS.md does not route this role"
done

for entry in AGENTS.md .agents/README.md; do
	grep -Fq 'no user permission needed' "$entry" ||
		fail "$entry no longer states that spawning a matching role agent is the default and needs no permission"
done

if [ "$failures" -ne 0 ]; then
	printf '\n%d check(s) failed.\n' "$failures" >&2
	exit 1
fi
printf '\nAgent configuration is consistent with %s %s.\n' "$module" "$recorded"
