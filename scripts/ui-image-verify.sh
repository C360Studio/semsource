#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"

image=${SEMSOURCE_UI_VERIFY_IMAGE:-semsource-ui:verify-local}
head_revision=$(git rev-parse HEAD)
version=${SEMSOURCE_UI_VERSION:-sha-$head_revision}
revision=${SEMSOURCE_UI_COMMIT:-$head_revision}
container="semsource-ui-verify-$$"
metadata=$(mktemp)

if ! printf '%s\n' "$revision" | grep -Eq '^[0-9a-f]{40}$'; then
	echo "ui:image:verify failed: revision must be a full lowercase Git commit SHA" >&2
	exit 1
fi
if ! printf '%s\n' "$version" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._-]*$'; then
	echo "ui:image:verify failed: invalid OCI version '$version'" >&2
	exit 1
fi
case "$version" in
sha-*)
	[ "$version" = "sha-$revision" ] || {
		echo "ui:image:verify failed: SHA version '$version' does not match revision '$revision'" >&2
		exit 1
	}
	;;
esac

cleanup() {
	docker rm -f "$container" >/dev/null 2>&1 || true
	rm -f "$metadata"
}
trap cleanup EXIT INT TERM

echo "Building SemSource-owned workbench image from clean context"
docker buildx build --no-cache --load --metadata-file "$metadata" --tag "$image" \
	--build-arg "SEMSOURCE_UI_VERSION=$version" \
	--build-arg "SEMSOURCE_UI_COMMIT=$revision" ./ui

user=$(docker image inspect --format '{{.Config.User}}' "$image")
if [ "$user" != "semsource" ]; then
	echo "ui:image:verify failed: image user is '$user', want 'semsource'" >&2
	exit 1
fi

actual_version=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' "$image")
actual_revision=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$image")
[ "$actual_version" = "$version" ] || {
	echo "ui:image:verify failed: OCI version is '$actual_version', want '$version'" >&2
	exit 1
}
[ "$actual_revision" = "$revision" ] || {
	echo "ui:image:verify failed: OCI revision is '$actual_revision', want '$revision'" >&2
	exit 1
}

image_id=$(docker image inspect --format '{{.Id}}' "$image")
content_digest=$(sed -n 's/.*"containerimage\.digest"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$metadata")
case "$image_id" in
	sha256:*) ;;
	*)
		echo "ui:image:verify failed: image ID is not content-addressed: $image_id" >&2
		exit 1
		;;
esac
case "$content_digest" in
	sha256:*) ;;
	*)
		echo "ui:image:verify failed: build did not report an immutable content digest" >&2
		exit 1
		;;
esac

docker run --detach --name "$container" "$image" >/dev/null

runtime_uid=$(docker exec "$container" id -u)
if [ "$runtime_uid" != "1001" ]; then
	echo "ui:image:verify failed: runtime UID is '$runtime_uid', want '1001'" >&2
	exit 1
fi

deadline=$(( $(date +%s) + ${UI_IMAGE_HEALTH_TIMEOUT_SECONDS:-45} ))
health=starting
while [ "$(date +%s)" -lt "$deadline" ]; do
	health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}' "$container")
	[ "$health" = "healthy" ] && break
	[ "$health" = "unhealthy" ] && break
	sleep 1
done

if [ "$health" != "healthy" ]; then
	echo "ui:image:verify failed: container health is '$health'" >&2
	docker inspect "$container" >&2
	docker logs "$container" >&2
	exit 1
fi

shell_html=$(docker exec "$container" wget --quiet --output-document=- http://127.0.0.1:3000/)
case "$shell_html" in
	*"SemSource"*"Source knowledge workbench"*) ;;
	*)
		echo "ui:image:verify failed: root response is not the SemSource workbench shell" >&2
	exit 1
		;;
esac

echo "Verified non-root production workbench image"
echo "image=$image"
echo "image_id=$image_id"
echo "content_digest=$content_digest"
