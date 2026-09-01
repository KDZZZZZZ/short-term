#!/usr/bin/env bash
set -euo pipefail

readonly podman=/usr/bin/podman
readonly curl_bin=/usr/bin/curl
readonly python_bin=/usr/bin/python3
readonly systemctl_bin=/usr/bin/systemctl
readonly loginctl_bin=/usr/bin/loginctl
readonly install_root=/opt/short-term
readonly state_dir=$install_root/state
readonly release_file=$state_dir/backend-release.env
readonly previous_release_file=$state_dir/backend-release.previous.env
readonly secret_file=$state_dir/backend-secrets.env
readonly runtime_dir=$install_root/runtime
readonly media_dir=$runtime_dir/media
readonly network_name=short-term-backend
readonly postgres_volume=short-term-postgres-data
readonly postgres_container=short-term-postgres
readonly gateway_port=18083
readonly gateway_management_port=19090

readonly app_containers=(
  short-term-gateway
  short-term-favorite
  short-term-messaging-worker
  short-term-messaging
  short-term-marketplace-worker
  short-term-marketplace
  short-term-account
)

die() {
  echo "short-term-backend: $*" >&2
  exit 1
}

require_deploy_user() {
  [[ $(id -u) -ne 0 ]] || die "must run as the non-root deployment user"
  [[ -x "$podman" ]] || die "podman is not installed at $podman"
  [[ -x "$curl_bin" ]] || die "curl is not installed at $curl_bin"
  [[ -x "$python_bin" ]] || die "python3 is not installed at $python_bin"
  [[ -x "$systemctl_bin" ]] || die "systemctl is not installed at $systemctl_bin"
  [[ -x "$loginctl_bin" ]] || die "loginctl is not installed at $loginctl_bin"
  [[ -d "$install_root" && ! -L "$install_root" && -w "$install_root" ]] || \
    die "$install_root must be a writable, non-symlink directory owned by the deployment user"
  [[ $("$podman" info --format '{{.Host.Security.Rootless}}') == true ]] || \
    die "Podman must run rootless for the deployment user"
}

require_linger() {
  local linger
  linger=$("$loginctl_bin" show-user "$(id -u)" -p Linger --value 2>/dev/null || true)
  [[ "$linger" == yes ]] || \
    die "user-service persistence is disabled; run deploy/backend/bootstrap-host.sh once as root"
}

user_systemctl() {
  "$systemctl_bin" --user "$@"
}

random_hex() {
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
}

validate_hex_secret() {
  local name=$1 value=$2
  [[ "$value" =~ ^[0-9a-f]{64}$ ]] || die "$name in $secret_file is invalid"
}

validate_assignment_file() {
  local file=$1 expected_lines=$2 pattern=$3
  [[ $(wc -l <"$file") -eq "$expected_lines" ]] || die "$file has an unexpected number of entries"
  if grep -Ev "$pattern" "$file" >/dev/null; then
    die "$file contains an unsafe assignment"
  fi
}

load_secrets() {
  [[ -f "$secret_file" && ! -L "$secret_file" ]] || die "missing $secret_file"
  validate_assignment_file "$secret_file" 6 \
    '^(POSTGRES_PASSWORD|ACCOUNT_DB_PASSWORD|MARKETPLACE_DB_PASSWORD|MESSAGING_DB_PASSWORD|FAVORITE_DB_PASSWORD|JWT_SIGNING_KEY)=[0-9a-f]{64}$'
  # The file is generated on the host, owned by the deployment user and mode 0600.
  # shellcheck disable=SC1090
  source "$secret_file"
  validate_hex_secret POSTGRES_PASSWORD "${POSTGRES_PASSWORD:-}"
  validate_hex_secret ACCOUNT_DB_PASSWORD "${ACCOUNT_DB_PASSWORD:-}"
  validate_hex_secret MARKETPLACE_DB_PASSWORD "${MARKETPLACE_DB_PASSWORD:-}"
  validate_hex_secret MESSAGING_DB_PASSWORD "${MESSAGING_DB_PASSWORD:-}"
  validate_hex_secret FAVORITE_DB_PASSWORD "${FAVORITE_DB_PASSWORD:-}"
  validate_hex_secret JWT_SIGNING_KEY "${JWT_SIGNING_KEY:-}"
}

