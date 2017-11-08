// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package gcs provides an API for building and using a Golomb-coded set filter.

Golomb-Coded Set

A Golomb-coded set is a probabilistic data structure used similarly to a Bloom
filter.  A filter uses constant-size overhead plus on average n+2 bits per item
added to the filter, where 2^-n is the desired false positive (collision)
probability.

GCS use in Decred

GCS filters are a mechanism for storing and transmitting per-block filters.  The
usage is intended to be the inverse of Bloom filters: a consensus-validating
full node commits to a single filter for every block and serves the filter to
SPV clients that match against the filter locally to determine if the block is
potentially relevant.  The suggested collision probability for Decred use is
2^-20.
*/
package gcs
