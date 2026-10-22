// Writes qs.parse results with Strapi's settings for a fixed corpus, used by
// internal/httpapi tests. Run inside the reference image.
const qs = require(require.resolve('qs', { paths: ['/app/node_modules/@strapi/core'] }));
const settings = { strictNullHandling: true, arrayLimit: 100, depth: 20 };
const cases = [
  '', 'a=1', 'a', 'a=', 'a=1&a=2', 'a[]=1&a[]=2', 'a[0]=x&a[1]=y', 'a[1]=y&a[0]=x', 'a[2]=x',
  'a[101]=x', 'a[100]=x', 'a[b]=1&a[c]=2', 'a[b][c][d]=1', 'filters[title][$eq]=Hello%20World',
  'filters[$or][0][views]=1&filters[$or][1][stage]=draft', 'sort=title:asc,views:desc',
  'sort[0]=title&sort[1]=views:desc', 'populate=*', 'populate[0]=a&populate[1]=b',
  'populate[author][populate][0]=picture', 'populate[tags][fields][0]=name&populate[tags][sort]=name',
  'pagination[page]=2&pagination[pageSize]=5', 'fields[0]=title&fields=slug', 'a[b]=1&a=2', 'a=2&a[b]=1',
  'a[]=1&a[b]=2', 'q=a+b%2Bc', 'x=%E9t%C3%A9', 'x=%ZZ', 'a[b=1', 'a]=1', '__proto__[x]=1', 'a[__proto__]=1',
  'filters[title][$in][0]=a&filters[title][$in][1]=b&filters[title][$in][2]=c',
  'a[0][b]=1&a[0][c]=2&a[1][b]=3', 'a[0]=1&a[0]=2', 'a[][b]=1&a[][b]=2',
  'k=' + 'x'.repeat(5), 'deep' + '[a]'.repeat(25) + '=1', 'status=published&locale=fr',
  'filters[author][name][$null]', 'a[b][]=1&a[b][]=2&a[b][5]=3', '=x', '&&a=1&&',
];
const out = cases.map((c) => ({ input: c, output: qs.parse(c, settings) }));
process.stdout.write(JSON.stringify(out, null, 1));
