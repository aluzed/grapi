// Dumps the content types Strapi ships outside src/ (admin, upload, i18n,
// users-permissions) and its internal models, as schema.json files under
// argv[2]. Reads only the Community Edition entry points (no ee/ path).
// Run inside the reference image.
const fs = require('fs');
const path = require('path');

const out = process.argv[2];
const base = '/app/node_modules/@strapi/';
const load = (p) => {
  const m = require(base + p);
  const v = m.default || m;
  return typeof v.__require === 'function' ? v.__require() : v;
};

const sources = {
  admin: load('admin/dist/server/server/src/content-types/index.js'),
  upload: load('upload/dist/server/content-types/index.js').contentTypes,
  i18n: load('i18n/dist/server/content-types/index.js'),
  'users-permissions': load('plugin-users-permissions/dist/server/content-types/index.js'),
};

const write = (file, value) => {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, JSON.stringify(value, null, 2) + '\n');
};

for (const [ns, cts] of Object.entries(sources)) {
  for (const [name, def] of Object.entries(cts)) {
    const schema = def.schema || def;
    write(path.join(out, ns, 'content-types', name, 'schema.json'), schema);
  }
}

const { coreStoreModel } = require(base + 'core/dist/services/core-store.js');
const { webhookModel } = require(base + 'core/dist/services/webhook-store.js');
for (const model of [coreStoreModel, webhookModel]) {
  write(path.join(out, 'models', model.tableName + '.json'), model);
}
