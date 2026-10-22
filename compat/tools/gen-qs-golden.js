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
  'a[01]=x', 'a[-1]=x', 'a[ 1]=x', 'a[1.5]=x', 'a[0x1]=x', 'a[1e2]=x', 'a.b=1', 'a[b.c]=1',
  'caf%C3%A9=1', 'a=1;b=2', 'a=%20', 'a=null', 'a=b=c', 'a[b]c=1', 'a[b][c]=1&a[b]=2',
  'a[b]=2&a[b][c]=1', 'a[][]=1', '[]=1', '[a]=1', 'a[]=', 'a[b][]=', 'a[]&a[]=2', 'a&a=2',
  'a[0]=1&a[2]=3', 'a[0]=1&a[100]=2', 'a[0]=1&a[101]=2', 'a[100]=1&a[0]=2', 'a[b][0]=1&a[b][1]=2&a[b][1]=3',
  'a=1&b=2&a=3', 'a[]=1&a=2', 'a=1&a[]=2', 'filters[slug][$eq]=premiers-pas-go&populate[0]=author&populate[1]=author.picture',
  'filters[$and][0][a][$eq]=1&filters[$and][1][b][$ne]=2', 'pagination[limit]=-1', 'pagination[start]=10&pagination[limit]=5',
  'sort=createdAt%3Adesc', 'a=%E2%82%AC', 'a=%F0%9F%98%80', 'a=%C3', 'a=%25', 'a%5Bb%5D=1', 'a%5bb%5d=1',
  'a[b%5Dc]=1', 'a[%5B]=1', 'deep' + '[a]'.repeat(20) + '=1', 'deep' + '[a]'.repeat(21) + '=1',
  'a[' + 'x'.repeat(30) + ']=1', 'locale=fr&status=draft&filters[title][$containsi]=Été',
  'fields[0]=title&fields[1]=description&fields[2]=slug&fields[3]=publishedAt',
  'populate[cover][fields][0]=url&populate[cover][fields][1]=formats', 'a[b]=1&a[b]=2',
  'x[y][]=1&x[y][]=2&x[y][]=3', 'k=v&k', 'k&k=v', 'a=+', 'a=%2B%2B', 'a[toString]=1', 'hasOwnProperty=1',
  Array.from({ length: 1005 }, (_, i) => `p${i}=${i}`).join('&'),
];
const out = cases.map((c) => ({ input: c, output: qs.parse(c, settings) }));
process.stdout.write(JSON.stringify(out, null, 1));
