// Package bip322 implements generic message signing. For more details on
// BIP-322 see: https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
package bip322

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

const (
	// TagBIP0322SignedMsg is the BIP-0322 tag for a signed message. It is
	// declared as an untyped string constant so it cannot be mutated at
	// runtime (which would silently break all sign/verify operations in
	// the same process).
	TagBIP0322SignedMsg = "BIP0322-signed-message"

	// PrefixSimple is the signature prefix for the "simple" variant of
	// BIP-322.
	PrefixSimple = "smp"

	// PrefixFull is the signature prefix for the "full" variant of BIP-322.
	PrefixFull = "ful"

	// PrefixProofOfFunds is the signature prefix for the "proof of funds"
	// variant of BIP-322.
	PrefixProofOfFunds = "pof"

	// maxWitnessElements is the maximum number of witness elements allowed
	// in a witness stack (and also the maximum length of a stack item).
	maxWitnessItems = 4_000_000

	// maxTxBaseSize is the maximum size of a stripped (no witness data) TX
	// according to consensus rules.
	maxTxBaseSize = 1_000_000
)

var (
	// ErrInvalidSignature is the error returned when everything is formally
	// correct, but the actual signature verification fails.
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrInvalidSigHashFlag is returned when a signature in the witness or
	// signature script uses a sighash flag other than the one required by
	// BIP-322 (SIGHASH_ALL for ECDSA, SIGHASH_DEFAULT for P2TR).
	ErrInvalidSigHashFlag = errors.New("invalid sighash flag")

	// ErrCodeSeparator is returned when an OP_CODESEPARATOR is found in any
	// of the revealed/executed scripts of an input. BIP-322's required
	// rules forbid the use of OP_CODESEPARATOR. The script engine only
	// rejects it in non-segwit scripts (via ScriptVerifyConstScriptCode),
	// so we inspect segwit witness scripts and taproot leaf scripts
	// ourselves.
	ErrCodeSeparator = errors.New("OP_CODESEPARATOR is forbidden")

	// ErrInconclusive is returned when verification cannot reach a
	// definitive valid/invalid result because of a BIP-322 "upgradeable
	// rules" violation (e.g., a tx version other than 0 or 2). Per BIP-322
	// the validator must stop and output the "inconclusive" state in this
	// case.
	ErrInconclusive = errors.New("inconclusive")

	// ErrInvalidToSign is returned when the to_sign transaction structure
	// does not match the BIP-322 specification (wrong output count, wrong
	// output script, wrong first input outpoint, etc.).
	ErrInvalidToSign = errors.New("invalid to_sign transaction")

	// errSimpleSegWitOnly is returned when a non-native SegWit output is
	// attempted to be signed with the "simple" variant of BIP-322.
	errSimpleSegWitOnly = errors.New(
		"only native SegWit outputs (P2WPKH, P2WSH, P2TR) are " +
			"supported for the simple variant",
	)

	// errMoreDataAvailable is returned when more data is present after
	// parsing the witness stack.
	errMoreDataAvailable = errors.New(
		"more data present after parsing witness stack",
	)

	// errNilAddress is returned when a nil address is passed to a
	// verification function.
	errNilAddress = errors.New("address must not be nil")

	// errSignatureTooLarge is returned when a raw signature payload
	// exceeds maxSignatureBytes.
	errSignatureTooLarge = errors.New("signature payload too large")

	// b64Encode is a shortcut for the standard base64 encoding function.
	b64Encode = base64.StdEncoding.EncodeToString

	// b64Decode is a shortcut for the standard base64 decoding function.
	b64Decode = base64.StdEncoding.DecodeString
)

// TimeConstraints specifies constraints on the validation result of a message
// signature. If a result is Constrained = true, it means the witness stack
// validated correctly but has time lock properties that need to be evaluated
// separately.
type TimeConstraints struct {
	// Constrained indicates whether the provided witness stack has time or
	// age based restrictions (e.g., uses LockTime or Sequence check
	// opcodes).
	Constrained bool

	// ValidAtTime indicates the LockTime value that must be satisfied for
	// the witness stack to be valid.
	ValidAtTime uint32

	// ValidAtAge indicates the Sequence value that must be satisfied for
	// the witness stack to be valid.
	ValidAtAge uint32
}

