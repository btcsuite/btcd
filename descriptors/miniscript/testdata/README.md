Text-based test vectors (`*.txt`) taken from:

https://github.com/rust-bitcoin/rust-miniscript/blob/698b97f/src/miniscript/ms_tests.rs

The type of a malleable expression used to list the "s", "f" and "e"
properties, to which BIP379 gives no meaning for one, so they were dropped from
the entries that carried them. That correction has since been made upstream
(rust-bitcoin/rust-miniscript#1037), and every expression in these files now
carries the same type as the file above.

The `*.tsv` vectors are generated from the same commit by the extraction
harness in the `test-vector-extraction` branch of that repository, which prints
the properties, the encoded scripts and the descriptor-level results the crate
computes.
