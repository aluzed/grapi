#!/bin/sh
# The devfreelance responses saved from Strapi, typed with the TypeScript
# types grapi generates, compiled by tsc in the reference image on the
# homelab (nothing runs in Docker here).
# usage: compat/scripts/typecheck.sh
set -eu
host=${COMPAT_HOST:-homelab}
root=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
remote=/tmp/grapi-typecheck-$$
trap 'rm -rf "$work"; ssh "$host" "rm -rf $remote"' EXIT
cd "$root"
go run ./cmd/grapi ts:generate-types -dir compat/fixtures/devfreelance -o "$work/types" >/dev/null
python3 compat/tools/typecheck-golden.py compat/golden/devfreelance-sqlite/devfreelance "$work/check.ts"
cat >"$work/tsconfig.json" <<'JSON'
{"compilerOptions": {"strict": true, "noEmit": true, "skipLibCheck": true, "moduleResolution": "node", "module": "commonjs", "target": "es2020"}, "include": ["check.ts", "types/**/*.d.ts"]}
JSON
scp -q -r "$work" "$host:$remote"
ssh "$host" "chmod -R a+rwX $remote && img=\$(docker images --format '{{.Repository}}:{{.Tag}}' | grep '^grapi-compat-strapi:' | head -1) && docker run --rm -v $remote:/app/tscheck --entrypoint sh \"\$img\" -c 'cd /app/tscheck && /app/node_modules/.bin/tsc -p tsconfig.json'"
echo "typecheck: the responses Strapi gave match the types grapi generates"
