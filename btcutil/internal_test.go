// Copyright (c) 2013-2017 The btcsuite developers
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
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"golang.org/x/crypto/ripemd160"
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

// TstAddressWitnessPubKeyHash creates an AddressWitnessPubKeyHash, initiating
// the fields as given.
func TstAddressWitnessPubKeyHash(version byte, program [20]byte,
	hrp string) *AddressWitnessPubKeyHash {

	return &AddressWitnessPubKeyHash{
		AddressSegWit{
			hrp:            hrp,
			witnessVersion: version,
			witnessProgram: program[:],
		},
	}
}

// TstAddressWitnessScriptHash creates an AddressWitnessScriptHash, initiating
// the fields as given.
func TstAddressWitnessScriptHash(version byte, program [32]byte,
	hrp string) *AddressWitnessScriptHash {

	return &AddressWitnessScriptHash{
		AddressSegWit{
			hrp:            hrp,
			witnessVersion: version,
			witnessProgram: program[:],
		},
	}
}

// TstAddressTaproot creates an AddressTaproot, initiating the fields as given.
func TstAddressTaproot(version byte, program [32]byte,
	hrp string) *AddressTaproot {

	return &AddressTaproot{
		AddressSegWit{
			hrp:            hrp,
			witnessVersion: version,
			witnessProgram: program[:],
		},
	}
}

// TstAddressPubKey makes an AddressPubKey, setting the unexported fields with
// the parameters.
func TstAddressPubKey(serializedPubKey []byte, pubKeyFormat PubKeyFormat,
	netID byte) *AddressPubKey {

	pubKey, _ := btcec.ParsePubKey(serializedPubKey)
	return &AddressPubKey{
		pubKeyFormat: pubKeyFormat,
		pubKey:       pubKey,
		pubKeyHashID: netID,
	}
}

// TstAddressSAddr returns the expected script address bytes for
// P2PKH and P2SH bitcoin addresses.
func TstAddressSAddr(addr string) []byte {
	decoded := base58.Decode(addr)
	return decoded[1 : 1+ripemd160.Size]
}

// TstAddressSegwitSAddr returns the expected witness program bytes for
// bech32 encoded P2WPKH and P2WSH bitcoin addresses.
func TstAddressSegwitSAddr(addr string) []byte {
	_, data, err := bech32.Decode(addr)
	if err != nil {
		return []byte{}
	}

	// First byte is version, rest is base 32 encoded data.
	data, err = bech32.ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return []byte{}
	}
	return data
}

// TstAddressTaprootSAddr returns the expected witness program bytes for a
// bech32m encoded P2TR bitcoin address.
func TstAddressTaprootSAddr(addr string) []byte {
	_, data, err := bech32.Decode(addr)
	if err != nil {
		return []byte{}
	}

	// First byte is version, rest is base 32 encoded data.
	data, err = bech32.ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return []byte{}
	}
	return data
}
