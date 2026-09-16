package bip322

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
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
)

// negativeVectorCase names one independently constructed negative vector. The
// closure keeps each malformed scenario next to its description while allowing
// shared deterministic setup to remain outside the table.
type negativeVectorCase struct {
	description string
	generate    func(*testing.T) errorVector
}

// runNegativeVectorCases executes a table of vector constructors as named
// subtests and stamps each returned vector with its table description.
func runNegativeVectorCases(t *testing.T,
	testCases []negativeVectorCase) []errorVector {

	t.Helper()

	vectors := make([]errorVector, 0, len(testCases))
	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			vector := testCase.generate(t)
			vector.Description = testCase.description
			vectors = append(vectors, vector)
		})
	}

	return vectors
}

// invalidNegativeVector returns the common vector shape for a definitive
// verification failure.
func invalidNegativeVector(message, addr, signature,
	errorSubstr string) errorVector {

	return errorVector{
		Message:        message,
		Address:        addr,
		Signature:      signature,
		ExpectedResult: errorResultInvalid,
		ErrorSubstr:    errorSubstr,
	}
}

// inconclusiveNegativeVector returns the common vector shape for a proof that
// encounters a BIP-322 upgradeable rule.
func inconclusiveNegativeVector(message, addr, signature,
	errorSubstr string) errorVector {

	return errorVector{
		Message:        message,
		Address:        addr,
		Signature:      signature,
		ExpectedResult: errorResultInconclusive,
		ErrorSubstr:    errorSubstr,
	}
}

// fixedNegativeKey returns a deterministic private key for generated vectors.
// Distinct non-zero tags produce distinct keys without introducing randomness
// into the checked-in fixture.
func fixedNegativeKey(tag byte) *btcec.PrivateKey {
	keyBytes := make([]byte, 32)
	keyBytes[len(keyBytes)-1] = tag
	key, _ := btcec.PrivKeyFromBytes(keyBytes)

	return key
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

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	return pkScript, addr.String()
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

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	return pkScript, addr.String()
}

