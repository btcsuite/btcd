package bip322

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

const (
	negativeVectorFileName = "generated-negative-test-vectors.json"

	// negativeMessage is the message all generated negative vectors are
	// created for.
	negativeMessage = "probe"
)

// fixedNegativeKey returns a deterministic private key for generated vectors.
// Distinct non-zero tags produce distinct keys without introducing randomness
// into the checked-in fixture.
func fixedNegativeKey(tag byte) *btcec.PrivateKey {
	keyBytes := make([]byte, 32)
	keyBytes[len(keyBytes)-1] = tag
	key, _ := btcec.PrivKeyFromBytes(keyBytes)

	return key
}

// negativeChallenge returns the scriptPubKey of the given address and its
// string encoding.
func negativeChallenge(t *testing.T, addr address.Address) ([]byte, string) {
	t.Helper()

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	return pkScript, addr.String()
}

// negativeP2WSHChallenge returns a P2WSH scriptPubKey and its mainnet address
// for a fixed witness script.
func negativeP2WSHChallenge(t *testing.T,
	witnessScript []byte) ([]byte, string) {

	t.Helper()

	scriptHash := sha256.Sum256(witnessScript)
	addr, err := address.NewAddressWitnessScriptHash(
		scriptHash[:], &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	return negativeChallenge(t, addr)
}

// negativeP2WPKHChallenge returns a P2WPKH scriptPubKey and its mainnet address
// for the given deterministic key.
func negativeP2WPKHChallenge(t *testing.T,
	key *btcec.PrivateKey) ([]byte, string) {

	t.Helper()

	pubKeyHash := address.Hash160(key.PubKey().SerializeCompressed())
	addr, err := address.NewAddressWitnessPubKeyHash(
		pubKeyHash, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	return negativeChallenge(t, addr)
}

// negativeTaprootChallenge commits a deterministic internal key to the given
// leaf script and returns the challenge, address, and control block.
func negativeTaprootChallenge(t *testing.T, leafScript []byte,
	internalKey *btcec.PrivateKey) ([]byte, string, []byte) {

	t.Helper()

	return negativeTapLeafChallenge(
		t, txscript.NewBaseTapLeaf(leafScript), internalKey,
	)
}

// negativeTapLeafChallenge commits a deterministic internal key to the given
// tap leaf of any leaf version and returns the challenge, address, and control
// block.
func negativeTapLeafChallenge(t *testing.T, leaf txscript.TapLeaf,
	internalKey *btcec.PrivateKey) ([]byte, string, []byte) {

	t.Helper()

	tree := txscript.AssembleTaprootScriptTree(leaf)
	rootHash := tree.RootNode.TapHash()
	outputKey := txscript.ComputeTaprootOutputKey(
		internalKey.PubKey(), rootHash[:],
	)

	addr, err := address.NewAddressTaproot(
		schnorr.SerializePubKey(outputKey), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	controlBlock := tree.LeafMerkleProofs[0].ToControlBlock(
		internalKey.PubKey(),
	)
	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	pkScript, addrString := negativeChallenge(t, addr)

	return pkScript, addrString, controlBlockBytes
}

// negativeKeyPathChallenge returns a deterministic BIP-86-style taproot
// challenge and its address.
func negativeKeyPathChallenge(t *testing.T,
	internalKey *btcec.PrivateKey) ([]byte, string) {

	t.Helper()

	outputKey := txscript.ComputeTaprootKeyNoScript(internalKey.PubKey())
	addr, err := address.NewAddressTaproot(
		schnorr.SerializePubKey(outputKey), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	return negativeChallenge(t, addr)
}

// negativeOpTrueChallenge returns the P2WSH challenge most vectors use, whose
// witness script is just OP_TRUE, along with its mainnet address and the
// serialized witness stack spending it.
func negativeOpTrueChallenge(t *testing.T) ([]byte, string, []byte) {
	t.Helper()

	witnessScript := []byte{txscript.OP_TRUE}
	pkScript, addr := negativeP2WSHChallenge(t, witnessScript)
	witnessBytes, err := SerializeTxWitness(wire.TxWitness{witnessScript})
	require.NoError(t, err)

	return pkScript, addr, witnessBytes
}

// encodeNegativeWitness serializes a raw witness payload as a simple BIP-322
// signature. The payload need not be well formed because parser failures are
// themselves negative-vector subjects.
func encodeNegativeWitness(rawWitness []byte) string {
	return PrefixSimple + b64Encode(rawWitness)
}

// makeNegativeBase64PaddingBitsNonZero changes unused bits in the final base64
// character while preserving the bytes a permissive decoder would produce.
func makeNegativeBase64PaddingBitsNonZero(t *testing.T, encoded string) string {
	t.Helper()

	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxy" +
		"z0123456789+/"
	switch {
	case strings.HasSuffix(encoded, "=="):
		idx := len(encoded) - 3
		value := strings.IndexByte(alphabet, encoded[idx])
		require.GreaterOrEqual(t, value, 0)

		return encoded[:idx] + string(alphabet[value|1]) +
			encoded[idx+1:]

	case strings.HasSuffix(encoded, "="):
		idx := len(encoded) - 2
		value := strings.IndexByte(alphabet, encoded[idx])
		require.GreaterOrEqual(t, value, 0)

		return encoded[:idx] + string(alphabet[value|1]) +
			encoded[idx+1:]

	default:
		require.Fail(t, "base64 value has no padding")

		return ""
	}
}

// negativeSerialize returns the serialization of the given transaction or
// packet.
func negativeSerialize(t *testing.T,
	serializer interface{ Serialize(io.Writer) error }) []byte {

	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, serializer.Serialize(&buf))

	return buf.Bytes()
}

// encodeNegativePoF serializes a packet directly as a proof-of-funds
// signature. Direct encoding is required for packets that SerializeSignature
// correctly refuses before they can reach the verifier.
func encodeNegativePoF(t *testing.T, packet *psbt.Packet) string {
	t.Helper()

	return PrefixProofOfFunds + b64Encode(negativeSerialize(t, packet))
}

// newNegativePoFPacket constructs the deterministic valid challenge input used
// as the base for malformed proof-of-funds vectors.
func newNegativePoFPacket(t *testing.T, pkScript, witnessBytes []byte,
	version int32) *psbt.Packet {

	t.Helper()

	packet, err := BuildToSignPacketFull(
		[]byte(negativeMessage), pkScript, version, 0, 0,
	)
	require.NoError(t, err)
	packet.Inputs[0].FinalScriptWitness = witnessBytes

	return packet
}

// negativePrevTx constructs a deterministic previous transaction containing
// the supplied outputs. Its otherwise-unused input makes the transaction shape
// match the PoF regression tests and gives it a stable non-zero txid.
func negativePrevTx(outputs ...*wire.TxOut) *wire.MsgTx {
	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
		Sequence:         0xffffffff,
	})
	for _, output := range outputs {
		prevTx.AddTxOut(output)
	}

	return prevTx
}

// appendNegativePoFInput adds one additional transaction input and its PSBT
// metadata while preserving their required positional correspondence.
func appendNegativePoFInput(packet *psbt.Packet, outPoint wire.OutPoint,
	input psbt.PInput) {

	packet.UnsignedTx.AddTxIn(&wire.TxIn{PreviousOutPoint: outPoint})
	packet.Inputs = append(packet.Inputs, input)
}

// encodeNegativeWitnessStack serializes a witness stack as a simple BIP-322
// signature.
func encodeNegativeWitnessStack(t *testing.T, witness wire.TxWitness) string {
	t.Helper()

	witnessBytes, err := SerializeTxWitness(witness)
	require.NoError(t, err)

	return encodeNegativeWitness(witnessBytes)
}

// negativeFullSignature returns a full BIP-322 signature for the given
// challenge, using the given transaction version, signature script and witness
// stack. A nil witness leaves the input without a witness. The transaction is
// encoded directly, so vectors can use values SerializeSignature would reject.
func negativeFullSignature(t *testing.T, pkScript []byte, version int32,
	sigScript []byte, witness wire.TxWitness) string {

	t.Helper()

	packet, err := BuildToSignPacketFull(
		[]byte(negativeMessage), pkScript, version, 0, 0,
	)
	require.NoError(t, err)
	packet.Inputs[0].FinalScriptSig = sigScript
	if witness != nil {
		witnessBytes, err := SerializeTxWitness(witness)
		require.NoError(t, err)
		packet.Inputs[0].FinalScriptWitness = witnessBytes
	}

	tx, err := psbt.Extract(packet)
	require.NoError(t, err)

	return PrefixFull + b64Encode(negativeSerialize(t, tx))
}

// negativeSimpleSigHashes returns the simple variant's to_sign transaction for
// the given challenge, along with the output it spends and its sighash cache,
// so its only input can be signed directly.
func negativeSimpleSigHashes(t *testing.T, pkScript []byte) (*wire.MsgTx,
	*wire.TxOut, *txscript.TxSigHashes) {

	t.Helper()

	packet, err := BuildToSignPacketSimple(
		[]byte(negativeMessage), pkScript,
	)
	require.NoError(t, err)

	tx := packet.UnsignedTx.Copy()
	utxo := packet.Inputs[0].WitnessUtxo
	fetcher := txscript.NewCannedPrevOutputFetcher(
		utxo.PkScript, utxo.Value,
	)

	return tx, utxo, txscript.NewTxSigHashes(tx, fetcher)
}

// negativeWitnessV0Signature signs the simple to_sign transaction of a segwit
// v0 challenge for the given sub script with the given key and sighash type.
func negativeWitnessV0Signature(t *testing.T, pkScript, subScript []byte,
	key *btcec.PrivateKey, hashType txscript.SigHashType) []byte {

	t.Helper()

	tx, utxo, sigHashes := negativeSimpleSigHashes(t, pkScript)
	sig, err := txscript.RawTxInWitnessSignature(
		tx, sigHashes, 0, utxo.Value, subScript, hashType, key,
	)
	require.NoError(t, err)

	return sig
}

// negativeKeySpendWitness signs the simple to_sign transaction of a taproot
// key path challenge with the given key and sighash type.
func negativeKeySpendWitness(t *testing.T, pkScript []byte,
	key *btcec.PrivateKey, hashType txscript.SigHashType) wire.TxWitness {

	t.Helper()

	tx, utxo, sigHashes := negativeSimpleSigHashes(t, pkScript)
	witness, err := txscript.TaprootWitnessSignature(
		tx, sigHashes, 0, utxo.Value, utxo.PkScript, hashType, key,
	)
	require.NoError(t, err)

	return witness
}

// negativeTapscriptSignature signs the simple to_sign transaction of a taproot
// script path challenge for the given leaf script with the given key and
// sighash type.
func negativeTapscriptSignature(t *testing.T, pkScript, leafScript []byte,
	key *btcec.PrivateKey, hashType txscript.SigHashType) []byte {

	t.Helper()

	tx, utxo, sigHashes := negativeSimpleSigHashes(t, pkScript)
	sig, err := txscript.RawTxInTapscriptSignature(
		tx, sigHashes, 0, utxo.Value, utxo.PkScript,
		txscript.NewBaseTapLeaf(leafScript), hashType, key,
	)
	require.NoError(t, err)

	return sig
}

// negativeMinimalIfChallenge returns a legacy P2SH challenge whose redeem
// script starts with the given conditional opcode and its mainnet address,
// along with a signature script that pushes the non-minimal true value 0x02 as
// the condition.
func negativeMinimalIfChallenge(t *testing.T, opcode byte) ([]byte, string,
	[]byte) {

	t.Helper()

	redeemScript, err := txscript.NewScriptBuilder().
		AddOp(opcode).
		AddOp(txscript.OP_TRUE).
		AddOp(txscript.OP_ELSE).
		AddOp(txscript.OP_TRUE).
		AddOp(txscript.OP_ENDIF).
		Script()
	require.NoError(t, err)

	sigScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_2).
		AddData(redeemScript).
		Script()
	require.NoError(t, err)

	pkScript, addr := negativeP2SHChallenge(t, redeemScript)

	return pkScript, addr, sigScript
}

