#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 <release> [archive.tar.gz]" >&2
  exit 2
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
fi

release=$1
archive=${2:-}
if [[ ! "$release" =~ ^[0-9A-Za-z._-]{7,128}$ ]]; then
  echo "release must contain only letters, digits, dot, underscore or dash" >&2
  exit 2
fi

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
revision=${GITHUB_SHA:-$(git -C "$repo_root" rev-parse HEAD)}
platform=${BACKEND_IMAGE_PLATFORM:-linux/amd64}
default_go_proxy='https://proxy.golang.org|https://goproxy.cn|direct'
go_proxy=${BACKEND_GOPROXY:-$default_go_proxy}
services=(account marketplace messaging favorite gateway)
images=()

for service in "${services[@]}"; do
  image="localhost/short-term/$service:$release"
  docker build \
    --platform "$platform" \
    --file "$repo_root/Dockerfile.backend" \
    --build-arg "SERVICE=$service" \
    --build-arg "VERSION=$release" \
    --build-arg "VCS_REF=$revision" \
    --build-arg "GOPROXY=$go_proxy" \
    --tag "$image" \
    "$repo_root"
  images+=("$image")
done

if [[ -n "$archive" ]]; then
  postgres_source='docker.io/library/postgres@sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15'
  postgres_image="localhost/short-term/postgres:18-$release"
  docker pull --platform "$platform" "$postgres_source"
  docker tag "$postgres_source" "$postgres_image"
  images+=("$postgres_image")

  mkdir -p "$(dirname "$archive")"
  docker save "${images[@]}" | gzip -1 >"$archive"
  archive_dir=$(cd "$(dirname "$archive")" && pwd)
  archive_name=$(basename "$archive")
  (cd "$archive_dir" && sha256sum "$archive_name" >"$archive_name.sha256")
fi

printf '%s\n' "${images[@]}"
