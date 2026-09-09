# Property test audit

The ordinary Go tests include two bounded, independent spending models:

- `miniscript/TestSemanticSpending` evaluates signature-bound AND/OR trees with
  up to four distinct keys, a SHA256 preimage, and relative/absolute height locks.
  It compares availability with satisfaction and lifted policy meaning, executes
  real segwit/tapscript witnesses, and checks stack and size bounds.
- `TestDescriptorSpendingProperties` exhausts signer subsets for thresholds in
  P2SH, P2WSH, nested P2WSH and taproot descriptors, including a recovery leaf.
  It checks planning refusal, completion retry, serialized size and execution.

These models do not claim completeness for arbitrary miniscripts. They
deliberately generate valid, non-malleable expressions so an unexpected refusal
is a failure, not a skipped case. Their `FuzzSemanticSpending` and
`FuzzDescriptorSpending` counterparts vary structure and transaction facts;
`make go-fuzz MODULES=descriptors fuzztime=60s` also runs the raw parser fuzzers.

Run the size-series benchmarks from the repository root:

```sh
go test -C descriptors ./... -run '^$' -bench 'Benchmark(ResourceParse|PlanningResources)$' -benchmem -count=5
```

Compare repeated results with `benchstat` if installed; it is not a dependency.
The benchmarks separate parsing, planning and completion and report allocations.
Unit tests assert deterministic resource bounds, not host-dependent time limits.
