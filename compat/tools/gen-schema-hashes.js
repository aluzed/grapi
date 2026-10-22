// Prints the hash Strapi's schema storage computes for each golden
// database-schema.json given as argument, plus the localeCompare order of
// tricky names. Run inside the reference image.
const crypto = require('crypto');
const fs = require('fs');

const hashSchema = (schema) => {
  const sorted = {
    ...schema,
    tables: schema.tables.toSorted((a, b) => a.name.localeCompare(b.name)),
  };
  return crypto.createHash('sha256').update(JSON.stringify(sorted)).digest('hex');
};

const excluded = ['strapi_release', 'strapi_workflows', 'strapi_history_', 'strapi_ai_'];
const out = { hashes: {}, order: [] };
for (const file of process.argv.slice(2)) {
  const schema = JSON.parse(fs.readFileSync(file, 'utf8'));
  const ce = { ...schema, tables: schema.tables.filter((t) => !excluded.some((p) => t.name.startsWith(p))) };
  out.hashes[file] = { full: hashSchema(schema), ce: hashSchema(ce) };
}
const names = [
  'a', 'a_b', 'a1', 'a_1', 'ab', 'a__b', 'a_', 'b', 'up_users', 'upload_folders', 'up_users_role_lnk',
  'files', 'files_folder_lnk', 'files_related_mph', 'z9', 'z_9', 'z09', 'x_y_z', 'xy_z', 'x_yz',
];
out.order = names.toSorted((a, b) => a.localeCompare(b));
console.log(JSON.stringify(out, null, 2));
