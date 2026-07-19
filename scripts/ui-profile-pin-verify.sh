#!/usr/bin/env sh
set -eu

fail() {
	echo "UI profile pin verification failed: $*" >&2
	exit 1
}

usage() {
	echo "usage: ui-profile-pin-verify.sh --expected IMAGE --observed IMAGE --source compose-render|running-container [--evidence FILE]" >&2
	exit 2
}

expected=
observed=
source_name=
evidence_file=
while [ "$#" -gt 0 ]; do
	case "$1" in
	--expected | --observed | --source | --evidence)
		[ "$#" -ge 2 ] || usage
		case "$1" in
		--expected) expected=$2 ;;
		--observed) observed=$2 ;;
		--source) source_name=$2 ;;
		--evidence) evidence_file=$2 ;;
		esac
		shift 2
		;;
	*) usage ;;
	esac
done

if ! printf '%s\n' "$expected" | grep -Eq '^ghcr\.io/c360studio/semsource-ui:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}@sha256:[0-9a-f]{64}$'; then
	fail "expected image is not an immutable SemSource UI tag@digest"
fi
case "$expected" in
ghcr.io/c360studio/semsource-ui:latest@sha256:*)
	fail "mutable tag 'latest' is not an acceptable profile pin"
	;;
esac
case "$source_name" in
compose-render) evidence_label="Compose-rendered UI image" ;;
running-container) evidence_label="Running UI container Config.Image" ;;
*) usage ;;
esac
[ "$observed" = "$expected" ] ||
	fail "$source_name observed '$observed', expected '$expected'"

printf '%s=%s\n' "$source_name" "$observed"
if [ -n "$evidence_file" ]; then
	printf -- '- %s: `%s`\n' "$evidence_label" "$observed" >>"$evidence_file"
fi
