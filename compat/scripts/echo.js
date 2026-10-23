// Webhook echo receiver for the bench: every POST is appended, with its
// path and the headers that matter, to /out/<path>.jsonl.
const http = require('http');
const fs = require('fs');
const keep = ['x-strapi-event', 'content-type', 'x-test'];
http
  .createServer((req, res) => {
    let body = '';
    req.on('data', (c) => (body += c));
    req.on('end', () => {
      const headers = Object.fromEntries(keep.filter((k) => req.headers[k] !== undefined).map((k) => [k, req.headers[k]]));
      let parsed = body;
      try {
        parsed = JSON.parse(body);
      } catch {}
      fs.appendFileSync(`/out${req.url.replace(/[^a-z/]/g, '')}.jsonl`, JSON.stringify({ headers, body: parsed }) + '\n');
      res.writeHead(200).end();
    });
  })
  .listen(9000);
