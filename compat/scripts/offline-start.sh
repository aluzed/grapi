#!/bin/sh
# Starts grapi on a fixture in a container without any network (homelab),
# to check that the default configuration opens no outgoing connection.
# usage: offline-start.sh [fixture]
set -eu
fixture=${1:-devfreelance}
host=${COMPAT_HOST:-homelab}
root=$(cd "$(dirname "$0")/../.." && pwd)
image=grapi-compat-strapi:5.59.0-0cf2404115c9
cd "$root"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o compat/bin/grapi ./cmd/grapi
rsync -a --delete --exclude report/ --exclude golden/ compat/ "$host:grapi-compat/compat/"
ssh "$host" "docker run --rm --network none -v ~/grapi-compat/compat/bin/grapi:/grapi:ro -v ~/grapi-compat/compat/fixtures/$fixture:/fx:ro --entrypoint sh $image -c '
P=/tmp/p; mkdir -p \$P && cp -r /fx/src \$P/src && cd \$P
(DATABASE_CLIENT=sqlite DATABASE_FILENAME=\$P/data.db JWT_SECRET=x PORT=1440 GRAPI_LOG_LEVEL=debug /grapi start -dir \$P > \$P/log 2>&1 &)
sleep 4
node -e \"fetch(\\\"http://127.0.0.1:1440/_health\\\").then(r => { console.log(\\\"health\\\", r.status); process.exit(r.status === 204 ? 0 : 1) })\"
if grep -qiE \"dial|lookup|network is unreachable|level=ERROR\" \$P/log; then cat \$P/log; exit 1; fi
echo \"no outgoing connection attempted\"
'"
