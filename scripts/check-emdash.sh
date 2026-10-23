#!/bin/sh
# Byte search keeps it portable: BSD grep on macOS has no -P.
set -eu
pattern=$(printf '\342\200\224')
# Vendored third-party files (Swagger UI) are left as published.
if git grep --untracked -n -I -F "$pattern" -- . ':!internal/docplugin/assets' ; then
  echo "error: em-dash (U+2014) found, use ':', parentheses or commas instead" >&2
  exit 1
fi
