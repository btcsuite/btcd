package txscript

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"golang.org/x/crypto/ripemd160"
)

const (
	// minPubKeyHashSigScriptLen is the minimum length of a signature script
	// that spends a P2PKH output. The length is composed of the following:
	//   Signature length (1 byte)
	//   Signature (min 8 bytes)
	//   Signature hash type (1 byte)
	//   Public key length (1 byte)
	//   Public key (33 byte)
	minPubKeyHashSigScriptLen = 1 + ecdsa.MinSigLen + 1 + 1 + 33

	// maxPubKeyHashSigScriptLen is the maximum length of a signature script
	// that spends a P2PKH output. The length is composed of the following:
	//   Signature length (1 byte)
	//   Signature (max 72 bytes)
	//   Signature hash type (1 byte)
	//   Public key length (1 byte)
	//   Public key (33 byte)
	maxPubKeyHashSigScriptLen = 1 + 72 + 1 + 1 + 33

	// compressedPubKeyLen is the length in bytes of a compressed public
	// key.
	compressedPubKeyLen = 33

	// pubKeyHashLen is the length of a P2PKH script.
	pubKeyHashLen = 25

	// witnessV0PubKeyHashLen is the length of a P2WPKH script.
	witnessV0PubKeyHashLen = 22

	// scriptHashLen is the length of a P2SH script.
	scriptHashLen = 23

	// witnessV0ScriptHashLen is the length of a P2WSH script.
	witnessV0ScriptHashLen = 34

	// witnessV1TaprootLen is the length of a P2TR script.
	witnessV1TaprootLen = 34

	// payToAnchorLen is the length of a P2A script.
	payToAnchorLen = 4

	// maxLen is the maximum script length supported by ParsePkScript.
	maxLen = witnessV0ScriptHashLen
)

var (
	// ErrUnsupportedScriptType is an error returned when we attempt to
	// parse/re-compute an output script into a PkScript struct.
	ErrUnsupportedScriptType = errors.New("unsupported script type")
)

// PkScript is a wrapper struct around a byte array, allowing it to be used
// as a map index.
type PkScript struct {
	// class is the type of the script encoded within the byte array. This
	// is used to determine the correct length of the script within the byte
	// array.
	class ScriptClass

	// script is the script contained within a byte array. If the script is
	// smaller than the length of the byte array, it will be padded with 0s
	// at the end.
	script [maxLen]byte
}

// ParsePkScript parses an output script into the PkScript struct.
// ErrUnsupportedScriptType is returned when attempting to parse an unsupported
// script type.
func ParsePkScript(pkScript []byte) (PkScript, error) {
	var outputScript PkScript
	scriptClass, _, _, err := ExtractPkScriptAddrs(
		pkScript, &chaincfg.MainNetParams,
	)
	if err != nil {
		return outputScript, fmt.Errorf("unable to parse script type: "+
			"%v", err)
	}

	if !isSupportedScriptType(scriptClass) {
		return outputScript, ErrUnsupportedScriptType
	}

	outputScript.class = scriptClass
	copy(outputScript.script[:], pkScript)

	return outputScript, nil
}

// isSupportedScriptType determines whether the script type is supported by the
// PkScript struct.
func isSupportedScriptType(class ScriptClass) bool {
	switch class {
	case PubKeyHashTy, WitnessV0PubKeyHashTy, ScriptHashTy,
		WitnessV0ScriptHashTy, WitnessV1TaprootTy, PayToAnchorTy:
		return true
	default:
		return false
	}
}

// Class returns the script type.
func (s PkScript) Class() ScriptClass {
	return s.class
}

