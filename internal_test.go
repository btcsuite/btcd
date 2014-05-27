// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
This test file is part of the btcutil package rather than than the
btcutil_test package so it can bridge access to the internals to properly test
cases which are either not possible or can't reliably be tested via the public
interface. The functions are only exported while the tests are being run.
*/

package btcutil

import (
	"code.google.com/p/go.crypto/ripemd160"
	"github.com/conformal/btcec"
)

// SetBlockBytes sets the internal serialized block byte buffer to the passed
// buffer.  It is used to inject errors and is only available to the test
// package.
func (b *Block) SetBlockBytes(buf []byte) {
	b.serializedBlock = buf
}

// TstAppDataDir makes the internal appDataDir function available to the test
// package.
func TstAppDataDir(goos, appName string, roaming bool) string {
	return appDataDir(goos, appName, roaming)
}

// TstAddressPubKeyHash makes an AddressPubKeyHash, setting the
// unexported fields with the parameters hash and netID.
func TstAddressPubKeyHash(hash [ripemd160.Size]byte,
	netID byte) *AddressPubKeyHash {

	return &AddressPubKeyHash{
		hash:  hash,
		netID: netID,
	}
}

// TstAddressScriptHash makes an AddressScriptHash, setting the
// unexported fields with the parameters hash and netID.
func TstAddressScriptHash(hash [ripemd160.Size]byte,
	netID byte) *AddressScriptHash {

	return &AddressScriptHash{
		hash:  hash,
		netID: netID,
	}
}

// TstAddressPubKey makes an AddressPubKey, setting the unexported fields with
// the parameters.
func TstAddressPubKey(serializedPubKey []byte, pubKeyFormat PubKeyFormat,
	netID byte) *AddressPubKey {

	pubKey, _ := btcec.ParsePubKey(serializedPubKey, btcec.S256())
	return &AddressPubKey{
		pubKeyFormat: pubKeyFormat,
		pubKey:       (*btcec.PublicKey)(pubKey),
		pubKeyHashID: netID,
	}
}

// TstAddressSAddr returns the expected script address bytes for
// P2PKH and P2SH bitcoin addresses.
func TstAddressSAddr(addr string) []byte {
	decoded := Base58Decode(addr)
	return decoded[1 : 1+ripemd160.Size]
}
