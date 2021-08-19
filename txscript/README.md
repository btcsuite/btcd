txscript
========

Package txscript implements the bitcoin transaction script language.  There is
a comprehensive test suite.

This package has been augmented to include support for LBRY's custom claim operations.
See https://lbry.tech/spec

## Bitcoin Scripts

Bitcoin provides a stack-based, FORTH-like language for the scripts in
the bitcoin transactions.  This language is not turing complete
although it is still fairly powerful.  A description of the language
can be found at https://en.bitcoin.it/wiki/Script