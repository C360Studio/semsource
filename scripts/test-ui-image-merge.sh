#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
script="$project_dir/scripts/ui-image-merge.sh"
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/ui-image-merge-test.XXXXXX")
cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

repo=ghcr.io/c360studio/semsource-ui
amd=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
arm=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
index=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc

mkdir -p "$tmp_dir/bin"
cat >"$tmp_dir/bin/docker" <<SH
#!/usr/bin/env sh
set -eu
echo "\$*" >>"$tmp_dir/docker.log"
case "\$*" in
"buildx imagetools create -t $repo:v1.2.3 -t $repo:1.2.3 $repo@$amd $repo@$arm") ;;
"buildx imagetools inspect $repo:v1.2.3") printf 'Name: $repo:v1.2.3\nDigest: %s\n' "\${FAKE_INDEX_DIGEST}" ;;
*) echo "unexpected docker arguments: \$*" >&2; exit 99 ;;
esac
SH
chmod +x "$tmp_dir/bin/docker"
export PATH="$tmp_dir/bin:$PATH"
export IMAGE_REPOSITORY=$repo
export PRIMARY_TAG=v1.2.3
export IMAGE_TAGS="$repo:v1.2.3
$repo:1.2.3"
export FAKE_INDEX_DIGEST=$index

fail() {
	echo "UI image merge test failed: $*" >&2
	exit 1
}

setup_digests() {
	rm -rf "$tmp_dir/d" "$tmp_dir/out" "$tmp_dir/docker.log"
	mkdir -p "$tmp_dir/d"
	printf '%s\n' "$amd" >"$tmp_dir/d/amd64"
	printf '%s\n' "$arm" >"$tmp_dir/d/arm64"
	: >"$tmp_dir/out"
}

expect_refusal() {
	if GITHUB_OUTPUT="$tmp_dir/out" "$script" "$tmp_dir/d" >/dev/null 2>&1; then
		fail "$1"
	fi
	if [ -f "$tmp_dir/docker.log" ] && grep -F "imagetools create" "$tmp_dir/docker.log" >/dev/null; then
		fail "$1 (and still published)"
	fi
}

# Both platform digests merge under every tag, and the index digest is reported.
setup_digests
GITHUB_OUTPUT="$tmp_dir/out" "$script" "$tmp_dir/d" >/dev/null
grep -Fx "digest=$index" "$tmp_dir/out" >/dev/null || fail "index digest not written to GITHUB_OUTPUT"

setup_digests
rm "$tmp_dir/d/arm64"
expect_refusal "merged without the arm64 digest"

setup_digests
printf 'sha256:short\n' >"$tmp_dir/d/arm64"
expect_refusal "merged a malformed digest"

setup_digests
printf '%s\n' "$amd" >"$tmp_dir/d/extra"
expect_refusal "merged with an unexpected extra digest"

setup_digests
(
	IMAGE_TAGS="ghcr.io/someone-else/ui:v1.2.3"
	export IMAGE_TAGS
	expect_refusal "published a tag outside the repository"
)

# A registry that reports no digest must not yield release evidence.
setup_digests
if FAKE_INDEX_DIGEST="" GITHUB_OUTPUT="$tmp_dir/out" "$script" "$tmp_dir/d" >/dev/null 2>&1; then
	fail "reported success without an index digest"
fi
grep -F "digest=" "$tmp_dir/out" >/dev/null && fail "wrote a digest output without an index digest"

echo "UI image merge tests passed"
