harness
=======
[![Build Status](http://img.shields.io/travis/btcsuite/btcd.svg)](https://travis-ci.org/btcsuite/btcd)
[![ISC License](http://img.shields.io/badge/license-ISC-blue.svg)](http://copyfree.org)

Package harness provides a btcd-specific RPC testing harness crafting and
executing integration tests by driving a `btcd` instance via the `RPC`
interface.

This package was designed specifically to act as an RPC testing harness for
`btcd`. However, the constructs presented are general enough to be adapted to
any project wishing to programmatically drive a `btcd` instance of its
systems/integration tests.

Subpackages:

 - [memwallet](https://github.com/btcsuite/btcd/tree/master/integration/harness/memwallet)
 offers a simple in-memory HD wallet capable of properly syncing to the
 generated chain, creating new addresses, and crafting fully signed transactions
 paying to an arbitrary set of outputs.

 - [testnode](https://github.com/btcsuite/btcd/tree/master/integration/harness/testnode)
 provides a default test node that launches a new
 `btcd`-instance using command-line call.

 - [simpleregtest](https://github.com/btcsuite/btcd/tree/master/integration/harness/simpleregtest)
 harbours a pre-configured test setup and unit tests to run RPC-driven node tests.

 ## License
 This code is licensed under the [copyfree](http://copyfree.org) ISC License.