#!/bin/sh
# Homelab side of bench.sh. Every container, network and data dir it creates
# is removed on exit, whatever happens.
set -eu
fixture=$1
mode=$2
db=$3
here=$(cd "$(dirname "$0")/.." && pwd)
. "$here/reference/versions.env"
image="grapi-compat-strapi:$STRAPI_VERSION-$(cd "$here/reference" && find . -type f | sort | xargs sha256sum | sha256sum | cut -c1-12)"
run_id="compat-$fixture-$$"
work=$(mktemp -d "/tmp/$run_id.XXXX")

cleanup() {
  docker ps -aq --filter "label=grapi-compat=$run_id" | xargs -r docker rm -f >/dev/null
  docker network rm "$run_id" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

if ! docker image inspect "$image" >/dev/null 2>&1; then
  docker build -q -t "$image" "$here/reference" >/dev/null
fi
docker network create "$run_id" >/dev/null

# start_ref <name>: one Strapi reference instance with a fresh database, on a
# port picked by Docker so that several benches can run at once.
start_ref() {
  name=$1
  mkdir -p "$work/$name"
  chmod 777 "$work/$name"
  set -- -e DATABASE_CLIENT=sqlite
  if [ "$db" = postgres ]; then
    docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-$name-pg" \
      -e POSTGRES_USER=strapi -e POSTGRES_PASSWORD=strapi -e POSTGRES_DB=strapi "$POSTGRES_IMAGE" >/dev/null
    until docker exec "$run_id-$name-pg" pg_isready -U strapi >/dev/null 2>&1; do sleep 1; done
    set -- -e DATABASE_CLIENT=postgres -e DATABASE_HOST="$run_id-$name-pg" -e DATABASE_NAME=strapi \
      -e DATABASE_USERNAME=strapi -e DATABASE_PASSWORD=strapi
  fi
  docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-$name" \
    -p 127.0.0.1::1337 "$@" \
    -v "$here/fixtures/$fixture:/fixture:ro" -v "$work/$name:/data" "$image" >/dev/null
}

port_of() {
  docker port "$run_id-$1" 1337/tcp | head -1 | sed 's/.*://'
}

wait_up() {
  name=$1
  port=$(port_of "$name")
  i=0
  until [ "$(curl -s -o /dev/null -w '%{http_code}' "127.0.0.1:$port/_health")" = 204 ]; do
    i=$((i + 1))
    if [ $i -gt 120 ]; then
      docker logs "$run_id-$name" 2>&1 | tail -30
      echo "error: $name did not start" >&2
      exit 1
    fi
    sleep 1
  done
}

case "$mode" in
self)
  start_ref a
  start_ref b
  wait_up a
  wait_up b
  b_url=http://127.0.0.1:$(port_of b)
  b_tokens=$work/b/tokens.json
  ;;
grapi)
  echo "error: grapi mode needs the grapi image (not built yet)" >&2
  exit 2
  ;;
*)
  echo "error: unknown mode $mode" >&2
  exit 2
  ;;
esac

"$here/bin/compat" run -corpus "$here/corpus" -fixture "$fixture" -url "http://127.0.0.1:$(port_of a)" -tokens "$work/a/tokens.json" -out "$work/out/a"
"$here/bin/compat" run -corpus "$here/corpus" -fixture "$fixture" -url "$b_url" -tokens "$b_tokens" -out "$work/out/b"
# dump_schema <name> <dir>: the schema Strapi computed (strapi_database_schema)
# and the physical one, as reference for grapi's metadata and DDL.
dump_schema() {
  name=$1
  dir=$2
  mkdir -p "$dir"
  if [ "$db" = postgres ]; then
    psql="docker exec $run_id-$name-pg psql -U strapi -d strapi -Atq -c"
    $psql "select schema from strapi_database_schema" >"$dir/database-schema.json"
    $psql "select coalesce(json_agg(c order by c.table_name, c.ordinal_position), '[]') from (select table_name, column_name, ordinal_position, data_type, is_nullable, column_default, character_maximum_length, numeric_precision, numeric_scale from information_schema.columns where table_schema = 'public') c" >"$dir/physical-columns.json"
    $psql "select coalesce(json_agg(i order by i.tablename, i.indexname), '[]') from (select tablename, indexname, indexdef from pg_indexes where schemaname = 'public') i" >"$dir/physical-indexes.json"
    $psql "select coalesce(json_agg(f order by f.table_name, f.constraint_name), '[]') from (select tc.table_name, tc.constraint_name, tc.constraint_type, pg_get_constraintdef(pc.oid) as definition from information_schema.table_constraints tc join pg_constraint pc on pc.conname = tc.constraint_name where tc.table_schema = 'public' and tc.constraint_type in ('FOREIGN KEY', 'UNIQUE', 'PRIMARY KEY')) f" >"$dir/physical-constraints.json"
  else
    docker exec -w /app "$run_id-$name" node -e "
      const D = require('better-sqlite3');
      const db = new D('/data/data.db', { readonly: true });
      const fs = require('fs');
      process.stdout.write(db.prepare('select schema from strapi_database_schema').get().schema);
    " >"$dir/database-schema.json"
    docker exec -w /app "$run_id-$name" node -e "
      const D = require('better-sqlite3');
      const db = new D('/data/data.db', { readonly: true });
      process.stdout.write(JSON.stringify(db.prepare(\"select type, name, tbl_name, sql from sqlite_master where name not like 'sqlite_%' order by tbl_name, type, name\").all(), null, 1));
    " >"$dir/physical-sqlite-master.json"
  fi
  python3 -m json.tool "$dir/database-schema.json" >"$dir/database-schema.tmp" && mv "$dir/database-schema.tmp" "$dir/database-schema.json"
}

mkdir -p "$here/report"
golden="$here/golden/$fixture-$db"
rm -rf "$golden"
mkdir -p "$golden"
cp -r "$work/out/a/." "$golden/"
rm -rf "$golden/raw"
dump_schema a "$golden/schema"
report="$here/report/$(date +%Y-%m-%d)-$fixture-$mode-$db.md"
status=0
"$here/bin/compat" diff -a "$work/out/a" -b "$work/out/b" -report "$report" \
  -title "Compat $fixture ($mode, $db, Strapi $STRAPI_VERSION)" || status=$?
cat "$report" | head -20
exit $status
