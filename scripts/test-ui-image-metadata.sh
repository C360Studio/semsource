#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
script="$project_dir/scripts/ui-image-metadata.sh"
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/ui-image-metadata-test.XXXXXX")
cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

repository=ghcr.io/c360studio/semsource-ui
revision=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa

"$script" --repository "$repository" --ref refs/heads/main --ref-name main \
	--revision "$revision" --output "$tmp_dir/main.out" >/dev/null
grep -Fx "version=sha-$revision" "$tmp_dir/main.out" >/dev/null
grep -Fx "revision=$revision" "$tmp_dir/main.out" >/dev/null
grep -Fx "primary_tag=sha-$revision" "$tmp_dir/main.out" >/dev/null
grep -Fx "$repository:latest" "$tmp_dir/main.out" >/dev/null
grep -Fx "$repository:sha-$revision" "$tmp_dir/main.out" >/dev/null

"$script" --repository "$repository" --ref refs/tags/v1.2.3-beta.4 --ref-name v1.2.3-beta.4 \
	--revision "$revision" --output "$tmp_dir/release.out" >/dev/null
grep -Fx "version=1.2.3-beta.4" "$tmp_dir/release.out" >/dev/null
grep -Fx "revision=$revision" "$tmp_dir/release.out" >/dev/null
grep -Fx "primary_tag=v1.2.3-beta.4" "$tmp_dir/release.out" >/dev/null
grep -Fx "$repository:v1.2.3-beta.4" "$tmp_dir/release.out" >/dev/null
grep -Fx "$repository:1.2.3-beta.4" "$tmp_dir/release.out" >/dev/null
if grep -F "sha-$revision" "$tmp_dir/release.out" >/dev/null; then
	echo "release-tag publication must not mutate the immutable main SHA tag" >&2
	exit 1
fi

if "$script" --repository "$repository" --ref refs/pull/73/merge --ref-name 73/merge \
	--revision "$revision" --output "$tmp_dir/pr.out" >"$tmp_dir/pr.log" 2>&1; then
	echo "pull-request refs must not produce publication metadata" >&2
	exit 1
fi

if "$script" --repository "$repository" --ref refs/tags/v1.2.3+build.1 --ref-name v1.2.3+build.1 \
	--revision "$revision" --output "$tmp_dir/build.out" >"$tmp_dir/build.log" 2>&1; then
	echo "Docker-incompatible build-metadata tags must be rejected" >&2
	exit 1
fi

for malformed_tag in v01.2.3 v1.2.3-.. v1.2.3-01 v1.2.3-alpha..1; do
	if "$script" --repository "$repository" --ref "refs/tags/$malformed_tag" --ref-name "$malformed_tag" \
		--revision "$revision" --output "$tmp_dir/malformed.out" >"$tmp_dir/malformed.log" 2>&1; then
		echo "malformed release tag must be rejected: $malformed_tag" >&2
		exit 1
	fi
done

echo "UI image metadata contract tests passed"
