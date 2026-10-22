// Loads the reference Strapi without its HTTP server, inserts the seed rows of
// a query cases file through db.query(uid).create, then runs every case
// through Strapi's query engine and writes the results.
// usage: node run-queries.js cases.json results.json (inside the reference image)
const fs = require('fs');
const { createStrapi, compileStrapi } = require(require.resolve('@strapi/strapi', { paths: ['/app'] }));

(async () => {
  const { seed = [], cases = [] } = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
  const app = await createStrapi(await compileStrapi()).load();
  app.log.level = 'error';

  for (const s of seed) {
    await app.db.query(s.uid).create({ data: s.data });
  }

  const out = [];
  for (const c of cases) {
    try {
      const q = app.db.query(c.uid);
      let result;
      if (c.op === 'count') {
        result = await q.count(c.params || {});
      } else if (c.op === 'findOne') {
        result = await q.findOne(c.params || {});
      } else {
        result = await q.findMany(c.params || {});
      }
      out.push({ name: c.name, result });
    } catch (e) {
      out.push({ name: c.name, error: e.message });
    }
  }
  fs.writeFileSync(process.argv[3], JSON.stringify(out, null, 1));
  await app.destroy();
  process.exit(0);
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
