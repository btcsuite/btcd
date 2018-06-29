// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package txscript implements the Decred transaction script language.

This package provides data structures and functions to parse and execute
decred transaction scripts.

Script Overview

Decred transaction scripts are written in a stack-base, FORTH-like language.

The Decred script language consists of a number of opcodes which fall into
several categories such pushing and popping data to and from the stack,
performing basic and bitwise arithmetic, conditional branching, comparing
hashes, and checking cryptographic signatures.  Scripts are processed from left
to right and intentionally do not provide loops.

The vast majority of Decred scripts at the time of this writing are of several
standard forms which consist of a spender providing a public key and a signature
which proves the spender owns the associated private key.  This information
is used to prove the the spender is authorized to perform the transaction.

One benefit of using a scripting language is added flexibility in specifying
what conditions must be met in order to spend decreds.

Errors

Errors returned by this package are of type txscript.Error.  This allows the
caller to programmatically determine the specific error by examining the
ErrorCode field of the type asserted txscript.Error while still providing rich
error messages with contextual information.  A convenience function named
IsErrorCode is also provided to allow callers to easily check for a specific
error code.  See ErrorCode in the package documentation for a full list.
*/
package txscript