ensure_secrets() {
  install -d -m 0700 "$state_dir"
  if [[ ! -e "$secret_file" ]]; then
    local temporary
    temporary=$(mktemp "$state_dir/backend-secrets.env.XXXXXX")
    chmod 0600 "$temporary"
    {
      printf 'POSTGRES_PASSWORD=%s\n' "$(random_hex)"
      printf 'ACCOUNT_DB_PASSWORD=%s\n' "$(random_hex)"
      printf 'MARKETPLACE_DB_PASSWORD=%s\n' "$(random_hex)"
      printf 'MESSAGING_DB_PASSWORD=%s\n' "$(random_hex)"
      printf 'FAVORITE_DB_PASSWORD=%s\n' "$(random_hex)"
      printf 'JWT_SIGNING_KEY=%s\n' "$(random_hex)"
    } >"$temporary"
    mv "$temporary" "$secret_file"
  fi
  [[ ! -L "$secret_file" ]] || die "$secret_file must not be a symlink"
  chmod 0600 "$secret_file"
  load_secrets
}

install_env_file() {
  local target=$1
  local temporary
  temporary=$(mktemp "$state_dir/$(basename "$target").XXXXXX")
  chmod 0600 "$temporary"
  cat >"$temporary"
  install -m 0600 "$temporary" "$target"
  rm -f "$temporary"
}

write_runtime_envs() {
  install_env_file "$state_dir/common.env" <<'EOF'
ENVIRONMENT=production
LOG_LEVEL=info
SHUTDOWN_TIMEOUT=15s
EOF

  install_env_file "$state_dir/postgres.env" <<EOF
POSTGRES_USER=postgres
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=postgres
POSTGRES_INITDB_ARGS=--locale-provider=icu --icu-locale=und-x-icu --encoding=UTF8
EOF

  install_env_file "$state_dir/account.env" <<EOF
ACCOUNT_DATABASE_URL=postgres://account_svc:$ACCOUNT_DB_PASSWORD@postgres:5432/account_db?sslmode=disable
ACCOUNT_AUTO_MIGRATE=false
ACCOUNT_GRPC_ADDR=:9001
JWT_SIGNING_KEY=$JWT_SIGNING_KEY
JWT_ISSUER=shortterm-account
JWT_AUDIENCE=shortterm-api
JWT_TTL=24h
EOF

  install_env_file "$state_dir/marketplace.env" <<EOF
MARKETPLACE_DATABASE_URL=postgres://marketplace_svc:$MARKETPLACE_DB_PASSWORD@postgres:5432/marketplace_db?sslmode=disable
MARKETPLACE_AUTO_MIGRATE=false
MARKETPLACE_GRPC_ADDR=:9002
MESSAGING_GRPC_TARGET=messaging:9003
MEDIA_ROOT=/var/lib/shortterm/media
MEDIA_PUBLIC_URL=/media
OUTBOX_INTERVAL=1s
OUTBOX_BATCH_SIZE=100
EOF

  install_env_file "$state_dir/messaging.env" <<EOF
MESSAGING_DATABASE_URL=postgres://messaging_svc:$MESSAGING_DB_PASSWORD@postgres:5432/messaging_db?sslmode=disable
MESSAGING_AUTO_MIGRATE=false
MESSAGING_GRPC_ADDR=:9003
MARKETPLACE_GRPC_TARGET=marketplace:9002
OUTBOX_INTERVAL=1s
OUTBOX_BATCH_SIZE=100
EOF

  install_env_file "$state_dir/favorite.env" <<EOF
FAVORITE_DATABASE_URL=postgres://favorite_svc:$FAVORITE_DB_PASSWORD@postgres:5432/favorite_db?sslmode=disable
FAVORITE_AUTO_MIGRATE=false
FAVORITE_GRPC_ADDR=:9004
MARKETPLACE_GRPC_TARGET=marketplace:9002
EOF

  install_env_file "$state_dir/gateway.env" <<EOF
GATEWAY_HTTP_ADDR=:8080
GATEWAY_MANAGEMENT_ADDR=:9090
GATEWAY_BASE_PATH=/api/v1
GATEWAY_MEDIA_DIR=/var/lib/shortterm/media
GATEWAY_MEDIA_PATH=/media
ACCOUNT_GRPC_TARGET=account:9001
MARKETPLACE_GRPC_TARGET=marketplace:9002
MESSAGING_GRPC_TARGET=messaging:9003
FAVORITE_GRPC_TARGET=favorite:9004
JWT_SIGNING_KEY=$JWT_SIGNING_KEY
JWT_ISSUER=shortterm-account
JWT_AUDIENCE=shortterm-api
GATEWAY_TRUST_PROXY_HEADERS=false
EOF
}