// buildToSpendTx constructs a transaction to spend an output using the
// specified message and output script. It computes the message hash,
// constructs the scriptSig, and creates the to_spend transaction, according to
// BIP-322.
func buildToSpendTx(message, outPkScript []byte) *wire.MsgTx {
	// Compute the message tagged hash:
	// SHA256(SHA256(tag) || SHA256(tag) || message).
	messageHash := *chainhash.TaggedHash(
		[]byte(TagBIP0322SignedMsg), message,
	)

	// Construct the scriptSig - OP_0 PUSH32[ message_hash ].
	scriptSig := append([]byte{0x00, 0x20}, messageHash[:]...)

	// Create to_spend transaction in accordance to BIP-322:
	// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki#full
	return &wire.MsgTx{
		Version:  0,
		LockTime: 0,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Index: 0xFFFFFFFF,
			},
			Sequence:        0,
			SignatureScript: scriptSig,
			Witness:         wire.TxWitness{},
		}},
		TxOut: []*wire.TxOut{{
			Value:    0,
			PkScript: outPkScript,
		}},
	}
}

// isNativeSegWitPkScript returns true iff pkScript is one of the native SegWit
// script types (P2WPKH, P2WSH, P2TR) supported by the BIP-322 "simple" variant.
func isNativeSegWitPkScript(pkScript []byte) bool {
	return txscript.IsPayToWitnessPubKeyHash(pkScript) ||
		txscript.IsPayToWitnessScriptHash(pkScript) ||
		txscript.IsPayToTaproot(pkScript)
}

// deduplicateNonWitnessUtxos deduplicates a PSBT packet's input NonSegwitUTXO
// fields for the special case allowed in BIP-322 for Proof of Funds packets.
func deduplicateNonWitnessUtxos(packet *psbt.Packet) {
	for idx := range packet.Inputs {
		if idx == 0 || packet.Inputs[idx].NonWitnessUtxo == nil {
			continue
		}

		currentTx := packet.Inputs[idx].NonWitnessUtxo
		for inner := 0; inner < idx; inner++ {
			if packet.Inputs[inner].NonWitnessUtxo == nil {
				continue
			}

			prevTx := packet.Inputs[inner].NonWitnessUtxo
			if prevTx.TxHash() == currentTx.TxHash() {
				// We already have this TX in a previous input,
				// so we can deduplicate it.
				packet.Inputs[idx].NonWitnessUtxo = nil
				break
			}
		}
	}
}

// BuildToSignPacketSimple constructs a transaction template PSBT packet to
// prepare for signing, using the message and spend pkScript for the "simple"
// variant of the BIP-322 specification. It creates the to_sign transaction
// template, according to BIP-322. This can only be used for native SegWit
// outputs (P2WPKH, P2WSH, P2TR), and the spend pkScript is validated
// accordingly.
func BuildToSignPacketSimple(message, pkScript []byte) (*psbt.Packet, error) {
	// Enforce an inclusion list: only native SegWit outputs are valid for
	// the simple variant. Any other script type (legacy P2PKH/P2SH, bare
	// multisig, OP_RETURN, unknown future witness versions, etc.) must use
	// the full variant.
	if !isNativeSegWitPkScript(pkScript) {
		return nil, errSimpleSegWitOnly
	}

	return BuildToSignPacketFull(message, pkScript, 0, 0, 0), nil
}

// BuildToSignPacketFull constructs a transaction template PSBT packet to
// prepare for signing, using the message, spend pkScript, and tx parameters for
// the "full" variant of the BIP-322 specification. It creates the to_sign
// transaction template, according to BIP-322.
func BuildToSignPacketFull(message, spendPkScript []byte,
	txVersion int32, lockTime, sequence uint32) *psbt.Packet {

	msg := string(message)
	spendTx := buildToSpendTx(message, spendPkScript)

	// Create to_sign transaction in accordance to BIP-322:
	// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki#full
	packet := &psbt.Packet{
		UnsignedTx: &wire.MsgTx{
			Version:  txVersion,
			LockTime: lockTime,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{
					Hash:  spendTx.TxHash(),
					Index: 0,
				},
				Sequence: sequence,
			}},
			TxOut: []*wire.TxOut{{
				Value:    0,
				PkScript: []byte{txscript.OP_RETURN},
			}},
		},
		Inputs: []psbt.PInput{{
			WitnessUtxo: &wire.TxOut{
				Value:    0,
				PkScript: spendPkScript,
			},
			NonWitnessUtxo: spendTx,
		}},
		Outputs:              []psbt.POutput{{}},
		GenericSignedMessage: &msg,
	}

	// Legacy scripts can't have a witness UTXO, otherwise the PSBT
	// extraction will fail.
	if txscript.IsPayToPubKey(spendPkScript) ||
		txscript.IsPayToPubKeyHash(spendPkScript) {

		packet.Inputs[0].WitnessUtxo = nil
	}

	return packet
}

