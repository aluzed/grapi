#!/bin/sh
# Homelab side of bench.sh. Every container, network and data dir it creates
# is removed on exit, whatever happens.
set -eu
fixture=$1
# "relations--write" runs querycases/relations--write.yaml on fixtures/relations.
fx=${fixture%%--*}
UP_JWT_MANAGEMENT=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("upConfig", {}).get("jwtManagement", "legacy-support"))' "$(cd "$(dirname "$0")/.." && pwd)/fixtures/$fx/compat.json" 2>/dev/null || echo legacy-support)
UP_COOKIE_SECURE=$(python3 -c 'import json,sys; v=json.load(open(sys.argv[1])).get("upConfig", {}).get("sessions", {}).get("cookie", {}).get("secure", False); print(str(v).lower())' "$(cd "$(dirname "$0")/.." && pwd)/fixtures/$fx/compat.json" 2>/dev/null || echo false)
BODY_MAX_FILE_SIZE=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("bodyMaxFileSize", 209715200))' "$(cd "$(dirname "$0")/.." && pwd)/fixtures/$fx/compat.json" 2>/dev/null || echo 209715200)
export UP_JWT_MANAGEMENT UP_COOKIE_SECURE BODY_MAX_FILE_SIZE
mode=$2
db=$3
here=$(cd "$(dirname "$0")/.." && pwd)
# A fixture with Go hooks (framework/, built by bench.sh) is served by its
# own build, and the reference runs the same rules (rules.js). The query mode
# runs grapi's engine in process, without them, so Strapi does not either.
serve_bin=$here/bin/grapi
COMPAT_ROW_RULES=0
if [ -x "$here/bin/grapi-$fx" ] && [ "$mode" != query ]; then
  serve_bin=$here/bin/grapi-$fx
  COMPAT_ROW_RULES=1
fi
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
  # Uploads land on the host, where grapi can serve them later.
  mkdir -p "$work/$name-public/uploads"
  chmod -R 777 "$work/$name-public"
  docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-$name" \
    -p 127.0.0.1::1337 "$@" -e COMPAT_ROW_RULES="$COMPAT_ROW_RULES" \
    -v "$here/fixtures/$fx:/fixture:ro" -v "$work/$name:/data" -v "$work/$name-public/uploads:/app/public/uploads" "$image" >/dev/null
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
toggle)
  # Draft and publish turned around on every api:: type of the fixture:
  # grapi migrates the database Strapi filled, then Strapi restarts on it
  # with the same schemas and must find nothing to replay.
  start_ref a
  wait_up a
  "$here/bin/compat" run -corpus "$here/corpus" -fixture "$fixture" -url "http://127.0.0.1:$(port_of a)" -tokens "$work/a/tokens.json" -out "$work/out/a" >/dev/null || true
  docker stop "$run_id-a" >/dev/null
  tfx=$work/fixture-toggled
  cp -r "$here/fixtures/$fx" "$tfx"
  python3 - "$tfx" <<'PY'
import glob, json, sys
for p in glob.glob(sys.argv[1] + "/src/api/*/content-types/*/schema.json"):
    s = json.load(open(p))
    o = s.setdefault("options", {})
    o["draftAndPublish"] = not o.get("draftAndPublish", False)
    json.dump(s, open(p, "w"), indent=2)
