// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

const (
	// satoshiPerBitcoin is the untyped version of SatoshiPerBitcoin.
	//
	// TODO(jrick): Switch the exported consts below to be untyped.
	satoshiPerBitcoin = 1e8

	// SatoshiPerBitcent is the number of satoshi in one bitcoin cent.
	SatoshiPerBitcent int64 = 1e6

	// SatoshiPerBitcoin is the number of satoshi in one bitcoin (1 BTC).
	SatoshiPerBitcoin int64 = satoshiPerBitcoin

	// MaxSatoshi is the maximum transaction amount allowed in satoshi.
	MaxSatoshi int64 = 21e6 * SatoshiPerBitcoin
)