// SerializeTxWitness returns the wire witness stack as raw bytes.
func SerializeTxWitness(txWitness wire.TxWitness) ([]byte, error) {
	var witnessBytes bytes.Buffer
	err := psbt.WriteTxWitness(&witnessBytes, txWitness)
	if err != nil {
		return nil, fmt.Errorf("error serializing witness: %w", err)
	}

	return witnessBytes.Bytes(), nil
}

// SerializeSignature serializes the signature of a finalized PSBT packet of a
// BIP-322 to_sign transaction. According to the rules described in the BIP,
// this writes the signature as one of the three formats:
//  1. Simple (smp): Version and LockTime are 0, only one input with Sequence 0.
//  2. Full (ful): Version or LockTime or Sequence are non-zero, single input.
//  3. Proof of Funds (pof): Version or LockTime or Sequence are non-zero,
//     multiple inputs.
func SerializeSignature(finalizedPacket *psbt.Packet) (string, error) {
	if finalizedPacket == nil || finalizedPacket.UnsignedTx == nil {
		return "", errors.New("nil or incomplete packet")
	}

	// Prevent us from panicking on either if the length doesn't match.
	if len(finalizedPacket.Inputs) != len(finalizedPacket.UnsignedTx.TxIn) {
		return "", errors.New("input and txin length mismatch")
	}

	// At this point at least one input must be provided.
	if len(finalizedPacket.Inputs) == 0 {
		return "", errors.New("missing inputs")
	}

	// There is no exported IsFinalized function. But calling
	// MaybeFinalizeAll on an already finalized packet should not produce an
	// error if it's already finalized.
	if err := psbt.MaybeFinalizeAll(finalizedPacket); err != nil {
		return "", fmt.Errorf("packet must be finalizable: %w", err)
	}

	strippedTxSize := finalizedPacket.UnsignedTx.SerializeSizeStripped()
	if strippedTxSize > maxTxBaseSize {
		str := fmt.Sprintf("serialized transaction is too big - got "+
			"%d, max %d", strippedTxSize, maxTxBaseSize)
		return "", fmt.Errorf("%w: %s", ErrInvalidToSign, str)
	}

	tx := finalizedPacket.UnsignedTx
	utxo := finalizedPacket.Inputs[0].WitnessUtxo
	if utxo == nil {
		if finalizedPacket.Inputs[0].NonWitnessUtxo == nil {
			return "", errors.New("missing utxo")
		}

		// The to_spend transaction must have exactly one output to be
		// a valid BIP-322 previous transaction to the to_sign's first
		// input.
		prevTx := finalizedPacket.Inputs[0].NonWitnessUtxo
		if len(prevTx.TxOut) != 1 {
			return "", errors.New("invalid non witness UTXO")
		}

		utxo = prevTx.TxOut[0]
	}

	// Detect the variant of the signature.
	switch {
	// Proof of Fund (pof) variant has multiple inputs.
	case len(finalizedPacket.Inputs) > 1:
		// Collapse duplicated NonWitnessUtxo fields into a single one
		// to save space (special case allowed by BIP-322).
		deduplicateNonWitnessUtxos(finalizedPacket)

		content, err := finalizedPacket.B64Encode()
		if err != nil {
			return "", fmt.Errorf("error encoding packet: %w", err)
		}

		return PrefixProofOfFunds + content, nil

	// Full (ful) variant has non-zero version, locktime, or sequence or a
	// non-native SegWit input.
	case tx.Version != 0 || tx.LockTime != 0 || tx.TxIn[0].Sequence != 0 ||
		!isNativeSegWitPkScript(utxo.PkScript):

		signedTx, err := psbt.Extract(finalizedPacket)
		if err != nil {
			return "", fmt.Errorf("error extracting packet: %w",
				err)
		}

		var signedTxBytes bytes.Buffer
		err = signedTx.Serialize(&signedTxBytes)
		if err != nil {
			return "", fmt.Errorf("error serializing signed tx: "+
				"%w", err)
		}

		content := b64Encode(signedTxBytes.Bytes())
		return PrefixFull + content, nil

	// The simple (smp) variant is used if the above cases don't match.
	default:
		signedTx, err := psbt.Extract(finalizedPacket)
		if err != nil {
			return "", fmt.Errorf("error extracting packet: %w",
				err)
		}

		witnessBytes, err := SerializeTxWitness(
			signedTx.TxIn[0].Witness,
		)
		if err != nil {
			return "", fmt.Errorf("error serializing witness: %w",
				err)
		}

		content := b64Encode(witnessBytes)
		return PrefixSimple + content, nil
	}
}