// Script returns the script as a byte slice without any padding.
func (s PkScript) Script() []byte {
	var script []byte

	switch s.class {
	case PubKeyHashTy:
		script = make([]byte, pubKeyHashLen)
		copy(script, s.script[:pubKeyHashLen])

	case WitnessV0PubKeyHashTy:
		script = make([]byte, witnessV0PubKeyHashLen)
		copy(script, s.script[:witnessV0PubKeyHashLen])

	case ScriptHashTy:
		script = make([]byte, scriptHashLen)
		copy(script, s.script[:scriptHashLen])

	case WitnessV0ScriptHashTy:
		script = make([]byte, witnessV0ScriptHashLen)
		copy(script, s.script[:witnessV0ScriptHashLen])

	case WitnessV1TaprootTy:
		script = make([]byte, witnessV1TaprootLen)
		copy(script, s.script[:witnessV1TaprootLen])

	case PayToAnchorTy:
		script = make([]byte, payToAnchorLen)
		copy(script, s.script[:payToAnchorLen])

	default:
		// Unsupported script type.
		return nil
	}

	return script
}

// Address encodes the script into an address for the given chain.
func (s PkScript) Address(chainParams *chaincfg.Params) (address.Address, error) {
	_, addrs, _, err := ExtractPkScriptAddrs(s.Script(), chainParams)
	if err != nil {
		return nil, fmt.Errorf("unable to parse address: %v", err)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("script does not have an associated address")
	}

	return addrs[0], nil
}

// String returns a hex-encoded string representation of the script.
func (s PkScript) String() string {
	str, _ := DisasmString(s.Script())
	return str
}

// ComputePkScript computes the script of an output by looking at the spending
// input's signature script or witness.
//
// NOTE: Only P2PKH, P2SH, P2WSH, P2WPKH and P2TR script path spends are
// supported. A P2TR key path spend reveals only a signature, so it returns
// ErrUnsupportedScriptType.
func ComputePkScript(sigScript []byte, witness wire.TxWitness) (PkScript, error) {
	switch {
	case len(sigScript) > 0:
		return computeNonWitnessPkScript(sigScript)
	case len(witness) > 0:
		return computeWitnessPkScript(witness)
	default:
		return PkScript{}, ErrUnsupportedScriptType
	}
}

// computeNonWitnessPkScript computes the script of an output by looking at the
// spending input's signature script.
func computeNonWitnessPkScript(sigScript []byte) (PkScript, error) {
	switch {
	// Since we only support P2PKH and P2SH scripts as the only non-witness
	// script types, we should expect to see a push only script.
	case !IsPushOnlyScript(sigScript):
		return PkScript{}, ErrUnsupportedScriptType

	// If a signature script is provided with a length long enough to
	// represent a P2PKH script, then we'll attempt to parse the compressed
	// public key from it.
	case len(sigScript) >= minPubKeyHashSigScriptLen &&
		len(sigScript) <= maxPubKeyHashSigScriptLen:

		// The public key should be found as the last part of the
		// signature script. We'll attempt to parse it to ensure this is
		// a P2PKH redeem script.
		pubKey := sigScript[len(sigScript)-compressedPubKeyLen:]
		if btcec.IsCompressedPubKey(pubKey) {
			pubKeyHash := hash160(pubKey)
			script, err := payToPubKeyHashScript(pubKeyHash)
			if err != nil {
				return PkScript{}, err
			}

			pkScript := PkScript{class: PubKeyHashTy}
			copy(pkScript.script[:], script)
			return pkScript, nil
		}

		fallthrough

	// If we failed to parse a compressed public key from the script in the
	// case above, or if the script length is not that of a P2PKH one, we
	// can assume it's a P2SH signature script.
	default:
		// The redeem script will always be the last data push of the
		// signature script, so we'll parse the script into opcodes to
		// obtain it.
		const scriptVersion = 0
		err := checkScriptParses(scriptVersion, sigScript)
		if err != nil {
			return PkScript{}, err
		}
		redeemScript := finalOpcodeData(scriptVersion, sigScript)

		scriptHash := hash160(redeemScript)
		script, err := payToScriptHashScript(scriptHash)
		if err != nil {
			return PkScript{}, err
		}

		pkScript := PkScript{class: ScriptHashTy}
		copy(pkScript.script[:], script)
		return pkScript, nil
	}
}