// negativeTaprootChallenge commits a deterministic internal key to the given
// leaf script and returns the challenge, address, and control block.
func negativeTaprootChallenge(t *testing.T, leafScript []byte,
	internalKey *btcec.PrivateKey) ([]byte, string, []byte) {

	t.Helper()

	leaf := txscript.NewBaseTapLeaf(leafScript)
	tree := txscript.AssembleTaprootScriptTree(leaf)
	rootHash := tree.RootNode.TapHash()
	outputKey := txscript.ComputeTaprootOutputKey(
		internalKey.PubKey(), rootHash[:],
	)

	addr, err := address.NewAddressTaproot(
		schnorr.SerializePubKey(outputKey), &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	controlBlock := tree.LeafMerkleProofs[0].ToControlBlock(
		internalKey.PubKey(),
	)
	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	return pkScript, addr.String(), controlBlockBytes
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

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	return pkScript, addr.String()
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

// encodeNegativeFull serializes a transaction as a full BIP-322 signature.
func encodeNegativeFull(t *testing.T, tx *wire.MsgTx) string {
	t.Helper()

	var txBytes bytes.Buffer
	require.NoError(t, tx.Serialize(&txBytes))

	return PrefixFull + b64Encode(txBytes.Bytes())
}

// encodeNegativePoF serializes a packet directly as a proof-of-funds
// signature. Direct encoding is required for packets that SerializeSignature
// correctly refuses before they can reach the verifier.
func encodeNegativePoF(t *testing.T, packet *psbt.Packet) string {
	t.Helper()

	var packetBytes bytes.Buffer
	require.NoError(t, packet.Serialize(&packetBytes))

	return PrefixProofOfFunds + b64Encode(packetBytes.Bytes())
}

// newNegativePoFPacket constructs the deterministic valid challenge input used
// as the base for malformed proof-of-funds vectors.
func newNegativePoFPacket(t *testing.T, message []byte, pkScript,
	witnessBytes []byte, version int32) *psbt.Packet {

	t.Helper()

	packet, err := BuildToSignPacketFull(
		message, pkScript, version, 0, 0,
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

// negativeFullTx extracts the finalized transaction from a single-input PSBT
// so vectors can alter fields that SerializeSignature would reject.
func negativeFullTx(t *testing.T, packet *psbt.Packet) *wire.MsgTx {
	t.Helper()

	tx, err := psbt.Extract(packet)
	require.NoError(t, err)

	return tx
}

// generateParserNegativeVectors creates compact malformed encodings that must
// be rejected before script execution.
func generateParserNegativeVectors(t *testing.T, addr string,
	basePacket *psbt.Packet) []errorVector {

	t.Helper()

	testCases := []negativeVectorCase{{
		description: "simple witness declares too many items",
		generate: func(t *testing.T) errorVector {
			// Just claim maxWitnessItems+1 items, no item data
			// follows. If the parser allocated first, it would
			// commit ~24 MB from a 5-byte input.
			return invalidNegativeVector(
				"probe", addr, encodeNegativeWitness([]byte{
					0xfe, 0x01, 0x00, 0x40, 0x00,
				}), "too many witness items",
			)
		},
	}, {
		description: "simple witness declares an oversized item",
		generate: func(t *testing.T) errorVector {
			// 1 item, declared length = maxWitnessItems + 1, no
			// witness body.
			return invalidNegativeVector(
				"probe", addr, encodeNegativeWitness([]byte{
					0x01, 0xfe, 0x01, 0x00, 0x40,
					0x00,
				}), "witness item too large",
			)
		},
	}, {
		description: "simple witness item exceeds remaining bytes",
		generate: func(t *testing.T) errorVector {
			// 1 item, declared length = 100_000, but only 1 body
			// byte available.
			return invalidNegativeVector(
				"probe", addr, encodeNegativeWitness([]byte{
					0x01, 0xfd, 0xa0, 0x86, 0xaa,
				}), "exceeds remaining input",
			)
		},
	}, {
		description: "full transaction contains trailing data",
		generate: func(t *testing.T) errorVector {
			// A valid transaction followed by extra bytes must not
			// be accepted as a canonical full signature.
			tx := wire.NewMsgTx(2)
			tx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Index: 0xffffffff,
				},
				Sequence: 0xffffffff,
			})
			tx.AddTxOut(&wire.TxOut{
				Value:    0,
				PkScript: []byte{txscript.OP_RETURN},
			})
			var txBytes bytes.Buffer
			require.NoError(t, tx.Serialize(&txBytes))
			trailingTx := append(
				bytes.Clone(txBytes.Bytes()),
				0xde, 0xad, 0xbe, 0xef,
			)

			return invalidNegativeVector(
				"probe", addr, PrefixFull+b64Encode(trailingTx),
				errMoreDataAvailable.Error(),
			)
		},
	}, {
		description: "proof-of-funds packet contains trailing data",
		generate: func(t *testing.T) errorVector {
			// The finalized PSBT container must consume its
			// complete encoding just like a full transaction.
			var packetBytes bytes.Buffer
			require.NoError(t, basePacket.Serialize(&packetBytes))
			trailingPacket := append(
				bytes.Clone(packetBytes.Bytes()),
				0xde, 0xad, 0xbe, 0xef,
			)

			return invalidNegativeVector(
				"probe", addr,
				PrefixProofOfFunds+b64Encode(trailingPacket),
				psbt.ErrInvalidPsbtFormat.Error(),
			)
		},
	}}

	return runNegativeVectorCases(t, testCases)
}

// generateRuleNegativeVectors creates deterministic signatures that violate
// BIP-322 required or upgradeable script rules.
func generateRuleNegativeVectors(t *testing.T) []errorVector {
	t.Helper()

	const message = "probe"
	messageBytes := []byte(message)

	// minimalIfVector constructs the two opcode variants from the same
	// legacy P2SH template while each remains a separately named table
	// case.
	minimalIfVector := func(
		opcode byte) func(*testing.T) errorVector {

		return func(t *testing.T) errorVector {
			redeemScript, err := txscript.NewScriptBuilder().
				AddOp(opcode).
				AddOp(txscript.OP_TRUE).
				AddOp(txscript.OP_ELSE).
				AddOp(txscript.OP_TRUE).
				AddOp(txscript.OP_ENDIF).
				Script()
			require.NoError(t, err)
			addr, err := address.NewAddressScriptHash(
				redeemScript, &chaincfg.MainNetParams,
			)
			require.NoError(t, err)
			pkScript, err := txscript.PayToAddrScript(addr)
			require.NoError(t, err)
			sigScript, err := txscript.NewScriptBuilder().
				AddOp(txscript.OP_2).
				AddData(redeemScript).
				Script()
			require.NoError(t, err)
			packet, err := BuildToSignPacketFull(
				messageBytes, pkScript, 0, 0, 0,
			)
			require.NoError(t, err)
			packet.Inputs[0].FinalScriptSig = sigScript

			return invalidNegativeVector(
				message, addr.String(), encodeNegativeFull(
					t, negativeFullTx(t, packet),
				), txscript.ErrMinimalIf.String(),
			)
		}
	}

	testCases := []negativeVectorCase{{
		description: "OP_CODESEPARATOR in P2WSH witness script",
		generate: func(t *testing.T) errorVector {
			codeSepScript := []byte{
				txscript.OP_CODESEPARATOR, txscript.OP_TRUE,
			}
			_, addr := negativeP2WSHChallenge(t, codeSepScript)
			witnessBytes, err := SerializeTxWitness(
				wire.TxWitness{codeSepScript},
			)
			require.NoError(t, err)

			return invalidNegativeVector(
				message, addr,
				encodeNegativeWitness(witnessBytes),
				ErrCodeSeparator.Error(),
			)
		},
	}, {
		description: "OP_CODESEPARATOR in tapscript leaf",
		generate: func(t *testing.T) errorVector {
			codeSepScript := []byte{
				txscript.OP_CODESEPARATOR, txscript.OP_TRUE,
			}
			_, addr, controlBlock := negativeTaprootChallenge(
				t, codeSepScript, fixedNegativeKey(1),
			)
			witnessBytes, err := SerializeTxWitness(wire.TxWitness{
				codeSepScript, controlBlock,
			})
			require.NoError(t, err)

			return invalidNegativeVector(
				message, addr,
				encodeNegativeWitness(witnessBytes),
				ErrCodeSeparator.Error(),
			)
		},
	}, {
		description: "P2WPKH simple signature uses SIGHASH_NONE",
		generate: func(t *testing.T) errorVector {
			// The signature is cryptographically valid, leaving its
			// prohibited sighash type as the only failure.
			key := fixedNegativeKey(2)
			pkScript, addr := negativeP2WPKHChallenge(t, key)
			packet, err := BuildToSignPacketSimple(
				messageBytes, pkScript,
			)
			require.NoError(t, err)
			tx := packet.UnsignedTx.Copy()
			utxo := packet.Inputs[0].WitnessUtxo
			fetcher := txscript.NewCannedPrevOutputFetcher(
				utxo.PkScript, utxo.Value,
			)
			sigHashes := txscript.NewTxSigHashes(tx, fetcher)
			sig, err := txscript.RawTxInWitnessSignature(
				tx, sigHashes, 0, utxo.Value, utxo.PkScript,
				txscript.SigHashNone, key,
			)
			require.NoError(t, err)
			witnessBytes, err := SerializeTxWitness(wire.TxWitness{
				sig, key.PubKey().SerializeCompressed(),
			})
			require.NoError(t, err)

			return invalidNegativeVector(
				message, addr,
				encodeNegativeWitness(witnessBytes),
				ErrInvalidSigHashFlag.Error(),
			)
		},
	}, {
		description: "taproot key-path simple signature uses " +
			"SIGHASH_SINGLE",
		generate: func(t *testing.T) errorVector {
			key := fixedNegativeKey(3)
			pkScript, addr := negativeKeyPathChallenge(t, key)
			packet, err := BuildToSignPacketSimple(
				messageBytes, pkScript,
			)
			require.NoError(t, err)
			tx := packet.UnsignedTx.Copy()
			utxo := packet.Inputs[0].WitnessUtxo
			fetcher := txscript.NewCannedPrevOutputFetcher(
				utxo.PkScript, utxo.Value,
			)
			sigHashes := txscript.NewTxSigHashes(tx, fetcher)
			witness, err := txscript.TaprootWitnessSignature(
				tx, sigHashes, 0, utxo.Value, utxo.PkScript,
				txscript.SigHashSingle, key,
			)
			require.NoError(t, err)
			witnessBytes, err := SerializeTxWitness(witness)
			require.NoError(t, err)

			return invalidNegativeVector(
				message, addr,
				encodeNegativeWitness(witnessBytes),
				ErrInvalidSigHashFlag.Error(),
			)
		},
	}, {
		description: "tapscript simple signature uses SIGHASH_SINGLE",
		generate: func(t *testing.T) errorVector {
			key := fixedNegativeKey(4)
			leafScript, err := txscript.NewScriptBuilder().
				AddData(schnorr.SerializePubKey(key.PubKey())).
				AddOp(txscript.OP_CHECKSIG).
				Script()
			require.NoError(t, err)
			pkScript, addr, ctrlBlock := negativeTaprootChallenge(
				t, leafScript, fixedNegativeKey(5),
			)
			packet, err := BuildToSignPacketSimple(
				messageBytes, pkScript,
			)
			require.NoError(t, err)
			tx := packet.UnsignedTx.Copy()
			utxo := packet.Inputs[0].WitnessUtxo
			fetcher := txscript.NewCannedPrevOutputFetcher(
				utxo.PkScript, utxo.Value,
			)
			sigHashes := txscript.NewTxSigHashes(tx, fetcher)
			sig, err := txscript.RawTxInTapscriptSignature(
				tx, sigHashes, 0, utxo.Value, utxo.PkScript,
				txscript.NewBaseTapLeaf(leafScript),
				txscript.SigHashSingle, key,
			)
			require.NoError(t, err)
			witnessBytes, err := SerializeTxWitness(wire.TxWitness{
				sig, leafScript, ctrlBlock,
			})
			require.NoError(t, err)

			return invalidNegativeVector(
				message, addr,
				encodeNegativeWitness(witnessBytes),
				ErrInvalidSigHashFlag.Error(),
			)
		},
	}, {
		description: "P2WSH multisig second signature uses " +
			"SIGHASH_SINGLE",
		generate: func(t *testing.T) errorVector {
			// CHECKMULTISIG must inspect a disallowed second
			// signature after consuming a valid SIGHASH_ALL
			// signature.
			key1 := fixedNegativeKey(6)
			key2 := fixedNegativeKey(7)
			witnessScript, err := txscript.NewScriptBuilder().
				AddOp(txscript.OP_2).
				AddData(key1.PubKey().SerializeCompressed()).
				AddData(key2.PubKey().SerializeCompressed()).
				AddOp(txscript.OP_2).
				AddOp(txscript.OP_CHECKMULTISIG).
				Script()
			require.NoError(t, err)
			pkScript, addr := negativeP2WSHChallenge(
				t, witnessScript,
			)
			packet, err := BuildToSignPacketSimple(
				messageBytes, pkScript,
			)
			require.NoError(t, err)
			tx := packet.UnsignedTx.Copy()
			utxo := packet.Inputs[0].WitnessUtxo
			fetcher := txscript.NewCannedPrevOutputFetcher(
				utxo.PkScript, utxo.Value,
			)
			sigHashes := txscript.NewTxSigHashes(tx, fetcher)
			sig1, err := txscript.RawTxInWitnessSignature(
				tx, sigHashes, 0, utxo.Value, witnessScript,
				txscript.SigHashAll, key1,
			)
			require.NoError(t, err)
			sig2, err := txscript.RawTxInWitnessSignature(
				tx, sigHashes, 0, utxo.Value, witnessScript,
				txscript.SigHashSingle, key2,
			)
			require.NoError(t, err)
			witnessBytes, err := SerializeTxWitness(wire.TxWitness{
				nil, sig1, sig2, witnessScript,
			})
			require.NoError(t, err)

			return invalidNegativeVector(
				message, addr,
				encodeNegativeWitness(witnessBytes),
				ErrInvalidSigHashFlag.Error(),
			)
		},
	}, {
		description: "legacy P2SH OP_IF uses non-minimal true",
		generate:    minimalIfVector(txscript.OP_IF),
	}, {
		description: "legacy P2SH OP_NOTIF uses non-minimal true",
		generate:    minimalIfVector(txscript.OP_NOTIF),
	}, {
		description: "reserved OP_NOP5 is inconclusive",
		generate: func(t *testing.T) errorVector {
			witnessScript := []byte{
				txscript.OP_NOP5, txscript.OP_TRUE,
			}
			_, addr := negativeP2WSHChallenge(t, witnessScript)
			witnessBytes, err := SerializeTxWitness(
				wire.TxWitness{witnessScript},
			)
			require.NoError(t, err)

			return inconclusiveNegativeVector(
				message, addr,
				encodeNegativeWitness(witnessBytes),
				ErrInconclusive.Error(),
			)
		},
	}, {
		description: "full transaction version 1 is inconclusive",
		generate: func(t *testing.T) errorVector {
			witnessScript := []byte{txscript.OP_TRUE}
			pkScript, addr := negativeP2WSHChallenge(
				t, witnessScript,
			)
			witnessBytes, err := SerializeTxWitness(
				wire.TxWitness{witnessScript},
			)
			require.NoError(t, err)
			packet, err := BuildToSignPacketFull(
				messageBytes, pkScript, 1, 0, 0,
			)
			require.NoError(t, err)
			packet.Inputs[0].FinalScriptWitness = witnessBytes

			return inconclusiveNegativeVector(
				message, addr, encodeNegativeFull(
					t, negativeFullTx(t, packet),
				), ErrInconclusive.Error(),
			)
		},
	}, {
		description: "invalid script precedes inconclusive " +
			"transaction version",
		generate: func(t *testing.T) errorVector {
			// The current script failure must win over the
			// version's upgradeable-rule classification.
			witnessScript := []byte{txscript.OP_FALSE}
			pkScript, addr := negativeP2WSHChallenge(
				t, witnessScript,
			)
			witnessBytes, err := SerializeTxWitness(
				wire.TxWitness{witnessScript},
			)
			require.NoError(t, err)
			packet, err := BuildToSignPacketFull(
				messageBytes, pkScript, 3, 0, 0,
			)
			require.NoError(t, err)
			packet.Inputs[0].FinalScriptWitness = witnessBytes

			return invalidNegativeVector(
				message, addr, encodeNegativeFull(
					t, negativeFullTx(t, packet),
				), ErrInvalidSignature.Error(),
			)
		},
	}}

	return runNegativeVectorCases(t, testCases)
}

// generatePoFNegativeVectors creates deterministic malformed proof-of-funds
// packets covering authenticated UTXO data and transaction structure rules.
func generatePoFNegativeVectors(t *testing.T) []errorVector {
	t.Helper()

	const message = "probe"
	messageBytes := []byte(message)
	witnessScript := []byte{txscript.OP_TRUE}
	pkScript, addr := negativeP2WSHChallenge(t, witnessScript)
	witnessBytes, err := SerializeTxWitness(wire.TxWitness{witnessScript})
	require.NoError(t, err)

	newPacket := func(t *testing.T, version int32) *psbt.Packet {
		return newNegativePoFPacket(
			t, messageBytes, pkScript, witnessBytes, version,
		)
	}
	newOutput := func(value int64, script []byte) *wire.TxOut {
		return &wire.TxOut{Value: value, PkScript: bytes.Clone(script)}
	}
	addValidInput := func(packet *psbt.Packet, tag byte) {
		var hash chainhash.Hash
		hash[0] = tag
		appendNegativePoFInput(
			packet, wire.OutPoint{Hash: hash}, psbt.PInput{
				WitnessUtxo:        newOutput(1, pkScript),
				FinalScriptWitness: witnessBytes,
			},
		)
	}

	conflictingUtxoVector := func(
		witnessUtxo func() *wire.TxOut) func(*testing.T) errorVector {

		return func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			prevTx := negativePrevTx(newOutput(1, pkScript))
			appendNegativePoFInput(
				packet, wire.OutPoint{Hash: prevTx.TxHash()},
				psbt.PInput{
					NonWitnessUtxo:     prevTx,
					WitnessUtxo:        witnessUtxo(),
					FinalScriptWitness: witnessBytes,
				},
			)

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"witness utxo does not match non witness utxo",
			)
		}
	}

	conflictingScript := bytes.Clone(pkScript)
	conflictingScript[len(conflictingScript)-1] ^= 1
	testCases := []negativeVectorCase{{
		description: "proof-of-funds legacy input has only " +
			"witness UTXO",
		generate: func(t *testing.T) errorVector {
			// A legacy signature does not authenticate the amount,
			// so a bare witness UTXO cannot establish the claimed
			// value.
			key := fixedNegativeKey(8)
			pubKeyHash := address.Hash160(
				key.PubKey().SerializeCompressed(),
			)
			legacyAddr, err := address.NewAddressPubKeyHash(
				pubKeyHash, &chaincfg.MainNetParams,
			)
			require.NoError(t, err)
			legacyScript, err := txscript.PayToAddrScript(
				legacyAddr,
			)
			require.NoError(t, err)
			packet := newPacket(t, 0)
			prevTx := negativePrevTx(newOutput(1, legacyScript))
			appendNegativePoFInput(packet, wire.OutPoint{
				Hash: prevTx.TxHash(),
			}, psbt.PInput{
				WitnessUtxo: newOutput(
					100_000_000, legacyScript,
				),
				FinalScriptWitness: []byte{0x00},
			})
			sigScript, err := txscript.SignatureScript(
				packet.UnsignedTx, 1, legacyScript,
				txscript.SigHashAll, key, true,
			)
			require.NoError(t, err)
			packet.Inputs[1].FinalScriptSig = sigScript

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"witness UTXO for spending a non-segwit script",
			)
		},
	}, {
		description: "proof-of-funds non-witness UTXO has wrong txid",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			prevTx := negativePrevTx(newOutput(1, pkScript))
			var wrongHash chainhash.Hash
			wrongHash[0] = 0x42
			appendNegativePoFInput(
				packet, wire.OutPoint{Hash: wrongHash},
				psbt.PInput{
					NonWitnessUtxo:     prevTx,
					FinalScriptWitness: witnessBytes,
				},
			)

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"non witness utxo does not match input prevout",
			)
		},
	}, {
		description: "proof-of-funds witness UTXO has " +
			"conflicting amount",
		generate: conflictingUtxoVector(func() *wire.TxOut {
			return newOutput(2, pkScript)
		}),
	}, {
		description: "proof-of-funds witness UTXO has " +
			"conflicting script",
		generate: conflictingUtxoVector(func() *wire.TxOut {
			return newOutput(1, conflictingScript)
		}),
	}, {
		description: "proof-of-funds contains duplicate " +
			"additional inputs",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			var duplicateHash chainhash.Hash
			duplicateHash[0] = 0x43
			for range 2 {
				appendNegativePoFInput(packet, wire.OutPoint{
					Hash: duplicateHash,
				}, psbt.PInput{
					WitnessUtxo: newOutput(
						1, pkScript,
					),
					FinalScriptWitness: witnessBytes,
				})
			}

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"duplicate inputs",
			)
		},
	}, {
		description: "proof-of-funds contains null additional input",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			appendNegativePoFInput(packet, wire.OutPoint{
				Index: 0xffffffff,
			}, psbt.PInput{
				WitnessUtxo: newOutput(
					1, pkScript,
				),
				FinalScriptWitness: witnessBytes,
			})

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"zero hash present",
			)
		},
	}, {
		description: "proof-of-funds witness declares too " +
			"many items",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			packet.Inputs[0].FinalScriptWitness = []byte{
				0xfe, 0xff, 0xff, 0xff, 0xff,
			}

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"too many witness items",
			)
		},
	}, {
		description: "proof-of-funds reuses a later " +
			"non-witness UTXO",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			prevTx := negativePrevTx(
				newOutput(1, pkScript),
				newOutput(1, pkScript),
			)
			appendNegativePoFInput(packet, wire.OutPoint{
				Hash: prevTx.TxHash(),
			}, psbt.PInput{
				FinalScriptWitness: witnessBytes,
			})
			appendNegativePoFInput(packet, wire.OutPoint{
				Hash:  prevTx.TxHash(),
				Index: 1,
			}, psbt.PInput{
				NonWitnessUtxo:     prevTx,
				FinalScriptWitness: witnessBytes,
			})

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"UTXO not found",
			)
		},
	}, {
		description: "proof-of-funds generic signed message " +
			"does not match",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			addValidInput(packet, 0x44)
			differentMessage := "different message"
			packet.GenericSignedMessage = &differentMessage

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"generic signed message field",
			)
		},
	}, {
		description: "proof-of-funds to_sign output is not OP_RETURN",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			packet.UnsignedTx.TxOut[0].PkScript = []byte{
				txscript.OP_TRUE,
			}

			return invalidNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				"single OP_RETURN byte",
			)
		},
	}, {
		description: "proof-of-funds transaction version 3 is " +
			"inconclusive",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 3)
			addValidInput(packet, 0x45)

			return inconclusiveNegativeVector(
				message, addr, encodeNegativePoF(t, packet),
				ErrInconclusive.Error(),
			)
		},
	}, {
		description: "proof-of-funds base64 contains embedded " +
			"newline",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			addValidInput(packet, 0x46)
			canonical := encodeNegativePoF(t, packet)
			signature := canonical[:len(PrefixProofOfFunds)+4] +
				"\n" +
				canonical[len(PrefixProofOfFunds)+4:]

			return invalidNegativeVector(
				message, addr, signature, "base64",
			)
		},
	}, {
		description: "proof-of-funds base64 has non-canonical " +
			"padding bits",
		generate: func(t *testing.T) errorVector {
			packet := newPacket(t, 0)
			addValidInput(packet, 0x46)

			// Add deterministic unknown data until the encoding has
			// padding whose unused bits can be changed.
			var canonical string
			for valueLen := 0; ; valueLen++ {
				packet.Unknowns = []*psbt.Unknown{{
					Key:   []byte{0xaa},
					Value: make([]byte, valueLen),
				}}
				canonical = encodeNegativePoF(t, packet)
				if strings.Contains(canonical, "=") {
					break
				}
			}
			signature := PrefixProofOfFunds +
				makeNegativeBase64PaddingBitsNonZero(
					t, canonical[len(PrefixProofOfFunds):],
				)

			return invalidNegativeVector(
				message, addr, signature, "illegal base64 data",
			)
		},
	}}

	return runNegativeVectorCases(t, testCases)
}

