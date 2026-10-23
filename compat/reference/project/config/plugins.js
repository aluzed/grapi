const fs = require('fs');

// A fixture may set users-permissions options in compat.json (upConfig).
const upConfig = () => {
  const file = process.env.COMPAT_FIXTURE_CONFIG || '/fixture/compat.json';
  return fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, 'utf8')).upConfig || {} : {};
};

module.exports = () => ({
  'users-permissions': { config: { jwtSecret: 'compat-users-jwt-secret', ...upConfig() } },
});
