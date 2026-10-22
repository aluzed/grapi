#!/usr/bin/env bash
# Every story must open with a frontmatter holding a known status.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
fail=0
while IFS= read -r f; do
  status=$(awk 'NR==1 && $0!="---" {exit} NR>1 && $0=="---" {exit} NR>1 && /^status:/ {print $2}' "$f")
  case "$status" in
    todo|in-progress|done) ;;
    *) echo "$f: missing or invalid status '${status}'"; fail=1 ;;
  esac
done < <(find docs/stories -name '*.md' ! -name README.md | sort)
exit $fail
