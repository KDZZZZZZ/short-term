#!/usr/bin/env bash
set -euo pipefail

# short-term web 前端的发布与回滚入口。
# 与 short-term-backend 同一惯例:仅限部署账号、rootless Podman、systemd 用户服务。

readonly podman_bin=/usr/bin/podman
readonly curl_bin=/usr/bin/curl
readonly systemctl_bin=/usr/bin/systemctl
readonly install_root=/opt/short-term
readonly frontend_root=$install_root/frontend
readonly releases_root=$frontend_root/releases
readonly current_link=$frontend_root/current
readonly previous_link=$frontend_root/previous
readonly state_dir=$install_root/state
readonly release_file=$state_dir/frontend-release.env
readonly previous_release_file=$state_dir/frontend-release.previous.env
readonly service_name=short-term-frontend.service
readonly container_name=short-term-frontend
readonly port=18084

die() {
  echo "short-term-frontend: $*" >&2
  exit 1
}

require_deploy_user() {
  [[ $(id -u) -ne 0 ]] || die "must run as the non-root deployment user"
  [[ -d "$install_root" && ! -L "$install_root" && -w "$install_root" ]] || \
    die "$install_root must be a writable, non-symlink directory owned by the deployment user"
  [[ -x "$podman_bin" && -x "$curl_bin" && -x "$systemctl_bin" ]] || \
    die "podman/curl/systemctl must be installed at /usr/bin"
}

# 原子替换符号链接:podman 在容器启动时解析挂载路径,所以换链后必须重启服务
switch_link() {
  local link=$1 target=$2
  target=$(readlink -f "$target")
  [[ -d "$target" ]] || die "release directory $target does not exist"
  ln -s "$target" "$link.replacing"
  mv -T "$link.replacing" "$link"
}

wait_ready() {
  local try
  for try in $(seq 1 30); do
    if "$curl_bin" --fail --silent --max-time 2 "http://127.0.0.1:$port/" 2>/dev/null | grep -q '<div id="root">'; then
      return 0
    fi
    sleep 1
  done
  return 1
}

expect_api_proxy() {
  # 未认证请求公开商品列表:拿到网关 JSON 错误即证明 /api/v1 代理链路生效
  "$curl_bin" --silent --max-time 5 "http://127.0.0.1:$port/api/v1/products" 2>/dev/null | grep -q '"code"'
}

usage() {
  echo "usage: short-term-frontend deploy <release-dir>" >&2
  echo "       short-term-frontend rollback" >&2
  echo "       short-term-frontend status" >&2
  exit 2
}

cmd_deploy() {
  local release_dir=$1 release_id
  [[ -d "$release_dir/html" && -f "$release_dir/nginx.conf" ]] || \
    die "$release_dir must contain html/ and nginx.conf"
  release_id=$(basename "$release_dir")

  install -d -m 0750 "$releases_root" "$frontend_root"
  if [[ -L "$current_link" ]]; then
    switch_link "$previous_link" "$current_link"
    cp "$release_file" "$previous_release_file" 2>/dev/null || true
  fi
  switch_link "$current_link" "$release_dir"
  printf 'FRONTEND_RELEASE=%s\n' "$release_id" > "$release_file"

  "$systemctl_bin" --user daemon-reload
  "$systemctl_bin" --user enable "$service_name" >/dev/null 2>&1 || true
  "$systemctl_bin" --user restart "$service_name"

  wait_ready || die "frontend did not become ready on 127.0.0.1:$port"
  expect_api_proxy || die "API proxy check failed on 127.0.0.1:$port"
  echo "frontend deployed: $release_id"
}

cmd_rollback() {
  [[ -L "$previous_link" ]] || die "no previous release to roll back to"
  local target
  target=$(readlink -f "$previous_link")
  switch_link "$current_link" "$target"
  [[ -f "$previous_release_file" ]] && cp "$previous_release_file" "$release_file"

  "$systemctl_bin" --user restart "$service_name"
  wait_ready || die "rollback did not become ready on 127.0.0.1:$port"
  echo "frontend rolled back to $(basename "$target")"
}

cmd_status() {
  "$podman_bin" ps --filter "name=$container_name" --format '{{.Names}}  {{.Status}}'
  "$curl_bin" --silent --max-time 3 -o /dev/null -w 'GET / -> %{http_code}\n' "http://127.0.0.1:$port/"
  "$curl_bin" --silent --max-time 5 -o /dev/null -w 'GET /api/v1/products -> %{http_code}\n' "http://127.0.0.1:$port/api/v1/products"
}

case ${1:-} in
  deploy)
    shift
    [[ $# -eq 1 ]] || usage
    require_deploy_user
    cmd_deploy "$1"
    ;;
  rollback)
    require_deploy_user
    cmd_rollback
    ;;
  status)
    cmd_status
    ;;
  *)
    usage
    ;;
esac
