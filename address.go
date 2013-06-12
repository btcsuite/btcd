// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

import (
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// ScriptType is an enum type that represents the type of a script. It is
// returned from ScriptToAddress as part of the metadata about the script.
// It implements the Stringer interface for nice printing.
type ScriptType int

// String Converts the enumeration to a nice string value instead of a number.
func (t ScriptType) String() string {
	if int(t) > len(scriptTypeToName) || int(t) < 0 {
		return "Invalid"
	}
	return scriptTypeToName[t]
}

// Constant types representing known types of script found in the wild
const (
	ScriptUnknown ScriptType = iota
	ScriptAddr
	ScriptPubKey
	ScriptStrange
	ScriptGeneration
)

var scriptTypeToName = []string{
	ScriptUnknown:    "Unknown",
	ScriptAddr:       "Addr",
	ScriptPubKey:     "Pubkey",
	ScriptStrange:    "Strange",
	ScriptGeneration: "Generation", // ScriptToAddress does not recieve enough information to identify Generation scripts.
}

type pkformat struct {
	addrtype  ScriptType
	parsetype int
	length    int
	databytes []pkbytes
	allowmore bool
}

type pkbytes struct {
	off int
	val byte
}

const (
	scrPayAddr = iota
	scrCollectAddr
	scrCollectAddrComp
	scrGeneratePubkeyAddr
	scrPubkeyAddr
	scrPubkeyAddrComp
	scrNoAddr
)

// ScriptToAddress extracts a payment address and the type out of a PkScript
func ScriptToAddress(script []byte) (ScriptType, string, error) {
	// Currently this only understands one form of PkScript
	validformats := []pkformat{
		{ScriptAddr, scrPayAddr, 25, []pkbytes{{0, OP_DUP}, {1, OP_HASH160}, {2, OP_DATA_20}, {23, OP_EQUALVERIFY}, {24, OP_CHECKSIG}}, true},
		{ScriptAddr, scrCollectAddr, 142, []pkbytes{{0, OP_DATA_75}, {76, OP_DATA_65}}, false},
		{ScriptAddr, scrCollectAddr, 141, []pkbytes{{0, OP_DATA_74}, {75, OP_DATA_65}}, false},
		{ScriptAddr, scrCollectAddr, 140, []pkbytes{{0, OP_DATA_73}, {74, OP_DATA_65}}, false},
		{ScriptAddr, scrCollectAddr, 139, []pkbytes{{0, OP_DATA_72}, {73, OP_DATA_65}}, false},
		{ScriptAddr, scrCollectAddr, 138, []pkbytes{{0, OP_DATA_71}, {72, OP_DATA_65}}, false},
		{ScriptAddr, scrCollectAddr, 137, []pkbytes{{0, OP_DATA_70}, {71, OP_DATA_65}}, false},
		{ScriptAddr, scrCollectAddr, 136, []pkbytes{{0, OP_DATA_69}, {70, OP_DATA_65}}, false},
		{ScriptAddr, scrCollectAddrComp, 110, []pkbytes{{0, OP_DATA_75}, {76, OP_DATA_33}}, false},
		{ScriptAddr, scrCollectAddrComp, 109, []pkbytes{{0, OP_DATA_74}, {75, OP_DATA_33}}, false},
		{ScriptAddr, scrCollectAddrComp, 108, []pkbytes{{0, OP_DATA_73}, {74, OP_DATA_33}}, false},
		{ScriptAddr, scrCollectAddrComp, 107, []pkbytes{{0, OP_DATA_72}, {73, OP_DATA_33}}, false},
		{ScriptAddr, scrCollectAddrComp, 106, []pkbytes{{0, OP_DATA_71}, {72, OP_DATA_33}}, false},
		{ScriptAddr, scrCollectAddrComp, 105, []pkbytes{{0, OP_DATA_70}, {71, OP_DATA_33}}, false},
		{ScriptAddr, scrCollectAddrComp, 104, []pkbytes{{0, OP_DATA_69}, {70, OP_DATA_33}}, false},
		{ScriptPubKey, scrGeneratePubkeyAddr, 74, []pkbytes{{0, OP_DATA_73}}, false},
		{ScriptPubKey, scrGeneratePubkeyAddr, 73, []pkbytes{{0, OP_DATA_72}}, false},
		{ScriptPubKey, scrGeneratePubkeyAddr, 72, []pkbytes{{0, OP_DATA_71}}, false},
		{ScriptPubKey, scrGeneratePubkeyAddr, 71, []pkbytes{{0, OP_DATA_70}}, false},
		{ScriptPubKey, scrGeneratePubkeyAddr, 70, []pkbytes{{0, OP_DATA_69}}, false},
		{ScriptPubKey, scrPubkeyAddr, 67, []pkbytes{{0, OP_DATA_65}, {66, OP_CHECKSIG}}, true},
		{ScriptPubKey, scrPubkeyAddrComp, 35, []pkbytes{{0, OP_DATA_33}, {34, OP_CHECKSIG}}, true},
		{ScriptStrange, scrNoAddr, 33, []pkbytes{{0, OP_DATA_32}}, false},
		{ScriptStrange, scrNoAddr, 33, []pkbytes{{0, OP_HASH160}, {1, OP_DATA_20}, {22, OP_EQUAL}}, false},
	}

	var format pkformat
	var success bool
	for _, format = range validformats {
		if format.length != len(script) {
			if len(script) < format.length {
				continue
			}
			if !format.allowmore {
				continue
			}
		}
		success = true
		for _, pkbyte := range format.databytes {
			if pkbyte.off >= len(script) {
				success = false
				break
			}
			if script[pkbyte.off] != pkbyte.val {
				log.Tracef("off at byte %v %v %v", pkbyte.off, script[pkbyte.off], pkbyte.val)
				success = false
				break
			} else {
				log.Tracef("match at byte %v: ok", pkbyte.off)
			}
		}
		if success == true {
			break
		}
	}

	if success == false && len(script) > 1 {
		// check for a few special case
		if script[len(script)-1] == OP_CHECK_MULTISIG {
			// Multisig ScriptPubKey
			return ScriptStrange, "Unknown", nil
		}
		if script[0] == OP_0 && (len(script) <= 75 && byte(len(script)) == script[1]+2) {
			// Multisig ScriptSig
			return ScriptStrange, "Unknown", nil
		}
		if script[0] == OP_HASH160 && len(script) == 23 && script[22] == OP_EQUAL {
			// Multisig ScriptSig
			return ScriptStrange, "Unknown", nil
		}
		if script[0] == OP_DATA_36 && len(script) == 37 {
			// Multisig ScriptSig
			return ScriptStrange, "Unknown", nil
		}

		return ScriptUnknown, "Unknown", StackErrUnknownAddress
	}

	var atype byte
	var abuf []byte
	var addr string
	switch format.parsetype {
	case scrPayAddr:
		atype = 0x00
		abuf = script[3:23]
	case scrCollectAddr:
		// script is replaced with the md160 of the pubkey
		slen := len(script)
		pubkey := script[slen-65:]
		abuf = calcHash160(pubkey)
	case scrCollectAddrComp:
		// script is replaced with the md160 of the pubkey
		slen := len(script)
		pubkey := script[slen-33:]
		abuf = calcHash160(pubkey)
	case scrGeneratePubkeyAddr:
		atype = 0x00
		addr = "Unknown"
	case scrNoAddr:
		addr = "Unknown"
	case scrPubkeyAddr:
		atype = 0x00
		pubkey := script[1:66]
		abuf = calcHash160(pubkey)
	case scrPubkeyAddrComp:
		atype = 0x00
		pubkey := script[1:34]
		abuf = calcHash160(pubkey)
	default:
		log.Warnf("parsetype is %v", format.parsetype)
	}

	if abuf != nil {
		addrbytes := append([]byte{atype}, abuf[:]...)

		cksum := btcwire.DoubleSha256(addrbytes)
		addrbytes = append(addrbytes, cksum[:4]...)
		addr = btcutil.Base58Encode(addrbytes)
	}

	return format.addrtype, addr, nil
}