// computeWitnessPkScript computes the script of an output by looking at the
// spending input's witness.
func computeWitnessPkScript(witness wire.TxWitness) (PkScript, error) {
	// Taproot is checked first, since a two item script path spend with
	// a 33 byte control block would otherwise be taken for P2WPKH.
	pkScript, isTaproot, err := computeTaprootPkScript(witness)
	if isTaproot {
		return pkScript, err
	}

	// We'll use the last item of the witness stack to determine the proper
	// witness type.
	lastWitnessItem := witness[len(witness)-1]

	switch {
	// If the witness stack has a size of 2 and its last item is a
	// compressed public key, then this is a P2WPKH witness.
	case len(witness) == 2 && len(lastWitnessItem) == compressedPubKeyLen:
		pubKeyHash := hash160(lastWitnessItem)
		script, err := payToWitnessPubKeyHashScript(pubKeyHash)
		if err != nil {
			return pkScript, err
		}

		pkScript.class = WitnessV0PubKeyHashTy
		copy(pkScript.script[:], script)

	// For any other witnesses, we'll assume it's a P2WSH witness.
	default:
		scriptHash := sha256.Sum256(lastWitnessItem)
		script, err := payToWitnessScriptHashScript(scriptHash[:])
		if err != nil {
			return pkScript, err
		}

		pkScript.class = WitnessV0ScriptHashTy
		copy(pkScript.script[:], script)
	}

	return pkScript, nil
}

// computeTaprootPkScript computes the script of a P2TR output from the
// witness of a script path spend. The second return value is false if the
// witness is not recognized as a taproot one.
//
// The witness is taproot if it ends in an annex, which starts with 0x50, or
// in a control block, whose first byte is a leaf version of at least 0xc0.
// Both are opcodes that fail when executed first in a witness script, so no
// spendable P2WPKH or P2WSH witness ends that way. A lone 64 or 65 byte item
// is taken as a key path signature, which has no script to derive the output
// key from, although it could also be a P2WSH script that takes no input.
func computeTaprootPkScript(witness wire.TxWitness) (PkScript, bool, error) {
	if len(witness) == 1 &&
		(len(witness[0]) == 64 || len(witness[0]) == 65) {

		return PkScript{}, true, ErrUnsupportedScriptType
	}

	stack := witness
	annexed := isAnnexedWitness(stack)
	if annexed {
		stack = stack[:len(stack)-1]
	}

	rawControlBlock := stack[len(stack)-1]
	isControlBlock := len(rawControlBlock) >= ControlBlockBaseSize &&
		len(rawControlBlock) <= ControlBlockMaxSize &&
		(len(rawControlBlock)-ControlBlockBaseSize)%
			ControlBlockNodeSize == 0 &&
		rawControlBlock[0]&TaprootLeafMask >= byte(BaseLeafVersion)

	switch {
	case !isControlBlock && !annexed:
		return PkScript{}, false, nil

	// A single item left is a key path spend.
	case !isControlBlock || len(stack) < 2:
		return PkScript{}, true, ErrUnsupportedScriptType
	}

	controlBlock, err := ParseControlBlock(rawControlBlock)
	if err != nil {
		return PkScript{}, true, err
	}

	script := stack[len(stack)-2]
	outputKey := ComputeTaprootOutputKey(
		controlBlock.InternalKey, controlBlock.RootHash(script),
	)
	pkScript, err := PayToTaprootScript(outputKey)
	if err != nil {
		return PkScript{}, true, err
	}

	result := PkScript{class: WitnessV1TaprootTy}
	copy(result.script[:], pkScript)

	return result, true, nil
}

// hash160 returns the RIPEMD160 hash of the SHA-256 HASH of the given data.
func hash160(data []byte) []byte {
	h := sha256.Sum256(data)
	return ripemd160h(h[:])
}

// ripemd160h returns the RIPEMD160 hash of the given data.
func ripemd160h(data []byte) []byte {
	h := ripemd160.New()
	h.Write(data)
	return h.Sum(nil)
}