// ParseTxWitness parses the raw witness bytes into a wire.TxWitness. This
// function rejects trailing data that isn't part of the expected encoding.
func ParseTxWitness(rawWitness []byte) (wire.TxWitness, error) {
	if len(rawWitness) > maxWitnessItems {
		return nil, fmt.Errorf("%w [size %d, max %d]",
			errSignatureTooLarge, len(rawWitness), maxWitnessItems)
	}

	var (
		buf    [8]byte
		reader = bytes.NewReader(rawWitness)
	)
	witCount, err := wire.ReadVarIntBuf(reader, 0, buf[:])
	if err != nil {
		return nil, err
	}

	// Prevent memory exhaustion via a huge witness item count. The upfront
	// make([][]byte, witCount) allocates ~24 bytes per entry on 64-bit.
	if witCount > maxWitnessItems {
		return nil, fmt.Errorf("too many witness items "+
			"[count %d, max %d]", witCount, maxWitnessItems)
	}

	// It is theoretically possible (and likely consensus valid in a
	// non-taproot context before MAX_STACK size was introduced) to create
	// a witness stack of maxWitnessItems/2 empty items. But to avoid a
	// trivial memory allocation of maxWitnessItems elements with a single
	// varInt claiming maxWitnessItems number of items without providing
	// them, we pre-allocate with a much smaller size. This doesn't prevent
	// a TX to actually contain maxWitnessItems empty items, but they
	// actually need to be encoded fully. Which means the raw witness would
	// be very long and noticeable by the user. So this simply prevents a
	// ~96MB memory allocation (on a 64bit system) for a seemingly harmless
	// and short encoded witness.
	allocCount := witCount
	if allocCount > txscript.MaxStackSize {
		allocCount = txscript.MaxStackSize
	}

	// Then for witCount number of stack items, each item has a varint
	// length prefix, followed by the witness item itself.
	witness := make([][]byte, 0, allocCount)
	for j := uint64(0); j < witCount; j++ {
		count, err := wire.ReadVarIntBuf(reader, 0, buf[:])
		if err != nil {
			return nil, err
		}

		// Enforce the per-item size cap.
		if count > maxWitnessItems {
			return nil, fmt.Errorf("witness item too large "+
				"[count %d, max %d]", count, maxWitnessItems)
		}

		// Validate against the remaining input length BEFORE
		// allocating. Without this, an attacker claiming a large item
		// but providing no data still triggers a full-size allocation.
		if int64(count) > int64(reader.Len()) {
			return nil, fmt.Errorf("witness item length %d "+
				"exceeds remaining input %d", count,
				reader.Len())
		}

		item := make([]byte, count)
		if _, err := io.ReadFull(reader, item); err != nil {
			return nil, err
		}

		witness = append(witness, item)
	}

	// If we didn't fully read all the data, this probably isn't a proper
	// witness stack.
	if reader.Len() > 0 {
		return nil, errMoreDataAvailable
	}

	return witness, nil
}

// ParseTx parses the raw transaction bytes into a wire.MsgTx. The input is
// capped at maxSignatureBytes to bound the worst-case memory usage during
// deserialization. This function rejects trailing data that isn't part of the
// expected encoding.
func ParseTx(rawTx []byte) (*wire.MsgTx, error) {
	if len(rawTx) > maxWitnessItems {
		return nil, fmt.Errorf("%w [size %d, max %d]",
			errSignatureTooLarge, len(rawTx), maxWitnessItems)
	}

	var (
		tx     wire.MsgTx
		reader = bytes.NewReader(rawTx)
	)
	err := tx.Deserialize(reader)
	if err != nil {
		return nil, fmt.Errorf("error parsing transaction: %w", err)
	}

	// If we didn't fully read all the data, this probably isn't a proper
	// transaction, if there's trailing data.
	if reader.Len() > 0 {
		return nil, errMoreDataAvailable
	}

	return &tx, nil
}

// ParsePsbt parses the given bytes as a PSBT packet. This function rejects
// trailing data that isn't part of the expected encoding.
func ParsePsbt(rawPacket []byte) (*psbt.Packet, error) {
	reader := bytes.NewReader(rawPacket)
	signaturePacket, err := psbt.NewFromRawBytes(reader,false)
	if err != nil {
		return nil, fmt.Errorf("error parsing packet: %w", err)
	}

	// NewBytesFromRawBytes already checks that the reader is exhausted, so
	// we don't need an additional check here.
	return signaturePacket, nil
}

