"""Core store round trip: writes the two input files, then compares a dump
with the entries one engine restored and the other read back."""
import json
import sys

TYPES = [("object", {"a": 1, "b": [True, "x"]}), ("string", "hello"), ("boolean", True), ("number", 42), ("array", [1, "two", None])]


def entries(prefix):
    return [{"key": f"plugin_compat_{prefix}_{t}", "value": json.dumps(v), "type": t, "environment": None, "tag": None} for t, v in TYPES]


def compare(prefix, dump_path, reader):
    with open(dump_path) as f:
        dumped = {e["key"]: e for e in json.load(f)}
    bad = []
    for e in entries(prefix):
        got = dumped.get(e["key"])
        if got is None or json.loads(got["value"]) != json.loads(e["value"]) or got["type"] != e["type"]:
            bad.append(f"{e['key']}: wrote {e['value']} ({e['type']}), read {got and got['value']} ({got and got['type']})")
    if bad:
        print(f"Core store written by the other engine, read by {reader}: DIFFERENT")
        print("\n".join(bad))
        return 1
    print(f"Core store written by the other engine, read by {reader}: identical ({len(TYPES)} types)")
    return 0


if __name__ == "__main__":
    if sys.argv[1] == "write":
        with open(sys.argv[3], "w") as f:
            json.dump(entries(sys.argv[2]), f)
    else:
        sys.exit(compare(sys.argv[2], sys.argv[3], sys.argv[4]))
