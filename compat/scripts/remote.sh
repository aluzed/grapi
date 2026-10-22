#!/bin/sh
# Homelab side of bench.sh. Every container, network and data dir it creates
# is removed on exit, whatever happens.
set -eu
fixture=$1
# "relations--write" runs querycases/relations--write.yaml on fixtures/relations.
fx=${fixture%%--*}
mode=$2
db=$3
here=$(cd "$(dirname "$0")/.." && pwd)
. "$here/reference/versions.env"
image="grapi-compat-strapi:$STRAPI_VERSION-$(cd "$here/reference" && find . -type f | sort | xargs sha256sum | sha256sum | cut -c1-12)"
run_id="compat-$fixture-$$"
work=$(mktemp -d "/tmp/$run_id.XXXX")

cleanup() {
  [ -n "${grapi_pid:-}" ] && kill "$grapi_pid" 2>/dev/null || true
  docker ps -aq --filter "label=grapi-compat=$run_id" | xargs -r docker rm -f >/dev/null
  docker network rm "$run_id" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

if ! docker image inspect "$image" >/dev/null 2>&1; then
  docker build -q -t "$image" "$here/reference" >/dev/null
fi
docker network create "$run_id" >/dev/null

# start_db <name>: the database of one instance (a data dir, plus a Postgres
# container with a port picked by Docker when db=postgres).
start_db() {
  name=$1
  mkdir -p "$work/$name"
  chmod 777 "$work/$name"
  if [ "$db" = postgres ]; then
    docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-$name-pg" -p 127.0.0.1::5432 \
      -e POSTGRES_USER=strapi -e POSTGRES_PASSWORD=strapi -e POSTGRES_DB=strapi "$POSTGRES_IMAGE" >/dev/null
    # Over TCP: during initdb the image runs a socket-only server, then restarts.
    until docker exec "$run_id-$name-pg" pg_isready -h 127.0.0.1 -U strapi >/dev/null 2>&1; do sleep 1; done
  fi
}

# start_strapi <name>: the Strapi reference instance on that database, on a
# port picked by Docker so that several benches can run at once.
start_strapi() {
  name=$1
  set -- -e DATABASE_CLIENT=sqlite
  if [ "$db" = postgres ]; then
    set -- -e DATABASE_CLIENT=postgres -e DATABASE_HOST="$run_id-$name-pg" -e DATABASE_NAME=strapi \
      -e DATABASE_USERNAME=strapi -e DATABASE_PASSWORD=strapi
  fi
  docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-$name" \
    -p 127.0.0.1::1337 "$@" \
    -v "$here/fixtures/$fx:/fixture:ro" -v "$work/$name:/data" "$image" >/dev/null
}

start_ref() {
  start_db "$1"
  start_strapi "$1"
}

# grapi_sync <name>: grapi's schema sync on that instance's database.
grapi_sync() {
  if [ "$db" = postgres ]; then
    pg_port=$(docker port "$run_id-$1-pg" 5432/tcp | head -1 | sed 's/.*://')
    "$here/bin/compat" schema-sync -fixture "$here/fixtures/$fx" -client postgres \
      -db "postgres://strapi:strapi@127.0.0.1:$pg_port/strapi?sslmode=disable"
  else
    "$here/bin/compat" schema-sync -fixture "$here/fixtures/$fx" -client sqlite -db "$work/$1/data.db"
  fi
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
roundtrip)
  # a: Strapi creates the database, grapi's sync must find nothing to do.
  start_ref a
  wait_up a
  docker stop "$run_id-a" >/dev/null
  status=$(grapi_sync a)
  echo "grapi sync on Strapi's database: $status"
  if [ "$status" != UNCHANGED ]; then
    echo "error: expected UNCHANGED" >&2
    exit 1
  fi
  docker start "$run_id-a" >/dev/null
  # b: grapi creates the database, Strapi runs on it.
  start_db b
  status=$(grapi_sync b)
  echo "grapi sync on an empty database: $status"
  if [ "$status" != CHANGED ]; then
    echo "error: expected CHANGED" >&2
    exit 1
  fi
  start_strapi b
  wait_up a
  wait_up b
  b_url=http://127.0.0.1:$(port_of b)
  b_tokens=$work/b/tokens.json
  ;;
query)
  # Strapi seeds the database and runs the cases through db.query(uid), then
  # grapi's query engine runs them on the same database.
  start_db a
  cases=$work/cases.json
  "$here/bin/compat" query-cases -in "$here/querycases/$fixture.yaml" -out "$cases"
  set -- -e DATABASE_CLIENT=sqlite
  grapi_db="$work/a/data.db"
  if [ "$db" = postgres ]; then
    set -- -e DATABASE_CLIENT=postgres -e DATABASE_HOST="$run_id-a-pg" -e DATABASE_NAME=strapi \
      -e DATABASE_USERNAME=strapi -e DATABASE_PASSWORD=strapi
    grapi_db="postgres://strapi:strapi@127.0.0.1:$(docker port "$run_id-a-pg" 5432/tcp | head -1 | sed 's/.*://')/strapi?sslmode=disable"
  fi
  cp "$cases" "$work/a/cases.json"
  docker run --rm --label "grapi-compat=$run_id" --network "$run_id" "$@" \
    -v "$here/fixtures/$fx:/fixture:ro" -v "$work/a:/data" -v "$here/tools:/tools:ro" \
    --entrypoint sh "$image" -c 'rm -rf /app/src && cp -r /fixture/src /app/src && cp /app/bootstrap/index.js /app/src/index.js && node /app/bootstrap/scaffold.js && node /tools/run-queries.js /data/cases.json /data/strapi.json'
  set --
  if grep -q '"isolated": true' "$cases"; then
    # grapi writes: it gets a database of its own and plays the seed itself.
    start_db b
    grapi_sync b >/dev/null
    grapi_db="$work/b/data.db"
    if [ "$db" = postgres ]; then
      grapi_db="postgres://strapi:strapi@127.0.0.1:$(docker port "$run_id-b-pg" 5432/tcp | head -1 | sed 's/.*://')/strapi?sslmode=disable"
    fi
    set -- -seed
    volatile=-volatile
  fi
  "$here/bin/compat" query -fixture "$here/fixtures/$fx" -client "$db" -db "$grapi_db" -cases "$cases" -out "$work/grapi.json" "$@"
  mkdir -p "$here/report"
  report="$here/report/$(date +%Y-%m-%d)-$fixture-query-$db.md"
  "$here/bin/compat" query-diff ${volatile:-} -a "$work/a/strapi.json" -b "$work/grapi.json" -report "$report" \
    -title "Query compat $fixture ($db, Strapi $STRAPI_VERSION)"
  exit $?
  ;;
