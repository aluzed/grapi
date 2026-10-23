#!/bin/sh
# Runs the compat bench on the homelab: nothing is started on this machine.
# usage: bench.sh <fixture> [self|roundtrip|grapi|toggle] [sqlite|postgres]
# roundtrip: grapi's schema sync must find nothing to do on a database Strapi
# created, and Strapi on a database grapi created must answer the corpus like
# Strapi on its own.
set -eu
fixture=${1:?fixture}
mode=${2:-self}
db=${3:-sqlite}
host=${COMPAT_HOST:-homelab}
root=$(cd "$(dirname "$0")/../.." && pwd)

cd "$root"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o compat/bin/compat ./compat/cmd/compat
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o compat/bin/grapi ./cmd/grapi
# A fixture with a framework/ main (its Go hooks) is served by that build.
fx=${fixture%%--*}
if [ -d "compat/fixtures/$fx/framework" ]; then
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "compat/bin/grapi-$fx" "./compat/fixtures/$fx/framework"
fi
ssh "$host" 'mkdir -p ~/grapi-compat'
rsync -a --delete --exclude report/ --exclude golden/ compat/ "$host:grapi-compat/compat/"
status=0
ssh "$host" "~/grapi-compat/compat/scripts/remote.sh $fixture $mode $db" || status=$?
mkdir -p compat/report
rsync -a "$host:grapi-compat/compat/report/" compat/report/
rsync -a "$host:grapi-compat/compat/golden/" compat/golden/
exit $status
