# miniscript

Miniscript as specified in BIP379: parse an expression, analyze it, build its
script, and produce a non-malleable satisfaction. Used by the
[`descriptors`](../README.md) package, but usable on its own.

## What is supported

**Every BIP379 fragment and wrapper**, plus `multi_a()` (BIP387) and
`sortedmulti_a()` (BIP387), including the syntactic sugar (`pk`, `pkh`, `and_n`,
`t:`, `l:`, `u:`).

**Three script contexts**, which decide the allowed fragments, the key
serialization and the resource limits:

| Context | Keys | Multisig | Script size | Ops | Other limits |
|---------|------|----------|-------------|-----|--------------|
| `P2WSH` | 33-byte compressed | `multi`, ≤ 20 keys | ≤ 3600 | ≤ 201 | ≤ 100 witness elements, ≤ 1000 stack elements |
| `P2TR` | 32-byte x-only | `multi_a`/`sortedmulti_a`, ≤ 999 keys | ≤ 10000 | - | ≤ 1000 stack elements |
| `Legacy` | 33-byte compressed | `multi`, ≤ 20 keys | ≤ 520 | ≤ 201 | ≤ 1650 byte scriptSig, no `or_i`, no `d:` |

**Analysis**: the correctness type system (`B`/`V`/`K`/`W` plus the
`zondumsfe` properties), malleability, timelock mixing, script size, op count,
witness element count, execution stack peak and satisfaction size - the whole
static analysis BIP379 describes.

**API**: `Parse`, `ParseInsane`, and on the resulting `AST`: `Script`,
`Satisfy`, `Keys`, `ApplyVars`, `Clone`, `Lift`, `DrawTree`, `IsSane`,
`IsValidTopLevel`, `ScriptLen`, `MaxSatisfactionSize`,
`MaxSatisfactionWitnessElements`.

## What is not supported

- **No compiler.** A policy cannot be compiled to miniscript; only the reverse
  (`Lift`). rust-miniscript has a compiler behind a feature flag.
- **No script decoding.** A miniscript cannot be recovered from raw script bytes,
  which both Core and rust can do.
- **Keys are opaque.** A key argument is an identifier until `ApplyVars`
  substitutes bytes for it, and only its length is checked; whether it is a
  valid curve point is the caller's business.

## Divergences worth knowing

- **`Parse` is sane by default.** It rejects expressions that are malleable,
  need no signature, are not a valid top level, mix timelock kinds, repeat a
  public key, or exceed a resource limit - the same set rust-miniscript's
  `from_str` enforces through `Ctx::SANE`. `ParseInsane` runs the analysis
  without those checks, for inspecting an expression that is known not to be
  sane.
- **The Tapscript script size limit is 10000 bytes**, not the block weight.
  Tapscript imposes no script size limit of its own, but the script builder
  cannot emit more, so the parse-time limit is what can actually be built rather
  than a limit that would let an expression parse and never compile.
- **The P2WSH witness element limit excludes the witness script.** BIP379 and
  Core count only the elements a satisfaction pushes (`policy.cpp:312` in Core
  `c4fbd3c7211`), so a satisfaction of exactly 100 elements is standard. rust
  compares a count that includes the witness script against the same limit
  (`Segwitv0::check_local_policy_validity`), and so rejects such an expression
  one element early.
- **The `d:` wrapper is unit in Tapscript.** MINIMALIF is a consensus rule
  there, so the only element that satisfies the `OP_IF` of a `d:` is the single
  byte `0x01`, which is what "(Tapscript only) u" in BIP379's correctness table
  means and what Core does (`miniscript.cpp:119` in Core `c4fbd3c7211`).
  rust-miniscript's type checker knows no script context, so it never assigns
  the property and rejects Tapscript expressions whose parent fragment requires
  it, such as `andor(dv:older(1),pk(A),pk(B))`.
- **The `Legacy` context is a rust concept.** Core does not accept miniscript
  inside `sh()` at all (`descriptor.cpp:2682` in Core `c4fbd3c7211`). Where the
  context exists here, it mirrors rust's: `or_i` and `d:` are rejected because
  an `OP_IF` argument is not required to be minimally encoded outside segwit, so
  a third party could malleate the branch selector. Unlike rust's, it takes
  compressed keys only.
- **The execution stack model differs from rust in three fragments.** For
  `thresh`, `or_d` and `multi`, this package uses a different bound: rust's
  value is an order-dependent conservative estimate for `thresh`, and one
  respectively two elements short of the peak for `or_d` (the `OP_IFDUP` of a
  satisfied first branch) and `multi` (the `<k>` and `<n>` around its keys). The
  differential test records the difference. Every other computed property
  matches rust exactly.
- **A malleable expression carries no malleability properties.** The "s", "f"
  and "e" properties describe an expression only if it meets the malleability
  requirement of every fragment it is built from, so the type of a malleable
  expression reports none of the three, as BIP379 states below its malleability
  table. Pending upstream: rust-miniscript and Core still report the three on a
  malleable expression, which is why the types in `testdata/*.txt` had to be
  edited (see its README); rust matches this once
  rust-bitcoin/rust-miniscript#1037 lands.
- **Malleability propagation differs in one corner.** The satisfaction type has
  no equivalent of rust's "impossible versus unavailable" distinction, so the
  malleable flag of a non-sane threshold branch without a signature can differ.
  Sane expressions are unaffected, and no satisfaction this produces is invalid.

## Testing

The package is checked against rust-miniscript by differential tests over about
8,200 expressions per context: the compared structural properties
(`testdata/props_from_rust*.tsv`) and the byte-exact script encoding
(`testdata/scripts_from_rust*.tsv`), plus parse agreement over roughly 13,700
expressions. The corpora from rust's own test suite (`testdata/*.txt`) cover
valid, invalid, malleable and timelock-conflicting expressions with their
expected types. `execute_test.go` and `tap_test.go` run real spends through the
btcd script engine, and `FuzzParse` fuzzes the parser and every downstream pass.
See [`testdata/README.md`](testdata/README.md) for how the corpora were
generated; they have since been contributed upstream as BIP379's test vectors.