// VerifyMessage verifies a message signed with the "simple", "full" or
// "Proof of Funds" variant of BIP-322 against the given address. The signature
// is expected to be base64-encoded.
//
// Return value semantics:
//   - On success: returns (true, {Constrained, ValidAtTime, validAtAge}, nil).
//   - On any form of verification failure (malformed signature, wrong message,
//     wrong address, invalid script, etc.): returns (false, {}, non-nil error).
//   - The boolean is never true when err != nil; callers may check either.
//
// Timelock semantics (full and proof of funds variant): the transaction
// version, locktime, and sequence are taken from the caller-supplied signature
// and used to rebuild the to_sign sighash. The signature commits to those
// values, so they can't be forged — but a signer may legitimately choose any
// values that satisfy CSV/CLTV opcodes inside the script. A successful
// verification therefore proves the signer can satisfy the script; it does NOT
// prove they could spend the associated on-chain output at the current chain
// tip.
func VerifyMessage(message string, address address.Address,
	signature string) (bool, TimeConstraints, error) {

	empty := TimeConstraints{}
	if address == nil {
		return false, empty, errNilAddress
	}

	challengeScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return false, empty, fmt.Errorf("error creating pkScript: %w",
			err)
	}

	return verifyMessageForChallenge(
		[]byte(message), challengeScript, signature,
	)
}

// verifyMessageForChallenge verifies the given signature against the given
// message and challenge script.
func verifyMessageForChallenge(message, challengeScript []byte,
	signature string) (bool, TimeConstraints, error) {

	// Even without a prefix, the signature needs to be at least 3
	// characters.
	empty := TimeConstraints{}
	if len(signature) < 3 {
		return false, empty, errors.New("signature too short")
	}

	switch {
	// Full (ful) variant.
	case strings.HasPrefix(signature, PrefixFull):
		signatureBytes, err := base64.StdEncoding.DecodeString(
			signature[len(PrefixFull):],
		)
		if err != nil {
			return false, empty, fmt.Errorf("error base64 "+
				"decoding signature as full variant: %w", err)
		}

		tx, err := ParseTx(signatureBytes)
		if err != nil {
			return false, empty, fmt.Errorf("error parsing "+
				"signature as full variant: %w", err)
		}

		if len(tx.TxIn) == 0 {
			return false, empty, errors.New("no inputs in " +
				"transaction")
		}

		// The Proof of Funds variant requires us to have the previous
		// outputs of the additional inputs. So it needs to be encoded
		// differently (the full finalized PSBT packet) and also have a
		// different prefix.
		if len(tx.TxIn) > 1 {
			return false, empty, errors.New("proof of funds " +
				"variant with incorrect prefix supplied")
		}

		// BIP-322 verification process basic validation: confirm the
		// supplied to_sign transaction has the structure required by
		// the BIP. This is checked here (before reconstruction) so
		// malformed to_sign transactions are rejected explicitly
		// rather than slipping through unchecked because reconstruction
		// ignores the parts that get rebuilt.
		toSpendHash := buildToSpendTx(message, challengeScript).TxHash()
		if err := validateToSignTx(tx, toSpendHash); err != nil {
			return false, empty, err
		}

		// BIP-322 upgradeable rules: tx version must be 0 or 2.
		// A failure here returns ErrInconclusive per spec.
		if err := validateUpgradeableRules(tx.Version); err != nil {
			return false, empty, err
		}

		firstIn := tx.TxIn[0]
		return VerifyMessageFull(
			message, challengeScript, firstIn.SignatureScript,
			firstIn.Witness, tx.Version, tx.LockTime,
			firstIn.Sequence,
		)

	// Proof of Funds (pof) variant.
	case strings.HasPrefix(signature, PrefixProofOfFunds):
		signatureBytes, err := base64.StdEncoding.DecodeString(
			signature[len(PrefixProofOfFunds):],
		)
		if err != nil {
			return false, empty, fmt.Errorf("error base64 "+
				"decoding signature as pof variant: %w", err)
		}

		signaturePacket, err := ParsePsbt(signatureBytes)
		if err != nil {
			return false, empty, fmt.Errorf("error parsing "+
				"signature as pof variant: %w", err)
		}

		return VerifyMessagePoF(
			message, challengeScript, signaturePacket,
		)

	// If there is no prefix, we fall back to simple.
	default:
		sig := strings.TrimPrefix(signature, PrefixSimple)
		signatureBytes, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			return false, empty, fmt.Errorf("error decoding "+
				"signature as base64: %w", err)
		}

		witness, err := ParseTxWitness(signatureBytes)
		if err != nil {
			return false, empty, fmt.Errorf("error parsing "+
				"signature as witness: %w", err)
		}

		return VerifyMessageSimple(message, challengeScript, witness)
	}
}