validate_release() {
  [[ "${RELEASE_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || die "invalid RELEASE_SHA"
  local name image expected
  for name in account marketplace messaging favorite gateway; do
    expected="localhost/short-term/$name:$RELEASE_SHA"
    case "$name" in
      account) image=${ACCOUNT_IMAGE:-} ;;
      marketplace) image=${MARKETPLACE_IMAGE:-} ;;
      messaging) image=${MESSAGING_IMAGE:-} ;;
      favorite) image=${FAVORITE_IMAGE:-} ;;
      gateway) image=${GATEWAY_IMAGE:-} ;;
    esac
    [[ "$image" == "$expected" ]] || die "invalid $name image in release"
  done
  [[ "${POSTGRES_IMAGE:-}" == "localhost/short-term/postgres:18-$RELEASE_SHA" ]] || die "invalid postgres image in release"
}

load_release() {
  local file=${1:-$release_file}
  [[ -f "$file" && ! -L "$file" ]] || die "missing release file $file"
  validate_assignment_file "$file" 7 \
    '^(RELEASE_SHA=[0-9a-f]{40}|(ACCOUNT_IMAGE|MARKETPLACE_IMAGE|MESSAGING_IMAGE|FAVORITE_IMAGE|GATEWAY_IMAGE)=localhost/short-term/(account|marketplace|messaging|favorite|gateway):[0-9a-f]{40}|POSTGRES_IMAGE=localhost/short-term/postgres:18-[0-9a-f]{40})$'
  # Release files contain only validated image tags and a Git commit SHA.
  # shellcheck disable=SC1090
  source "$file"
  validate_release
}

ensure_network_and_storage() {
  "$podman" network exists "$network_name" || "$podman" network create "$network_name" >/dev/null
  "$podman" volume exists "$postgres_volume" || "$podman" volume create "$postgres_volume" >/dev/null
  install -d -m 0700 "$runtime_dir"
  if [[ ! -d "$media_dir" ]]; then
    mkdir -m 0750 "$media_dir"
  fi
  "$podman" unshare chown -R 65532:65532 "$media_dir"
}

remove_container() {
  local name=$1
  if "$podman" container exists "$name"; then
    "$podman" rm --force --time 15 "$name" >/dev/null
  fi
}

start_postgres() {
  remove_container "$postgres_container"
  "$podman" run --detach \
    --name "$postgres_container" \
    --network "$network_name" \
    --network-alias postgres \
    --env-file "$state_dir/postgres.env" \
    --volume "$postgres_volume:/var/lib/postgresql:Z" \
    --health-cmd 'pg_isready -U postgres -d postgres' \
    --health-interval 5s \
    --health-timeout 5s \
    --health-retries 20 \
    --restart always \
    "$POSTGRES_IMAGE" >/dev/null

  local attempt
  for attempt in $(seq 1 60); do
    if "$podman" exec "$postgres_container" pg_isready -U postgres -d postgres >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  "$podman" logs --tail 100 "$postgres_container" >&2 || true
  die "PostgreSQL did not become ready"
}

