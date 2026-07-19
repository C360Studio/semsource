#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
script="$project_dir/scripts/ui-profile-pin-verify.sh"
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/ui-profile-pin-test.XXXXXX")
cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
expected="ghcr.io/c360studio/semsource-ui:v1.2.3@$digest"
"$script" --expected "$expected" --observed "$expected" --source compose-render \
	--evidence "$tmp_dir/evidence.md" >/dev/null
grep -F "Compose-rendered UI image" "$tmp_dir/evidence.md" >/dev/null

if "$script" --expected "$expected" \
	--observed "ghcr.io/c360studio/semsource-ui:v1.2.3@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" \
	--source running-container >"$tmp_dir/wrong.out" 2>&1; then
	echo "a running container with the wrong pin must fail" >&2
	exit 1
fi
grep -F "expected '$expected'" "$tmp_dir/wrong.out" >/dev/null

latest="ghcr.io/c360studio/semsource-ui:latest@$digest"
if "$script" --expected "$latest" --observed "$latest" --source compose-render \
	>"$tmp_dir/latest.out" 2>&1; then
	echo "digest-qualified latest must not be accepted as a profile pin" >&2
	exit 1
fi
grep -F "mutable tag 'latest'" "$tmp_dir/latest.out" >/dev/null

echo "UI profile pin contract tests passed"
