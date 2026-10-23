#!/bin/sh
# End to end admin invitation and settings test e-mail through a real SMTP
# server: Mailpit runs on the homelab (nothing in Docker locally), grapi runs
# here on a throwaway copy of the basic fixture. Invite, read the mail in
# Mailpit, follow the link, register, log in; reset the password from its
# e-mail, log out; then send the test e-mail. End users: reset the password
# and confirm the e-mail from users-permissions' mails.
# usage: scripts/e2e-invite-mailpit.sh
set -eu
host=${COMPAT_HOST:-homelab}
name="grapi-mailpit-$$"
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)

cleanup() {
  [ -n "${grapi_pid:-}" ] && kill "$grapi_pid" 2>/dev/null || true
  [ -n "${tunnel:-}" ] && kill "$tunnel" 2>/dev/null || true
  ssh "$host" "docker rm -f $name >/dev/null 2>&1" || true
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

free_port() { python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }

ssh "$host" "docker run -d --name $name -p 127.0.0.1::1025 -p 127.0.0.1::8025 axllent/mailpit:v1.27.10 >/dev/null"
smtp_remote=$(ssh "$host" "docker port $name 1025/tcp | head -1 | sed 's/.*://'")
api_remote=$(ssh "$host" "docker port $name 8025/tcp | head -1 | sed 's/.*://'")
smtp=$(free_port)
api=$(free_port)
ssh -N -o ControlMaster=no -o ControlPath=none -o ExitOnForwardFailure=yes \
  -L "$smtp:127.0.0.1:$smtp_remote" -L "$api:127.0.0.1:$api_remote" "$host" &
tunnel=$!
i=0
until curl -sf "http://127.0.0.1:$api/api/v1/info" >/dev/null 2>&1; do
  i=$((i + 1))
  [ $i -gt 30 ] && { echo "error: Mailpit is not reachable" >&2; exit 1; }
  sleep 1
done

cp -R "$root/compat/fixtures/basic/src" "$work/src"
mkdir -p "$work/config"
cat >"$work/config/plugins.yaml" <<EOF
email:
  config:
    provider: smtp
    providerOptions:
      host: 127.0.0.1
      port: $smtp
    settings:
      defaultFrom: grapi@example.test
EOF
(cd "$root" && go build -o "$work/grapi" ./cmd/grapi)
port=$(free_port)
base=http://127.0.0.1:$port/admin/api/v1
env HOST=127.0.0.1 PORT="$port" DATABASE_CLIENT=sqlite DATABASE_FILENAME="$work/data.db" \
  JWT_SECRET=e2e ADMIN_JWT_SECRET=e2e-admin API_TOKEN_SALT=e2e-salt \
  "$work/grapi" start -dir "$work" >"$work/grapi.log" 2>&1 &
grapi_pid=$!
i=0
until [ "$(curl -s -o /dev/null -w '%{http_code}' "127.0.0.1:$port/_health")" = 204 ]; do
  i=$((i + 1))
  if [ $i -gt 30 ] || ! kill -0 "$grapi_pid" 2>/dev/null; then
    tail -20 "$work/grapi.log"
    echo "error: grapi did not start" >&2
    exit 1
  fi
  sleep 1
done

token=$(curl -sf -H 'Content-Type: application/json' -d '{"firstname":"Ada","email":"ada@example.test","password":"E2e-Passw0rd"}' "$base/auth/register-admin" | json 'd["data"]["token"]')
role=$(curl -sf -H "Authorization: Bearer $token" "$base/roles" | json '[r["id"] for r in d["data"] if r["code"]=="strapi-editor"][0]')
curl -sf -o /dev/null -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d "{\"firstname\":\"Ed\",\"email\":\"ed@example.test\",\"roles\":[$role]}" "$base/users"

i=0
until [ "$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages_count"]')" -ge 1 ]; do
  i=$((i + 1))
  [ $i -gt 20 ] && { tail -20 "$work/grapi.log"; echo "error: no mail reached Mailpit" >&2; exit 1; }
  sleep 1
done
id=$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages"][0]["ID"]')
mail=$(curl -sf "http://127.0.0.1:$api/api/v1/message/$id")
to=$(printf '%s' "$mail" | json 'd["To"][0]["Address"]')
reg=$(printf '%s' "$mail" | json '__import__("re").search(r"registrationToken=([A-Za-z0-9]+)", d["Text"] + d["HTML"]).group(1)')
[ "$to" = ed@example.test ] || { echo "error: the mail went to $to" >&2; exit 1; }
email=$(curl -sf "$base/auth/registration-info?registrationToken=$reg" | json 'd["data"]["email"]')
[ "$email" = ed@example.test ] || { echo "error: registration-info says $email" >&2; exit 1; }
curl -sf -o /dev/null -H 'Content-Type: application/json' \
  -d "{\"registrationToken\":\"$reg\",\"userInfo\":{\"firstname\":\"Ed\",\"password\":\"E2e-Ed1tor\"}}" "$base/auth/register"
login=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"email":"ed@example.test","password":"E2e-Ed1tor"}' "$base/auth/login")
[ "$login" = 200 ] || { echo "error: the invited admin cannot log in ($login)" >&2; exit 1; }
curl -sf -X DELETE "http://127.0.0.1:$api/api/v1/messages" >/dev/null
curl -sf -o /dev/null -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{"to":"ops@example.test"}' "$base/email/test"
i=0
until [ "$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages_count"]')" -ge 1 ]; do
  i=$((i + 1))
  [ $i -gt 20 ] && { echo "error: the test e-mail did not reach Mailpit" >&2; exit 1; }
  sleep 1