sql_identifier_allowed() {
  [[ "$1" =~ ^[a-z_]+$ ]]
}

provision_role_and_database() {
  local role=$1 password=$2 database=$3
  sql_identifier_allowed "$role" || die "unsafe database role"
  sql_identifier_allowed "$database" || die "unsafe database name"
  validate_hex_secret "$role password" "$password"

  if [[ $("$podman" exec "$postgres_container" psql -U postgres -d postgres -Atqc "SELECT 1 FROM pg_roles WHERE rolname = '$role'") != 1 ]]; then
    "$podman" exec "$postgres_container" psql -v ON_ERROR_STOP=1 -U postgres -d postgres \
      -c "CREATE ROLE $role LOGIN PASSWORD '$password'" >/dev/null
  else
    "$podman" exec "$postgres_container" psql -v ON_ERROR_STOP=1 -U postgres -d postgres \
      -c "ALTER ROLE $role WITH LOGIN PASSWORD '$password'" >/dev/null
  fi

  if [[ $("$podman" exec "$postgres_container" psql -U postgres -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname = '$database'") != 1 ]]; then
    "$podman" exec "$postgres_container" createdb -U postgres -O "$role" "$database"
  fi
  "$podman" exec "$postgres_container" psql -v ON_ERROR_STOP=1 -U postgres -d postgres \
    -c "ALTER DATABASE $database OWNER TO $role" \
    -c "REVOKE CONNECT ON DATABASE $database FROM PUBLIC" \
    -c "GRANT CONNECT ON DATABASE $database TO $role" >/dev/null
}

provision_databases() {
  provision_role_and_database account_svc "$ACCOUNT_DB_PASSWORD" account_db
  provision_role_and_database marketplace_svc "$MARKETPLACE_DB_PASSWORD" marketplace_db
  provision_role_and_database messaging_svc "$MESSAGING_DB_PASSWORD" messaging_db
  provision_role_and_database favorite_svc "$FAVORITE_DB_PASSWORD" favorite_db
}

run_migration() {
  local service=$1 image=$2
  "$podman" run --rm \
    --name "short-term-$service-migrate" \
    --network "$network_name" \
    --env-file "$state_dir/common.env" \
    --env-file "$state_dir/$service.env" \
    --read-only \
    --tmpfs /tmp:rw,noexec,nosuid,size=32m \
    --cap-drop all \
    --security-opt no-new-privileges \
    --entrypoint /usr/local/bin/migrate \
    "$image"
}

run_migrations() {
  run_migration account "$ACCOUNT_IMAGE"
  run_migration marketplace "$MARKETPLACE_IMAGE"
  run_migration messaging "$MESSAGING_IMAGE"
  run_migration favorite "$FAVORITE_IMAGE"
}

base_run_args() {
  local name=$1 alias=$2 env_file=$3
  BASE_RUN_ARGS=(
    --detach
    --name "$name"
    --network "$network_name"
    --env-file "$state_dir/common.env"
    --env-file "$env_file"
    --read-only
    --tmpfs /tmp:rw,noexec,nosuid,size=64m
    --cap-drop all
    --security-opt no-new-privileges
    --pids-limit 256
    --restart always
  )
  if [[ -n "$alias" ]]; then
    BASE_RUN_ARGS+=(--network-alias "$alias")
  fi
}

start_apps() {
  local name
  for name in "${app_containers[@]}"; do
    remove_container "$name"
  done

  base_run_args short-term-account account "$state_dir/account.env"
  "$podman" run "${BASE_RUN_ARGS[@]}" "$ACCOUNT_IMAGE" >/dev/null

  base_run_args short-term-marketplace marketplace "$state_dir/marketplace.env"
  "$podman" run "${BASE_RUN_ARGS[@]}" --volume "$media_dir:/var/lib/shortterm/media:rw,z" "$MARKETPLACE_IMAGE" >/dev/null

  base_run_args short-term-messaging messaging "$state_dir/messaging.env"
  "$podman" run "${BASE_RUN_ARGS[@]}" "$MESSAGING_IMAGE" >/dev/null

  base_run_args short-term-favorite favorite "$state_dir/favorite.env"
  "$podman" run "${BASE_RUN_ARGS[@]}" "$FAVORITE_IMAGE" >/dev/null

  base_run_args short-term-marketplace-worker '' "$state_dir/marketplace.env"
  "$podman" run "${BASE_RUN_ARGS[@]}" --volume "$media_dir:/var/lib/shortterm/media:rw,z" \
    --entrypoint /usr/local/bin/worker "$MARKETPLACE_IMAGE" >/dev/null

  base_run_args short-term-messaging-worker '' "$state_dir/messaging.env"
  "$podman" run "${BASE_RUN_ARGS[@]}" --entrypoint /usr/local/bin/worker "$MESSAGING_IMAGE" >/dev/null

  base_run_args short-term-gateway gateway "$state_dir/gateway.env"
  "$podman" run "${BASE_RUN_ARGS[@]}" \
    --publish "127.0.0.1:$gateway_port:8080" \
    --publish "127.0.0.1:$gateway_management_port:9090" \
    --volume "$media_dir:/var/lib/shortterm/media:ro,z" \
    "$GATEWAY_IMAGE" >/dev/null
}

wait_ready() {
  local attempt
  for attempt in $(seq 1 60); do
    if "$curl_bin" --fail --silent --show-error --max-time 3 \
      "http://127.0.0.1:$gateway_port/readyz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  for name in short-term-account short-term-marketplace short-term-messaging short-term-favorite short-term-gateway; do
    echo "--- $name" >&2
    "$podman" logs --tail 80 "$name" >&2 || true
  done
  return 1
}

up_stack() {
  load_release
  ensure_secrets
  write_runtime_envs
  ensure_network_and_storage
  start_postgres
  provision_databases
  run_migrations
  start_apps
  wait_ready || die "Gateway readiness did not become healthy"
}

down_stack() {
  local name
  for name in "${app_containers[@]}"; do
    remove_container "$name"
  done
  remove_container "$postgres_container"
}

write_release_file() {
  local source=$1 target=$2
  local temporary
  temporary=$(mktemp "$state_dir/backend-release.env.XXXXXX")
  install -m 0644 "$source" "$temporary"
  mv "$temporary" "$target"
}

load_image_bundle() {
  local release_dir=$1
  [[ "$release_dir" =~ ^/opt/short-term/releases/[0-9a-f]{40}$ ]] || die "unsafe release directory"
  [[ -d "$release_dir" && ! -L "$release_dir" ]] || die "release directory is missing or unsafe"
  [[ -f "$release_dir/backend-images.tar.gz" && ! -L "$release_dir/backend-images.tar.gz" ]] || die "image archive is missing"
  [[ -f "$release_dir/backend-images.tar.gz.sha256" && ! -L "$release_dir/backend-images.tar.gz.sha256" ]] || die "image checksum is missing"
  [[ -f "$release_dir/manifest.env" && ! -L "$release_dir/manifest.env" ]] || die "release manifest is missing"

  (cd "$release_dir" && sha256sum -c backend-images.tar.gz.sha256)
  gzip -dc "$release_dir/backend-images.tar.gz" | "$podman" load
  load_release "$release_dir/manifest.env"

  local image
  for image in "$ACCOUNT_IMAGE" "$MARKETPLACE_IMAGE" "$MESSAGING_IMAGE" "$FAVORITE_IMAGE" "$GATEWAY_IMAGE" "$POSTGRES_IMAGE"; do
    "$podman" image exists "$image" || die "loaded bundle does not contain $image"
  done
}

rollback_release() {
  if [[ -f "$previous_release_file" ]]; then
    echo "new release failed readiness; restoring previous application images" >&2
    cp "$previous_release_file" "$release_file"
    user_systemctl restart short-term-backend.service
    wait_ready
  else
    echo "initial release failed; no prior image release is available" >&2
    rm -f "$release_file"
    user_systemctl stop short-term-backend.service || true
    return 1
  fi
}

deploy_release() {
  local release_dir=$1
  require_linger
  load_image_bundle "$release_dir"
  ensure_secrets
  write_runtime_envs

  if [[ -f "$release_file" ]]; then
    cp "$release_file" "$previous_release_file"
  else
    rm -f "$previous_release_file"
  fi
  write_release_file "$release_dir/manifest.env" "$release_file"

  user_systemctl daemon-reload
  user_systemctl enable short-term-backend.service >/dev/null
  if ! user_systemctl restart short-term-backend.service; then
    rollback_release || true
    return 1
  fi
  if ! wait_ready; then
    rollback_release || true
    return 1
  fi
}

manual_rollback() {
  if [[ ! -f "$previous_release_file" ]]; then
    echo "initial release has no previous version; stopping the failed application stack" >&2
    rm -f "$release_file"
    user_systemctl stop short-term-backend.service || true
    return 0
  fi
  local failed_release
  failed_release=$(mktemp "$state_dir/backend-release.failed.env.XXXXXX")
  cp "$release_file" "$failed_release"
  cp "$previous_release_file" "$release_file"
  cp "$failed_release" "$previous_release_file"
  rm -f "$failed_release"
  user_systemctl restart short-term-backend.service
  wait_ready || die "rolled-back release did not become ready"
}

show_status() {
  load_release
  "$podman" ps --filter name=short-term --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
  "$curl_bin" --fail --silent --show-error "http://127.0.0.1:$gateway_port/healthz"
  echo
  "$curl_bin" --fail --silent --show-error "http://127.0.0.1:$gateway_port/readyz"
  echo
  echo "marketplace_outbox_pending=$(outbox_pending marketplace_db)"
  echo "messaging_outbox_pending=$(outbox_pending messaging_db)"
}

sql_count() {
  local database=$1 query=$2
  sql_identifier_allowed "$database" || die "unsafe database name"
  "$podman" exec "$postgres_container" psql -U postgres -d "$database" -Atqc "$query"
}

show_metrics() {
  load_release

  echo '# HELP shortterm_outbox_pending Unpublished events currently waiting in the transactional Outbox.'
  echo '# TYPE shortterm_outbox_pending gauge'
  printf 'shortterm_outbox_pending{service="marketplace"} %s\n' "$(outbox_pending marketplace_db)"
  printf 'shortterm_outbox_pending{service="messaging"} %s\n' "$(outbox_pending messaging_db)"

  echo '# HELP shortterm_outbox_retrying Unpublished events with at least one failed publication attempt.'
  echo '# TYPE shortterm_outbox_retrying gauge'
  printf 'shortterm_outbox_retrying{service="marketplace"} %s\n' \
    "$(sql_count marketplace_db 'SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND attempts > 0')"
  printf 'shortterm_outbox_retrying{service="messaging"} %s\n' \
    "$(sql_count messaging_db 'SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND attempts > 0')"

  echo '# HELP shortterm_trades Current trades grouped by state.'
  echo '# TYPE shortterm_trades gauge'
  local status transition event_type
  for status in PENDING ACCEPTED COMPLETED CANCELLED; do
    printf 'shortterm_trades{status="%s"} %s\n' "$status" \
      "$(sql_count marketplace_db "SELECT count(*) FROM trades WHERE status = '$status'")"
  done

  echo '# HELP shortterm_trade_transitions_total Durable trade transition facts recorded in the Outbox.'
  echo '# TYPE shortterm_trade_transitions_total counter'
  for transition in created accepted cancelled completed; do
    event_type="marketplace.trade.$transition"
    printf 'shortterm_trade_transitions_total{transition="%s"} %s\n' "$transition" \
      "$(sql_count marketplace_db "SELECT count(*) FROM outbox_events WHERE event_type = '$event_type'")"
  done

  "$curl_bin" --fail --silent --show-error "http://127.0.0.1:$gateway_management_port/metrics"
}

outbox_pending() {
  local database=$1
  sql_identifier_allowed "$database" || die "unsafe database name"
  "$podman" exec "$postgres_container" psql -U postgres -d "$database" -Atqc \
    'SELECT count(*) FROM outbox_events WHERE published_at IS NULL'
}

verify_outbox() {
  local attempt marketplace_pending messaging_pending
  for attempt in $(seq 1 30); do
    marketplace_pending=$(outbox_pending marketplace_db)
    messaging_pending=$(outbox_pending messaging_db)
    if [[ "$marketplace_pending" == 0 && "$messaging_pending" == 0 ]]; then
      echo "outbox_pending marketplace=0 messaging=0"
      return 0
    fi
    sleep 1
  done
  die "outbox remains pending: marketplace=$marketplace_pending messaging=$messaging_pending"
}

trace_id_for_request() {
  local container=$1 request_id=$2 line
  line=$("$podman" logs --since 15m "$container" 2>&1 | grep -F "\"request_id\":\"$request_id\"" | tail -n 1 || true)
  [[ -n "$line" ]] || return 1
  printf '%s\n' "$line" | "$python_bin" -c 'import json,sys; print(json.load(sys.stdin).get("trace_id", ""))'
}

verify_trace() {
  local request_id=$1
  [[ "$request_id" =~ ^[0-9A-Za-z._-]{1,128}$ ]] || die "invalid request ID"
  local expected_trace='' container current_trace attempt worker_seen=false

  for container in short-term-gateway short-term-marketplace short-term-messaging short-term-account; do
    current_trace=''
    for attempt in $(seq 1 20); do
      current_trace=$(trace_id_for_request "$container" "$request_id" || true)
      [[ -n "$current_trace" ]] && break
      sleep 1
    done
    [[ "$current_trace" =~ ^[0-9a-f]{32}$ ]] || die "$container has no trace for request $request_id"
    if [[ -z "$expected_trace" ]]; then
      expected_trace=$current_trace
    elif [[ "$current_trace" != "$expected_trace" ]]; then
      die "trace IDs differ between containers for request $request_id"
    fi
  done

  for attempt in $(seq 1 20); do
    if "$podman" logs --since 15m short-term-marketplace-worker 2>&1 | \
      grep -F "\"trace_id\":\"$expected_trace\"" >/dev/null; then
      worker_seen=true
      break
    fi
    sleep 1
  done
  [[ "$worker_seen" == true ]] || die "Marketplace Outbox did not preserve trace $expected_trace"
  echo "trace_propagation request_id=$request_id trace_id=$expected_trace services=4 outbox=ok"
}

main() {
  require_deploy_user
  case "${1:-}" in
    deploy)
      [[ $# -eq 2 ]] || die "usage: $0 deploy /opt/short-term/releases/<sha>"
      deploy_release "$2"
      ;;
    up)
      [[ $# -eq 1 ]] || die "usage: $0 up"
      up_stack
      ;;
    down)
      [[ $# -eq 1 ]] || die "usage: $0 down"
      down_stack
      ;;
    status)
      [[ $# -eq 1 ]] || die "usage: $0 status"
      show_status
      ;;
    verify-outbox)
      [[ $# -eq 1 ]] || die "usage: $0 verify-outbox"
      verify_outbox
      ;;
    verify-trace)
      [[ $# -eq 2 ]] || die "usage: $0 verify-trace <request-id>"
      verify_trace "$2"
      ;;
    metrics)
      [[ $# -eq 1 ]] || die "usage: $0 metrics"
      show_metrics
      ;;
    rollback)
      [[ $# -eq 1 ]] || die "usage: $0 rollback"
      manual_rollback
      ;;
    *)
      die "usage: $0 {deploy <release-dir>|up|down|status|metrics|verify-outbox|verify-trace <request-id>|rollback}"
      ;;
  esac
}

main "$@"