PY
  chmod -R a+rX "$tfx"
  mkdir -p "$work/snap"
  "$here/bin/compat" snapshot -fixture-dir "$tfx" -name "$fixture-snapshot" -out "$work/snap/snapshot.yaml"
  toggle_grapi() {
    if [ "$db" = postgres ]; then
      set -- DATABASE_CLIENT=postgres DATABASE_HOST=127.0.0.1 DATABASE_PORT="$(docker port "$run_id-a-pg" 5432/tcp | head -1 | sed 's/.*://')" \
        DATABASE_NAME=strapi DATABASE_USERNAME=strapi DATABASE_PASSWORD=strapi
    else
      set -- DATABASE_CLIENT=sqlite DATABASE_FILENAME="$work/a/data.db"
    fi
    port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
    env "$@" HOST=127.0.0.1 PORT="$port" "$serve_bin" start -dir "$tfx" -config "$here/reference/grapi-config" >"$work/grapi-toggle.log" 2>&1 &
    pid=$!
    i=0
    until [ "$(curl -s -o /dev/null -w '%{http_code}' "127.0.0.1:$port/_health")" = 204 ]; do
      i=$((i + 1))
      if [ $i -gt 60 ] || ! kill -0 "$pid" 2>/dev/null; then
        tail -30 "$work/grapi-toggle.log"
        echo "error: grapi did not start on the toggled schemas" >&2
        exit 1
      fi
      sleep 1
    done
    "$here/bin/compat" run -corpus "$work/snap" -fixture "$fixture-snapshot" -url "http://127.0.0.1:$port" -tokens "$work/a/tokens.json" -out "$work/out/toggle-grapi"
    kill "$pid"
    wait "$pid" 2>/dev/null || true
  }
  toggle_grapi
  grep -iE "draft and publish|localization" "$work/grapi-toggle.log" | head -5 || true
  if [ "$db" = postgres ]; then
    set -- -e DATABASE_CLIENT=postgres -e DATABASE_HOST="$run_id-a-pg" -e DATABASE_NAME=strapi -e DATABASE_USERNAME=strapi -e DATABASE_PASSWORD=strapi
  else
    set -- -e DATABASE_CLIENT=sqlite
  fi
  docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-a3" -p 127.0.0.1::1337 "$@" -e COMPAT_ROW_RULES="$COMPAT_ROW_RULES" \
    -v "$tfx:/fixture:ro" -v "$work/a:/data" "$image" >/dev/null
  wait_up a3
  "$here/bin/compat" run -corpus "$work/snap" -fixture "$fixture-snapshot" -url "http://127.0.0.1:$(port_of a3)" -tokens "$work/a/tokens.json" -out "$work/out/toggle-strapi"
  mkdir -p "$here/report"
  report="$here/report/$(date +%F)-$fixture-toggle-$db.md"
  "$here/bin/compat" diff -a "$work/out/toggle-strapi" -b "$work/out/toggle-grapi" -report "$report" \
    -title "Toggle $fixture ($db): Strapi restarted after grapi migrated vs grapi"
  status=$?
  sed -n 5p "$report"
  exit $status
  ;;
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
  # Cases with hooks: true run the fixture's project code on both sides.
  hooks=0
  grep -q '"hooks": true' "$cases" && hooks=1
  docker run --rm --label "grapi-compat=$run_id" --network "$run_id" "$@" -e COMPAT_ROW_RULES="$hooks" \
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
  [ "$hooks" = 1 ] && set -- "$@" -hooks "$fx" -config "$here/reference/grapi-config"
  "$here/bin/compat" query -fixture "$here/fixtures/$fx" -client "$db" -db "$grapi_db" -cases "$cases" -out "$work/grapi.json" "$@"
  mkdir -p "$here/report"
  report="$here/report/$(date +%Y-%m-%d)-$fixture-query-$db.md"
  "$here/bin/compat" query-diff ${volatile:-} -a "$work/a/strapi.json" -b "$work/grapi.json" -report "$report" \
    -title "Query compat $fixture ($db, Strapi $STRAPI_VERSION)"
  exit $?
  ;;
grapi | perf)
  # b: Strapi creates the database (tokens, permissions), then grapi serves it.
  start_ref a
  start_ref b
  wait_up a
  wait_up b
  # An admin registered by Strapi must log in on grapi (same tables, bcrypt).
  admin_body='{"firstname":"Compat","lastname":"Admin","email":"compat-admin@example.com","password":"Compat1234"}'
  strapi_admin=$(curl -sf -H 'Content-Type: application/json' -d "$admin_body" "http://127.0.0.1:$(port_of b)/admin/register-admin") || {
    echo "error: Strapi refused register-admin" >&2
    exit 1
  }
  # A file uploaded by Strapi must be served by grapi at the same URL.
  python3 -c 'import struct,sys,zlib
