// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package chaingen provides facilities for generating a full chain of blocks.

Overview

Many consensus-related tests require a full chain of valid blocks with several
pieces of contextual information such as versions and votes.  Generating such a
chain is not a trivial task due to things such as the fact that tickets must be
purchased (at the correct ticket price), the appropriate winning votes must be
cast (which implies keeping track of all live tickets and implementing the
lottery selection algorithm), and all of the state-specific header fields such
as the pool size and the proof-of-work and proof-of-stake difficulties must be
set properly.

In order to simplify this complex process, this package provides a generator
that keeps track of all of the necessary state and generates and solves blocks
accordingly while allowing the caller to manipulate the blocks via munge
functions.
*/
package chaingen
