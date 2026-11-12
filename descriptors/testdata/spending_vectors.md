# Planning and satisfaction vector format

`spending_vectors.json` is a JSON object with `version` (currently `1`),
`description` (informational text), and `cases` (an array of test cases).
Each case describes a descriptor, the assets available when planning its
spend, and independent attempts to complete the same selected plan.

## Encoding conventions

Hex strings are lowercase without `0x`. Stacks are arrays ordered bottom to
top; an empty string denotes an actual empty stack element. Amounts are
integer satoshis. Missing lookup tables mean unavailable data, not a wildcard.

Keys in lookup tables are definite descriptor key expressions: multipath and
wildcard steps are resolved, while origins and extended keys are retained.
Hash values, including `hash256` and TapLeaf hashes, use forward Script byte
order, not reversed display order.

Errors specify the stage (`parse`, `plan`, or `satisfy`), not a particular error
message. An error expectation replaces the successful result fields.

## Case fields

- `id`: a unique case identifier.
- `descriptor`: the public-key output descriptor.
- `multipath_index`: the zero-based BIP389 branch index.
- `derivation_index`: the BIP32 wildcard index.
- `tx`: optional `version`, `sequence`, and `lock_time` fields used for
  BIP65/68/112 planning checks. Absent fields are unknown, not zero.
- `assets`: signature and preimage availability, described below.
- `expected_plan`: a planning/parsing error or expected serialized sizes.
- `script_pubkey`: expected output script bytes, checked when planning succeeds.
- `completions`: attempts to fill the selected plan with concrete data.
- `transaction`: optional transaction and previous-output data for verifying
  completed spends.

## Planning assets

`assets` contains availability only, without concrete signatures or preimages:

- `ecdsa`: an array of definite key expressions.
- `tap_key`: an array of `{key, size}` objects identifying the internal key
  and advertised key-path signature length in bytes.
- `tap_leaf`: an array of `{key, leaf_hash, size}` objects identifying each
  leaf-specific signature and its advertised length.
- `preimages`: an array of `{function, hash}` objects. `function` is `sha256`,
  `hash256`, `ripemd160`, or `hash160`.

Availability for one key, hash function, digest, or Taproot leaf does not imply
availability for another. Invalid advertised lengths can appear in negative
cases.

## Expected plan

`expected_plan` is either `{"error": "parse"}`, `{"error": "plan"}`, or an
object with these three fields:

- `witness_size`: complete serialized witness size, including its CompactSize
  element count, every element's length prefix, revealed scripts and control
  blocks. Legacy inputs use zero here; an empty witness count needed because
  another input uses segwit is transaction-level overhead.
- `script_sig_size`: serialized scriptSig size including its CompactSize
  length prefix, Script push opcodes and any redeem script. An empty scriptSig
  therefore has size one.
- `weight`: `witness_size + 4 * script_sig_size`, in weight units. Outpoints,
  sequence and transaction-level overhead are excluded.

ECDSA estimates assume a 72-byte low-S DER signature including its sighash
byte. Taproot uses the advertised 64- or 65-byte signature length. Concrete
ECDSA signatures may be shorter than the planning estimate.

## Completions

Each completion has an `id` unique within its case, `data`, `expected`, and an
optional `verify` Boolean (default false).

`data` supplies concrete lookup results independently of the planning assets:

- `ecdsa` and `tap_key`: objects mapping definite keys to signature hex.
  `tap_key` contains at most the descriptor's single internal key.
- `tap_leaf`: an array of `{key, leaf_hash, signature}` objects.
- `preimages`: an array of `{function, hash, preimage}` objects.

`expected` is either `{"error": "satisfy"}` or an object with exact `witness`
(an array of element hex strings) and `script_sig` (hex) fields.

Every completion applies to the same fixed plan. Additional signatures must
not change its branch or multisig subset. Missing required data must fail even
if another path could satisfy the descriptor. Completion attempts are
independent: failure in one must not prevent another from succeeding.

## Transaction verification

`transaction` contains:

- `unsigned_tx`: the transaction in non-witness serialization, encoded as hex.
- `input_index`: the zero-based input receiving the completed satisfaction.
- `prevouts`: previous outputs in transaction-input order, each with `value`
  (satoshis) and `script_pubkey` (hex).

When a completion sets `verify: true`, `expected` additionally contains a
Boolean `valid`. Insert the actual completed scriptSig and witness into the
specified input and verify it against the supplied previous outputs, including
signature checks and standard Script verification flags.

Assembly success and script validity are separate expectations. A well-encoded
but cryptographically invalid signature may be assembled successfully and then
fail verification. Completions without `verify: true` test assembly only; their
signature bytes need not sign a transaction.