done
to=$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages"][0]["To"][0]["Address"]')
[ "$to" = ops@example.test ] || { echo "error: the test e-mail went to $to" >&2; exit 1; }
curl -sf -X DELETE "http://127.0.0.1:$api/api/v1/messages" >/dev/null
curl -sf -o /dev/null -H 'Content-Type: application/json' -d '{"email":"ed@example.test"}' "$base/auth/forgot-password"
i=0
until [ "$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages_count"]')" -ge 1 ]; do
  i=$((i + 1))
  [ $i -gt 20 ] && { echo "error: no reset mail reached Mailpit" >&2; exit 1; }
  sleep 1
done
id=$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages"][0]["ID"]')
code=$(curl -sf "http://127.0.0.1:$api/api/v1/message/$id" | json '__import__("re").search(r"code=([A-Za-z0-9]+)", d["Text"] + d["HTML"]).group(1)')
curl -sf -o /dev/null -H 'Content-Type: application/json' -d "{\"resetPasswordToken\":\"$code\",\"password\":\"E2e-Res3tPass\"}" "$base/auth/reset-password"
jar="$work/cookies"
login=$(curl -s -c "$jar" -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d '{"email":"ed@example.test","password":"E2e-Res3tPass"}' "$base/auth/login")
[ "$login" = 200 ] || { echo "error: login after the reset ($login)" >&2; exit 1; }
curl -sf -b "$jar" -c "$jar" -o /dev/null -X POST "$base/auth/logout"
refresh=$(curl -s -b "$jar" -o /dev/null -w '%{http_code}' -X POST "$base/auth/access-token")
[ "$refresh" = 401 ] || { echo "error: the session survives the logout ($refresh)" >&2; exit 1; }
# End users: the reset and confirmation links of users-permissions' mails.
wait_mail() {
  i=0
  until [ "$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages_count"]')" -ge 1 ]; do
    i=$((i + 1))
    [ $i -gt 20 ] && { tail -20 "$work/grapi.log"; echo "error: $1 did not reach Mailpit" >&2; exit 1; }
    sleep 1
  done
  curl -sf "http://127.0.0.1:$api/api/v1/message/$(curl -sf "http://127.0.0.1:$api/api/v1/messages" | json 'd["messages"][0]["ID"]')"
}
users=http://127.0.0.1:$port/api/auth
curl -sf -X DELETE "http://127.0.0.1:$api/api/v1/messages" >/dev/null
curl -sf -o /dev/null -H 'Content-Type: application/json' -d '{"username":"enduser","email":"enduser@example.test","password":"E2e-User1"}' "$users/local/register"
curl -sf -o /dev/null -H 'Content-Type: application/json' -d '{"email":"enduser@example.test"}' "$users/forgot-password"
code=$(wait_mail "the end user's reset mail" | json '__import__("re").search(r"code=([A-Za-z0-9]+)", d["Text"] + d["HTML"]).group(1)')
reset=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d "{\"code\":\"$code\",\"password\":\"E2e-User2\",\"passwordConfirmation\":\"E2e-User2\"}" "$users/reset-password")
[ "$reset" = 200 ] || { echo "error: end user reset ($reset)" >&2; exit 1; }
login=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d '{"identifier":"enduser@example.test","password":"E2e-User2"}' "$users/local")
[ "$login" = 200 ] || { echo "error: end user login after the reset ($login)" >&2; exit 1; }
curl -sf -o /dev/null -X PUT -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{"email_confirmation":true}' "$base/users-permissions/advanced"
curl -sf -X DELETE "http://127.0.0.1:$api/api/v1/messages" >/dev/null
curl -sf -o /dev/null -H 'Content-Type: application/json' -d '{"username":"confirmed","email":"confirmed@example.test","password":"E2e-User1"}' "$users/local/register"
before=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d '{"identifier":"confirmed@example.test","password":"E2e-User1"}' "$users/local")
[ "$before" = 400 ] || { echo "error: an unconfirmed user logs in ($before)" >&2; exit 1; }
confirmation=$(wait_mail "the confirmation mail" | json '__import__("re").search(r"confirmation=([A-Za-z0-9]+)", d["Text"] + d["HTML"]).group(1)')
followed=$(curl -s -o /dev/null -w '%{http_code}' "$users/email-confirmation?confirmation=$confirmation")
[ "$followed" = 301 ] || [ "$followed" = 302 ] || { echo "error: confirmation link ($followed)" >&2; exit 1; }
after=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d '{"identifier":"confirmed@example.test","password":"E2e-User1"}' "$users/local")
[ "$after" = 200 ] || { echo "error: login after the confirmation ($after)" >&2; exit 1; }
echo "end user e2e: reset link and confirmation link received by mail and followed, logins work"
echo "invitation e2e: mail delivered by SMTP to Mailpit, link followed, invited admin logged in"
echo "password reset e2e: link received by mail, new password works; logout ends the session"
echo "test e-mail e2e: Settings > E-mail test delivered to Mailpit"
