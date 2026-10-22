#!/bin/sh
# /fixture is mounted read-only; the app works on a copy so scaffolding stays out of the repo.
set -eu
rm -rf /app/src
cp -r /fixture/src /app/src
cp /app/bootstrap/index.js /app/src/index.js
node /app/bootstrap/scaffold.js
exec node node_modules/@strapi/strapi/bin/strapi.js start
