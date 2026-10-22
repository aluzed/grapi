// Loads the reference Strapi without its HTTP server, inserts the seed rows of
// a query cases file through db.query(uid).create, then runs every case
// through Strapi's query engine and writes the results.
// usage: node run-queries.js cases.json results.json (inside the reference image)
const fs = require('fs');
const { createStrapi, compileStrapi } = require(require.resolve('@strapi/strapi', { paths: ['/app'] }));

(async () => {
  const parsed = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
  const seed = parsed.seed || [];
  const cases = parsed.cases || [];
  const app = await createStrapi(await compileStrapi()).load();
  app.log.level = 'error';

  for (const s of seed) {
    await app.db.query(s.uid).create({ data: s.data });
  }

  const vars = {};
  const fill = (v) => {
    if (typeof v === 'string') {
      const whole = v.match(/^\{\{(\w+)\}\}$/);
      if (whole) return vars[whole[1]];
      return v.replace(/\{\{(\w+)\}\}/g, (_, k) => String(vars[k]));
    }
    if (Array.isArray(v)) return v.map(fill);
    if (v && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, fill(x)]));
    return v;
  };
  const pick = (obj, path) => path.split('.').reduce((o, k) => (o == null ? o : o[k]), obj);
  const out = [];
  for (const c of cases) {
    try {
      const p = fill(c.params || {});
      let result;
      if (c.op && c.op.startsWith('doc')) {
        const d = app.documents(c.uid);
        const { documentId, ...rest } = p;
        switch (c.op) {
          case 'docFindMany': result = await d.findMany(rest); break;
          case 'docFindFirst': result = await d.findFirst(rest); break;
          case 'docFindOne': result = await d.findOne({ documentId, ...rest }); break;
          case 'docCount': result = await d.count(rest); break;
          case 'docCreate': result = await d.create(rest); break;
          case 'docUpdate': result = await d.update({ documentId, ...rest }); break;
          case 'docDelete': result = await d.delete({ documentId, ...rest }); break;
          case 'docPublish': result = await d.publish({ documentId, ...rest }); break;
          case 'docUnpublish': result = await d.unpublish({ documentId, ...rest }); break;
          case 'docDiscardDraft': result = await d.discardDraft({ documentId, ...rest }); break;
          default: throw new Error('unknown op ' + c.op);
        }
      } else if (c.op === 'table') {
        result = await app.db.connection(c.uid).select('*').orderBy('id');
      } else {
        const q = app.db.query(c.uid);
        switch (c.op) {
          case 'count': result = await q.count(p); break;
          case 'findOne': result = await q.findOne(p); break;
          case 'create': result = await q.create(p); break;
          case 'createMany': result = await q.createMany(p); break;
          case 'update': result = await q.update(p); break;
          case 'updateMany': result = await q.updateMany(p); break;
          case 'delete': result = await q.delete(p); break;
          case 'deleteMany': result = await q.deleteMany(p); break;
          default: result = await q.findMany(p);
        }
      }
      for (const [k, path] of Object.entries(c.save || {})) vars[k] = pick(result, path);
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
