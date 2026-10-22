// Writes the outputs of Strapi's Identifiers for a fixed corpus, used by
// internal/db/naming tests. Run inside the reference image.
const path = require('path');
const dbDir = path.dirname(require.resolve('@strapi/database'));
const { Identifiers } = require(path.join(dbDir, 'utils', 'identifiers', 'index.js'));

const words = [
  'a', 'up', 'post', 'articles', 'components_shared_seos', 'categories', 'theme_pages',
  'a_really_long_collection_name_for_testing', 'attribute_with_a_long_name_too',
  'createdBy', 'updatedBy', 'parentTheme', 'localizations', 'related', 'documentId',
  'x'.repeat(54), 'y'.repeat(55), 'z'.repeat(56), 'w'.repeat(80),
  'plugin_users_permissions_permissions', 'strapi_transfer_token_permissions',
];
const methods = {
  getTableName: 1, getColumnName: 1, getJoinTableName: 2, getMorphTableName: 2,
  getJoinColumnAttributeIdName: 1, getInverseJoinColumnAttributeIdName: 1,
  getOrderColumnName: 1, getInverseOrderColumnName: 1, getMorphColumnJoinTableIdName: 1,
  getMorphColumnAttributeIdName: 1, getMorphColumnTypeName: 1, getIndexName: 1,
  getFkIndexName: 1, getUniqueIndexName: 1, getPrimaryIndexName: 1, getInverseFkIndexName: 1,
  getOrderFkIndexName: 1, getOrderInverseFkIndexName: 1, getIdColumnIndexName: 1, getOrderIndexName: 1,
};
const out = [];
for (const maxLength of [55, 0, 20]) {
  const ids = new Identifiers({ maxLength });
  for (const [method, arity] of Object.entries(methods)) {
    for (let i = 0; i < words.length; i++) {
      const args = arity === 1 ? [words[i]] : [words[i], words[(i * 7 + 3) % words.length]];
      let result;
      let error;
      try {
        result = ids[method](...args);
      } catch (e) {
        error = true;
      }
      out.push({ maxLength, method, args, result, error: !!error });
    }
  }
  for (const [names, prefix, suffix] of [
    [['a', 'b', 'c', 'd'], 'inv', 'order'],
    [['first_long_part_name', 'second_long_part_name', 'third_part'], undefined, 'links'],
    [['q'.repeat(30), 'r'.repeat(30), 's'.repeat(30)], 'inv', 'id'],
  ]) {
    let result;
    let error;
    try {
      result = ids.getName(names, { prefix, suffix });
    } catch (e) {
      error = true;
    }
    out.push({ maxLength, method: 'getName', args: names, prefix: prefix || '', suffix: suffix || '', result, error: !!error });
  }
}
process.stdout.write(JSON.stringify(out, null, 1) + '\n');
