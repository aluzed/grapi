#!/bin/sh
# Runs a command with GRAPI_TEST_S3_* pointing at a throwaway S3 compatible
# store on the homelab (SeaweedFS: MinIO no longer publishes its community
# images), reached through an ssh tunnel. Nothing runs in Docker locally.
# usage: scripts/with-homelab-s3.sh go test -tags integration -run S3 .
set -eu
host=${COMPAT_HOST:-homelab}
name="grapi-test-s3-$$"
image=chrislusf/seaweedfs:3.80
key=grapi-key
secret=grapi-secret-0123456789

cleanup() {
  [ -n "${tunnel:-}" ] && kill "$tunnel" 2>/dev/null || true
  ssh "$host" "docker rm -f $name >/dev/null 2>&1; rm -rf /tmp/$name" || true
}
trap cleanup EXIT INT TERM

ssh "$host" "mkdir -p /tmp/$name && cat >/tmp/$name/s3.json" <<JSON
{"identities": [
  {"name": "grapi", "credentials": [{"accessKey": "$key", "secretKey": "$secret"}], "actions": ["Admin", "Read", "List", "Tagging", "Write"]},
  {"name": "anonymous", "actions": ["Read"]}
]}
JSON
ssh "$host" "docker run -d --name $name -p 127.0.0.1::8333 -v /tmp/$name/s3.json:/etc/s3.json:ro $image server -dir=/data -s3 -s3.port=8333 -s3.config=/etc/s3.json >/dev/null"
remote_port=$(ssh "$host" "docker port $name 8333/tcp | head -1 | sed 's/.*://'")
ssh "$host" "i=0; until curl -s -o /dev/null --aws-sigv4 'aws:amz:us-east-1:s3' --user '$key:$secret' -X PUT http://127.0.0.1:$remote_port/grapi; do i=\$((i+1)); [ \$i -gt 60 ] && exit 1; sleep 1; done; sleep 2; curl -s -o /dev/null -w '%{http_code}' --aws-sigv4 'aws:amz:us-east-1:s3' --user '$key:$secret' -X PUT http://127.0.0.1:$remote_port/grapi" >/dev/null
local_port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
ssh -N -o ControlMaster=no -o ControlPath=none -o ExitOnForwardFailure=yes -L "$local_port:127.0.0.1:$remote_port" "$host" &
tunnel=$!
i=0
until nc -z 127.0.0.1 "$local_port" 2>/dev/null; do
  i=$((i + 1))
  [ $i -gt 30 ] && { echo "error: tunnel did not open" >&2; exit 1; }
  sleep 1
done
GRAPI_TEST_S3_ENDPOINT="http://127.0.0.1:$local_port" GRAPI_TEST_S3_KEY=$key GRAPI_TEST_S3_SECRET=$secret "$@"
