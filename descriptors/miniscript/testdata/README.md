Text-based test vectors (`*.txt`) taken from:

https://github.com/rust-bitcoin/rust-miniscript/blob/59ad2ed/src/miniscript/ms_tests.rs

The type of a malleable expression there still lists the "s", "f" and "e"
properties, to which BIP379 gives no meaning for one, so they were dropped from
the entries that carried them.

The `*.tsv` vectors were extracted with slight modifications to the
`rust-bitcoin` library to extract more test data, using an agentic differential
test approach.
