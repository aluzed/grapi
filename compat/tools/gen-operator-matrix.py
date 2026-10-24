"""Writes compat/querycases/basic--operators.yaml: every attribute operator
on every filterable attribute type of the basic fixture, with the seed of
querycases/basic.yaml. The query mode runs each case through Strapi's
db.query and grapi's engine on the same database."""
import os

root = os.path.join(os.path.dirname(__file__), '..')
src = open(os.path.join(root, 'querycases/basic.yaml')).read()
seed = src[:src.index('cases:')]

# attribute: (a value the seed holds, another one)
values = {
    'title': ('"Hello World"', '"Zebra 42"'),
    'slug': ('"hello"', '"zebra"'),
    'summary': ('"plain"', '"50%"'),
    'body': ('"x"', '"y"'),
    'contact': ('"bob@example.com"', '"Alice@Example.com"'),
    'views': ('42', '1000'),
    'bigCounter': ('"123456789012"', '"1"'),
    'rating': ('3', '0.5'),
    'price': ('10.5', '99.99'),
    'releaseDate': ('"2026-01-15"', '"2026-06-30"'),
    'releaseTime': ('"14:30:00"', '"08:05:09.120"'),
    'scheduledAt': ('"2026-03-01T10:00:00.000Z"', '"2026-01-01T00:00:00.000Z"'),
    'importedAt': ('"2026-02-01T00:00:00.000Z"', '"2026-01-01T00:00:00.000Z"'),
    'featured': ('true', 'false'),
    'stage': ('"review"', '"published"'),
}
scalar = ['$eq', '$eqi', '$ne', '$nei', '$lt', '$lte', '$gt', '$gte', '$contains', '$notContains',
          '$containsi', '$notContainsi', '$startsWith', '$startsWithi', '$endsWith', '$endsWithi']

lines = [seed.rstrip(), 'cases:']


def case(name, where):
    lines.append('  - { name: %s, uid: api::article.article, params: { where: %s, orderBy: [{ id: asc }] } }' % (name, where))


for attr, (a, b) in values.items():
    for op in scalar:
        case('%s-%s' % (attr, op[1:].lower()), '{ %s: { %s: %s } }' % (attr, op, a))
    case('%s-in' % attr, '{ %s: { $in: [%s, %s] } }' % (attr, a, b))
    case('%s-notin' % attr, '{ %s: { $notIn: [%s, %s] } }' % (attr, a, b))
    # Both orders: between does not sort its bounds.
    case('%s-between' % attr, '{ %s: { $between: [%s, %s] } }' % (attr, a, b))
    case('%s-between-reversed' % attr, '{ %s: { $between: [%s, %s] } }' % (attr, b, a))
    case('%s-null' % attr, '{ %s: { $null: true } }' % attr)
    case('%s-notnull' % attr, '{ %s: { $notNull: true } }' % attr)
    case('%s-not' % attr, '{ $not: { %s: { $eq: %s } } }' % (attr, a))
    case('%s-or' % attr, '{ $or: [{ %s: %s }, { %s: %s }] }' % (attr, a, attr, b))
    case('%s-and' % attr, '{ $and: [{ %s: { $notNull: true } }, { %s: { $ne: %s } }] }' % (attr, attr, a))

open(os.path.join(root, 'querycases/basic--operators.yaml'), 'w').write('\n'.join(lines) + '\n')
print(sum(1 for l in lines if l.startswith('  - { name:')), 'cases')
