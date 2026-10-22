#!/bin/sh
# Lists the directories of a Strapi checkout that are not MIT.
# Detection relies on LICENSE contents, not on the ee/ folder name: whole
# packages (review-workflows, content-releases) are Enterprise-licensed too.
set -eu
root=${1:?usage: list-forbidden-paths.sh <strapi-checkout>}
cd "$root"
{
  find . -name node_modules -prune -o -type f -name 'LICENSE*' -print |
    while read -r f; do
      if grep -q 'Enterprise License' "$f" && ! grep -q 'MIT Expat License' "$f"; then
        dirname "$f"
      fi
    done
  find . -name node_modules -prune -o -type d -name ee -print
} | sed 's|^\./||' | sort -u
