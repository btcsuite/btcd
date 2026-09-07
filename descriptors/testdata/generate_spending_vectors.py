#!/usr/bin/env python3
"""Build byte-level spending fixtures without either descriptor implementation.

This is test-data tooling, not wallet or cryptographic production code. Scripts
and stacks are specified explicitly below using BIP16/65/112/141/341/342/379.
Run with --check to verify the committed JSON has not drifted.
"""

import argparse
import hashlib
import hmac
import itertools
import json
from pathlib import Path

P = 2**256 - 2**32 - 977
N = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141
G = (
    0x79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798,
    0x483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8,
)


def add(a, b):
    """Add affine curve points for deterministic public test keys."""
    if a is None:
        return b
    if b is None:
        return a
    x, y = a
    u, v = b
    if x == u and (y != v or y == 0):
        return None
    m = ((3 * x * x) * pow(2 * y, -1, P) if a == b else (v - y) * pow(u - x, -1, P)) % P
    z = (m * m - x - u) % P
    return z, (m * (x - z) - y) % P


def mul(n, point=G):
    """Multiply a public test point by an integer."""
    result = None
    while n:
        if n & 1:
            result = add(result, point)
        point = add(point, point)
        n >>= 1
    return result


def sha(data):
    return hashlib.sha256(data).digest()


def h160(data):
    return hashlib.new("ripemd160", sha(data)).digest()


def tagged(tag, data):
    h = sha(tag.encode())
    return sha(h + h + data)


def compact(n):
    if n < 253:
        return bytes([n])
    if n <= 65535:
        return b"\xfd" + n.to_bytes(2, "little")
    return b"\xfe" + n.to_bytes(4, "little")


def push(data):
    """Encode a minimal Script push, not a witness CompactSize prefix."""
    n = len(data)
    if not data:
        return b"\x00"
    if n == 1 and 1 <= data[0] <= 16:
        return bytes([0x50 + data[0]])
    if data == b"\x81":
        return b"\x4f"
    if n < 76:
        return bytes([n]) + data
    if n <= 255:
        return b"\x4c" + bytes([n]) + data
    return b"\x4d" + n.to_bytes(2, "little") + data


