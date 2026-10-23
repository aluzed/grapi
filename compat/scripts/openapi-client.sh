#!/bin/sh
# The OpenAPI document grapi generates for devfreelance: validated by
# Redocly, turned into types by openapi-typescript, and a typed
# openapi-fetch client compiled with tsc --strict, on the homelab (nothing
# runs in Docker here). openapi-generator is left out: its OpenAPI 3.1
# support is experimental and its typescript-fetch client misses the enum
# types of query parameters.
# usage: compat/scripts/openapi-client.sh
set -eu
host=${COMPAT_HOST:-homelab}
root=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
remote=/tmp/grapi-openapi-$$
trap 'rm -rf "$work"; ssh "$host" "rm -rf $remote"' EXIT
cd "$root"
DATABASE_CLIENT=sqlite DATABASE_FILENAME="$work/data.db" JWT_SECRET=openapi \
  go run ./cmd/grapi openapi -dir compat/fixtures/devfreelance -o "$work/oas.json"
rm -f "$work"/data.db*
mkdir -p "$work/client"
cat >"$work/client/client.ts" <<'TS'
import createClient from "openapi-fetch";
import type { paths } from "./schema";

const client = createClient<paths>({ baseUrl: "http://localhost:1337/api" });

export async function postTitles() {
  const { data, error } = await client.GET("/posts", { params: { query: { sort: "title" } } });
  if (error) throw new Error(JSON.stringify(error));
  return data?.data?.map((p) => p.title);
}
TS
scp -q -r "$work" "$host:$remote"
ssh "$host" "chmod -R a+rwX $remote && docker run --rm -v $remote:/spec redocly/cli:1.25.11 lint --extends=minimal /spec/oas.json 2>&1 | grep -q 'Your API description is valid' && echo 'redocly: valid'"
ssh "$host" "img=\$(docker images --format '{{.Repository}}:{{.Tag}}' | grep '^grapi-compat-strapi:' | head -1) && docker run --rm -v $remote:/w -w /w/client --entrypoint sh \"\$img\" -c 'npm init -y >/dev/null 2>&1 && npm install --no-audit --no-fund openapi-typescript@7.4.4 openapi-fetch@0.13.4 typescript@5.6.3 >/dev/null 2>&1 && npx openapi-typescript /w/oas.json -o schema.d.ts >/dev/null && npx tsc --noEmit --strict --target es2020 --lib es2020,dom --moduleResolution bundler --module esnext --skipLibCheck client.ts'"
echo "openapi: a typed client generated from grapi's document compiles"
