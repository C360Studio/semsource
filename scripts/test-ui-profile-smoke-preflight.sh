#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/ui-profile-preflight-test.XXXXXX")
cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

mkdir -p "$tmp_dir/bin"
cat >"$tmp_dir/bin/docker" <<'SH'
#!/usr/bin/env sh
if [ "$*" = "compose version" ]; then
	exit 0
fi
echo "unexpected Docker call after preflight: $*" >&2
exit 99
SH
chmod +x "$tmp_dir/bin/docker"

digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
if PATH="$tmp_dir/bin:$PATH" \
	SEMSOURCE_UI_IMAGE="ghcr.io/c360studio/semsource-ui:latest@$digest" \
	"$project_dir/scripts/ui-profile-smoke.sh" released >"$tmp_dir/latest.out" 2>&1; then
	echo "released smoke must reject digest-qualified latest during preflight" >&2
	exit 1
fi
grep -F "cannot use mutable tag 'latest'" "$tmp_dir/latest.out" >/dev/null

echo "UI profile smoke preflight contract tests passed"