def num(n):
    b = n.to_bytes(max(1, (n.bit_length() + 7) // 8), "little")
    if b[-1] & 128:
        b += b"\x00"
    return push(b) if n else b"\x00"


def ws(stack):
    return len(compact(len(stack))) + sum(len(compact(len(x))) + len(x) for x in stack)


KEYS = []
for scalar in range(1, 254):
    x, y = mul(scalar)
    KEYS.append(bytes([2 + (y & 1)]) + x.to_bytes(32, "big"))


def signature(i, tap=False, size=None):
    """Return distinct, structurally valid fixture signatures (not tx signatures)."""
    if tap:
        s = bytes([i + 1]) * 64
        return s + b"\x01" if size == 65 else s
    # DER: 33-byte positive R and 32-byte low S, plus SIGHASH_ALL: 72 bytes.
    return (
        bytes.fromhex("304502210080")
        + bytes([i + 1]) * 31
        + bytes.fromhex("022011")
        + bytes([i + 1]) * 31
        + b"\x01"
    )


def pk(i, tap=False):
    return KEYS[i][1:] if tap else KEYS[i]


def pkscript(i, tap=False):
    return push(pk(i, tap)) + b"\xac"


def pkhscript(i, tap=False):
    return b"\x76\xa9" + push(h160(pk(i, tap))) + b"\x88\xac"


def leafhash(script):
    return tagged("TapLeaf", b"\xc0" + compact(len(script)) + script)


def tapoutput(scripts):
    """Construct the output and proofs for an explicit binary tree of scripts."""

    def tree(t):
        if isinstance(t, bytes):
            return leafhash(t), [(t, b"")]
        a, aa = tree(t[0])
        b, bb = tree(t[1])
        return tagged("TapBranch", min(a, b) + max(a, b)), [
            (s, p + b) for s, p in aa
        ] + [(s, p + a) for s, p in bb]

    root, leaves = tree(scripts) if scripts is not None else (b"", [])
    internal = pk(4, True)
    x = int.from_bytes(internal, "big")
    y = pow((x * x * x + 7) % P, (P + 1) // 4, P)
    q = add(
        (x, y if y % 2 == 0 else P - y),
        mul(int.from_bytes(tagged("TapTweak", internal + root), "big")),
    )
    controls = {s: bytes([0xC0 + (q[1] & 1)]) + internal + p for s, p in leaves}
    return b"\x51\x20" + q[0].to_bytes(32, "big"), controls


CASES = []
DETAILS = {}


def case(
    name,
    expr,
    script,
    stack,
    signers,
    kind="wsh",
    tap=False,
    tx=None,
    preimages=(),
    asset_signers=None,
    tree=None,
    tree_expr=None,
):
    """Add a case with independently assembled scripts, sizes, and final bytes."""
    assets = {"ecdsa": [], "tap_key": [], "tap_leaf": [], "preimages": []}
    data = {"ecdsa": {}, "tap_key": {}, "tap_leaf": [], "preimages": []}
    signers = list(signers)
    available = list(asset_signers if asset_signers is not None else signers)
    if tap:
        scripts = tree if tree is not None else script
        output, controls = tapoutput(scripts)
        desc = (
            f"tr({pk(4, True).hex()}"
            + ("," + (tree_expr or expr) if script is not None else "")
            + ")"
        )
        for i in available:
            if script is None:
                assets["tap_key"].append({"key": pk(4, True).hex(), "size": 64})
                data["tap_key"][pk(4, True).hex()] = signature(i, True).hex()
            else:
                entry = {"key": pk(i, True).hex(), "leaf_hash": leafhash(script).hex()}
                assets["tap_leaf"].append(dict(entry, size=64))
                data["tap_leaf"].append(dict(entry, signature=signature(i, True).hex()))
        witness = stack + ([script, controls[script]] if script is not None else [])
        script_sig = b""
    else:
        assets["ecdsa"] = [pk(i).hex() for i in available]
        data["ecdsa"] = {pk(i).hex(): signature(i).hex() for i in available}
        witness, script_sig = [], b""
        if kind == "bare":
            desc, output = expr, script
            script_sig = b"".join(map(push, stack))
        elif kind == "sh":
            desc, output = f"sh({expr})", b"\xa9" + push(h160(script)) + b"\x87"
            script_sig = b"".join(map(push, stack + [script]))
        elif kind in ("wpkh", "sh-wpkh"):
            redeem = b"\x00" + push(h160(pk(signers[0])))
            desc = f"wpkh({pk(signers[0]).hex()})"
            output, witness = redeem, stack
            if kind == "sh-wpkh":
                desc, output, script_sig = (
                    f"sh({desc})",
                    b"\xa9" + push(h160(redeem)) + b"\x87",
                    push(redeem),
                )
        else:
            redeem = b"\x00" + push(sha(script))
            desc, output, witness = f"wsh({expr})", redeem, stack + [script]
            if kind == "sh-wsh":
                desc, output, script_sig = (
                    f"sh({desc})",
                    b"\xa9" + push(h160(redeem)) + b"\x87",
                    push(redeem),
                )
    for function, digest, value in preimages:
        entry = {"function": function, "hash": digest.hex()}
        assets["preimages"].append(entry)
        data["preimages"].append(dict(entry, preimage=value.hex()))
    wsize = ws(witness) if witness else 0
    ssize = len(compact(len(script_sig))) + len(script_sig)
    item = {
        "id": name,
        "descriptor": desc,
        "multipath_index": 0,
        "derivation_index": 0,
        "tx": tx or {},
        "assets": assets,
        "expected_plan": {
            "witness_size": wsize,
            "script_sig_size": ssize,
            "weight": wsize + 4 * ssize,
        },
        "script_pubkey": output.hex(),
        "completions": [
            dict(
                id="available",
                data=data,
                expected={
                    "witness": [x.hex() for x in witness],
                    "script_sig": script_sig.hex(),
                },
            )
        ],
    }
    # Every positive also tests the same fixed plan with all data missing.
    if signers or preimages:
        item["completions"].append(
            {"id": "missing", "data": {}, "expected": {"error": "satisfy"}}
        )
    CASES.append(item)
    DETAILS[name] = (script, kind, tap)
    return item


def negative(base, suffix, update):
    item = json.loads(json.dumps(base))
    item["id"] += "-" + suffix
    item["expected_plan"] = {"error": "plan"}
    item["completions"] = []
    update(item)
    CASES.append(item)


def generate():
    """Enumerate script families and spending choices, including absent assets."""
    CASES.clear()
    DETAILS.clear()
    for kind in ("bare", "sh", "wsh", "sh-wsh"):
        for frag in ("pk", "pkh"):
            script = pkscript(0) if frag == "pk" else pkhscript(0)
            stack = [signature(0)] + ([pk(0)] if frag == "pkh" else [])
            v = case(
                f"{kind}-{frag}", f"{frag}({pk(0).hex()})", script, stack, [0], kind
            )
            negative(v, "no-assets", lambda x: x.update(assets={}))
    for kind in ("wpkh", "sh-wpkh"):
        v = case(kind, "", b"", [signature(0), pk(0)], [0], kind)
        negative(v, "no-assets", lambda x: x.update(assets={}))

    # Cross Script PUSHDATA1/PUSHDATA2 and CompactSize 252/253 boundaries.
    for tap, counts in ((False, (7, 8, 15, 16, 20)), (True, (7, 8, 20, 251, 252, 253))):
        for count in counts:
            for threshold in (1, count):
                order = list(range(count))
                fn = "multi_a" if tap else "multi"
                expr = (
                    f"{fn}({threshold},"
                    + ",".join(pk(i, tap).hex() for i in order)
                    + ")"
                )
                script = (
                    b"".join(
                        push(pk(i, True)) + bytes([0xAC if i == 0 else 0xBA])
                        for i in order
                    )
                    + num(threshold)
                    + b"\x9c"
                    if tap
                    else num(threshold)
                    + b"".join(push(pk(i)) for i in order)
                    + num(count)
                    + b"\xae"
                )
                chosen = order[:threshold]
                stack = (
                    [
                        signature(i, True) if i in chosen else b""
                        for i in reversed(order)
                    ]
                    if tap
                    else [b""] + [signature(i) for i in chosen]
                )
                for kind in (("tr",) if tap else ("wsh", "sh-wsh")):
                    v = case(
                        f"{kind}-boundary-{threshold}-of-{count}",
                        expr,
                        script,
                        stack,
                        chosen,
                        kind,
                        tap,
                    )
                    negative(
                        v,
                        "one-missing",
                        lambda x: x["assets"]["tap_leaf" if tap else "ecdsa"].pop(),
                    )

    for tap in (False, True):
        for kind in (("tr",) if tap else ("bare", "sh", "wsh", "sh-wsh")):
            for sorted_keys in (False, True):
                for threshold in (1, 2, 3):
                    order = (
                        sorted(range(3), key=lambda i: pk(i, tap))
                        if sorted_keys
                        else [2, 0, 1]
                    )
                    fn = ("sortedmulti" if sorted_keys else "multi") + (
                        "_a" if tap else ""
                    )
                    expr = (
                        f"{fn}({threshold},"
                        + ",".join(pk(i, tap).hex() for i in [2, 0, 1])
                        + ")"
                    )
                    if tap:
                        script = (
                            b"".join(
                                push(pk(i, True)) + bytes([0xAC if j == 0 else 0xBA])
                                for j, i in enumerate(order)
                            )
                            + num(threshold)
                            + b"\x9c"
                        )
                    else:
                        script = (
                            num(threshold)
                            + b"".join(push(pk(i)) for i in order)
                            + num(3)
                            + b"\xae"
                        )
                    for chosen in itertools.combinations(range(3), threshold):
                        stack = (
                            [
                                signature(i, True) if i in chosen else b""
                                for i in reversed(order)
                            ]
                            if tap
                            else [b""] + [signature(i) for i in order if i in chosen]
                        )
                        v = case(
                            f"{kind}-{fn}-{threshold}-" + "".join(map(str, chosen)),
                            expr,
                            script,
                            stack,
                            chosen,
                            kind,
                            tap,
                        )
                        negative(
                            v,
                            "short",
                            lambda x: x["assets"].update(
                                {"tap_leaf" if tap else "ecdsa": []}
                            ),
                        )

        for kind in (("tr",) if tap else ("wsh", "sh-wsh", "sh")):
            a, b, c = [f"pk({pk(i,tap).hex()})" for i in range(3)]
            sa, sb, sc = [pkscript(i, tap) for i in range(3)]
            sig = lambda i: signature(i, tap)
            policies = [
                (
                    "and-v",
                    f"and_v(v:{a},{b})",
                    sa[:-1] + b"\xad" + sb,
                    [("both", [sig(1), sig(0)], [0, 1])],
                ),
                (
                    "and-b",
                    f"and_b({a},s:{b})",
                    sa + b"\x7c" + sb + b"\x9a",
                    [("both", [sig(1), sig(0)], [0, 1])],
                ),
                (
                    "or-i",
                    f"or_i({a},{b})",
                    b"\x63" + sa + b"\x67" + sb + b"\x68",
                    [("left", [sig(0), b"\x01"], [0]), ("right", [sig(1), b""], [1])],
                ),
                (
                    "or-d",
                    f"or_d({a},{b})",
                    sa + b"\x73\x64" + sb + b"\x68",
                    [("left", [sig(0)], [0]), ("right", [sig(1), b""], [1])],
                ),
                (
                    "or-b",
                    f"or_b({a},s:{b})",
                    sa + b"\x7c" + sb + b"\x9b",
                    [("left", [b"", sig(0)], [0]), ("right", [sig(1), b""], [1])],
                ),
                (
                    "or-c",
                    f"t:or_c({a},v:{b})",
                    sa + b"\x64" + sb[:-1] + b"\xad\x68\x51",
                    [("left", [sig(0)], [0]), ("right", [sig(1), b""], [1])],
                ),
                (
                    "andor",
                    f"andor({a},{b},{c})",
                    sa + b"\x64" + sc + b"\x67" + sb + b"\x68",
                    [("yes", [sig(1), sig(0)], [0, 1]), ("no", [sig(2), b""], [2])],
                ),
                (
                    "thresh",
                    f"thresh(2,{a},s:{b},s:{c})",
                    sa + b"\x7c" + sb + b"\x93\x7c" + sc + b"\x93\x52\x87",
                    [
                        ("ab", [b"", sig(1), sig(0)], [0, 1]),
                        ("ac", [sig(2), b"", sig(0)], [0, 2]),
                        ("bc", [sig(2), sig(1), b""], [1, 2]),
                    ],
                ),
                (
                    "j",
                    f"j:{a}",
                    b"\x82\x92\x63" + sa + b"\x68",
                    [("yes", [sig(0)], [0])],
                ),
                ("n", f"n:{a}", sa + b"\x92", [("yes", [sig(0)], [0])]),
                (
                    "a",
                    f"and_b({a},a:{b})",
                    sa + b"\x6b" + sb + b"\x6c\x9a",
                    [("both", [sig(1), sig(0)], [0, 1])],
                ),
                (
                    "and-n",
                    f"and_n({a},{b})",
                    sa + b"\x64\x00\x67" + sb + b"\x68",
                    [("both", [sig(1), sig(0)], [0, 1])],
                ),
                ("t", f"tv:{a}", sa[:-1] + b"\xad\x51", [("yes", [sig(0)], [0])]),
                (
                    "u",
                    f"u:{a}",
                    b"\x63" + sa + b"\x67\x00\x68",
                    [("yes", [sig(0), b"\x01"], [0])],
                ),
                (
                    "l",
                    f"l:{a}",
                    b"\x63\x00\x67" + sa + b"\x68",
                    [("yes", [sig(0), b""], [0])],
                ),
            ]
            for name, expr, script, paths in policies:
                for path, stack, chosen in paths:
                    v = case(
                        f"{kind}-{name}-{path}", expr, script, stack, chosen, kind, tap
                    )
                    if kind == "sh" and name in ("or-i", "u", "l"):
                        v["expected_plan"] = {"error": "parse"}
                        v["completions"] = []
                        continue
                    negative(v, "no-assets", lambda x: x.update(assets={}))
            if kind != "sh":
                v = case(
                    f"{kind}-d-older",
                    f"and_v(v:{a},dv:older(17))",
                    sa[:-1] + b"\xad\x76\x63" + num(17) + b"\xb2\x69\x68",
                    [b"\x01", sig(0)],
                    [0],
                    kind,
                    tap,
                    tx={"version": 2, "sequence": 17},
                )
                negative(v, "no-context", lambda x: x.update(tx={}))
            for function in ("sha256", "hash256", "ripemd160", "hash160"):
                preimage = bytes(range(32))
                digest = {
                    "sha256": sha,
                    "hash256": lambda x: sha(sha(x)),
                    "ripemd160": lambda x: hashlib.new("ripemd160", x).digest(),
                    "hash160": h160,
                }[function](preimage)
                op = {
                    "sha256": 0xA8,
                    "hash256": 0xAA,
                    "ripemd160": 0xA6,
                    "hash160": 0xA9,
                }[function]
                hs = b"\x82\x01\x20\x88" + bytes([op]) + push(digest) + b"\x87"
                v = case(
                    f"{kind}-{function}",
                    f"and_v(v:{a},{function}({digest.hex()}))",
                    sa[:-1] + b"\xad" + hs,
                    [preimage, sig(0)],
                    [0],
                    kind,
                    tap,
                    preimages=[(function, digest, preimage)],
                )
                negative(v, "no-preimage", lambda x: x["assets"].update(preimages=[]))
                for size in (0, 31, 33):
                    data = json.loads(json.dumps(v["completions"][0]["data"]))
                    data["preimages"][0]["preimage"] = (b"\x42" * size).hex()
                    v["completions"].append(
                        dict(
                            id=f"preimage-size-{size}",
                            data=data,
                            expected={"error": "satisfy"},
                        )
                    )
            for fn, op in (("older", 0xB2), ("after", 0xB1)):
                for value in (
                    (1, 16, 17, 65535, 4194305)
                    if fn == "older"
                    else (1, 16, 17, 499999999, 500000000)
                ):
                    tx = {
                        "version": 2,
                        "sequence": value if fn == "older" else 0xFFFFFFFE,
                        "lock_time": value if fn == "after" else 0,
                    }
                    v = case(
                        f"{kind}-{fn}-{value}",
                        f"and_v(v:{a},{fn}({value}))",
                        sa[:-1] + b"\xad" + num(value) + bytes([op]),
                        [sig(0)],
                        [0],
                        kind,
                        tap,
                        tx,
                    )
                    negative(v, "no-context", lambda x: x.update(tx={}))
                    negative(v, "final", lambda x: x["tx"].update(sequence=0xFFFFFFFF))
                    if fn == "older":
                        for version in (0, 1, -1):
                            negative(
                                v,
                                f"version-{version}",
                                lambda x, v=version: x["tx"].update(version=v),
                            )
                        negative(
                            v,
                            "disabled",
                            lambda x: x["tx"].update(sequence=value | 0x80000000),
                        )
                        negative(
                            v,
                            "unit",
                            lambda x: x["tx"].update(sequence=value ^ 0x400000),
                        )
                    negative(
                        v,
                        "below",
                        lambda x: x["tx"].update(
                            {"sequence" if fn == "older" else "lock_time": value - 1}
                        ),
                    )

    v = case("tr-key", "", None, [signature(4, True)], [4], tap=True)
    negative(v, "no-assets", lambda x: x.update(assets={}))
    for size in (0, 1, 63, 66, 0xFFFFFFFF):
        negative(
            v,
            f"asset-size-{size}",
            lambda x, s=size: x["assets"]["tap_key"][0].update(size=s),
        )
    for size in (0, 1, 63, 66):
        data = {"tap_key": {pk(4, True).hex(): (b"\x11" * size).hex()}}
        v["completions"].append(
            dict(id=f"signature-size-{size}", data=data, expected={"error": "satisfy"})
        )
    for flag in (0, 4, 0x80, 0xFF):
        data = {
            "tap_key": {pk(4, True).hex(): (signature(4, True) + bytes([flag])).hex()}
        }
        v["completions"].append(
            dict(id=f"sighash-{flag}", data=data, expected={"error": "satisfy"})
        )
    for i in range(3):
        scripts = [pkscript(j, True) for j in range(3)]
        exprs = [f"pk({pk(j,True).hex()})" for j in range(3)]
        case(
            f"tr-tree-leaf-{i}",
            exprs[i],
            scripts[i],
            [signature(i, True)],
            [i],
            tap=True,
            tree=(scripts[0], (scripts[1], scripts[2])),
            tree_expr="{" + exprs[0] + ",{" + exprs[1] + "," + exprs[2] + "}}",
        )
    for source in list(CASES):
        if source["id"] not in (
            "wsh-or-i-right",
            "sh-wsh-or-i-right",
            "tr-or-i-right",
            "wsh-or-d-left",
            "sh-wsh-or-d-left",
            "sh-or-d-left",
            "tr-or-d-left",
            "tr-tree-leaf-0",
        ):
            continue
        v = json.loads(json.dumps(source))
        v["id"] += "-cheapest"
        first = v["completions"][0]
        if v["descriptor"].startswith("tr("):
            leaf = v["assets"]["tap_leaf"][0]["leaf_hash"]
            v["assets"]["tap_leaf"] = [
                {
                    "key": pk(i, True).hex(),
                    "leaf_hash": (
                        leafhash(pkscript(i, True)).hex() if "tree" in v["id"] else leaf
                    ),
                    "size": 64,
                }
                for i in range(3)
            ]
            first["data"]["tap_leaf"] = [
                dict(a, signature=signature(i, True).hex())
                for i, a in enumerate(v["assets"]["tap_leaf"])
            ]
            for a in first["data"]["tap_leaf"]:
                a.pop("size")
        else:
            v["assets"]["ecdsa"] = [pk(i).hex() for i in range(2)]
            first["data"]["ecdsa"] = {pk(i).hex(): signature(i).hex() for i in range(2)}
        CASES.append(v)
        # Tree-selection checks use independently computed control paths; the
        # individually forced leaves already exercise signature verification.
        if "tree" not in v["id"]:
            DETAILS[v["id"]] = DETAILS[source["id"]]

    v = json.loads(json.dumps(next(c for c in CASES if c["id"] == "tr-tree-leaf-0")))
    key_case = next(c for c in CASES if c["id"] == "tr-key")
    v.update(id="tr-tree-key-preferred", expected_plan=key_case["expected_plan"])
    v["assets"]["tap_key"] = key_case["assets"]["tap_key"]
    v["completions"] = [json.loads(json.dumps(key_case["completions"][0]))]
    CASES.append(v)
    # A plan describes a fixed path. Extra signatures at completion must not
    # select a new path, and replacing a required signature must fail even if
    # the descriptor has another satisfiable path.
    for v in CASES:
        if not v["completions"]:
            continue
        first = v["completions"][0]
        data = json.loads(json.dumps(first["data"]))
        if v["descriptor"].startswith("tr("):
            if not data.get("tap_leaf"):
                continue
            leaf = data["tap_leaf"][0]["leaf_hash"]
            present = {p["key"] for p in data["tap_leaf"]}
            data["tap_leaf"] += [
                {
                    "key": pk(i, True).hex(),
                    "leaf_hash": leaf,
                    "signature": signature(i, True).hex(),
                }
                for i in range(5)
                if pk(i, True).hex() not in present
            ]
        else:
            data["ecdsa"].update({pk(i).hex(): signature(i).hex() for i in range(5)})
        v["completions"].append(
            dict(id="extra-signatures", data=data, expected=first["expected"])
        )
        if v["id"].endswith("or-d-left"):
            replacement = json.loads(json.dumps(first["data"]))
            if v["descriptor"].startswith("tr("):
                replacement["tap_leaf"][0].update(
                    key=pk(1, True).hex(), signature=signature(1, True).hex()
                )
            else:
                replacement["ecdsa"] = {pk(1).hex(): signature(1).hex()}
            v["completions"].append(
                dict(
                    id="replacement-path",
                    data=replacement,
                    expected={"error": "satisfy"},
                )
            )
    for source in list(CASES):
        if (
            not source["completions"]
            or not source["assets"].get("tap_leaf")
            or source["assets"].get("tap_key")
        ):
            continue
        negative(
            source,
            "wrong-leaf",
            lambda x: [a.update(leaf_hash="00" * 32) for a in x["assets"]["tap_leaf"]],
        )
        data = json.loads(json.dumps(source["completions"][0]["data"]))
        for a in data["tap_leaf"]:
            a["leaf_hash"] = "00" * 32
        source["completions"].append(
            dict(id="wrong-leaf", data=data, expected={"error": "satisfy"})
        )

    for name in ("tr-key", "tr-or-d-left", "tr-multi_a-2-01"):
        source = next(c for c in CASES if c["id"] == name)
        for flag in (1, 2, 3, 0x81, 0x82, 0x83):
            v = json.loads(json.dumps(source))
            v["id"] += f"-sighash-{flag}"
            v["completions"] = v["completions"][:2]
            first = v["completions"][0]
            count = 0
            for group in ("tap_key", "tap_leaf"):
                for a in v["assets"][group]:
                    a["size"] = 65
                    count += 1
                if group == "tap_key":
                    first["data"][group] = {
                        k: s + bytes([flag]).hex()
                        for k, s in first["data"][group].items()
                    }
                else:
                    for a in first["data"][group]:
                        a["signature"] += bytes([flag]).hex()
            first["expected"]["witness"] = [
                s + bytes([flag]).hex() if len(s) == 128 else s
                for s in first["expected"]["witness"]
            ]
            for field in ("weight", "witness_size"):
                v["expected_plan"][field] += count
            CASES.append(v)
            DETAILS[v["id"]] = DETAILS[name]
    add_derivation_cases()
    add_transactions()
    return {
        "version": 1,
        "description": "BIP-derived planning and fixed-plan completion vectors; assembly signatures are not transaction signatures.",
        "cases": CASES,
    }


def add_derivation_cases():
    """Exercise definite lookup identities, origins and BIP32/389 selection."""
    seed = hmac.new(b"Bitcoin seed", bytes(range(16)), hashlib.sha512).digest()
    secret, chain = int.from_bytes(seed[:32], "big"), seed[32:]
    point = mul(secret)
    pub = bytes([2 + point[1] % 2]) + point[0].to_bytes(32, "big")
    payload = bytes.fromhex("0488b21e") + b"\x00" * 9 + chain + pub
    raw = payload + sha(sha(payload))[:4]
    alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
    encoded, value = "", int.from_bytes(raw, "big")
    while value:
        value, digit = divmod(value, 58)
        encoded = alphabet[digit] + encoded
    for origin in ("", "[d34db33f/84'/0'/0']"):
        for mp in (0, 1):
            for idx in (0, 1, 17, 0x7FFFFFFF):
                key, cc = secret, chain
                for child in (mp, idx):
                    q = mul(key)
                    data = (
                        bytes([2 + q[1] % 2])
                        + q[0].to_bytes(32, "big")
                        + child.to_bytes(4, "big")
                    )
                    h = hmac.new(cc, data, hashlib.sha512).digest()
                    key, cc = (key + int.from_bytes(h[:32], "big")) % N, h[32:]
                q = mul(key)
                pubkey = bytes([2 + q[1] % 2]) + q[0].to_bytes(32, "big")
                definite = f"{origin}{encoded}/{mp}/{idx}"
                source = next(c for c in CASES if c["id"] == "wpkh")
                v = json.loads(json.dumps(source))
                v.update(
                    id=f"wpkh-derived-{bool(origin)}-{mp}-{idx}",
                    descriptor=f"wpkh({origin}{encoded}/<0;1>/*)",
                    multipath_index=mp,
                    derivation_index=idx,
                    script_pubkey=(b"\x00\x14" + h160(pubkey)).hex(),
                )
                v["assets"]["ecdsa"] = [definite]
                v["completions"] = v["completions"][:2]
                v["completions"][0]["data"]["ecdsa"] = {definite: signature(0).hex()}
                v["completions"][0]["expected"]["witness"][1] = pubkey.hex()
                CASES.append(v)
                negative(
                    v, "multipath-out-of-range", lambda x: x.update(multipath_index=2)
                )

                def hard_index(x):
                    x["derivation_index"] = 0x80000000
                    x["assets"]["ecdsa"] = [f"{origin}{encoded}/{mp}/2147483648"]

                negative(v, "hardened-index", hard_index)
    for path, idx in (("/*", 0x80000000), ("/*", 0xFFFFFFFF), ("/0'/*", 0), ("/*'", 0)):
        v = json.loads(json.dumps(next(c for c in CASES if c["id"] == "tr-key")))
        definite = encoded + path.replace("*", str(idx))
        v.update(
            id=f"tr-unavailable-derivation-{path}-{idx}",
            descriptor=f"tr({encoded}{path})",
            derivation_index=idx,
            expected_plan={"error": "plan"},
            completions=[],
        )
        v["assets"]["tap_key"] = [{"key": definite, "size": 64}]
        CASES.append(v)


def ecdsa_sign(secret, message):
    """Sign with a deterministic test-only nonce and low S, returning DER+ALL."""
    nonce = (
        int.from_bytes(
            sha(b"spending-vector-ecdsa" + secret.to_bytes(32, "big") + message), "big"
        )
        % N
    )
    r = mul(nonce)[0] % N
    s = pow(nonce, -1, N) * (int.from_bytes(message, "big") + r * secret) % N
    s = min(s, N - s)

    def integer(n):
        b = n.to_bytes((n.bit_length() + 7) // 8, "big")
        if b[0] & 128:
            b = b"\x00" + b
        return b"\x02" + bytes([len(b)]) + b

    body = integer(r) + integer(s)
    return b"\x30" + bytes([len(body)]) + body + b"\x01"


def schnorr_sign(secret, message):
    """Implement BIP340 default signing with all-zero auxiliary randomness."""
    q = mul(secret)
    d = secret if q[1] % 2 == 0 else N - secret
    public = q[0].to_bytes(32, "big")
    aux = tagged("BIP0340/aux", b"\x00" * 32)
    t = bytes(a ^ b for a, b in zip(d.to_bytes(32, "big"), aux))
    k = int.from_bytes(tagged("BIP0340/nonce", t + public + message), "big") % N
    r = mul(k)
    k = k if r[1] % 2 == 0 else N - k
    rx = r[0].to_bytes(32, "big")
    e = int.from_bytes(tagged("BIP0340/challenge", rx + public + message), "big") % N
    return rx + ((k + e * d) % N).to_bytes(32, "big")


def add_transactions():
    """Sign fixture transactions independently, plus cryptographic negatives.

    One input, one output, no annex or CODESEPARATOR. This is deliberately not
    a general transaction signer. BIP143 and BIP341 define the preimages below.
    """
    dsha = lambda b: sha(sha(b))
    outpoint = b"\x11" * 32 + b"\x01\x00\x00\x00"
    output = (90000).to_bytes(8, "little") + b"\x01\x51"
    amount = (100000).to_bytes(8, "little")
    key_numbers = {
        pk(i, tap).hex(): i + 1 for i in range(len(KEYS)) for tap in (False, True)
    }
    for v in CASES:
        if v["id"] not in DETAILS or not v["completions"]:
            continue
        script, kind, tap = DETAILS[v["id"]]
        first = v["completions"][0]
        # Large multisigs need no additional signing permutations: the smaller
        # cases cover cryptography, and their boundary stacks remain pure vectors.
        if (
            sum(len(first["data"].get(k, [])) for k in ("ecdsa", "tap_leaf", "tap_key"))
            > 20
        ):
            continue
        version = v["tx"].get("version", 2).to_bytes(4, "little", signed=True)
        sequence = v["tx"].get("sequence", 0xFFFFFFFE).to_bytes(4, "little")
        lock = v["tx"].get("lock_time", 0).to_bytes(4, "little")
        unsigned = (
            version + b"\x01" + outpoint + b"\x00" + sequence + b"\x01" + output + lock
        )
        spk = bytes.fromhex(v["script_pubkey"])
        v["transaction"] = {
            "unsigned_tx": unsigned.hex(),
            "input_index": 0,
            "prevouts": [{"value": 100000, "script_pubkey": spk.hex()}],
        }
        signed = json.loads(json.dumps(first))
        signed.update(id="signed", verify=True)
        signed["expected"]["valid"] = True
        replacements = {}
        if tap:
            groups = [(a["key"], a["signature"], a) for a in signed["data"]["tap_leaf"]]
            groups += [
                (key, sig, None) for key, sig in signed["data"]["tap_key"].items()
            ]
            for key, old, entry in groups:
                flag = bytes.fromhex(old)[64] if len(old) == 130 else 0
                msg = bytes([flag]) + version + lock
                if not flag & 128:
                    msg += (
                        sha(outpoint)
                        + sha(amount)
                        + sha(compact(len(spk)) + spk)
                        + sha(sequence)
                    )
                if flag & 3 not in (2, 3):
                    msg += sha(output)
                msg += bytes([2 if entry else 0])
                msg += (
                    outpoint + amount + compact(len(spk)) + spk + sequence
                    if flag & 128
                    else b"\x00" * 4
                )
                if flag & 3 == 3:
                    msg += sha(output)
                secret = key_numbers[key]
                if entry:
                    msg += leafhash(script) + b"\x00" + b"\xff" * 4
                else:
                    secret = secret if mul(secret)[1] % 2 == 0 else N - secret
                    secret = (
                        secret
                        + int.from_bytes(tagged("TapTweak", bytes.fromhex(key)), "big")
                    ) % N
                new = schnorr_sign(secret, tagged("TapSighash", b"\x00" + msg)) + (
                    bytes([flag]) if flag else b""
                )
                if entry is not None:
                    entry["signature"] = new.hex()
                else:
                    signed["data"]["tap_key"][key] = new.hex()
                replacements[old] = new.hex()
        else:
            script_code = pkhscript(0) if kind in ("wpkh", "sh-wpkh") else script
            code = compact(len(script_code)) + script_code
            if kind in ("bare", "sh"):
                message = dsha(
                    version
                    + b"\x01"
                    + outpoint
                    + code
                    + sequence
                    + b"\x01"
                    + output
                    + lock
                    + b"\x01\x00\x00\x00"
                )
            else:
                message = dsha(
                    version
                    + dsha(outpoint)
                    + dsha(sequence)
                    + outpoint
                    + code
                    + amount
                    + sequence
                    + dsha(output)
                    + lock
                    + b"\x01\x00\x00\x00"
                )
            for key, old in signed["data"]["ecdsa"].items():
                new = ecdsa_sign(key_numbers[key], message).hex()
                signed["data"]["ecdsa"][key] = new
                replacements[old] = new

        def replace(expected, changes):
            expected["witness"] = [changes.get(s, s) for s in expected["witness"]]
            raw = bytes.fromhex(expected["script_sig"])
            for old, new in changes.items():
                raw = raw.replace(push(bytes.fromhex(old)), push(bytes.fromhex(new)))
            expected["script_sig"] = raw.hex()

        replace(signed["expected"], replacements)
        v["completions"].append(signed)
        corrupt = json.loads(json.dumps(signed))
        corrupt["id"] = "invalid-signature"
        corrupt["expected"]["valid"] = False
        changes = {}

        def damage(sig):
            b = bytearray.fromhex(sig)
            b[-2] ^= 1  # Preserve DER, sighash type and Schnorr length.
            changes[sig] = b.hex()
            return b.hex()

        for group in ("ecdsa", "tap_key"):
            corrupt["data"][group] = {
                key: damage(s) for key, s in corrupt["data"][group].items()
            }
        for a in corrupt["data"]["tap_leaf"]:
            a["signature"] = damage(a["signature"])
        replace(corrupt["expected"], changes)
        v["completions"].append(corrupt)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    contents = json.dumps(generate(), indent=2) + "\n"
    path = Path(__file__).with_name("spending_vectors.json")
    if args.check:
        assert path.read_text() == contents, "spending_vectors.json needs regeneration"
    else:
        path.write_text(contents)
    print(
        f"{len(CASES)} planning cases; {sum(len(c['completions']) for c in CASES)} completion cases"
    )
