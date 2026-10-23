const fs = require('fs');

// A fixture may lower formidable's maxFileSize in compat.json (bodyMaxFileSize).
const maxFileSize = () => {
  const file = process.env.COMPAT_FIXTURE_CONFIG || '/fixture/compat.json';
  const n = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, 'utf8')).bodyMaxFileSize : undefined;
  return n || 200 * 1024 * 1024;
};

module.exports = [
  'strapi::logger',
  'strapi::errors',
  'strapi::security',
  'strapi::cors',
  'strapi::poweredBy',
  'strapi::query',
  { name: 'strapi::body', config: { formidable: { maxFileSize: maxFileSize() } } },
  'strapi::session',
  'strapi::favicon',
  'strapi::public',
];