def chunk(t, d): return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d))
raw = b"".join(b"\0" + bytes([200, 40, 40]) * 4 for _ in range(4))
sys.stdout.buffer.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 4, 4, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b""))' >"$work/strapi-upload.png"
  upload_png() {
    curl -s -H "Authorization: Bearer $(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["full"])' "$work/$1/tokens.json")" \
      -F "files=@$work/strapi-upload.png;type=image/png" "http://127.0.0.1:$(port_of "$1")/api/upload" |
      python3 -c 'import json,sys; r=json.load(sys.stdin); print(r[0]["url"] if isinstance(r,list) else r, file=sys.stdout if isinstance(r,list) else sys.stderr)'
  }
  strapi_file_url=$(upload_png b)
  # The reference gets the same file, so both corpus runs start with it.
  upload_png a >/dev/null
  # A folder tree made by Strapi must read the same on grapi.
  s_auth="Authorization: Bearer $(printf '%s' "$strapi_admin" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])')"
  s_url=http://127.0.0.1:$(port_of b)
  folder_id() { python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["id"])'; }
  f_a=$(curl -sf -H "$s_auth" -H 'Content-Type: application/json' -d '{"name":"Compat A","parent":null}' "$s_url/upload/folders" | folder_id)
  f_b=$(curl -sf -H "$s_auth" -H 'Content-Type: application/json' -d "{\"name\":\"Compat B\",\"parent\":$f_a}" "$s_url/upload/folders" | folder_id)
  curl -sf -o /dev/null -H "$s_auth" -H 'Content-Type: application/json' -d "{\"name\":\"Compat C\",\"parent\":$f_b}" "$s_url/upload/folders"
  curl -sf -o /dev/null -H "$s_auth" -H 'Content-Type: application/json' -d '{"name":"Compat D","parent":null}' "$s_url/upload/folders"
  curl -sf -H "$s_auth" "$s_url/upload/folder-structure" >"$work/folders-strapi.json" || admin_failed=1
  docker stop "$run_id-b" >/dev/null
  grapi_port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
  if [ "$db" = postgres ]; then
    pg_port=$(docker port "$run_id-b-pg" 5432/tcp | head -1 | sed 's/.*://')
    set -- DATABASE_CLIENT=postgres DATABASE_HOST=127.0.0.1 DATABASE_PORT="$pg_port" DATABASE_NAME=strapi \
      DATABASE_USERNAME=strapi DATABASE_PASSWORD=strapi
  else
    set -- DATABASE_CLIENT=sqlite DATABASE_FILENAME="$work/b/data.db"
  fi
  env "$@" HOST=127.0.0.1 PORT="$grapi_port" GRAPI_PUBLIC_DIR="$work/b-public" GRAPI_LOG_LEVEL="$([ "$mode" = perf ] && echo info || echo debug)" \
    "$serve_bin" start -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" >"$work/grapi.log" 2>&1 &
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
  login=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
    -d '{"email":"compat-admin@example.com","password":"Compat1234"}' "$b_url/admin/api/v1/auth/login")
  echo "Strapi admin login on grapi: $login"
  [ "$login" = 200 ] || admin_failed=1
  env "$@" "$here/bin/grapi" admin:reset-user-password -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" \
    --email compat-admin@example.com --password Compat5678 >/dev/null || admin_failed=1
  relogin=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
    -d '{"email":"compat-admin@example.com","password":"Compat5678"}' "$b_url/admin/api/v1/auth/login")
  echo "Login after admin:reset-user-password: $relogin"
  [ "$relogin" = 200 ] || admin_failed=1
  # Strapi re-encodes images on upload: compare with what it wrote.
  served=$(curl -s "$b_url$strapi_file_url" | md5sum | cut -c1-32)
  if [ -n "$strapi_file_url" ] && [ "$served" = "$(md5sum <"$work/b-public$strapi_file_url" | cut -c1-32)" ]; then
    echo "Strapi upload served by grapi at $strapi_file_url: identical"
  else
    echo "Strapi upload served by grapi at '$strapi_file_url': DIFFERENT"
    admin_failed=1
  fi
  g_token=$(curl -s -H 'Content-Type: application/json' -d '{"email":"compat-admin@example.com","password":"Compat5678"}' "$b_url/admin/api/v1/auth/login" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])')
  # A token Strapi created and encrypted is shown again, decrypted, by grapi.
  full_id=$(curl -s -H "Authorization: Bearer $g_token" "$b_url/admin/api/v1/api-tokens" |
    python3 -c 'import json,sys; print(next(t["id"] for t in json.load(sys.stdin)["data"] if t["name"] == "compat-full"))')
  shown=$(curl -s -H "Authorization: Bearer $g_token" "$b_url/admin/api/v1/api-tokens/$full_id" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"].get("accessKey") or "")')
  if [ -n "$shown" ] && [ "$shown" = "$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["full"])' "$work/b/tokens.json")" ]; then
    echo "Strapi's encrypted token shown by grapi: identical"
  else
    echo "Strapi's encrypted token shown by grapi: DIFFERENT"
    admin_failed=1
  fi
  curl -sf -H "Authorization: Bearer $g_token" "$b_url/admin/api/v1/upload/folder-structure" >"$work/folders-grapi.json" || admin_failed=1
  tree='import json,sys
def norm(nodes): return sorted(({"id": n["id"], "name": n["name"], "children": norm(n.get("children") or [])} for n in nodes), key=lambda n: n["id"])
print(json.dumps(norm(json.load(open(sys.argv[1]))["data"]), sort_keys=True))'
  if [ "$(python3 -c "$tree" "$work/folders-strapi.json")" = "$(python3 -c "$tree" "$work/folders-grapi.json")" ]; then
    echo "Strapi folder tree on grapi: identical ($(grep -o '"name"' "$work/folders-grapi.json" | wc -l | tr -d ' ') folders)"
  else
    echo "Strapi folder tree on grapi: DIFFERENT"
    python3 -c "$tree" "$work/folders-strapi.json"
    python3 -c "$tree" "$work/folders-grapi.json"
    admin_failed=1
  fi
  ;;
*)
  echo "error: unknown mode $mode" >&2
  exit 2
  ;;
