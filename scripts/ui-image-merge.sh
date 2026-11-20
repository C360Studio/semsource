#!/usr/bin/env sh
# Merge the per-platform workbench digests pushed by publish-ui-build into one
# tagged multi-platform index, and report the index digest (semsource#219).
#
# Usage: IMAGE_REPOSITORY=... IMAGE_TAGS=... PRIMARY_TAG=... GITHUB_OUTPUT=... \
#   ui-image-merge.sh DIGEST_DIR
# DIGEST_DIR holds one file per platform (amd64, arm64), each one sha256 digest.
set -eu

fail() {
	echo "UI image merge failed: $*" >&2
	exit 1
}

[ "$#" -eq 1 ] || fail "usage: ui-image-merge.sh DIGEST_DIR"
digest_dir=$1
repository=${IMAGE_REPOSITORY:?IMAGE_REPOSITORY is required}
tags=${IMAGE_TAGS:?IMAGE_TAGS is required}
primary_tag=${PRIMARY_TAG:?PRIMARY_TAG is required}
output_file=${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}

# Both platforms or nothing: a partial index would publish a tag that the
# release smoke rejects only after the tag has already moved.
[ "$(find "$digest_dir" -type f | wc -l | tr -d ' ')" = 2 ] ||
	fail "expected exactly two platform digests in $digest_dir"

set --
for arch in amd64 arm64; do
	[ -f "$digest_dir/$arch" ] || fail "missing $arch digest"
	digest=$(cat "$digest_dir/$arch")
	printf '%s\n' "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$' ||
		fail "$arch digest is not a lowercase sha256 digest: $digest"
	set -- "$@" "$repository@$digest"
done

tag_args=
for tag in $tags; do
	case "$tag" in
	"$repository":*) ;;
	*) fail "tag $tag is not in $repository" ;;
	esac
	tag_args="$tag_args -t $tag"
done
[ -n "$tag_args" ] || fail "no tags to publish"

# shellcheck disable=SC2086 # tag_args is a validated, space-free list
docker buildx imagetools create $tag_args "$@" || fail "imagetools create failed"

inspect=$(docker buildx imagetools inspect "$repository:$primary_tag") ||
	fail "cannot inspect $repository:$primary_tag"
index_digest=$(printf '%s\n' "$inspect" | awk '$1 == "Digest:" { print $2; exit }')
printf '%s\n' "$index_digest" | grep -Eq '^sha256:[0-9a-f]{64}$' ||
	fail "registry did not report an index digest for $repository:$primary_tag"

echo "digest=$index_digest" >>"$output_file"
echo "published $repository:$primary_tag@$index_digest"
