"""Compares the webhook deliveries of Strapi and grapi recorded by echo.js.

Ids, documentIds and timestamps differ by nature: they become aliases by
first appearance, on each side, before the comparison."""
import json
import sys

TIMES = {"createdAt", "updatedAt", "publishedAt", "firstPublishedAt"}


def load(path):
    with open(path) as f:
        return [json.loads(line) for line in f if line.strip()]


def normalize(deliveries):
    ids, docs = {}, {}

    def alias(table, prefix, v):
        return table.setdefault(str(v), f"{prefix}{len(table) + 1}")

    def walk(key, v):
        if isinstance(v, dict):
            return {k: walk(k, v[k]) for k in sorted(v)}
        if isinstance(v, list):
            return [walk(key, x) for x in v]
        if v is None:
            return None
        if key in TIMES:
            return "<ts>"
        if key == "id":
            return alias(ids, "#id", v)
        if key == "documentId":
            return alias(docs, "#doc", v)
        return v

    return [walk("", d) for d in deliveries]


def unordered_runs(deliveries):
    """Consecutive deliveries of one event (the draft and the published
    version of a deleted document) come in the database's row order, which
    Strapi leaves unspecified: they compare as a set."""
    out, run = [], []
    for d in deliveries + [None]:
        if run and (d is None or d["headers"].get("x-strapi-event") != run[0]["headers"].get("x-strapi-event")):
            out.extend(sorted(run, key=lambda x: json.dumps(x, sort_keys=True)))
            run = []
        if d is not None:
            run.append(d)
    return out


def main(strapi, grapi):
    a, b = unordered_runs(normalize(load(strapi))), unordered_runs(normalize(load(grapi)))
    events = lambda xs: [x["headers"].get("x-strapi-event") for x in xs]
    print("Strapi webhook events:", ", ".join(events(a)))
    if a == b:
        print(f"Webhook deliveries on grapi: identical ({len(a)})")
        return 0
    print("Webhook deliveries on grapi: DIFFERENT")
    print("grapi events:", ", ".join(events(b)))
    for i, (x, y) in enumerate(zip(a, b)):
        if x != y:
            print(f"#{i + 1} Strapi: {json.dumps(x, sort_keys=True)}")
            print(f"#{i + 1} grapi:  {json.dumps(y, sort_keys=True)}")
    return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1], sys.argv[2]))