esac

"$here/bin/compat" run -corpus "$here/corpus" -fixture "$fixture" -url "http://127.0.0.1:$(port_of a)" -tokens "$work/a/tokens.json" -out "$work/out/a"
"$here/bin/compat" run -corpus "$here/corpus" -fixture "$fixture" -url "$b_url" -tokens "$b_tokens" -out "$work/out/b"
if [ "$mode" = grapi ]; then
  # Forward check: a second grapi opens the database Strapi (a) wrote with
  # the corpus, and must read every content type the way Strapi serves it.
  forward_check() {
    mkdir -p "$work/snap"
    "$here/bin/compat" snapshot -fixture-dir "$here/fixtures/$fx" -name "$fixture-snapshot" -out "$work/snap/snapshot.yaml"
    "$here/bin/compat" run -corpus "$work/snap" -fixture "$fixture-snapshot" -url "http://127.0.0.1:$(port_of a)" -tokens "$work/a/tokens.json" -out "$work/out/fwd-strapi"
    docker stop "$run_id-a" >/dev/null
    if [ "$db" = postgres ]; then
      set -- DATABASE_CLIENT=postgres DATABASE_HOST=127.0.0.1 DATABASE_PORT="$(docker port "$run_id-a-pg" 5432/tcp | head -1 | sed 's/.*://')" \
        DATABASE_NAME=strapi DATABASE_USERNAME=strapi DATABASE_PASSWORD=strapi
    else
      set -- DATABASE_CLIENT=sqlite DATABASE_FILENAME="$work/a/data.db"
    fi
    fwd_port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
    env "$@" HOST=127.0.0.1 PORT="$fwd_port" GRAPI_PUBLIC_DIR="$work/a-public" \
      "$serve_bin" start -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" >"$work/grapi-forward.log" 2>&1 &
    fwd_pid=$!
    i=0
    until [ "$(curl -s -o /dev/null -w '%{http_code}' "127.0.0.1:$fwd_port/_health")" = 204 ]; do
      i=$((i + 1))
      if [ $i -gt 60 ] || ! kill -0 "$fwd_pid" 2>/dev/null; then
        tail -30 "$work/grapi-forward.log"
        echo "error: grapi did not start on Strapi's database" >&2
        return 1
      fi
      sleep 1
    done
    "$here/bin/compat" run -corpus "$work/snap" -fixture "$fixture-snapshot" -url "http://127.0.0.1:$fwd_port" -tokens "$work/a/tokens.json" -out "$work/out/fwd-grapi"
    kill "$fwd_pid" 2>/dev/null || true
    wait "$fwd_pid" 2>/dev/null || true
    mkdir -p "$here/report"
    fwd_report="$here/report/$(date +%F)-$fixture-forward-$db.md"
    "$here/bin/compat" diff -a "$work/out/fwd-strapi" -b "$work/out/fwd-grapi" -report "$fwd_report" \
      -title "Forward $fixture ($db): grapi reading Strapi's writes vs Strapi" || admin_failed=1
    sed -n 5p "$fwd_report"
  }
  forward_check || admin_failed=1
