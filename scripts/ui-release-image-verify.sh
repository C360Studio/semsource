#!/usr/bin/env sh
set -eu

fail() {
	echo "ui release image verification failed: $*" >&2
	exit 1
}

usage() {
	cat >&2 <<'USAGE'
usage: ui-release-image-verify.sh --repository REPOSITORY --tag TAG \
  --digest sha256:DIGEST --version VERSION --revision COMMIT \
  --run-url URL --run-attempt NUMBER [--output GITHUB_OUTPUT] [--evidence FILE]
USAGE
	exit 2
}

repository=
tag=
digest=
version=
revision=
run_url=
run_attempt=
output_file=
evidence_file=

while [ "$#" -gt 0 ]; do
	case "$1" in
	--repository | --tag | --digest | --version | --revision | --run-url | --run-attempt | --output | --evidence)
		[ "$#" -ge 2 ] || usage
		case "$1" in
		--repository) repository=$2 ;;
		--tag) tag=$2 ;;
		--digest) digest=$2 ;;
		--version) version=$2 ;;
		--revision) revision=$2 ;;
		--run-url) run_url=$2 ;;
		--run-attempt) run_attempt=$2 ;;
		--output) output_file=$2 ;;
		--evidence) evidence_file=$2 ;;
		esac
		shift 2
		;;
	*) usage ;;
	esac
done

[ -n "$repository" ] || usage
[ -n "$tag" ] || usage
[ -n "$digest" ] || usage
[ -n "$version" ] || usage
[ -n "$revision" ] || usage
[ -n "$run_url" ] || usage
[ -n "$run_attempt" ] || usage

command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"

case "$repository" in
*@* | *:*) fail "repository must not contain a tag or digest: $repository" ;;
ghcr.io/c360studio/semsource-ui) ;;
*) fail "unexpected repository: $repository" ;;
esac
if ! printf '%s\n' "$tag" | grep -Eq '^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$'; then
	fail "invalid tag: $tag"
fi
[ "$tag" != "latest" ] || fail "mutable tag 'latest' is not release evidence"
if ! printf '%s\n' "$version" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]*$'; then
	fail "invalid OCI version: $version"
fi
if ! printf '%s\n' "$digest" | grep -Eq '^sha256:[0-9a-f]{64}$'; then
	fail "digest must be a lowercase sha256 digest"
fi
if ! printf '%s\n' "$revision" | grep -Eq '^[0-9a-f]{40}$'; then
	fail "revision must be a full lowercase Git commit SHA"
fi
case "$run_url" in
https://github.com/*/actions/runs/[0-9]*) ;;
*) fail "run URL is not an authoritative GitHub Actions run URL: $run_url" ;;
esac
if ! printf '%s\n' "$run_attempt" | grep -Eq '^[1-9][0-9]*$'; then
	fail "run attempt must be a positive integer"
fi

tag_ref="$repository:$tag"
image_ref="$tag_ref@$digest"
inspect_file=$(mktemp "${TMPDIR:-/tmp}/semsource-ui-index.XXXXXX")
cleanup() {
	rm -f "$inspect_file"
}
trap cleanup EXIT INT TERM

# Inspect the tag separately. A digest-qualified pull alone can still succeed
# after a mutable tag has moved, which is not acceptable release evidence.
tag_inspect=$(docker buildx imagetools inspect "$tag_ref") ||
	fail "cannot inspect published tag $tag_ref"
tag_digest=$(printf '%s\n' "$tag_inspect" | awk '$1 == "Digest:" { print $2; exit }')
[ -n "$tag_digest" ] || fail "registry response did not report the tag digest"
[ "$tag_digest" = "$digest" ] ||
	fail "tag resolves to $tag_digest, expected $digest"

docker buildx imagetools inspect --raw "$image_ref" >"$inspect_file" ||
	fail "cannot inspect exact image reference $image_ref"
jq -e '.manifests | type == "array"' "$inspect_file" >/dev/null ||
	fail "published image is not a multi-platform OCI index"

platforms=$(jq -r '.manifests[] | [.platform.os, .platform.architecture] | join("/")' "$inspect_file")
for required_platform in linux/amd64 linux/arm64; do
	printf '%s\n' "$platforms" | grep -Fx "$required_platform" >/dev/null ||
		fail "missing required platform $required_platform"
done

for child_digest in $(jq -r '.manifests[] | select(.platform.os == "linux" and (.platform.architecture == "amd64" or .platform.architecture == "arm64")) | .digest' "$inspect_file"); do
	labels=$(docker buildx imagetools inspect "$repository@$child_digest" --format '{{json .Image.Config.Labels}}') ||
		fail "cannot inspect image config $child_digest"
	actual_version=$(printf '%s\n' "$labels" | jq -r '."org.opencontainers.image.version" // empty')
	actual_revision=$(printf '%s\n' "$labels" | jq -r '."org.opencontainers.image.revision" // empty')
	[ "$actual_version" = "$version" ] ||
		fail "$child_digest version label is '$actual_version', expected '$version'"
	[ "$actual_revision" = "$revision" ] ||
		fail "$child_digest revision label is '$actual_revision', expected '$revision'"
done

# Pull the same tag@manifest-digest that the profile will receive. Proving the
# local RepoDigest prevents a successful remote manifest check from being
# mistaken for evidence that the runtime actually selected that manifest.
docker pull "$image_ref" >/dev/null || fail "cannot pull exact image reference $image_ref"
repo_digests=$(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "$image_ref") ||
	fail "cannot inspect locally pulled image $image_ref"
expected_repo_digest="$repository@$digest"
observed_repo_digest=$(printf '%s\n' "$repo_digests" | grep -Fx "$expected_repo_digest" | head -n 1)
[ "$observed_repo_digest" = "$expected_repo_digest" ] ||
	fail "local RepoDigests do not contain published manifest $expected_repo_digest"

result=$(printf 'image_repository=%s\nimage_tag=%s\nimage_digest=%s\nimage_ref=%s\nimage_version=%s\nimage_revision=%s\nobserved_repo_digest=%s\n' \
	"$repository" "$tag" "$digest" "$image_ref" "$version" "$revision" "$observed_repo_digest")
printf '%s\n' "$result"

if [ -n "$output_file" ]; then
	printf '%s\n' "$result" >>"$output_file"
fi
if [ -n "$evidence_file" ]; then
	{
		printf '# SemSource workbench release image evidence\n\n'
		printf -- '- Repository: `%s`\n' "$repository"
		printf -- '- Tag: `%s`\n' "$tag"
		printf -- '- Digest: `%s`\n' "$digest"
		printf -- '- Exact reference: `%s`\n' "$image_ref"
		printf -- '- OCI version: `%s`\n' "$version"
		printf -- '- OCI revision: `%s`\n' "$revision"
		printf -- '- Platforms: `linux/amd64`, `linux/arm64`\n'
		printf -- '- Observed local RepoDigest: `%s`\n' "$observed_repo_digest"
		printf -- '- GitHub Actions run: [%s](%s)\n' "$run_url" "$run_url"
		printf -- '- Run attempt: `%s`\n' "$run_attempt"
		printf -- '- Released-profile gate: `task ui:smoke` passed before this evidence was published.\n'
	} >"$evidence_file"
fi
