// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
This test file is part of the database package rather than than the
database_test package so it can bridge access to the internals to properly test
cases which are either not possible or can't reliably be tested via the public
interface.  The functions, constants, and variables are only exported while the
tests are being run.
*/

package database

// TstNumErrorCodes makes the internal numErrorCodes parameter available to the
// test package.
const TstNumErrorCodes = numErrorCodes
