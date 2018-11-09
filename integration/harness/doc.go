// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package harness

/*

Package harness provides RPC testing harness crafting and
executing integration tests by driving a node instance via the `RPC`
interface.

Each instance of an active harness comes equipped with a test wallet
capable of properly syncing to the generated chain,
creating new addresses, and crafting fully signed transactions paying to an
arbitrary set of outputs.

This package was designed to be adapted to any project wishing to
programmatically drive its systems/integration tests.

*/