fi
if [ "$mode" = grapi ] && { [ "$fx" = basic ] || [ "$fx" = relations ]; }; then
  # Webhooks: the same hook and the same writes on Strapi (a) and grapi (b),
  # recorded by an echo receiver on the bench network, then compared.
  mkdir -p "$work/echo"
  chmod 777 "$work/echo"
  docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-echo" -p 127.0.0.1::9000 \
    -v "$here/scripts/echo.js:/echo.js:ro" -v "$work/echo:/out" --entrypoint node "$image" /echo.js >/dev/null
  echo_port=$(docker port "$run_id-echo" 9000/tcp | head -1 | sed 's/.*://')
  hook_events='["entry.create","entry.update","entry.delete","entry.publish","entry.unpublish"]'
  # In production Strapi refuses a webhook URL that is not public: the
  # reference restarts in development on its own database for this check.
  docker stop "$run_id-a" >/dev/null
  # A function keeps the positional parameters: grapi restarts with them below.
  start_dev_reference() {
    if [ "$db" = postgres ]; then
      set -- -e DATABASE_CLIENT=postgres -e DATABASE_HOST="$run_id-a-pg" -e DATABASE_NAME=strapi -e DATABASE_USERNAME=strapi -e DATABASE_PASSWORD=strapi
    else
      set -- -e DATABASE_CLIENT=sqlite
    fi
    docker run -d --label "grapi-compat=$run_id" --network "$run_id" --name "$run_id-a2" -p 127.0.0.1::1337 "$@" -e NODE_ENV=development -e COMPAT_ROW_RULES="$COMPAT_ROW_RULES" \
      -v "$here/fixtures/$fx:/fixture:ro" -v "$work/a:/data" "$image" >/dev/null
  }
  start_dev_reference
  wait_up a2
  a_url=http://127.0.0.1:$(port_of a2)
  a_admin=$(curl -sf -H 'Content-Type: application/json' -d "$admin_body" "$a_url/admin/register-admin" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])')
  curl -sf -o /dev/null -H "Authorization: Bearer $a_admin" -H 'Content-Type: application/json' \
    -d "{\"name\":\"compat\",\"url\":\"http://$run_id-echo:9000/strapi\",\"headers\":{\"X-Test\":\"1\"},\"events\":$hook_events}" "$a_url/admin/webhooks" || admin_failed=1
  g_admin=$(curl -s -H 'Content-Type: application/json' -d '{"email":"compat-admin@example.com","password":"Compat5678"}' "$b_url/admin/api/v1/auth/login" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])')
  curl -sf -o /dev/null -H "Authorization: Bearer $g_admin" -H 'Content-Type: application/json' \
    -d "{\"name\":\"compat\",\"url\":\"http://127.0.0.1:$echo_port/grapi\",\"headers\":{\"X-Test\":\"1\"},\"events\":$hook_events,\"isEnabled\":true}" "$b_url/admin/api/v1/webhooks" || admin_failed=1
  for side in a b; do
    if [ $side = a ]; then url=$a_url; tok=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["full"])' "$work/a/tokens.json")
    else url=$b_url; tok=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["full"])' "$b_tokens"); fi
    hdr="Authorization: Bearer $tok"
    doc_of() { python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["documentId"])'; }
    tag=$(curl -s -H "$hdr" -H 'Content-Type: application/json' -d '{"data":{"name":"webhook-tag"}}' "$url/api/tags" | doc_of)
    if [ "$fx" = relations ]; then
      plural=posts
      doc=$(curl -s -H "$hdr" -H 'Content-Type: application/json' -d "{\"data\":{\"title\":\"Webhook\",\"tags\":[\"$tag\"]}}" "$url/api/posts?status=published" | doc_of)
    else
      plural=articles
      doc=$(curl -s -H "$hdr" -H 'Content-Type: application/json' -d '{"data":{"title":"Webhook"}}' "$url/api/articles?status=published" | doc_of)
    fi
    sleep 1
    curl -s -o /dev/null -X PUT -H "$hdr" -H 'Content-Type: application/json' -d '{"data":{"title":"Webhook 2"}}' "$url/api/$plural/$doc"
    sleep 1
    curl -s -o /dev/null -X DELETE -H "$hdr" "$url/api/$plural/$doc"
  done
  sleep 3
  python3 "$here/scripts/webhook-compare.py" "$work/echo/strapi.jsonl" "$work/echo/grapi.jsonl" || admin_failed=1
