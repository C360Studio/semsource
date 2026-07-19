#!/usr/bin/env sh
set -eu

fail() {
	echo "UI image metadata failed: $*" >&2
	exit 1
}

validate_release_tag() {
	candidate=$1
	if ! printf '%s\n' "$candidate" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'; then
		return 1
	fi
	case "$candidate" in
	*-*) prerelease=${candidate#*-} ;;
	*) return 0 ;;
	esac
	case "$prerelease" in
	.* | *. | *..*) return 1 ;;
	esac
	old_ifs=$IFS
	IFS=.
	# shellcheck disable=SC2086 # prerelease identifiers are intentionally split on dots.
	set -- $prerelease
	IFS=$old_ifs
	for identifier in "$@"; do
		case "$identifier" in
		"" | *[!0-9A-Za-z-]*) return 1 ;;
		*[!0-9]*) ;;
		0 | [1-9] | [1-9][0-9]*) ;;
		*) return 1 ;;
		esac
	done
}

usage() {
	cat >&2 <<'USAGE'
usage: ui-image-metadata.sh --repository REPOSITORY --ref GIT_REF \
  --ref-name GIT_REF_NAME --revision COMMIT --output GITHUB_OUTPUT
USAGE
	exit 2
}

repository=
source_ref=
source_ref_name=
revision=
output_file=
while [ "$#" -gt 0 ]; do
	case "$1" in
	--repository | --ref | --ref-name | --revision | --output)
		[ "$#" -ge 2 ] || usage
		case "$1" in
		--repository) repository=$2 ;;
		--ref) source_ref=$2 ;;
		--ref-name) source_ref_name=$2 ;;
		--revision) revision=$2 ;;
		--output) output_file=$2 ;;
		esac
		shift 2
		;;
	*) usage ;;
	esac
done

[ "$repository" = "ghcr.io/c360studio/semsource-ui" ] ||
	fail "unexpected repository: $repository"
if ! printf '%s\n' "$revision" | grep -Eq '^[0-9a-f]{40}$'; then
	fail "revision must be a full lowercase Git commit SHA"
fi
[ -n "$output_file" ] || usage

if [ "$source_ref" = "refs/heads/main" ]; then
	[ "$source_ref_name" = "main" ] || fail "main ref has unexpected ref name: $source_ref_name"
	version="sha-$revision"
	primary_tag=$version
	tags=$(printf '%s:latest\n%s:%s\n' "$repository" "$repository" "$version")
else
	[ "$source_ref" = "refs/tags/$source_ref_name" ] ||
		fail "ref is not trusted for UI image publication: $source_ref"
	if ! validate_release_tag "$source_ref_name"; then
		fail "release tag is not valid v-prefixed semver: $source_ref_name"
	fi
	primary_tag=$source_ref_name
	version=${source_ref_name#v}
	tags=$(printf '%s:%s\n%s:%s\n' "$repository" "$source_ref_name" "$repository" "$version")
fi

{
	echo "repository=$repository"
	echo "version=$version"
	echo "revision=$revision"
	echo "primary_tag=$primary_tag"
	echo "tags<<SEMSOURCE_UI_TAGS"
	printf '%s\n' "$tags"
	echo "SEMSOURCE_UI_TAGS"
} >>"$output_file"

printf 'repository=%s\nversion=%s\nrevision=%s\nprimary_tag=%s\n%s\n' \
	"$repository" "$version" "$revision" "$primary_tag" "$tags"
