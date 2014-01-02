// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

// ScriptType is an enum type that represents the type of a script. It is
// returned from ScriptToAddrHash as part of the metadata about the script.
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
	ScriptPayToScriptHash
	ScriptMultiSig
	ScriptStrange
	ScriptGeneration
)

var scriptTypeToName = []string{
	ScriptUnknown:         "Unknown",
	ScriptAddr:            "Addr",
	ScriptPubKey:          "Pubkey",
	ScriptPayToScriptHash: "PayToScriptHash",
	ScriptMultiSig:        "MultiSig",
	ScriptStrange:         "Strange",
	ScriptGeneration:      "Generation", // ScriptToAddrHash does not recieve enough information to identify Generation scripts.
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
	scrPayToScriptHash
	scrNoAddr
)

// ScriptToAddrHash extracts a 20-byte public key hash and the type out of a PkScript
func ScriptToAddrHash(script []byte) (ScriptType, []byte, error) {
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
		{ScriptPayToScriptHash, scrPayToScriptHash, 23, []pkbytes{{0, OP_HASH160}, {1, OP_DATA_20}, {22, OP_EQUAL}}, false},
		{ScriptStrange, scrNoAddr, 33, []pkbytes{{0, OP_DATA_32}}, false},
	}
	return scriptToAddrHashTemplate(script, validformats)
}

func scriptToAddrHashTemplate(script []byte, validformats []pkformat) (ScriptType, []byte, error) {
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
				return ScriptUnknown, nil,
					StackErrInvalidAddrOffset
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

	if success == false {
		if len(script) > 1 {
			// check for a few special case
			if script[len(script)-1] == OP_CHECK_MULTISIG {
				return ScriptStrange, nil, nil
			}
			if script[0] == OP_0 && (len(script) <= 75 && byte(len(script)) == script[1]+2) {
				return ScriptStrange, nil, nil
			}
			if script[0] == OP_DATA_36 && len(script) == 37 {
				// Multisig ScriptSig
				return ScriptStrange, nil, nil
			}
		}

		return ScriptUnknown, nil, StackErrUnknownAddress
	}

	var addrhash []byte
	switch format.parsetype {
	case scrPayAddr:
		addrhash = script[3:23]
	case scrCollectAddr:
		// script is replaced with the md160 of the pubkey
		slen := len(script)
		pubkey := script[slen-65:]
		addrhash = calcHash160(pubkey)
	case scrCollectAddrComp:
		// script is replaced with the md160 of the pubkey
		slen := len(script)
		pubkey := script[slen-33:]
		addrhash = calcHash160(pubkey)
	case scrGeneratePubkeyAddr:
		// unable to determine address hash from script
	case scrNoAddr:
		// unable to determine address hash from script
	case scrPubkeyAddr:
		pubkey := script[1:66]
		addrhash = calcHash160(pubkey)
	case scrPubkeyAddrComp:
		pubkey := script[1:34]
		addrhash = calcHash160(pubkey)
	case scrPayToScriptHash:
		addrhash = script[2:22]
	default:
		return ScriptUnknown, nil, StackErrInvalidParseType
	}

	return format.addrtype, addrhash, nil
}

// ScriptToAddrHashes extracts multiple 20-byte public key hashes
// from a MultiSig script.
func ScriptToAddrHashes(script []byte) (ScriptType, int, [][]byte, error) {
	pops, err := parseScript(script)
	if err != nil {
		return ScriptUnknown, 0, nil, StackErrUnknownAddress
	}

	if !isMultiSig(pops) {
		return ScriptUnknown, 0, nil, StackErrUnknownAddress
	}

	l := len(pops)
	addrHashes := make([][]byte, l-3)
	for i, pop := range pops[1 : l-2] {
		addrHashes[i] = calcHash160(pop.data)
	}

	reqSigs := int(pops[0].opcode.value - (OP_1 - 1))

	return ScriptMultiSig, reqSigs, addrHashes, nil
}
