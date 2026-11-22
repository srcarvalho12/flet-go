"""
Extracts a machine-readable spec of Flet controls, value types, events and
enums from the Flet Python SDK sources, using only the standard library.

Usage:
    python3 -I tools/specgen/extract.py <flet-repo> spec/flet.json

It also copies the icon tables (icons.json, cupertino_icons.json) next to
the output file.

<flet-repo> is a checkout of https://github.com/flet-dev/flet at the tag that
matches the client version pinned in internal/client (e.g. v1.1.2).
"""

import ast
import json
import pathlib
import subprocess
import sys

CONTROL_DECORATORS = {"control"}
VALUE_DECORATORS = {"value", "dataclass", "_lazy_value"}
ENUM_BASES = {"Enum", "IntEnum", "StrEnum", "IntFlag", "Flag"}


def unparse(node):
    return ast.unparse(node) if node is not None else None


def decorator_name(d):
    if isinstance(d, ast.Call):
        d = d.func
    if isinstance(d, ast.Name):
        return d.id
    if isinstance(d, ast.Attribute):
        return d.attr
    return None


def base_name(b):
    # Event[EventControlType] -> Event, flet.Control -> Control
    if isinstance(b, ast.Subscript):
        b = b.value
    if isinstance(b, ast.Attribute):
        return b.attr
    if isinstance(b, ast.Name):
        return b.id
    return unparse(b)


def first_sentence_doc(text):
    if not text:
        return ""
    return text.strip()


def literal(node):
    try:
        return ast.literal_eval(node)
    except Exception:
        return None


def parse_field(stmt, next_stmt):
    """Returns a field dict for an annotated class attribute, or None."""
    if not isinstance(stmt.target, ast.Name):
        return None
    name = stmt.target.id
    ann = unparse(stmt.annotation)
    if ann.startswith(("ClassVar", "InitVar")):
        return None

    f = {"name": name, "type": ann, "default": None, "metadata": {}}
    v = stmt.value
    if isinstance(v, ast.Call) and decorator_name(v) in ("field", "Prop"):
        for kw in v.keywords:
            if kw.arg == "default":
                f["default"] = unparse(kw.value)
            elif kw.arg == "default_factory":
                f["default_factory"] = unparse(kw.value)
            elif kw.arg == "metadata":
                f["metadata"] = literal(kw.value) or {}
            elif kw.arg == "init" and literal(kw.value) is False:
                f["init"] = False
    elif isinstance(v, ast.Call) and decorator_name(v) == "skip_field":
        f["skip"] = True
    elif v is not None:
        f["default"] = unparse(v)

    if (
        isinstance(next_stmt, ast.Expr)
        and isinstance(next_stmt.value, ast.Constant)
        and isinstance(next_stmt.value.value, str)
    ):
        f["doc"] = first_sentence_doc(next_stmt.value.value)
    return f


def parse_enum(cls):
    members = []
    body = cls.body
    for i, stmt in enumerate(body):
        if not isinstance(stmt, ast.Assign) or len(stmt.targets) != 1:
            continue
        t = stmt.targets[0]
        if not isinstance(t, ast.Name) or t.id.startswith("_"):
            continue
        m = {"name": t.id, "value": literal(stmt.value)}
        nxt = body[i + 1] if i + 1 < len(body) else None
        if (
            isinstance(nxt, ast.Expr)
            and isinstance(nxt.value, ast.Constant)
            and isinstance(nxt.value.value, str)
        ):
            m["doc"] = first_sentence_doc(nxt.value.value)
        members.append(m)
    return members


def parse_class(cls, module):
    decos = {decorator_name(d): d for d in cls.decorator_list}
    bases = [base_name(b) for b in cls.bases]
    entry = {
        "name": cls.name,
        "module": module,
        "bases": bases,
        "doc": first_sentence_doc(ast.get_docstring(cls)),
    }

    if ENUM_BASES & set(bases):
        entry["kind"] = "enum"
        entry["members"] = parse_enum(cls)
        return entry

    if CONTROL_DECORATORS & decos.keys():
        entry["kind"] = "control"
        d = decos["control"]
        if isinstance(d, ast.Call) and d.args:
            entry["dart_name"] = literal(d.args[0])
    elif VALUE_DECORATORS & decos.keys():
        entry["kind"] = "value"
    else:
        entry["kind"] = "class"

    fields = []
    body = cls.body
    for i, stmt in enumerate(body):
        if isinstance(stmt, ast.AnnAssign):
            nxt = body[i + 1] if i + 1 < len(body) else None
            f = parse_field(stmt, nxt)
            if f:
                fields.append(f)
    entry["fields"] = fields
    return entry


def parse_aliases(tree):
    """Module-level type aliases, e.g. `ColorValue = Union[str, Colors]`."""
    out = {}
    for stmt in tree.body:
        target, value = None, None
        if isinstance(stmt, ast.Assign) and len(stmt.targets) == 1:
            target, value = stmt.targets[0], stmt.value
        elif isinstance(stmt, ast.AnnAssign) and stmt.value is not None:
            if unparse(stmt.annotation) in ("TypeAlias", "typing.TypeAlias"):
                target, value = stmt.target, stmt.value
        if not isinstance(target, ast.Name):
            continue
        if isinstance(value, (ast.Subscript, ast.BinOp)) or (
            isinstance(value, ast.Name) and value.id[:1].isupper()
        ):
            out[target.id] = unparse(value)
        elif (
            isinstance(value, ast.Call)
            and decorator_name(value) == "TypeVar"
        ):
            out[target.id] = "TypeVar"
    return out


def main():
    repo = pathlib.Path(sys.argv[1]).resolve()
    out = pathlib.Path(sys.argv[2])
    src = repo / "sdk/python/packages/flet/src"

    version = subprocess.run(
        ["git", "-C", str(repo), "describe", "--tags"],
        capture_output=True,
        text=True,
    ).stdout.strip()

    classes, aliases = [], {}
    for path in sorted((src / "flet/controls").rglob("*.py")):
        module = ".".join(path.relative_to(src).with_suffix("").parts)
        tree = ast.parse(path.read_text(encoding="utf-8"))
        for node in tree.body:
            if isinstance(node, ast.ClassDef):
                classes.append(parse_class(node, module))
        for k, v in parse_aliases(tree).items():
            aliases[k] = v

    spec = {"flet_version": version, "classes": classes, "aliases": aliases}
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(spec, indent=1, ensure_ascii=False) + "\n")

    # Icon name -> code point tables, used to generate the icons package.
    controls = src / "flet/controls"
    for name, rel in (
        ("icons.json", "material/icons.json"),
        ("cupertino_icons.json", "cupertino/cupertino_icons.json"),
    ):
        (out.parent / name).write_bytes((controls / rel).read_bytes())

    kinds = {}
    for c in classes:
        kinds[c["kind"]] = kinds.get(c["kind"], 0) + 1
    print(f"{version}: {kinds}, {len(aliases)} aliases -> {out}")


if __name__ == "__main__":
    main()