// VerifyMessageSimple verifies a message signed with the "simple" variant of
// BIP-322. The witness stack must be the complete stack, including signatures,
// script inputs, the script itself, the control block, and so on (depending on
// the script type). This variant can only be used for fully native SegWit
// outputs (P2WPKH, P2WSH, P2TR). Any other script type (legacy, nested, bare,
// or unknown future witness versions) must use the "full" variant.
func VerifyMessageSimple(message, pkScript []byte,
	witness wire.TxWitness) (bool, TimeConstraints, error) {

	if !isNativeSegWitPkScript(pkScript) {
		return false, TimeConstraints{}, errSimpleSegWitOnly
	}

	return VerifyMessageFull(message, pkScript, nil, witness, 0, 0, 0)
}

// VerifyMessageFull verifies a message signed with the "full" variant of
// BIP-322. The witness stack must be the complete stack, including signatures,
// script inputs, the script itself, the control block, and so on (depending on
// the script type). The sigScript must only be set for legacy (P2PKH, P2SH,
// NP2WPKH) address types.
func VerifyMessageFull(message, pkScript []byte, sigScript []byte,
	witness wire.TxWitness, txVersion int32, lockTime,
	sequence uint32) (bool, TimeConstraints, error) {

	// First, we create the full version of the to_sign tx.
	toSign := BuildToSignPacketFull(
		message, pkScript, txVersion, lockTime, sequence,
	)

	// We know this should never be the case, but let's just be safe.
	empty := TimeConstraints{}
	if len(toSign.Inputs) != 1 {
		return false, empty, errors.New("to_sign tx should have one " +
			"input")
	}

	witnessBytes, err := SerializeTxWitness(witness)
	if err != nil {
		return false, empty, fmt.Errorf("error serializing witness: %w",
			err)
	}

	// The finalized packet is just the toSign with the final script and
	// witness sig applied.
	toSign.Inputs[0].FinalScriptWitness = witnessBytes
	toSign.Inputs[0].FinalScriptSig = sigScript

	return VerifyMessagePoF(message, pkScript, toSign)
}

