#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/ui-release-image-verify-test.XXXXXX")

cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

mkdir -p "$tmp_dir/bin"
cat >"$tmp_dir/bin/docker" <<'SH'
#!/usr/bin/env sh
set -eu

case "$*" in
"buildx imagetools inspect ghcr.io/c360studio/semsource-ui:v1.2.3")
	printf 'Name: ghcr.io/c360studio/semsource-ui:v1.2.3\nDigest: %s\n' "${FAKE_TAG_DIGEST}"
	;;
"buildx imagetools inspect --raw ghcr.io/c360studio/semsource-ui:v1.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	printf '%s\n' "${FAKE_INDEX_JSON}"
	;;
"buildx imagetools inspect ghcr.io/c360studio/semsource-ui@sha256:amd64 --format {{json .Image.Config.Labels}}" | \
"buildx imagetools inspect ghcr.io/c360studio/semsource-ui@sha256:arm64 --format {{json .Image.Config.Labels}}")
	if [ -n "${FAKE_LABEL_JSON:-}" ]; then
		printf '%s\n' "$FAKE_LABEL_JSON"
	else
		printf '{"org.opencontainers.image.version":"%s","org.opencontainers.image.revision":"%s"}\n' \
			"${FAKE_VERSION}" "${FAKE_REVISION}"
	fi
	;;
"pull ghcr.io/c360studio/semsource-ui:v1.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	printf 'pulled\n'
	;;
"image inspect --format {{range .RepoDigests}}{{println .}}{{end}} ghcr.io/c360studio/semsource-ui:v1.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	printf '%s\n' "${FAKE_REPO_DIGEST}"
	;;
*)
	echo "unexpected docker arguments: $*" >&2
	exit 99
	;;
esac
SH
chmod +x "$tmp_dir/bin/docker"

digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
revision=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
export PATH="$tmp_dir/bin:$PATH"
export FAKE_TAG_DIGEST=$digest
export FAKE_VERSION=1.2.3
export FAKE_REVISION=$revision
export FAKE_REPO_DIGEST="ghcr.io/c360studio/semsource-ui@$digest"
export FAKE_INDEX_JSON='{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":"sha256:amd64","platform":{"architecture":"amd64","os":"linux"}},{"digest":"sha256:arm64","platform":{"architecture":"arm64","os":"linux"}}]}'

run_verify() {
	"$project_dir/scripts/ui-release-image-verify.sh" \
		--repository ghcr.io/c360studio/semsource-ui \
		--tag v1.2.3 \
		--digest "$digest" \
		--version 1.2.3 \
		--revision "$revision" \
		--run-url https://github.com/C360Studio/semsource/actions/runs/1234 \
		--run-attempt 2 "$@"
}

output=$(run_verify --output "$tmp_dir/github.out" --evidence "$tmp_dir/evidence.md")
printf '%s\n' "$output" | grep -F "image_ref=ghcr.io/c360studio/semsource-ui:v1.2.3@$digest" >/dev/null
printf '%s\n' "$output" | grep -F "image_digest=$digest" >/dev/null
grep -Fx "observed_repo_digest=ghcr.io/c360studio/semsource-ui@$digest" "$tmp_dir/github.out" >/dev/null
grep -F "https://github.com/C360Studio/semsource/actions/runs/1234" "$tmp_dir/evidence.md" >/dev/null
grep -F "Run attempt: \`2\`" "$tmp_dir/evidence.md" >/dev/null

if "$project_dir/scripts/ui-release-image-verify.sh" \
	--repository ghcr.io/c360studio/semsource-ui \
	--tag latest \
	--digest "$digest" \
	--version 1.2.3 \
	--revision "$revision" \
	--run-url https://github.com/C360Studio/semsource/actions/runs/1234 \
	--run-attempt 2 >"$tmp_dir/latest.out" 2>&1; then
	echo "digest-qualified latest must not be accepted as release evidence" >&2
	exit 1
fi
grep -F "mutable tag 'latest'" "$tmp_dir/latest.out" >/dev/null

export FAKE_TAG_DIGEST=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
if run_verify >"$tmp_dir/moved.out" 2>&1; then
	echo "expected a moved tag to fail verification" >&2
	exit 1
fi
grep -F "tag resolves to" "$tmp_dir/moved.out" >/dev/null

export FAKE_TAG_DIGEST=$digest
export FAKE_REVISION=cccccccccccccccccccccccccccccccccccccccc
if run_verify >"$tmp_dir/revision.out" 2>&1; then
	echo "expected a mismatched revision label to fail verification" >&2
	exit 1
fi
grep -F "revision label" "$tmp_dir/revision.out" >/dev/null

export FAKE_REVISION=$revision
export FAKE_VERSION=9.9.9
if run_verify >"$tmp_dir/version.out" 2>&1; then
	echo "expected a mismatched version label to fail verification" >&2
	exit 1
fi
grep -F "version label" "$tmp_dir/version.out" >/dev/null

export FAKE_VERSION=1.2.3
export FAKE_LABEL_JSON='{}'
if run_verify >"$tmp_dir/labels.out" 2>&1; then
	echo "expected missing OCI labels to fail verification" >&2
	exit 1
fi
grep -F "version label" "$tmp_dir/labels.out" >/dev/null
unset FAKE_LABEL_JSON

export FAKE_INDEX_JSON='{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":"sha256:amd64","platform":{"architecture":"amd64","os":"linux"}}]}'
if run_verify >"$tmp_dir/platform.out" 2>&1; then
	echo "expected a missing arm64 platform to fail verification" >&2
	exit 1
fi
grep -F "missing required platform linux/arm64" "$tmp_dir/platform.out" >/dev/null

export FAKE_INDEX_JSON='{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"digest":"sha256:amd64","platform":{"architecture":"amd64","os":"linux"}},{"digest":"sha256:arm64","platform":{"architecture":"arm64","os":"linux"}}]}'
export FAKE_REPO_DIGEST="ghcr.io/c360studio/semsource-ui@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
if run_verify >"$tmp_dir/repo-digest.out" 2>&1; then
	echo "expected a mismatched local RepoDigest to fail verification" >&2
	exit 1
fi
grep -F "local RepoDigests" "$tmp_dir/repo-digest.out" >/dev/null

echo "ui release image verifier contract tests passed"