fi
if [ "$mode" = perf ]; then
  # Same data on both sides (the corpus just ran on each): load the
  # variable free GET cases, and sample the memory before, during, after.
  mem_strapi() { docker stats --no-stream --format '{{.MemUsage}}' "$run_id-a" | awk '{print $1}'; }
  mem_grapi() { awk '/VmRSS/ {printf "%.1fMiB\n", $2/1024}' "/proc/$grapi_pid/status"; }
  peak() { # peak <sampler> <seconds>: highest sample, in MiB
    end=$(( $(date +%s) + $2 )); max=0
    while [ "$(date +%s)" -lt "$end" ]; do
      v=$($1 | sed 's/MiB//;s/GiB/*1024/;s/KiB/\/1024/' | bc -l 2>/dev/null || echo 0)
      max=$(echo "if ($v > $max) $v else $max" | bc -l); sleep 0.5
    done; printf '%.1fMiB' "$max"
  }
  report="$here/report/$(date +%F)-$fixture-perf-$db.md"
  {
    echo "# Perf $fixture ($db): Strapi $STRAPI_VERSION (Node, container) vs grapi (native)"
    echo
    echo "Load: the variable free GET cases of the corpus, round robin, ${PERF_C:-32} workers, ${PERF_D:-30s}, after a 5 s warm-up."
    echo
    echo "Memory: \`docker stats\` usage for the Strapi container, VmRSS for the grapi process."
    echo
    echo "| Engine | req/s | p50 ms | p95 ms | p99 ms | errors | memory idle | memory peak | memory 10 s after |"
    echo "|---|---|---|---|---|---|---|---|---|"
  } >"$report"
  for engine in strapi grapi; do
    if [ $engine = strapi ]; then url="http://127.0.0.1:$(port_of a)"; sampler=mem_strapi; else url=$b_url; sampler=mem_grapi; fi
    "$here/bin/compat" perf -corpus "$here/corpus" -fixture "$fixture" -url "$url" -c 8 -d 5s >/dev/null
    idle=$($sampler)
    peak $sampler 30 >"$work/peak-$engine" &
    peak_pid=$!
    res=$("$here/bin/compat" perf -corpus "$here/corpus" -fixture "$fixture" -url "$url" -c "${PERF_C:-32}" -d "${PERF_D:-30s}")
    wait "$peak_pid"
    sleep 10
    after=$($sampler)
    echo "$res" | python3 -c "import json,sys; r=json.load(sys.stdin); print('| $engine | %.0f | %.1f | %.1f | %.1f | %d | $idle | %s | $after |' % (r['rps'], r['p50ms'], r['p95ms'], r['p99ms'], r['errors'], open('$work/peak-$engine').read()))" >>"$report"
  done
  # What grapi answered under load: statuses, and the messages that are not
  # request lines (errors, warnings).
  {
    echo
    echo "grapi statuses under load:"
    grep -oE '"status":[0-9]+|status=[0-9]+' "$work/grapi.log" | grep -oE '[0-9]+$' | sort | uniq -c | sort -rn | head -10
    echo "grapi messages other than requests:"
    grep -vE '"level":"HTTP"|level=HTTP' "$work/grapi.log" | grep -oE '"msg":"[^"]*"|msg="[^"]*"|msg=[^ ]+' | sort | uniq -c | sort -rn | head -10
  } >>"$report"
  cat "$report"
  exit 0
