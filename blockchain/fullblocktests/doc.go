// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package fullblocktests provides a set of block consensus validation tests.

All of the generated test instances involve full blocks that are to be used for
testing the consensus validation rules.  The tests are intended to be flexible
enough to allow both unit-style tests directly against the blockchain code as
well as integration style tests over the peer-to-peer network.  To achieve that
goal, each test contains additional information about the expected result,
however that information can be ignored when doing comparison tests between two
independent versions over the peer-to-peer network.

This package has intentionally been designed so it can be used as a standalone
package for any projects needing to test their implementation against a full set
of blocks that exercise the consensus validation rules.
*/
package fullblocktests
