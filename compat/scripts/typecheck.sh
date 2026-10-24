#!/bin/sh
# A fixture's responses saved from Strapi, and the writes Strapi accepted,
# typed with the TypeScript types grapi generates, compiled by tsc in the reference image on the homelab
# (nothing runs in Docker here). A private fixture is read from
# $GRAPI_PRIVATE_COMPAT.
# usage: compat/scripts/typecheck.sh [fixture]
set -eu
host=${COMPAT_HOST:-homelab}
root=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
remote=/tmp/grapi-typecheck-$$
trap 'rm -rf "$work"; ssh "$host" "rm -rf $remote"' EXIT
fixture=${1:-basic}
cd "$root"
base=compat
[ -d "compat/fixtures/$fixture" ] || base=${GRAPI_PRIVATE_COMPAT:?private fixture: set GRAPI_PRIVATE_COMPAT}
go run ./cmd/grapi ts:generate-types -dir "$base/fixtures/$fixture" -o "$work/types" >/dev/null
python3 compat/tools/typecheck-golden.py "$base/golden/$fixture-sqlite/$fixture" "$work/check.ts"
python3 compat/tools/typecheck-golden.py "$base/golden/$fixture-sqlite/$fixture" "$work/client.ts" client
python3 compat/tools/typecheck-inputs.py "$base/fixtures/$fixture" "$base/corpus" "$base/golden/$fixture-sqlite" "$work/inputs.ts"
cat >"$work/tsconfig.json" <<'JSON'
{"compilerOptions": {"strict": true, "noEmit": true, "skipLibCheck": true, "moduleResolution": "node", "module": "commonjs", "target": "es2020"}, "include": ["check.ts", "client.ts", "inputs.ts", "types/**/*.d.ts"]}
JSON
scp -q -r "$work" "$host:$remote"
ssh "$host" "chmod -R a+rwX $remote && img=\$(docker images --format '{{.Repository}}:{{.Tag}}' | grep '^grapi-compat-strapi:' | head -1) && docker run --rm -v $remote:/app/tscheck --entrypoint sh \"\$img\" -c 'cd /app/tscheck && /app/node_modules/.bin/tsc -p tsconfig.json'"
echo "typecheck: the responses Strapi gave match the types grapi generates (Strapi format and api.d.ts)"
