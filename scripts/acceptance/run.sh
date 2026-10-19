#!/usr/bin/env bash
# Runs the acceptance tests against a throwaway Spotlight control plane.
#
#   scripts/acceptance/run.sh              start it, run make testacc, remove it
#   eval "$(scripts/acceptance/run.sh up)" start it and export its SPOTLIGHT_* variables
#   scripts/acceptance/run.sh down         remove it
#
#   SPOTLIGHT_VERSION   the control-plane image tag: canary (the latest staging build) if not set, or a Spotlight release
#   SPOTLIGHT_ACC_PORT  the local port of the control plane, 3300 if not set
set -euo pipefail

: "${SPOTLIGHT_VERSION:=canary}"
: "${SPOTLIGHT_ACC_PORT:=3300}"
: "${COMPOSE_PROJECT_NAME:=spotlight-acc}"
export SPOTLIGHT_VERSION SPOTLIGHT_ACC_PORT COMPOSE_PROJECT_NAME

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
url="http://localhost:${SPOTLIGHT_ACC_PORT}"

compose() {
  docker compose -f "${here}/compose.yml" "$@"
}

log() {
  echo "$@" >&2
}

down() {
  BETTER_AUTH_SECRET=- SPOTLIGHT_SECRETS_KEY=- compose down --volumes --remove-orphans >&2
}

random() {
  openssl rand -hex "$1"
}

api() {
  local method=$1 path=$2 body=$3
  shift 3
  curl -sS --fail-with-body -X "$method" "${url}/api${path}" \
    -H 'Content-Type: application/json' -d "$body" "$@"
}

up() {
  BETTER_AUTH_SECRET=$(random 32)
  SPOTLIGHT_SECRETS_KEY=$(openssl rand -base64 32)
  export BETTER_AUTH_SECRET SPOTLIGHT_SECRETS_KEY

  log "Starting Spotlight ${SPOTLIGHT_VERSION} at ${url}"
  if ! compose up --detach --wait --quiet-pull >&2; then
    compose logs migrate control-plane >&2 || true
    return 1
  fi
  log "Spotlight ${SPOTLIGHT_VERSION} is build $(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' \
    "${SPOTLIGHT_IMAGES:-ghcr.io/usespotlightai}/spotlight-control-plane:${SPOTLIGHT_VERSION}")."

  local user session slug
  user=$(random 16)
  session=$(random 32)
  slug="acc-$(random 4)"

  compose exec -T db psql -q -U postgres -d postgres -v ON_ERROR_STOP=1 >&2 <<SQL
insert into "user" (id, name, email, email_verified)
  values ('${user}', 'Acceptance', 'acceptance-${user}@example.com', true);
insert into session (id, token, user_id, expires_at, updated_at)
  values ('${user}', '${session}', '${user}', now() + interval '1 day', now());
SQL

  local signature cookie
  signature=$(printf '%s' "$session" | openssl dgst -sha256 -hmac "$BETTER_AUTH_SECRET" -binary | base64)
  cookie="better-auth.session_token=${session}.$(printf '%s' "$signature" | sed 's/+/%2B/g; s#/#%2F#g; s/=/%3D/g')"

  local workspace
  workspace=$(api POST /workspaces "{\"name\":\"Acceptance\",\"slug\":\"${slug}\",\"teamName\":\"Acceptance\"}" \
    -H "Cookie: ${cookie}" | jq -er .id)

  local token
  token=$(api POST "/resources/workspaces/${workspace}/tokens" '{"name":"acceptance","scopes":["configure","secrets","runners","trusts"]}' \
    -H "Cookie: ${cookie}" -H "Idempotency-Key: $(random 16)" -o /dev/null -D - |
    tr -d '\r' | sed -n 's/^[Ss]potlight-[Ww]orkspace-[Tt]oken: //p')
  if [ -z "$token" ]; then
    log "The control plane issued no workspace token."
    return 1
  fi

  log "Workspace ${slug} is ready."
  printf 'export SPOTLIGHT_URL=%q\n' "$url"
  printf 'export SPOTLIGHT_API_TOKEN=%q\n' "$token"
  printf 'export SPOTLIGHT_ACC_WORKSPACE=%q\n' "$slug"
}

case "${1:-test}" in
  up)
    up
    ;;
  down)
    down
    ;;
  test)
    trap down EXIT
    exports=$(up)
    eval "$exports"
    make -C "${here}/../.." testacc
    ;;
  *)
    log "usage: $0 [test|up|down]"
    exit 2
    ;;
esac