grapi)
  # b: Strapi creates the database (tokens, permissions), then grapi serves it.
  start_ref a
  start_ref b
  wait_up a
  wait_up b
  docker stop "$run_id-b" >/dev/null
  grapi_port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
  if [ "$db" = postgres ]; then
    pg_port=$(docker port "$run_id-b-pg" 5432/tcp | head -1 | sed 's/.*://')
    set -- DATABASE_CLIENT=postgres DATABASE_HOST=127.0.0.1 DATABASE_PORT="$pg_port" DATABASE_NAME=strapi \
      DATABASE_USERNAME=strapi DATABASE_PASSWORD=strapi
  else
    set -- DATABASE_CLIENT=sqlite DATABASE_FILENAME="$work/b/data.db"
  fi
  env "$@" HOST=127.0.0.1 PORT="$grapi_port" GRAPI_LOG_LEVEL=debug \
    "$here/bin/grapi" start -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" >"$work/grapi.log" 2>&1 &
  grapi_pid=$!
  i=0
  until [ "$(curl -s -o /dev/null -w '%{http_code}' "127.0.0.1:$grapi_port/_health")" = 204 ]; do
    i=$((i + 1))
    if [ $i -gt 60 ] || ! kill -0 "$grapi_pid" 2>/dev/null; then
      tail -30 "$work/grapi.log"
      echo "error: grapi did not start" >&2
      exit 1
    fi
    sleep 1
  done
  b_url=http://127.0.0.1:$grapi_port
  b_tokens=$work/b/tokens.json
  ;;
*)
  echo "error: unknown mode $mode" >&2
  exit 2
  ;;
esac

"$here/bin/compat" run -corpus "$here/corpus" -fixture "$fixture" -url "http://127.0.0.1:$(port_of a)" -tokens "$work/a/tokens.json" -out "$work/out/a"
"$here/bin/compat" run -corpus "$here/corpus" -fixture "$fixture" -url "$b_url" -tokens "$b_tokens" -out "$work/out/b"
if [ "$mode" = grapi ]; then
  mkdir -p "$here/report"
  cp "$work/grapi.log" "$here/report/$(date +%F)-$fixture-grapi-$db.log"
fi
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
if [ "$mode" = self ]; then
  golden="$here/golden/$fixture-$db"
  rm -rf "$golden"
  mkdir -p "$golden"
  cp -r "$work/out/a/." "$golden/"
  rm -rf "$golden/raw"
  dump_schema a "$golden/schema"
fi
report="$here/report/$(date +%Y-%m-%d)-$fixture-$mode-$db.md"
status=0
"$here/bin/compat" diff -a "$work/out/a" -b "$work/out/b" -report "$report" \
  -title "Compat $fixture ($mode, $db, Strapi $STRAPI_VERSION)" || status=$?
cat "$report" | head -20
exit $status
