#!/usr/bin/env bash
set -euo pipefail

readonly install_root=/opt/short-term
readonly loginctl_bin=/usr/bin/loginctl

die() {
  echo "short-term bootstrap: $*" >&2
  exit 1
}

[[ $(id -u) -eq 0 ]] || die "run this script as root"
[[ $# -eq 1 ]] || die "usage: $0 <deployment-user>"

deploy_user=$1
[[ "$deploy_user" =~ ^[a-z_][a-z0-9_-]*[$]?$ ]] || die "invalid deployment user"

passwd_entry=$(getent passwd "$deploy_user" || true)
[[ -n "$passwd_entry" ]] || die "deployment user does not exist"

deploy_uid=$(printf '%s\n' "$passwd_entry" | cut -d: -f3)
deploy_gid=$(printf '%s\n' "$passwd_entry" | cut -d: -f4)
[[ "$deploy_uid" =~ ^[0-9]+$ && "$deploy_uid" -ne 0 ]] || die "deployment user must not be root"
[[ "$deploy_gid" =~ ^[0-9]+$ ]] || die "deployment group is invalid"

[[ ! -L "$install_root" ]] || die "$install_root must not be a symlink"
install -d -o "$deploy_uid" -g "$deploy_gid" -m 0750 "$install_root"
install -d -o "$deploy_uid" -g "$deploy_gid" -m 0750 \
  "$install_root/bin" \
  "$install_root/incoming" \
  "$install_root/releases"
install -d -o "$deploy_uid" -g "$deploy_gid" -m 0700 \
  "$install_root/runtime" \
  "$install_root/state"

"$loginctl_bin" enable-linger "$deploy_user"
[[ $("$loginctl_bin" show-user "$deploy_user" -p Linger --value) == yes ]] || \
  die "failed to enable user-service persistence"

echo "short-term bootstrap complete: user=$deploy_user uid=$deploy_uid linger=yes"
