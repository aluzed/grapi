// Fixtures only ship schema.json files: generate the default Strapi
// router/controller/service for every content type that lacks them.
const fs = require('fs');
const path = require('path');

const apiDir = path.join(process.cwd(), 'src', 'api');
if (fs.existsSync(apiDir)) {
  for (const api of fs.readdirSync(apiDir)) {
    const ctDir = path.join(apiDir, api, 'content-types');
    if (!fs.existsSync(ctDir)) continue;
    for (const ct of fs.readdirSync(ctDir)) {
      const uid = `api::${api}.${ct}`;
      const files = {
        routes: `module.exports = require('@strapi/strapi').factories.createCoreRouter('${uid}');\n`,
        controllers: `module.exports = require('@strapi/strapi').factories.createCoreController('${uid}');\n`,
        services: `module.exports = require('@strapi/strapi').factories.createCoreService('${uid}');\n`,
      };
      for (const [dir, body] of Object.entries(files)) {
        const file = path.join(apiDir, api, dir, `${ct}.js`);
        if (fs.existsSync(file)) continue;
        fs.mkdirSync(path.dirname(file), { recursive: true });
        fs.writeFileSync(file, body);
      }
    }
  }
}
