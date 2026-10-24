# Appends to a check file the data of every create and update the corpus
# sends that Strapi accepted, typed as ContentTypeInputs[uid] from api.d.ts.
# usage: typecheck-inputs.py <fixture dir> <corpus dir> <golden dir> <out>
import glob
import json
import os
import re
import sys

import yaml

fixture, corpus, golden, out = sys.argv[1:5]
name = os.path.basename(fixture.rstrip('/'))
# Accepted by Strapi, refused by the types on purpose.
refused = {'create-dz-component-not-allowed', 'writer-create-locale-in-body'}
plural, media = {}, set()
for f in glob.glob(os.path.join(fixture, 'src/api/*/content-types/*/schema.json')) + glob.glob(os.path.join(fixture, 'src/components/*/*.json')):
    s = json.load(open(f))
    media |= {n for n, a in s.get('attributes', {}).items() if a.get('type') == 'media'}
    if 'pluralName' in s.get('info', {}):
        parts = f.split(os.sep)
        plural[s['info']['pluralName']] = 'api::%s.%s' % (parts[-4], parts[-2])
        plural[s['info']['singularName']] = plural[s['info']['pluralName']]


def ids(v, key=None):
    """The corpus writes saved ids as "{{name}}": numbers once sent."""
    if isinstance(v, dict):
        return {k: ids(x, k) for k, x in v.items()}
    if isinstance(v, list):
        return [ids(x, key) for x in v]
    if isinstance(v, str) and re.fullmatch(r'\{\{\w+\}\}', v) and key in media | {'id'}:
        return 1
    return v


lines = ["import type { ContentTypeInputs } from './types/api';", '']
for c in sorted(glob.glob(os.path.join(corpus, name + '*.yaml'))):
    suite = os.path.basename(c)[:-5]
    cases = (yaml.safe_load(open(c)) or {}).get('cases') or []
    saved = {os.path.basename(g)[4:-5]: g for g in glob.glob(os.path.join(golden, suite, '*.json'))}
    for case in cases:
        body = case.get('body')
        m = re.fullmatch(r'/api/([a-z0-9-]+)(/[^?]*)?(\?.*)?', case.get('path', ''))
        if case.get('method') not in ('POST', 'PUT') or not m or m.group(1) not in plural:
            continue
        if not isinstance(body, dict) or not isinstance(body.get('data'), dict) or case['name'] not in saved:
            continue
        if json.load(open(saved[case['name']])).get('status') not in (200, 201):
            continue
        if case['name'] in refused:
            lines.append('// @ts-expect-error')
        lines.append("export const i%d: ContentTypeInputs['%s'] = %s;" % (len(lines), plural[m.group(1)], json.dumps(ids(body['data']), ensure_ascii=False)))
open(out, 'a').write('\n'.join(lines) + '\n')
print(len(lines) - 2, 'inputs')
