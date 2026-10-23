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

// Optional per-fixture settings: { "locales": [{ "code", "name" }], "publicPermissions": ["<action>"] }
function readFixtureConfig() {
  const file = process.env.COMPAT_FIXTURE_CONFIG || '/fixture/compat.json';
  return fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, 'utf8')) : {};
}

async function createLocales(strapi, locales) {
  const service = strapi.plugin('i18n').service('locales');
  for (const { code, name } of locales) {
    if (!(await service.findByCode(code))) await service.create({ code, name });
  }
}

async function grantPublic(strapi, actions) {
  const role = await strapi.db
    .query('plugin::users-permissions.role')
    .findOne({ where: { type: 'public' } });
  for (const action of actions) {
    await strapi.db
      .query('plugin::users-permissions.permission')
      .create({ data: { action, role: role.id } });
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
  // A fixture's row rules (rules.js), when the bench runs its Go port too.
  register({ strapi }) {
    if (process.env.COMPAT_ROW_RULES === '1' && fs.existsSync('/fixture/rules.js')) require('/fixture/rules.js')(strapi);
  },
  async bootstrap({ strapi }) {
    const config = readFixtureConfig();
    await createLocales(strapi, config.locales || []);
    await grantPublicRead(strapi);
    await grantPublic(strapi, config.publicPermissions || []);
    await createTokens(strapi);
  },
};
