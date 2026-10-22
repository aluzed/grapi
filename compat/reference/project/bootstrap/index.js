const fs = require('fs');

// Public gets read access to every api:: type; the corpus uses the two tokens
// written to COMPAT_TOKENS_FILE for everything else.
async function grantPublicRead(strapi) {
  const role = await strapi.db
    .query('plugin::users-permissions.role')
    .findOne({ where: { type: 'public' } });
  for (const [uid, ct] of Object.entries(strapi.contentTypes)) {
    if (!uid.startsWith('api::')) continue;
    const actions = ct.kind === 'singleType' ? ['find'] : ['find', 'findOne'];
    for (const a of actions) {
      await strapi.db
        .query('plugin::users-permissions.permission')
        .create({ data: { action: `${uid}.${a}`, role: role.id } });
    }
  }
}

async function createTokens(strapi) {
  const service = strapi.service('admin::api-token');
  const tokens = {};
  for (const [name, type] of [
    ['full', 'full-access'],
    ['read', 'read-only'],
  ]) {
    const token = await service.create({ name: `compat-${name}`, type, lifespan: null });
    tokens[name] = token.accessKey;
  }
  fs.writeFileSync(process.env.COMPAT_TOKENS_FILE || '/data/tokens.json', JSON.stringify(tokens));
}

module.exports = {
  register() {},
  async bootstrap({ strapi }) {
    await grantPublicRead(strapi);
    await createTokens(strapi);
  },
};
