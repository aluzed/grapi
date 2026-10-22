module.exports = () => ({
  serveAdminPanel: false,
  auth: { secret: 'compat-admin-jwt-secret' },
  apiToken: { salt: 'compat-api-token-salt' },
  transfer: { token: { salt: 'compat-transfer-token-salt' } },
  secrets: { encryptionKey: 'compat-encryption-key-32-bytes!!' },
});
