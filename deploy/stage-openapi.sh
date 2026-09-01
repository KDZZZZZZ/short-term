#!/usr/bin/env bash
set -euo pipefail

source_file=/opt/short-term/incoming/openapi.bundle.json
target_dir=/var/lib/short-term-openapi
target_file="$target_dir/openapi.bundle.json"
swagger_image=m.daocloud.io/docker.io/swaggerapi/swagger-ui@sha256:3d93169968848d371a6a56ca1ab18b47a8906ba461b8eba0688866354f5431d5

if [[ ! -f "$source_file" || -L "$source_file" ]]; then
  echo "OpenAPI bundle is missing or is not a regular file" >&2
  exit 1
fi

python3 -m json.tool "$source_file" >/dev/null

if ! /usr/bin/podman image exists "$swagger_image"; then
  /usr/bin/podman pull "$swagger_image"
fi

install -d -o root -g root -m 0755 "$target_dir"

temporary_file="$(mktemp "$target_dir/openapi.bundle.json.XXXXXX")"
trap 'rm -f "$temporary_file"' EXIT
install -o root -g root -m 0644 "$source_file" "$temporary_file"
mv -f "$temporary_file" "$target_file"
trap - EXIT