// generateNegativeTestVectors builds the complete deterministic vector set.
// Resource-limit cases that require multi-megabyte payloads intentionally stay
// as direct unit tests instead of inflating the checked-in fixture.
func generateNegativeTestVectors(t *testing.T) *testVectors {
	t.Helper()

	witnessScript := []byte{txscript.OP_TRUE}
	pkScript, addr := negativeP2WSHChallenge(t, witnessScript)
	witnessBytes, err := SerializeTxWitness(wire.TxWitness{witnessScript})
	require.NoError(t, err)
	basePacket := newNegativePoFPacket(
		t, []byte("probe"), pkScript, witnessBytes, 0,
	)

	vectors := generateParserNegativeVectors(t, addr, basePacket)
	vectors = append(vectors, generateRuleNegativeVectors(t)...)
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

		wrongMessageVector := invalidNegativeVector(
			wrongMessage, vector.Address, signature, errorSubstr,
		)
		wrongMessageVector.Description = "wrong message for " +
			vector.Type + " " + variant + " signature"
		result = append(result, wrongMessageVector)

		wrongAddressVector := invalidNegativeVector(
			vector.Message, wrongAddr, signature, errorSubstr,
		)
		wrongAddressVector.Description = "wrong address for " +
			vector.Type + " " + variant + " signature"
		result = append(result, wrongAddressVector)
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
