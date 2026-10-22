#!/bin/sh
# Runs a command with GRAPI_TEST_PG_DSN pointing at a throwaway Postgres on the
# homelab, reached through an ssh tunnel. Nothing runs in Docker locally.
# usage: scripts/with-homelab-postgres.sh go test -tags integration ./...
set -eu
host=${COMPAT_HOST:-homelab}
name="grapi-test-pg-$$"
image=$(sed -n 's/^POSTGRES_IMAGE=//p' "$(dirname "$0")/../compat/reference/versions.env")

cleanup() {
  [ -n "${tunnel:-}" ] && kill "$tunnel" 2>/dev/null || true
  ssh "$host" "docker rm -f $name >/dev/null 2>&1" || true
}
trap cleanup EXIT INT TERM

ssh "$host" "docker run -d --name $name -p 127.0.0.1::5432 -e POSTGRES_USER=grapi -e POSTGRES_PASSWORD=grapi -e POSTGRES_DB=grapi $image >/dev/null && until docker exec $name pg_isready -U grapi >/dev/null 2>&1; do sleep 1; done"
remote_port=$(ssh "$host" "docker port $name 5432/tcp | head -1 | sed 's/.*://'")
local_port=${GRAPI_TEST_PG_PORT:-$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')}
# Own connection: a forward added to a shared ControlMaster outlives this script.
ssh -N -o ControlMaster=no -o ControlPath=none -o ExitOnForwardFailure=yes -L "$local_port:127.0.0.1:$remote_port" "$host" &
tunnel=$!
i=0
until nc -z 127.0.0.1 "$local_port" 2>/dev/null; do
  i=$((i + 1))
  [ $i -gt 30 ] && { echo "error: tunnel did not open" >&2; exit 1; }
  sleep 1
done
# The server reports ready before it accepts external connections for a moment.
sleep 2
GRAPI_TEST_PG_DSN="postgres://grapi:grapi@127.0.0.1:$local_port/grapi?sslmode=disable" "$@"