// VerifyMessagePoF verifies a message signed with the "Proof of Funds" variant
// of BIP-322. The sigPacket must be the complete, finalized PSBT packet of the
// to_sign transaction, including all witness stacks and UTXO information.
func VerifyMessagePoF(message, pkScript []byte,
	sigPacket *psbt.Packet) (bool, TimeConstraints, error) {

	// Do some basic validation on the signature packet. We can use the
	// SerializeSignature function that checks the inputs and UTXO for the
	// first input at least.
	empty := TimeConstraints{}
	_, err := SerializeSignature(sigPacket)
	if err != nil {
		return false, empty, fmt.Errorf("invalid signature packet: %w",
			err)
	}

	// BIP-322 verification process basic validation, applied to
	// the unsigned tx embedded in the PSBT.
	toSpendHash := buildToSpendTx(message, pkScript).TxHash()
	err = validateToSignTx(sigPacket.UnsignedTx, toSpendHash)
	if err != nil {
		return false, empty, err
	}

	// BIP-322 upgradeable rules: tx version must be 0 or 2.
	err = validateUpgradeableRules(sigPacket.UnsignedTx.Version)
	if err != nil {
		return false, empty, err
	}

	// Check the version, which must be 0 or 2 for BIP-322.
	if sigPacket.UnsignedTx.Version != 0 &&
		sigPacket.UnsignedTx.Version != 2 {

		return false, empty, fmt.Errorf("%w: invalid tx version",
			ErrInconclusive)
	}

	// We now create the toSign packet as if we only had a single input.
	toSign := BuildToSignPacketFull(
		message, pkScript, sigPacket.UnsignedTx.Version,
		sigPacket.UnsignedTx.LockTime,
		sigPacket.UnsignedTx.TxIn[0].Sequence,
	)

	// We then ONLY copy over the witness for the first input and the
	// additional inputs from the supplied packet, everything else must come
	// from the packet we just built for the verification to be meaningful.
	// Otherwise, a user could just supply whatever.
	toSign.Inputs[0].FinalScriptWitness =
		sigPacket.Inputs[0].FinalScriptWitness
	toSign.Inputs[0].FinalScriptSig = sigPacket.Inputs[0].FinalScriptSig
	for idx := 1; idx < len(sigPacket.Inputs); idx++ {
		toSign.UnsignedTx.TxIn = append(
			toSign.UnsignedTx.TxIn, sigPacket.UnsignedTx.TxIn[idx],
		)
		toSign.Inputs = append(toSign.Inputs, sigPacket.Inputs[idx])
	}

	// findUtxo is a helper for finding the UTXO information for an input at
	// a given index. It implements the special BIP-0322 optimization where
	// multiple non-witness inputs that spend from the same transaction can
	// re-use the same NonWitnessUtxo field.
	findUtxo := func(idx int) (*wire.TxOut, error) {
		if idx < 0 || idx >= len(toSign.Inputs) {
			return nil, fmt.Errorf("invalid input index %d", idx)
		}

		in := toSign.Inputs[idx]
		txIn := toSign.UnsignedTx.TxIn[idx]
		switch {
		// The non-witness UTXO is present, we need to look up the
		// correct output. We prefer the NonWitnessUtxo over the
		// WitnessUtxo because it allows signers to validate all
		// amounts. That's why a lot of wallets set both fields.
		case in.NonWitnessUtxo != nil:
			prevTx := in.NonWitnessUtxo
			prevOutIdx := txIn.PreviousOutPoint.Index
			if prevOutIdx >= uint32(len(prevTx.TxOut)) {
				return nil, fmt.Errorf("invalid non witness "+
					"utxo for input index %d", idx)
			}

			if prevTx.TxHash() != txIn.PreviousOutPoint.Hash {
				return nil, fmt.Errorf("non witness utxo " +
					"does not match input prevout")
			}

			return prevTx.TxOut[prevOutIdx], nil

		// Only the witness UTXO is present, which we only allow for
		// witness inputs.
		case in.WitnessUtxo != nil:
			if len(in.FinalScriptWitness) == 0 {
				return nil, fmt.Errorf("only witness utxo " +
					"present for non-witness script type")
			}

			return in.WitnessUtxo, nil

		// Neither is present. This normally wouldn't be valid, but the
		// BIP-0322 defines a special exception to save space: If
		// multiple non-witness inputs spend from the same transaction,
		// only the first one in the list needs to specify the
		// NonWitnessUtxo and the others can re-use it. So we do a
		// lookup here.
		default:
			// If there is only a single input, then this special
			// case doesn't apply and the packet is invalid.
			numInputs := len(toSign.Inputs)
			if numInputs == 1 {
				return nil, fmt.Errorf("invalid signature "+
					"packet: no UTXO for input index %d",
					idx)
			}

			// The way a BIP-0322 packet is created, the very first
			// input spends the to_spend transaction, which can't
			// be re-used. So we start at index
			targetHash := txIn.PreviousOutPoint
			for lookupIdx := 1; lookupIdx < numInputs; lookupIdx++ {
				lookupInput := toSign.Inputs[lookupIdx]
				prevTx := lookupInput.NonWitnessUtxo
				if prevTx == nil {
					continue
				}

				if prevTx.TxHash() != targetHash.Hash {
					continue
				}

				prevOutIdx := txIn.PreviousOutPoint.Index
				if prevOutIdx >= uint32(len(prevTx.TxOut)) {
					return nil, fmt.Errorf("invalid non "+
						"witness utxo for input "+
						"index %d", idx)
				}

				return prevTx.TxOut[prevOutIdx], nil
			}

			// If we got here without finding a matching UTXO,
			// the packet is invalid.
			return nil, fmt.Errorf("invalid signature packet: "+
				"UTXO not found for input index %d", idx)
		}
	}

	utxos := make(map[wire.OutPoint]*wire.TxOut, len(toSign.Inputs))
	for idx := range toSign.Inputs {
		txIn := toSign.UnsignedTx.TxIn[idx]

		// Duplicate inputs are not allowed.
		_, exists := utxos[txIn.PreviousOutPoint]
		if exists {
			return false, empty, fmt.Errorf("duplicate inputs " +
				"present")
		}

		// The to_sign transaction also must have valid previous
		// transaction hashes, which means a null hash is not allowed.
		if txIn.PreviousOutPoint.Hash == (chainhash.Hash{}) {
			return false, empty, fmt.Errorf("zero hash present " +
				"in input")
		}

		utxos[txIn.PreviousOutPoint], err = findUtxo(idx)
		if err != nil {
			return false, empty, fmt.Errorf("error finding UTXO: "+
				"%w", err)
		}
	}

	// Make sure the previous output amounts don't violate any consensus
	// rules that can be checked without having the chain (mostly min/max
	// values).
	err = validateAmounts(slices.Collect(maps.Values(utxos)))
	if err != nil {
		return false, empty, err
	}

	// Prepare the validation helpers and then extract the final transaction
	// from the packet, now that we've parsed the UTXO information from it.
	finalTx, err := psbt.Extract(toSign)
	if err != nil {
		return false, empty, fmt.Errorf("error extracting final "+
			"transaction: %w", err)
	}

	prevOutFetcher := txscript.NewMultiPrevOutFetcher(utxos)
	sigHashes := txscript.NewTxSigHashes(finalTx, prevOutFetcher)

	// In case there are any time locks involved in the unlocking script, we
	// need to inform the validator about that. The BIP explicitly only
	// mentions the sequence of the first input (the input related to the
	// challenge pkScript).
	constraints := TimeConstraints{
		ValidAtTime: finalTx.LockTime,
		ValidAtAge:  toSign.UnsignedTx.TxIn[0].Sequence,
	}
	constraints.Constrained = constraints.ValidAtAge != 0 ||
		constraints.ValidAtTime != 0

	// Verify the signature (full witness stack) of each input using the
	// btcd script engine. This is what makes it possible to verify multisig
	// or even custom scripts.
	for idx := range toSign.Inputs {
		txIn := toSign.UnsignedTx.TxIn[idx]
		finalIn := finalTx.TxIn[idx]
		utxo := utxos[txIn.PreviousOutPoint]

		// BIP-322 required rule: the use of OP_CODESEPARATOR is
		// forbidden. The script engine (even with StandardVerifyFlags)
		// only rejects it in non-segwit scripts, so we inspect the
		// segwit witness scripts and taproot leaf scripts here.
		err = validateNoCodeSeparator(
			utxo.PkScript, finalIn.Witness, finalIn.SignatureScript,
		)
		if err != nil {
			return false, empty, err
		}

		// BIP-322 required rule: all signatures must use SIGHASH_ALL
		// (or SIGHASH_DEFAULT for BIP341 P2TR inputs). We enforce this
		// with the ScriptVerifyRestrictSigHash flag, which makes the
		// script engine reject any signature using a different sighash
		// type. Unlike inspecting the witness up front, this constrains
		// every signature the engine actually verifies, including those
		// consumed by multisig and custom (e.g. tapscript) scripts.
		vm, err := txscript.NewEngine(
			utxo.PkScript, finalTx, idx,
			txscript.StandardVerifyFlags|
				txscript.ScriptVerifyRestrictSigHash,
			nil, sigHashes, utxo.Value, prevOutFetcher,
		)
		if err != nil {
			return false, empty, fmt.Errorf("error creating "+
				"txscript engine: %w", err)
		}

		err = vm.Execute()
		if err != nil {
			// The Error() function on the VM's Error struct doesn't
			// return a nice error message. So we parse the error to
			// get something more descriptive. If parsing fails, we
			// at least make the error detectable as signature
			// verification having failed.
			var vmErr txscript.Error
			if !errors.As(err, &vmErr) {
				return false, empty, fmt.Errorf("%w: %w",
					ErrInvalidSignature, err)
			}

			// Re-map some of the VM internal errors to explicit
			// BIP-322 errors.
			switch vmErr.ErrorCode {
			// Surface a disallowed sighash type as a dedicated
			// BIP-322 error so callers can distinguish it from a
			// generic signature failure.
			case txscript.ErrDisallowedSigHashType:
				return false, empty, ErrInvalidSigHashFlag

			// Map the "upgradeable rules" errors from the VM to the
			// BIP-322 "inconclusive" error.
			case txscript.ErrDiscourageUpgradableNOPs,
				txscript.ErrDiscourageUpgradableWitnessProgram:

				return false, empty, ErrInconclusive

			// NOTE: ErrDiscourageUpgradeableTaprootVersion (unknown
			// tapscript leaf version) and ErrDiscourageOpSuccess
			// (tapscript OP_SUCCESSx opcode) are NOT remapped here.
			// Both are arguably "upgradeable" in spirit
			// (BIP-341/342 reserve them for future soft forks), but
			// BIP-322 §4 does not list them, so we treat them as
			// `invalid` - matching the literal spec text.
			default:
				return false, empty, fmt.Errorf("%w: "+
					"err_code=%s, err_desc=%s",
					ErrInvalidSignature,
					vmErr.ErrorCode.String(),
					vmErr.Description)
			}
		}
	}

	// Success, we have a valid signature.
	return true, constraints, nil
}
