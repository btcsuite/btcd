# descriptors

Output script descriptors as specified in BIP380 through BIP389: parse a
descriptor, derive its addresses and scripts, estimate its spending weight, lift
it to a semantic policy, and plan and complete a spend.

The miniscript engine lives in [`miniscript/`](miniscript/README.md), which has
its own notes.

## What is supported

**Script expressions**: `pk()`, `pkh()`, `wpkh()`, `sh()`, `wsh()`, `tr()` with
an arbitrary script tree, `multi()`, `sortedmulti()`, and miniscript expressions
(in `wsh()`, in `tr()` leaves, and in `sh()`, see the divergences below). Each
is accepted only in the positions its BIP allows, so `sh(sh(…))`,
`wsh(wsh(…))`, `wsh(wpkh(…))`, `tr()` outside the top level and a bare
miniscript expression are rejected at parse time.

**Key expressions**: hex public keys (33-byte compressed, 65-byte uncompressed,
32-byte x-only in `tr()`), WIF private keys, `xpub`/`xprv` with derivation
paths, `[fingerprint/path]` key origins, hardened steps, `/*` and `/*h`
wildcards, and BIP389 `<a;b;…>` multipath elements. Which serialization is valid
where follows BIP380 to BIP386: uncompressed keys in the pre-segwit positions
only, x-only inside `tr()` only.

**API**: `NewDescriptor`, `String` (with checksum), `Keys`, `DescType`,
`MultipathLen`, `AddressAt`, `ScriptCodeAt`, `MaxWeightToSatisfy`, `Lift`, and
`PlanAt` returning a `Plan` with `SatisfactionWeight`, `ScriptSigSize`,
`WitnessSize` and `Satisfy`.

## What is not supported

| Expression | BIP | Why |
|------------|-----|-----|
| `combo()` | 384 | Stands for two or four output scripts; the API is one script per descriptor |
| `raw()`, `addr()` | 385 | No keys and no satisfaction, so most of the API is meaningless for them |
| `musig()` | 390 | Needs BIP327 key aggregation and BIP328 derivation |
| `rawtr()`, `sp()` | - | Not implemented |

Also absent, by design: no policy-to-miniscript compiler (rust-miniscript has
one), no descriptor inference from an existing script (Core's
`InferDescriptor`), no signing, and no PSBT integration - `Plan.Satisfy` takes
finished signatures and returns raw witness and scriptSig bytes.

## Divergences from Bitcoin Core and rust-miniscript

Checked against Bitcoin Core `c4fbd3c7211` and rust-miniscript v13.

| Behavior | Here | Core | rust |
|----------|------|------|------|
| Bare `multi()` above 3 keys | rejected | rejected (`descriptor.cpp:2419`) | accepted |
| Miniscript inside `sh()` | accepted, compressed keys only | rejected entirely (`descriptor.cpp:2682`) | accepted, also with uncompressed keys |
| `tr()` leaf other than `pk()`, e.g. `pkh()` | accepted | accepted | accepted |
| `sh()` redeem script over 520 bytes | rejected | rejected (`descriptor.cpp:2427`) | rejected |
| Plan scriptSig size for P2SH | counts the redeem script | n/a | excludes it, unlike its own `max_weight_to_satisfy` |
| Plan witness size for P2WSH | counts the witness script | n/a | excludes it, although its own `Plan::satisfy` reveals it |

Both plan rows are cases where rust's plan disagrees with rust's own weight
API, and rust-bitcoin/rust-miniscript#1045 fixes two of the three: its
`witness_size` counts the witness script, and the scriptSig of a P2SH-wrapped
segwit spend gains the length byte it was missing. The redeem script of a legacy
`sh()` stays out of rust's template and is supplied separately to a PSBT
input. However, its direct `Plan::satisfy` API also omits the redeem script,
so that returned scriptSig is not transaction-ready. The shared spending
vectors exercise this distinction with signed transactions. The older TSV
differential test keeps adding the size difference back until
`testdata/descriptors_from_rust.tsv` is regenerated with a rust that has the
fix, because the values in it come from v13.

The `tr()` leaf row is a divergence from the *letter of BIP386*, not from the
implementations: BIP386 says only `pk()` may appear in a tree expression, but
BIP379 and BIP387 postdate it and allow any miniscript fragment plus
`multi_a()`/`sortedmulti_a()`.

## Behavior worth knowing

- **A plan is transaction-ready.** `Plan.Satisfy` returns the witness and
  scriptSig bytes a spend needs, including every script it has to reveal: the
  witness script of a P2WSH output, the redeem script of a P2SH one, and the
  leaf script and control block of a taproot script path. `WitnessSize`,
  `ScriptSigSize` and `SatisfactionWeight` account for those scripts and their
  serialization prefixes. Signature sizes are estimates: 72 bytes for ECDSA,
  and the advertised 64 or 65 bytes for Schnorr.
- **Key validity is checked at derivation, not at parse time.** A hex key of the
  right length that is not a point on the curve parses, and `AddressAt` is where
  it fails. Core rejects it at parse time.
- **A hardened step needs the private extended key.** `pkh(xpub…/0h/*)` is a
  valid descriptor, but deriving from it fails; the same descriptor with an
  `xprv` derives. This matches BIP380, where such an expression is valid.
- **The network is a parameter of derivation, not of the descriptor.**
  `AddressAt` takes the chain parameters, and the network bytes of an extended
  key or a WIF key are ignored.
- **The parsed miniscript must be sane**, i.e. non-malleable, signature-bound
  and inside every resource limit of its context. See the miniscript README.
- **Nesting is bounded**, including tap tree depth (128, BIP341), and accepted
  scripts must meet their context's resource limits. These checks do not impose
  a total input-length or CPU budget. Threshold selection uses sorting instead
  of enumerating signer subsets. Callers accepting untrusted descriptors should
  separately bound input size and processing resources.

## Testing

See [property test audit](testdata/property_tests.md) for the
independent spending models, structured fuzzers and resource benchmarks.

Besides unit tests, the package is checked against three external references:
the BIP test vectors of BIP380 to BIP389 (`testdata/bip_vectors.json`, run by
`TestBIPVectors`, which also pins down what is unsupported), a
rust-miniscript-generated corpus of descriptors with their addresses, script
codes, weights, lifted policies and plans (`testdata/descriptors_from_rust.tsv`),
and reference derivations from the descriptors-go implementation
(`testdata/derivation.json`). `FuzzNewDescriptor` fuzzes the parser and the
derivation paths.
