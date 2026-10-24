#!/bin/sh
# Runs the compat bench on the homelab: nothing is started on this machine.
# usage: bench.sh <fixture> [self|roundtrip|grapi|toggle] [sqlite|postgres]
# roundtrip: grapi's schema sync must find nothing to do on a database Strapi
# created, and Strapi on a database grapi created must answer the corpus like
# Strapi on its own.
# A private fixture (a real project's schemas and rules) lives outside this
# repository, in $GRAPI_PRIVATE_COMPAT laid out as compat/ (fixtures/,
# corpus/, querycases/, golden/, hooks/); its goldens are written back there.
set -eu
fixture=${1:?fixture}
mode=${2:-self}
db=${3:-sqlite}
host=${COMPAT_HOST:-homelab}
root=$(cd "$(dirname "$0")/../.." && pwd)
private=${GRAPI_PRIVATE_COMPAT:-}

cd "$root"
fx=${fixture%%--*}
gowork=
if [ -n "$private" ]; then
  private=$(cd "$private" && pwd)
  # The private module (its go.mod is the parent of compat/) and grapi build together.
  gowork=$(mktemp -d)/go.work
  (cd "$(dirname "$gowork")" && go work init "$root" "$(dirname "$private")")
  for f in "$private"/hooks/zz_private_*.go; do
    [ -e "$f" ] && cp "$f" compat/cmd/compat/
  done
  trap 'rm -f compat/cmd/compat/zz_private_*.go' EXIT
fi
GOWORK=${gowork:-off} GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o compat/bin/compat ./compat/cmd/compat
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o compat/bin/grapi ./cmd/grapi
# A fixture with a framework/ main (its Go hooks) is served by that build.
if [ -d "compat/fixtures/$fx/framework" ]; then
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "compat/bin/grapi-$fx" "./compat/fixtures/$fx/framework"
elif [ -n "$private" ] && [ -d "$private/fixtures/$fx/framework" ]; then
  (cd "$private/fixtures/$fx/framework" && GOWORK=$gowork GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$root/compat/bin/grapi-$fx" .)
fi
ssh "$host" 'mkdir -p ~/grapi-compat'
rsync -a --delete --exclude report/ --exclude golden/ compat/ "$host:grapi-compat/compat/"
if [ -n "$private" ]; then
  for d in fixtures corpus querycases; do
    [ -d "$private/$d" ] && rsync -a "$private/$d/" "$host:grapi-compat/compat/$d/"
  done
fi
status=0
ssh "$host" "~/grapi-compat/compat/scripts/remote.sh $fixture $mode $db" || status=$?
mkdir -p compat/report
rsync -a "$host:grapi-compat/compat/report/" compat/report/
# Only the goldens of the fixture just run, and only a public one: the
# homelab keeps the private ones, which never land in this repository.
if [ -d "compat/fixtures/$fx" ]; then
  rsync -a --include "$fx-*/***" --exclude '*' "$host:grapi-compat/compat/golden/" compat/golden/
fi
if [ -n "$private" ] && [ -d "$private/fixtures/$fx" ]; then
  mkdir -p "$private/golden"
  rsync -a --include "$fx-*/***" --exclude '*' "$host:grapi-compat/compat/golden/" "$private/golden/"
fi
exit $status