// negativeP2SHChallenge returns a P2SH scriptPubKey and its mainnet address
// for the given redeem script.
func negativeP2SHChallenge(t *testing.T, redeemScript []byte) ([]byte,
	string) {

	t.Helper()

	addr, err := address.NewAddressScriptHash(
		redeemScript, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	return negativeChallenge(t, addr)
}

// encodeNegativeDERSignature encodes the given R and S values as a DER
// signature with the given sighash type appended. The values are encoded as
// given, so vectors can create signatures that violate the strict encoding
// rules.
func encodeNegativeDERSignature(r, s []byte,
	hashType txscript.SigHashType) []byte {

	sig := []byte{0x30, byte(4 + len(r) + len(s)), 0x02, byte(len(r))}
	sig = append(sig, r...)
	sig = append(sig, 0x02, byte(len(s)))
	sig = append(sig, s...)

	return append(sig, byte(hashType))
}

// splitNegativeDERSignature returns the R and S values of a strictly encoded
// DER signature with an appended sighash type.
func splitNegativeDERSignature(sig []byte) ([]byte, []byte) {
	rLen := int(sig[3])
	r := sig[4 : 4+rLen]
	sLen := int(sig[5+rLen])
	s := sig[6+rLen : 6+rLen+sLen]

	return r, s
}

// generateParserNegativeVectors creates compact malformed encodings that must
// be rejected before script execution.
func generateParserNegativeVectors(t *testing.T) []errorVector {
	t.Helper()

	pkScript, addr, witnessBytes := negativeOpTrueChallenge(t)
	basePacket := newNegativePoFPacket(t, pkScript, witnessBytes, 0)

	// A valid transaction and the finalized base packet, each followed by
	// extra bytes. Neither is a canonical encoding, as the PSBT container
	// must consume its complete encoding just like a full transaction.
	trailingData := []byte{0xde, 0xad, 0xbe, 0xef}
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
		Sequence:         0xffffffff,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    0,
		PkScript: []byte{txscript.OP_RETURN},
	})

	return []errorVector{{
		// Just claim maxWitnessItems+1 items, no item data follows. If
		// the parser allocated first, it would commit ~24 MB from a
		// 5-byte input.
		Description: "simple witness declares too many items",
		Message:     negativeMessage,
		Address:     addr,
		Signature: encodeNegativeWitness([]byte{
			0xfe, 0x01, 0x00, 0x40, 0x00,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "too many witness items",
	}, {
		// 1 item, declared length = maxWitnessItems + 1, no witness
		// body.
		Description: "simple witness declares an oversized item",
		Message:     negativeMessage,
		Address:     addr,
		Signature: encodeNegativeWitness([]byte{
			0x01, 0xfe, 0x01, 0x00, 0x40, 0x00,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "witness item too large",
	}, {
		// 1 item, declared length = 100_000, but only 1 body byte
		// available.
		Description: "simple witness item exceeds remaining bytes",
		Message:     negativeMessage,
		Address:     addr,
		Signature: encodeNegativeWitness([]byte{
			0x01, 0xfd, 0xa0, 0x86, 0xaa,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "exceeds remaining input",
	}, {
		Description: "full transaction contains trailing data",
		Message:     negativeMessage,
		Address:     addr,
		Signature: PrefixFull + b64Encode(append(
			negativeSerialize(t, tx), trailingData...,
		)),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    errMoreDataAvailable.Error(),
	}, {
		Description: "proof-of-funds packet contains trailing data",
		Message:     negativeMessage,
		Address:     addr,
		Signature: PrefixProofOfFunds + b64Encode(append(
			negativeSerialize(t, basePacket), trailingData...,
		)),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    psbt.ErrInvalidPsbtFormat.Error(),
	}}
}

// generateRuleNegativeVectors creates deterministic signatures that violate
// BIP-322 required or upgradeable script rules.
func generateRuleNegativeVectors(t *testing.T) []errorVector {
	t.Helper()

	// OP_CODESEPARATOR is forbidden in both P2WSH witness scripts and
	// tapscript leaves.
	codeSepScript := []byte{txscript.OP_CODESEPARATOR, txscript.OP_TRUE}
	_, codeSepP2WSHAddr := negativeP2WSHChallenge(t, codeSepScript)
	_, codeSepTaprootAddr, codeSepCtrlBlock := negativeTaprootChallenge(
		t, codeSepScript, fixedNegativeKey(1),
	)

	// The sighash vectors use cryptographically valid signatures, leaving
	// their prohibited sighash types as the only failure.
	p2wpkhKey := fixedNegativeKey(2)
	p2wpkhPkScript, p2wpkhAddr := negativeP2WPKHChallenge(t, p2wpkhKey)

	keyPathKey := fixedNegativeKey(3)
	keyPathPkScript, keyPathAddr := negativeKeyPathChallenge(t, keyPathKey)

	tapscriptKey := fixedNegativeKey(4)
	tapLeafScript, err := txscript.NewScriptBuilder().
		AddData(schnorr.SerializePubKey(tapscriptKey.PubKey())).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(t, err)
	tapPkScript, tapAddr, tapCtrlBlock := negativeTaprootChallenge(
		t, tapLeafScript, fixedNegativeKey(5),
	)

	// CHECKMULTISIG must inspect a disallowed second signature after
	// consuming a valid SIGHASH_ALL signature.
	multiSigKey1 := fixedNegativeKey(6)
	multiSigKey2 := fixedNegativeKey(7)
	multiSigScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_2).
		AddData(multiSigKey1.PubKey().SerializeCompressed()).
		AddData(multiSigKey2.PubKey().SerializeCompressed()).
		AddOp(txscript.OP_2).
		AddOp(txscript.OP_CHECKMULTISIG).
		Script()
	require.NoError(t, err)
	multiSigPkScript, multiSigAddr := negativeP2WSHChallenge(
		t, multiSigScript,
	)

	// STRICTENC rejects hybrid public keys, which encode the parity of the
	// Y coordinate in the prefix byte of an otherwise uncompressed key.
	hybridKey := fixedNegativeKey(9)
	hybridPubKey := hybridKey.PubKey().SerializeUncompressed()
	hybridPubKey[0] = 0x06 | hybridPubKey[64]&0x01
	hybridScript, err := txscript.NewScriptBuilder().
		AddData(hybridPubKey).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(t, err)
	hybridPkScript, hybridAddr := negativeP2WSHChallenge(t, hybridScript)

	ifPkScript, ifAddr, ifSigScript := negativeMinimalIfChallenge(
		t, txscript.OP_IF,
	)
	notIfPkScript, notIfAddr, notIfSigScript := negativeMinimalIfChallenge(
		t, txscript.OP_NOTIF,
	)

	nop5Script := []byte{txscript.OP_NOP5, txscript.OP_TRUE}
	_, nop5Addr := negativeP2WSHChallenge(t, nop5Script)

	// A violation of a consensus or required rule after a reserved NOP
	// takes precedence over the NOP's inconclusive result.
	nop5FalseScript := []byte{txscript.OP_NOP5, txscript.OP_FALSE}
	_, nop5FalseAddr := negativeP2WSHChallenge(t, nop5FalseScript)
	nop5PushScript := []byte{
		txscript.OP_NOP5, txscript.OP_PUSHDATA1, 0x01, 0x05,
	}
	_, nop5PushAddr := negativeP2WSHChallenge(t, nop5PushScript)

	// The script failure of OP_FALSE must win over the upgradeable-rule
	// classification of the transaction version.
	trueScript := []byte{txscript.OP_TRUE}
	truePkScript, trueAddr := negativeP2WSHChallenge(t, trueScript)
	falseScript := []byte{txscript.OP_FALSE}
	falsePkScript, falseAddr := negativeP2WSHChallenge(t, falseScript)

	return []errorVector{{
		Description: "OP_CODESEPARATOR in P2WSH witness script",
		Message:     negativeMessage,
		Address:     codeSepP2WSHAddr,
		Signature: encodeNegativeWitnessStack(
			t, wire.TxWitness{codeSepScript},
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    ErrCodeSeparator.Error(),
	}, {
		Description: "OP_CODESEPARATOR in tapscript leaf",
		Message:     negativeMessage,
		Address:     codeSepTaprootAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			codeSepScript, codeSepCtrlBlock,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    ErrCodeSeparator.Error(),
	}, {
		Description: "P2WPKH simple signature uses SIGHASH_NONE",
		Message:     negativeMessage,
		Address:     p2wpkhAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			negativeWitnessV0Signature(
				t, p2wpkhPkScript, p2wpkhPkScript, p2wpkhKey,
				txscript.SigHashNone,
			),
			p2wpkhKey.PubKey().SerializeCompressed(),
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    ErrInvalidSigHashFlag.Error(),
	}, {
		Description: "taproot key-path simple signature uses " +
			"SIGHASH_SINGLE",
		Message: negativeMessage,
		Address: keyPathAddr,
		Signature: encodeNegativeWitnessStack(
			t, negativeKeySpendWitness(
				t, keyPathPkScript, keyPathKey,
				txscript.SigHashSingle,
			),
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    ErrInvalidSigHashFlag.Error(),
	}, {
		Description: "tapscript simple signature uses SIGHASH_SINGLE",
		Message:     negativeMessage,
		Address:     tapAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			negativeTapscriptSignature(
				t, tapPkScript, tapLeafScript,
				tapscriptKey, txscript.SigHashSingle,
			),
			tapLeafScript, tapCtrlBlock,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    ErrInvalidSigHashFlag.Error(),
	}, {
		Description: "P2WSH multisig second signature uses " +
			"SIGHASH_SINGLE",
		Message: negativeMessage,
		Address: multiSigAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			nil,
			negativeWitnessV0Signature(
				t, multiSigPkScript, multiSigScript,
				multiSigKey1, txscript.SigHashAll,
			),
			negativeWitnessV0Signature(
				t, multiSigPkScript, multiSigScript,
				multiSigKey2, txscript.SigHashSingle,
			),
			multiSigScript,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    ErrInvalidSigHashFlag.Error(),
	}, {
		Description: "P2WSH witness script uses a hybrid public key",
		Message:     negativeMessage,
		Address:     hybridAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			negativeWitnessV0Signature(
				t, hybridPkScript, hybridScript, hybridKey,
				txscript.SigHashAll,
			),
			hybridScript,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrPubKeyType.String(),
	}, {
		Description: "legacy P2SH OP_IF uses non-minimal true",
		Message:     negativeMessage,
		Address:     ifAddr,
		Signature: negativeFullSignature(
			t, ifPkScript, 0, ifSigScript, nil,
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrMinimalIf.String(),
	}, {
		Description: "legacy P2SH OP_NOTIF uses non-minimal true",
		Message:     negativeMessage,
		Address:     notIfAddr,
		Signature: negativeFullSignature(
			t, notIfPkScript, 0, notIfSigScript, nil,
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrMinimalIf.String(),
	}, {
		Description: "reserved OP_NOP5 is inconclusive",
		Message:     negativeMessage,
		Address:     nop5Addr,
		Signature: encodeNegativeWitnessStack(
			t, wire.TxWitness{nop5Script},
		),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr:    ErrInconclusive.Error(),
	}, {
		Description: "consensus failure after reserved OP_NOP5 is " +
			"invalid",
		Message: negativeMessage,
		Address: nop5FalseAddr,
		Signature: encodeNegativeWitnessStack(
			t, wire.TxWitness{nop5FalseScript},
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrEvalFalse.String(),
	}, {
		Description: "required rule violation after reserved OP_NOP5 " +
			"is invalid",
		Message: negativeMessage,
		Address: nop5PushAddr,
		Signature: encodeNegativeWitnessStack(
			t, wire.TxWitness{nop5PushScript},
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrMinimalData.String(),
	}, {
		Description: "full transaction version 1 is inconclusive",
		Message:     negativeMessage,
		Address:     trueAddr,
		Signature: negativeFullSignature(
			t, truePkScript, 1, nil, wire.TxWitness{trueScript},
		),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr:    ErrInconclusive.Error(),
	}, {
		Description: "invalid script precedes inconclusive " +
			"transaction version",
		Message: negativeMessage,
		Address: falseAddr,
		Signature: negativeFullSignature(
			t, falsePkScript, 3, nil, wire.TxWitness{falseScript},
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    ErrInvalidSignature.Error(),
	}}
}

// generateVerifyFlagNegativeVectors creates deterministic signatures that each
// violate a single rule enforced by one of the script verification flags (see
// scriptVerifyFlags) and that aren't already covered by
// generateRuleNegativeVectors. Each script would be valid without the rule, so
// the vectors detect a missing flag.
func generateVerifyFlagNegativeVectors(t *testing.T) []errorVector {
	t.Helper()

	// DERSIG (consensus): the R value of an otherwise valid signature has
	// an unnecessary padding byte.
	derKey := fixedNegativeKey(10)
	derPkScript, derAddr := negativeP2WPKHChallenge(t, derKey)
	derR, derS := splitNegativeDERSignature(negativeWitnessV0Signature(
		t, derPkScript, derPkScript, derKey, txscript.SigHashAll,
	))
	derSig := encodeNegativeDERSignature(
		append([]byte{0x00}, derR...), derS, txscript.SigHashAll,
	)

	// CHECKLOCKTIMEVERIFY and CHECKSEQUENCEVERIFY (consensus): the scripts
	// require a lock time and relative lock time of 1, while the to_sign
	// transaction uses 0 for both.
	cltvScript := []byte{txscript.OP_1, txscript.OP_CHECKLOCKTIMEVERIFY}
	_, cltvAddr := negativeP2WSHChallenge(t, cltvScript)
	csvScript := []byte{txscript.OP_1, txscript.OP_CHECKSEQUENCEVERIFY}
	csvPkScript, csvAddr := negativeP2WSHChallenge(t, csvScript)

	// NULLDUMMY (consensus): the dummy element consumed by
	// OP_CHECKMULTISIG isn't empty.
	nullDummyKey := fixedNegativeKey(11)
	nullDummyScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_1).
		AddData(nullDummyKey.PubKey().SerializeCompressed()).
		AddOp(txscript.OP_1).
		AddOp(txscript.OP_CHECKMULTISIG).
		Script()
	require.NoError(t, err)
	nullDummyPkScript, nullDummyAddr := negativeP2WSHChallenge(
		t, nullDummyScript,
	)

	// P2SH (consensus): the redeem script matches the script hash but
	// evaluates to false.
	falseRedeemScript := []byte{txscript.OP_1, txscript.OP_NOT}
	falseRedeemPkScript, falseRedeemAddr := negativeP2SHChallenge(
		t, falseRedeemScript,
	)

	// LOW_S (required): the S value of an otherwise valid signature is
	// replaced by its negation, which is equally valid for ECDSA.
	highSKey := fixedNegativeKey(12)
	highSPkScript, highSAddr := negativeP2WPKHChallenge(t, highSKey)
	highSR, lowS := splitNegativeDERSignature(negativeWitnessV0Signature(
		t, highSPkScript, highSPkScript, highSKey, txscript.SigHashAll,
	))
	var highS btcec.ModNScalar
	highS.SetByteSlice(lowS)
	highSBytes := highS.Negate().Bytes()
	highSValue := highSBytes[:]
	if highSValue[0]&0x80 != 0 {
		highSValue = append([]byte{0x00}, highSValue...)
	}
	highSSig := encodeNegativeDERSignature(
		highSR, highSValue, txscript.SigHashAll,
	)

	// NULLFAIL (required): the signature is well formed but signs a
	// different transaction. The script negates the result of the check,
	// so it would succeed without the rule.
	nullFailKey := fixedNegativeKey(13)
	nullFailScript, err := txscript.NewScriptBuilder().
		AddData(nullFailKey.PubKey().SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).
		AddOp(txscript.OP_NOT).
		Script()
	require.NoError(t, err)
	_, nullFailAddr := negativeP2WSHChallenge(t, nullFailScript)
	otherPkScript, _, _ := negativeOpTrueChallenge(t)

	// MINIMALDATA (required): a single byte is pushed with OP_PUSHDATA1
	// instead of a small integer opcode.
	minimalDataScript := []byte{txscript.OP_PUSHDATA1, 0x01, 0x05}
	_, minimalDataAddr := negativeP2WSHChallenge(t, minimalDataScript)

	// CLEANSTACK (required): the redeem script leaves two elements on the
	// stack. Segwit scripts require a clean stack by consensus, so only
	// non-segwit scripts depend on the rule.
	cleanStackRedeemScript := []byte{txscript.OP_TRUE, txscript.OP_TRUE}
	cleanStackPkScript, cleanStackAddr := negativeP2SHChallenge(
		t, cleanStackRedeemScript,
	)

	// MINIMALIF (required): the condition of a SegWit v0 script is 0x02.
	// The non-segwit variants are covered by the rule vectors.
	minimalIfScript := []byte{
		txscript.OP_IF, txscript.OP_TRUE, txscript.OP_ELSE,
		txscript.OP_TRUE, txscript.OP_ENDIF,
	}
	_, minimalIfAddr := negativeP2WSHChallenge(t, minimalIfScript)

	// No OP_CODESEPARATOR (required): the script engine rejects it in
	// non-segwit scripts. The segwit variants are covered by the rule
	// vectors.
	codeSepRedeemScript := []byte{
		txscript.OP_CODESEPARATOR, txscript.OP_TRUE,
	}
	codeSepPkScript, codeSepAddr := negativeP2SHChallenge(
		t, codeSepRedeemScript,
	)

	// No witness versions greater than 1 (upgradeable): btcd can't encode
	// native addresses for them, so the program is wrapped in P2SH. The
	// program must not be all zero bytes, as the redeem script has to leave
	// a true value on the stack by consensus.
	futureProgram, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_2).
		AddData(bytes.Repeat([]byte{0x01}, 32)).
		Script()
	require.NoError(t, err)
	futurePkScript, futureAddr := negativeP2SHChallenge(t, futureProgram)

	// The upgrade mechanisms of taproot (upgradeable): the consensus rules
	// let all of these scripts succeed, as they are reserved for future
	// soft forks. OP_SUCCESS80 is the tapscript meaning of 0x50.
	internalKey := fixedNegativeKey(14)
	opSuccessLeaf := []byte{0x50}
	_, opSuccessAddr, opSuccessCtrlBlock := negativeTaprootChallenge(
		t, opSuccessLeaf, internalKey,
	)
	unknownLeafScript := []byte{txscript.OP_TRUE}
	_, unknownLeafAddr, unknownLeafCtrlBlock := negativeTapLeafChallenge(
		t, txscript.NewTapLeaf(0xc2, unknownLeafScript), internalKey,
	)
	unknownKeyLeaf, err := txscript.NewScriptBuilder().
		AddData(fixedNegativeKey(15).PubKey().SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(t, err)
	_, unknownKeyAddr, unknownKeyCtrlBlock := negativeTaprootChallenge(
		t, unknownKeyLeaf, internalKey,
	)
	annexLeaf := []byte{txscript.OP_TRUE}
	_, annexAddr, annexCtrlBlock := negativeTaprootChallenge(
		t, annexLeaf, internalKey,
	)

	return []errorVector{{
		Description: "P2WPKH signature R value has unnecessary padding",
		Message:     negativeMessage,
		Address:     derAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			derSig, derKey.PubKey().SerializeCompressed(),
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrSigTooMuchRPadding.String(),
	}, {
		Description: "P2WSH CHECKLOCKTIMEVERIFY lock time is not " +
			"satisfied",
		Message: negativeMessage,
		Address: cltvAddr,
		Signature: encodeNegativeWitnessStack(
			t, wire.TxWitness{cltvScript},
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrUnsatisfiedLockTime.String(),
	}, {
		Description: "P2WSH CHECKSEQUENCEVERIFY relative lock time " +
			"is not satisfied",
		Message: negativeMessage,
		Address: csvAddr,
		Signature: negativeFullSignature(
			t, csvPkScript, 2, nil, wire.TxWitness{csvScript},
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrUnsatisfiedLockTime.String(),
	}, {
		Description: "P2WSH CHECKMULTISIG dummy element is not empty",
		Message:     negativeMessage,
		Address:     nullDummyAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			{0x01},
			negativeWitnessV0Signature(
				t, nullDummyPkScript, nullDummyScript,
				nullDummyKey, txscript.SigHashAll,
			),
			nullDummyScript,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrSigNullDummy.String(),
	}, {
		Description: "legacy P2SH redeem script evaluates to false",
		Message:     negativeMessage,
		Address:     falseRedeemAddr,
		Signature: negativeFullSignature(
			t, falseRedeemPkScript, 0,
			pushDataScript(t, falseRedeemScript), nil,
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrEvalFalse.String(),
	}, {
		Description: "P2WPKH signature has a high S value",
		Message:     negativeMessage,
		Address:     highSAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			highSSig, highSKey.PubKey().SerializeCompressed(),
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrSigHighS.String(),
	}, {
		Description: "P2WSH failed signature check uses a non-empty " +
			"signature",
		Message: negativeMessage,
		Address: nullFailAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			negativeWitnessV0Signature(
				t, otherPkScript, nullFailScript, nullFailKey,
				txscript.SigHashAll,
			),
			nullFailScript,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrNullFail.String(),
	}, {
		Description: "P2WSH witness script uses a non-minimal push",
		Message:     negativeMessage,
		Address:     minimalDataAddr,
		Signature: encodeNegativeWitnessStack(
			t, wire.TxWitness{minimalDataScript},
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrMinimalData.String(),
	}, {
		Description: "legacy P2SH redeem script leaves more than one " +
			"stack element",
		Message: negativeMessage,
		Address: cleanStackAddr,
		Signature: negativeFullSignature(
			t, cleanStackPkScript, 0,
			pushDataScript(t, cleanStackRedeemScript), nil,
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrCleanStack.String(),
	}, {
		Description: "P2WSH OP_IF uses non-minimal true",
		Message:     negativeMessage,
		Address:     minimalIfAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			{0x02}, minimalIfScript,
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrMinimalIf.String(),
	}, {
		Description: "OP_CODESEPARATOR in legacy P2SH redeem script",
		Message:     negativeMessage,
		Address:     codeSepAddr,
		Signature: negativeFullSignature(
			t, codeSepPkScript, 0,
			pushDataScript(t, codeSepRedeemScript), nil,
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrCodeSeparator.String(),
	}, {
		Description: "P2SH-wrapped witness version 2 program is " +
			"inconclusive",
		Message: negativeMessage,
		Address: futureAddr,
		Signature: negativeFullSignature(
			t, futurePkScript, 0, pushDataScript(t, futureProgram),
			wire.TxWitness{{txscript.OP_TRUE}},
		),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr: txscript.ErrDiscourageUpgradableWitnessProgram.
			String(),
	}, {
		Description: "tapscript OP_SUCCESS80 is inconclusive",
		Message:     negativeMessage,
		Address:     opSuccessAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			opSuccessLeaf, opSuccessCtrlBlock,
		}),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr:    txscript.ErrDiscourageOpSuccess.String(),
	}, {
		Description: "unknown tapscript leaf version is inconclusive",
		Message:     negativeMessage,
		Address:     unknownLeafAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			unknownLeafScript, unknownLeafCtrlBlock,
		}),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr: txscript.ErrDiscourageUpgradeableTaprootVersion.
			String(),
	}, {
		Description: "unknown tapscript public key type is " +
			"inconclusive",
		Message: negativeMessage,
		Address: unknownKeyAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			make([]byte, 64), unknownKeyLeaf, unknownKeyCtrlBlock,
		}),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr: txscript.ErrDiscourageUpgradeablePubKeyType.
			String(),
	}, {
		Description: "taproot annex is inconclusive",
		Message:     negativeMessage,
		Address:     annexAddr,
		Signature: encodeNegativeWitnessStack(t, wire.TxWitness{
			annexLeaf, annexCtrlBlock, {txscript.TaprootAnnexTag},
		}),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr:    "taproot annex is reserved",
	}}
}

// generatePoFNegativeVectors creates deterministic malformed proof-of-funds
// packets covering authenticated UTXO data and transaction structure rules.
func generatePoFNegativeVectors(t *testing.T) []errorVector {
	t.Helper()

	pkScript, addr, witnessBytes := negativeOpTrueChallenge(t)

	// pofInput is an additional input of a proof-of-funds packet.
	type pofInput struct {
		outPoint wire.OutPoint
		input    psbt.PInput
	}

	newPacket := func(version int32, inputs ...pofInput) *psbt.Packet {
		packet := newNegativePoFPacket(
			t, pkScript, witnessBytes, version,
		)
		for _, input := range inputs {
			appendNegativePoFInput(
				packet, input.outPoint, input.input,
			)
		}

		return packet
	}
	newSignature := func(version int32, inputs ...pofInput) string {
		return encodeNegativePoF(t, newPacket(version, inputs...))
	}
	newOutput := func(value int64, script []byte) *wire.TxOut {
		return &wire.TxOut{Value: value, PkScript: bytes.Clone(script)}
	}
	validInput := func(tag byte) pofInput {
		return pofInput{
			outPoint: wire.OutPoint{Hash: chainhash.Hash{tag}},
			input: psbt.PInput{
				WitnessUtxo:        newOutput(1, pkScript),
				FinalScriptWitness: witnessBytes,
			},
		}
	}

	// A previous transaction paying to the challenge, for the inputs that
	// carry a non-witness UTXO.
	prevTx := negativePrevTx(newOutput(1, pkScript))

	// A legacy signature does not authenticate the amount, so a bare
	// witness UTXO cannot establish the claimed value.
	legacyKey := fixedNegativeKey(8)
	legacyAddr, err := address.NewAddressPubKeyHash(
		address.Hash160(legacyKey.PubKey().SerializeCompressed()),
		&chaincfg.MainNetParams,
	)
	require.NoError(t, err)
	legacyScript, _ := negativeChallenge(t, legacyAddr)
	legacyPrevTx := negativePrevTx(newOutput(1, legacyScript))
	legacyPacket := newPacket(0, pofInput{
		outPoint: wire.OutPoint{Hash: legacyPrevTx.TxHash()},
		input: psbt.PInput{
			WitnessUtxo: newOutput(
				100_000_000, legacyScript,
			),
			FinalScriptWitness: []byte{0x00},
		},
	})
	legacySigScript, err := txscript.SignatureScript(
		legacyPacket.UnsignedTx, 1, legacyScript, txscript.SigHashAll,
		legacyKey, true,
	)
	require.NoError(t, err)
	legacyPacket.Inputs[1].FinalScriptSig = legacySigScript

	// When both UTXO representations are present, the witness UTXO must
	// match the referenced output of the non-witness UTXO in both amount
	// and script.
	conflictingInput := func(witnessUtxo *wire.TxOut) pofInput {
		return pofInput{
			outPoint: wire.OutPoint{Hash: prevTx.TxHash()},
			input: psbt.PInput{
				NonWitnessUtxo:     prevTx,
				WitnessUtxo:        witnessUtxo,
				FinalScriptWitness: witnessBytes,
			},
		}
	}
	conflictingScript := bytes.Clone(pkScript)
	conflictingScript[len(conflictingScript)-1] ^= 1

	// Two outputs of the same transaction, so a later input could supply
	// the non-witness UTXO for an earlier one.
	reusedPrevTx := negativePrevTx(
		newOutput(1, pkScript),
		newOutput(1, pkScript),
	)

	differentMessage := "different message"
	differentMessagePacket := newPacket(0, validInput(0x44))
	differentMessagePacket.GenericSignedMessage = &differentMessage

	nonOpReturnPacket := newPacket(0)
	nonOpReturnPacket.UnsignedTx.TxOut[0].PkScript = []byte{
		txscript.OP_TRUE,
	}

	// Every additional input spends a P2SH redeem script with the maximum
	// number of unexecuted signature operations. Each script executes
	// successfully, but six of them together exceed the consensus limit.
	// Only the first input carries the previous transaction, the others
	// reuse it to keep the vector small.
	const numSigOpsInputs = 6
	sigOpsRedeemScript := maxSigOpsRedeemScript(t)
	sigOpsPkScript := p2shScript(t, sigOpsRedeemScript)
	sigOpsSigScript := pushDataScript(t, sigOpsRedeemScript)
	sigOpsOutputs := make([]*wire.TxOut, numSigOpsInputs)
	for idx := range sigOpsOutputs {
		sigOpsOutputs[idx] = newOutput(1, sigOpsPkScript)
	}
	sigOpsPrevTx := negativePrevTx(sigOpsOutputs...)
	sigOpsInputs := make([]pofInput, numSigOpsInputs)
	for idx := range uint32(numSigOpsInputs) {
		sigOpsInputs[idx] = pofInput{
			outPoint: wire.OutPoint{
				Hash:  sigOpsPrevTx.TxHash(),
				Index: idx,
			},
			input: psbt.PInput{FinalScriptSig: sigOpsSigScript},
		}
	}
	sigOpsInputs[0].input.NonWitnessUtxo = sigOpsPrevTx

	// scriptInput returns an additional input spending a P2WSH output of
	// the given witness script without any further witness items.
	scriptInput := func(tag byte, witnessScript []byte) pofInput {
		inputPkScript, _ := negativeP2WSHChallenge(t, witnessScript)
		inputWitness, err := SerializeTxWitness(
			wire.TxWitness{witnessScript},
		)
		require.NoError(t, err)

		return pofInput{
			outPoint: wire.OutPoint{Hash: chainhash.Hash{tag}},
			input: psbt.PInput{
				WitnessUtxo:        newOutput(1, inputPkScript),
				FinalScriptWitness: inputWitness,
			},
		}
	}

	// BIP-322 checks the required rules before the upgradeable rules, so
	// an input violating a required rule makes the signature invalid even
	// if an earlier input makes it inconclusive.
	inconclusiveInput := scriptInput(
		0x47, []byte{txscript.OP_NOP5, txscript.OP_TRUE},
	)
	invalidInput := scriptInput(0x48, []byte{txscript.OP_FALSE})

	// A canonical encoding, with a newline inserted shortly after the
	// prefix.
	canonical := newSignature(0, validInput(0x46))
	newlineIdx := len(PrefixProofOfFunds) + 4

	// Add deterministic unknown data until the encoding has padding whose
	// unused bits can be changed.
	paddedPacket := newPacket(0, validInput(0x46))
	var padded string
	for valueLen := 0; ; valueLen++ {
		paddedPacket.Unknowns = []*psbt.Unknown{{
			Key:   []byte{0xaa},
			Value: make([]byte, valueLen),
		}}
		padded = encodeNegativePoF(t, paddedPacket)
		if strings.Contains(padded, "=") {
			break
		}
	}

	return []errorVector{{
		Description: "proof-of-funds legacy input has only " +
			"witness UTXO",
		Message:        negativeMessage,
		Address:        addr,
		Signature:      encodeNegativePoF(t, legacyPacket),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "witness UTXO for spending a non-segwit script",
	}, {
		Description: "proof-of-funds non-witness UTXO has wrong txid",
		Message:     negativeMessage,
		Address:     addr,
		Signature: newSignature(0, pofInput{
			outPoint: wire.OutPoint{Hash: chainhash.Hash{0x42}},
			input: psbt.PInput{
				NonWitnessUtxo:     prevTx,
				FinalScriptWitness: witnessBytes,
			},
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "non witness utxo does not match input prevout",
	}, {
		Description: "proof-of-funds witness UTXO has " +
			"conflicting amount",
		Message: negativeMessage,
		Address: addr,
		Signature: newSignature(
			0, conflictingInput(newOutput(2, pkScript)),
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "witness utxo does not match non witness utxo",
	}, {
		Description: "proof-of-funds witness UTXO has " +
			"conflicting script",
		Message: negativeMessage,
		Address: addr,
		Signature: newSignature(
			0, conflictingInput(newOutput(1, conflictingScript)),
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "witness utxo does not match non witness utxo",
	}, {
		Description: "proof-of-funds contains duplicate " +
			"additional inputs",
		Message: negativeMessage,
		Address: addr,
		Signature: newSignature(
			0, validInput(0x43), validInput(0x43),
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "duplicate inputs",
	}, {
		Description: "proof-of-funds contains null additional input",
		Message:     negativeMessage,
		Address:     addr,
		Signature: newSignature(0, pofInput{
			outPoint: wire.OutPoint{Index: 0xffffffff},
			input: psbt.PInput{
				WitnessUtxo:        newOutput(1, pkScript),
				FinalScriptWitness: witnessBytes,
			},
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "zero hash present",
	}, {
		Description: "proof-of-funds witness declares too " +
			"many items",
		Message: negativeMessage,
		Address: addr,
		Signature: encodeNegativePoF(t, newNegativePoFPacket(
			t, pkScript, []byte{0xfe, 0xff, 0xff, 0xff, 0xff}, 0,
		)),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "too many witness items",
	}, {
		Description: "proof-of-funds reuses a later " +
			"non-witness UTXO",
		Message: negativeMessage,
		Address: addr,
		Signature: newSignature(0, pofInput{
			outPoint: wire.OutPoint{Hash: reusedPrevTx.TxHash()},
			input:    psbt.PInput{FinalScriptWitness: witnessBytes},
		}, pofInput{
			outPoint: wire.OutPoint{
				Hash:  reusedPrevTx.TxHash(),
				Index: 1,
			},
			input: psbt.PInput{
				NonWitnessUtxo:     reusedPrevTx,
				FinalScriptWitness: witnessBytes,
			},
		}),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "UTXO not found",
	}, {
		Description: "proof-of-funds generic signed message " +
			"does not match",
		Message:        negativeMessage,
		Address:        addr,
		Signature:      encodeNegativePoF(t, differentMessagePacket),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "generic signed message field",
	}, {
		Description: "proof-of-funds to_sign output is not " +
			"OP_RETURN",
		Message:        negativeMessage,
		Address:        addr,
		Signature:      encodeNegativePoF(t, nonOpReturnPacket),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "single OP_RETURN byte",
	}, {
		Description: "proof-of-funds exceeds signature operation " +
			"cost limit",
		Message:        negativeMessage,
		Address:        addr,
		Signature:      newSignature(0, sigOpsInputs...),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "signature operation cost",
	}, {
		Description: "proof-of-funds invalid input takes precedence " +
			"over inconclusive input",
		Message: negativeMessage,
		Address: addr,
		Signature: newSignature(
			0, inconclusiveInput, invalidInput,
		),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    txscript.ErrEvalFalse.String(),
	}, {
		Description: "proof-of-funds transaction version 3 is " +
			"inconclusive",
		Message:        negativeMessage,
		Address:        addr,
		Signature:      newSignature(3, validInput(0x45)),
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr:    ErrInconclusive.Error(),
	}, {
		Description: "proof-of-funds base64 contains embedded " +
			"newline",
		Message: negativeMessage,
		Address: addr,
		Signature: canonical[:newlineIdx] + "\n" +
			canonical[newlineIdx:],
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "base64",
	}, {
		Description: "proof-of-funds base64 has non-canonical " +
			"padding bits",
		Message: negativeMessage,
		Address: addr,
		Signature: PrefixProofOfFunds +
			makeNegativeBase64PaddingBitsNonZero(
				t, padded[len(PrefixProofOfFunds):],
			),
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    "illegal base64 data",
	}}
}

// generateNegativeTestVectors builds the complete deterministic vector set.
// Resource-limit cases that require multi-megabyte payloads intentionally stay
// as direct unit tests instead of inflating the checked-in fixture.
func generateNegativeTestVectors(t *testing.T) *testVectors {
	t.Helper()

	vectors := generateParserNegativeVectors(t)
	vectors = append(vectors, generateRuleNegativeVectors(t)...)
	vectors = append(vectors, generateVerifyFlagNegativeVectors(t)...)
	vectors = append(vectors, generatePoFNegativeVectors(t)...)
	vectors = append(vectors, generateBindingNegativeVectors(t)...)

	return &testVectors{Error: vectors}
}

// deterministicWrongAddress returns a stable challenge of the same script type
// as a generated positive vector. Keeping the type unchanged ensures rejection
// proves challenge binding rather than a variant/script incompatibility.
func deterministicWrongAddress(t *testing.T, typeName string) string {
	t.Helper()

	inputType, err := parseInputType(typeName)
	require.NoError(t, err)

	_, addr, _, _, _ := inputType.output(t, []*btcec.PrivateKey{
		fixedNegativeKey(0xf0),
		fixedNegativeKey(0xf1),
		fixedNegativeKey(0xf2),
	})

	return addr
}

// generateBindingNegativeVectors derives wrong-message and wrong-address cases
// from every checked-in generated positive vector.
func generateBindingNegativeVectors(t *testing.T) []errorVector {
	t.Helper()

	const wrongMessage = "not the correct message"

	vectors := loadTestVectors(t, "generated-test-vectors.json")
	result := make(
		[]errorVector, 0,
		2*(len(vectors.Simple)+len(vectors.Full)+
			len(vectors.ProofOfFunds)),
	)
	appendVectors := func(variant, errorSubstr string,
		vector simpleSignatureVector) {

		require.NotEmpty(t, vector.Bip322Signatures)
		signature := vector.Bip322Signatures[0]
		wrongAddr := deterministicWrongAddress(t, vector.Type)
		require.NotEqual(t, vector.Message, wrongMessage)
		require.NotEqual(t, vector.Address, wrongAddr)

		target := vector.Type + " " + variant + " signature"
		result = append(result, errorVector{
			Description:    "wrong message for " + target,
			Message:        wrongMessage,
			Address:        vector.Address,
			Signature:      signature,
			ExpectedResult: errorResultInvalid,
			ErrorSubstr:    errorSubstr,
		}, errorVector{
			Description:    "wrong address for " + target,
			Message:        vector.Message,
			Address:        wrongAddr,
			Signature:      signature,
			ExpectedResult: errorResultInvalid,
			ErrorSubstr:    errorSubstr,
		})
	}

	for _, vector := range vectors.Simple {
		appendVectors("simple", "invalid signature", vector)
	}
	for _, vector := range vectors.Full {
		appendVectors(
			"full", "invalid to_sign transaction",
			vector.simpleSignatureVector,
		)
	}
	for _, vector := range vectors.ProofOfFunds {
		appendVectors(
			"proof-of-funds", "invalid to_sign transaction",
			vector.simpleSignatureVector,
		)
	}

	return result
}

// marshalNegativeTestVectors produces the canonical bytes used both for the
// checked-in fixture and its freshness comparison.
func marshalNegativeTestVectors(t *testing.T,
	vectors *testVectors) []byte {

	t.Helper()

	data, err := json.MarshalIndent(vectors, "", "  ")
	require.NoError(t, err)

	// A final newline keeps the generated JSON friendly to text tools while
	// remaining part of the byte-for-byte freshness contract.
	return append(data, '\n')
}

// TestNegativeTestVectorsUpToDate regenerates the deterministic fixture in
// memory so ordinary test runs fail when the checked-in JSON is stale.
func TestNegativeTestVectorsUpToDate(t *testing.T) {
	if os.Getenv("BIP322_REGEN") != "" {
		t.Skip("the update test owns the fixture in regeneration mode")
	}

	expected := marshalNegativeTestVectors(
		t, generateNegativeTestVectors(t),
	)
	path := filepath.Join("testdata", negativeVectorFileName)
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(
		t, expected, actual, "negative test vectors are stale; "+
			"run cd bip322 && BIP322_REGEN=1 go test ./ -run "+
			"TestUpdateNegativeTestVectors",
	)
}

// TestUpdateNegativeTestVectors writes the deterministic negative fixture when
// explicitly requested. Normal test runs never modify the working tree.
func TestUpdateNegativeTestVectors(t *testing.T) {
	if os.Getenv("BIP322_REGEN") == "" {
		t.Skip("set BIP322_REGEN=1 to regenerate negative test vectors")
	}

	data := marshalNegativeTestVectors(t, generateNegativeTestVectors(t))
	path := filepath.Join("testdata", negativeVectorFileName)
	require.NoError(t, os.WriteFile(path, data, 0644))
}