fi
if [ "$mode" = grapi ]; then
  mkdir -p "$here/report"
  # Reverse check: Strapi restarts on the database grapi just wrote, and must
  # read every content type the way grapi serves it.
  mkdir -p "$work/snap"
  "$here/bin/compat" snapshot -fixture-dir "$here/fixtures/$fx" -name "$fixture-snapshot" -out "$work/snap/snapshot.yaml"
  "$here/bin/compat" run -corpus "$work/snap" -fixture "$fixture-snapshot" -url "$b_url" -tokens "$b_tokens" -out "$work/out/snap-grapi"
  # Core store round trip, one way: grapi writes, Strapi reads back below.
  python3 "$here/scripts/core-store.py" write g "$work/cs-grapi-in.json"
  env "$@" "$here/bin/grapi" configuration:restore -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" -f "$work/cs-grapi-in.json" -s merge >/dev/null || admin_failed=1
  env "$@" "$here/bin/grapi" configuration:dump -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" -f "$work/b/grapi-dump.json" >/dev/null || admin_failed=1
  kill "$grapi_pid" 2>/dev/null || true
  wait "$grapi_pid" 2>/dev/null || true
  cp "$work/grapi.log" "$here/report/$(date +%F)-$fixture-grapi-$db.log"
  docker start "$run_id-b" >/dev/null
  wait_up b
  "$here/bin/compat" run -corpus "$work/snap" -fixture "$fixture-snapshot" -url "http://127.0.0.1:$(port_of b)" -tokens "$b_tokens" -out "$work/out/snap-strapi"
  "$here/bin/compat" diff -a "$work/out/snap-strapi" -b "$work/out/snap-grapi" -report "$here/report/$(date +%F)-$fixture-reverse-$db.md" \
    -title "Reverse $fixture ($db): Strapi reading grapi's writes vs grapi" || true
  sed -n 5p "$here/report/$(date +%F)-$fixture-reverse-$db.md"
  strapi_cli() { docker exec -w /app "$run_id-b" node node_modules/@strapi/strapi/bin/strapi.js "$@" >/dev/null 2>&1; }
  strapi_cli configuration:dump -f /data/cs-strapi-dump.json || admin_failed=1
  cp "$work/b/cs-strapi-dump.json" "$here/report/$fixture-strapi-core-store-$db.json" 2>/dev/null || true
  python3 "$here/scripts/core-store.py" compare g "$work/b/cs-strapi-dump.json" Strapi || admin_failed=1
  # And the other way: Strapi writes, grapi reads back once restarted.
  python3 "$here/scripts/core-store.py" write s "$work/b/cs-strapi-in.json"
  strapi_cli configuration:restore -f /data/cs-strapi-in.json -s merge || admin_failed=1
  if strapi_cli configuration:restore -f /data/grapi-dump.json -s merge; then echo "A grapi dump restored by Strapi: ok"; else echo "A grapi dump restored by Strapi: FAILED"; admin_failed=1; fi
  # An end user registered by Strapi must log in on grapi (same table, bcrypt).
  reg=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
    -d '{"username":"compat-enduser","email":"compat-enduser@example.com","password":"Compat1234!"}' "http://127.0.0.1:$(port_of b)/api/auth/local/register")
  docker stop "$run_id-b" >/dev/null
  env "$@" HOST=127.0.0.1 PORT="$grapi_port" "$serve_bin" start -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" >"$work/grapi-enduser.log" 2>&1 &
  grapi_pid=$!
  i=0
  until [ "$(curl -s -o /dev/null -w '%{http_code}' "127.0.0.1:$grapi_port/_health")" = 204 ]; do
    i=$((i + 1))
    [ $i -gt 60 ] && break
    sleep 1
  done
  enduser=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
    -d '{"identifier":"compat-enduser@example.com","password":"Compat1234!"}' "$b_url/api/auth/local")
  echo "Strapi end user login on grapi: $enduser (registered by Strapi: $reg)"
  env "$@" "$here/bin/grapi" configuration:dump -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" -f "$work/cs-grapi-dump.json" >/dev/null || admin_failed=1
  python3 "$here/scripts/core-store.py" compare s "$work/cs-grapi-dump.json" grapi || admin_failed=1
  if env "$@" "$here/bin/grapi" configuration:restore -dir "$here/fixtures/$fx" -config "$here/reference/grapi-config" -f "$work/b/cs-strapi-dump.json" -s merge >/dev/null; then
    echo "A Strapi dump restored by grapi: ok"
  else
    echo "A Strapi dump restored by grapi: FAILED"
    admin_failed=1
  fi
  { [ "$reg" = 200 ] && [ "$enduser" = 200 ]; } || admin_failed=1
  kill "$grapi_pid" 2>/dev/null || true
  wait "$grapi_pid" 2>/dev/null || true
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
if [ "${admin_failed:-0}" = 1 ]; then
  tail -20 "$work/grapi.log"
  exit 1
fi
exit $status
