# Builds check.ts: each saved Strapi response of a create (posts, categories,
# themes, theme pages, media) typed as Data.ContentType<uid>, ids and dates
# restored from the normalized goldens. tsc then checks them against the
# types grapi generates.
import glob
import json
import os
import re
import sys

golden, out = sys.argv[1], sys.argv[2]
kinds = [('seed-post-', 'api::post.post'), ('seed-category-', 'api::category.category'),
         ('seed-theme-page-', 'api::theme-page.theme-page'), ('seed-theme-', 'api::theme.theme'),
         ('seed-medium-', 'api::medium.medium')]


def denorm(v):
    if isinstance(v, dict):
        return {k: denorm(x) for k, x in v.items()}
    if isinstance(v, list):
        return [denorm(x) for x in v]
    if isinstance(v, str):
        if m := re.fullmatch(r'#id(\d+)', v):
            return int(m.group(1))
        if m := re.fullmatch(r'#doc(\d+)', v):
            return 'doc' + m.group(1)
        if v == '<ts>':
            return '2026-10-23T12:00:00.000Z'
    return v


lines = ["import type { Data } from '@strapi/types';", '']
for f in sorted(glob.glob(os.path.join(golden, '*.json'))):
    name = os.path.basename(f)[4:-5]
    uid = next((u for p, u in kinds if name.startswith(p)), None)
    d = json.load(open(f))
    data = d.get('body', {}).get('data') if isinstance(d.get('body'), dict) else None
    if uid and d.get('status') in (200, 201) and isinstance(data, dict):
        lines.append("export const e%d: Data.ContentType<'%s'> = %s;" % (len(lines), uid, json.dumps(denorm(data), ensure_ascii=False)))
open(out, 'w').write('\n'.join(lines) + '\n')
print(len(lines) - 2, 'responses')
