// Writes slugify outputs for a fixed corpus, used by internal/slug tests.
const slugify = require('@sindresorhus/slugify');
const inputs = [
  'Hello World', 'Ça va très bien', 'Straße', 'Ærøskøbing', 'foo&bar', 'fooBarBaz',
  'XMLHttpRequest2', '__private', '  leading--and--trailing  ', '日本語', 'emoji 🦄 unicorn',
  '♥ love', 'a\\b', 'Đồng Hới', 'Łódź', 'İstanbul', 'ﬁancé', 'é', 'Version2Beta',
  'ABCDef', 'multiple___under', 'x.y.z', '100% sure', "C'est l'été", 'Ünïcödé Çhàrs', 'ŒUVRE',
  'my-api-v2', 'Theme Page', 'React.js & Node.js', 'already-a-slug', '', '---', 'A1B2C3', 'iPhone 15 Pro',
];
const optionSets = [
  {},
  { separator: '_' },
  { separator: '_', lowercase: false, decamelize: false },
  { lowercase: false },
  { decamelize: false },
  { customReplacements: [['&', ' et '], ['%', ' pct ']] },
  { preserveLeadingUnderscore: true },
  { separator: '--' },
  { separator: '' },
];
const cases = [];
for (const options of optionSets) {
  for (const input of inputs) cases.push({ input, options, output: slugify(input, options) });
}
process.stdout.write(JSON.stringify(cases, null, 1) + '\n');
