package silentpayments

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestCreateOutputs tests the generation of silent payment outputs.
func TestCreateOutputs(t *testing.T) {
	vectors, err := ReadTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
		vector := vector
		success := t.Run(vector.Comment, func(tt *testing.T) {
			runCreateOutputTest(tt, vector)
		})

		if !success {
			break
		}
	}
}

// runCreateOutputTest tests the generation of silent payment outputs.
func runCreateOutputTest(t *testing.T, vector *TestVector) {
	for _, sending := range vector.Sending {
		inputs := parseInputs(t, sending.Given.Vin)

		recipients := make(
			[]Address, 0, len(sending.Given.Recipients),
		)
		for _, recipient := range sending.Given.Recipients {
			addr, err := DecodeAddress(recipient.Address)
			require.NoError(t, err)

			// A recipient may carry a count to request multiple
			// outputs to the same address; an unset count means a
			// single output.
			count := recipient.Count
			if count == 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				recipients = append(recipients, *addr)
			}
		}

		result, err := CreateOutputs(inputs, recipients)

		// Special case for when the input keys add up to zero, when no
		// compatible input is provided, or when a recipient group
		// exceeds the maximum size. In all of these the sender fails
		// and the vector expects no outputs.
		if errors.Is(err, ErrInputKeyZero) ||
			errors.Is(err, ErrNoInputs) ||
			errors.Is(err, ErrTooManyRecipientsPerGroup) {

			require.Empty(t, sending.Expected.Outputs[0])

			continue
		}

		require.NoError(t, err)

		if len(result) == 0 {
			require.Empty(t, sending.Expected.Outputs[0])

			continue
		}

		require.Len(t, result, len(sending.Expected.Outputs[0]))

		resultStrings := make([]string, len(result))
		for idx, output := range result {
			resultStrings[idx] = hex.EncodeToString(
				schnorr.SerializePubKey(output.OutputKey),
			)
		}

		resultsContained(t, sending.Expected.Outputs, resultStrings)
	}
}

func parseInputs(t *testing.T, vIn []*VIn) []Input {
	inputs := make([]Input, len(vIn))
	for idx, vin := range vIn {
		txid, err := chainhash.NewHashFromStr(vin.Txid)
		require.NoError(t, err)

		outpoint := wire.NewOutPoint(txid, vin.Vout)

		pkScript, err := hex.DecodeString(
			vin.PrevOut.ScriptPubKey.Hex,
		)
		require.NoError(t, err)
		utxo := wire.NewTxOut(0, pkScript)

		sigScript, err := hex.DecodeString(vin.ScriptSig)
		require.NoError(t, err)

		witnessBytes, err := hex.DecodeString(vin.TxInWitness)
		require.NoError(t, err)

		skip := shouldSkip(t, pkScript, sigScript, witnessBytes)

		privKeyBytes, err := hex.DecodeString(vin.PrivateKey)
		require.NoError(t, err)
		privKey, _ := btcec.PrivKeyFromBytes(privKeyBytes)

		inputs[idx] = Input{
			OutPoint:  *outpoint,
			Utxo:      *utxo,
			PrivKey:   *privKey,
			SkipInput: skip,
		}
	}

	return inputs
}

func shouldSkip(t *testing.T, pkScript, sigScript, witnessBytes []byte) bool {
	script, err := txscript.ParsePkScript(pkScript)
	require.NoError(t, err)

	// Special case for P2PKH:
	if script.Class() == txscript.PubKeyHashTy {
		return checkPubKeyScriptSig(t, sigScript)
	}

	// Special case for P2SH with script sig only:
	if script.Class() == txscript.ScriptHashTy && len(witnessBytes) == 0 &&
		len(sigScript) != 0 {

		return checkPubKeyScriptSig(t, sigScript)

	}

	if len(witnessBytes) == 0 {
		return false
	}

	witness, err := parseWitness(witnessBytes)
	require.NoError(t, err)

	if len(witness) == 0 {
		return true
	}

	switch script.Class() {
	case txscript.WitnessV0PubKeyHashTy:
		lastWitness := witness[len(witness)-1]

		return len(lastWitness) != btcec.PubKeyBytesLenCompressed

	case txscript.ScriptHashTy:
		lastWitness := witness[len(witness)-1]

		return len(lastWitness) != btcec.PubKeyBytesLenCompressed

	case txscript.WitnessV1TaprootTy:
		return isNUMSWitness(witnessBytes)

	default:
		return true
	}
}

func checkPubKeyScriptSig(t *testing.T, sigScript []byte) bool {
	// If the sigScript isn't set, we just assume a valid key.
	if len(sigScript) == 0 {
		return false
	}

	tokenizer := txscript.MakeScriptTokenizer(0, sigScript)
	for tokenizer.Next() {
		if tokenizer.Opcode() == txscript.OP_DATA_33 &&
			len(tokenizer.Data()) == 33 {

			return false
		}
	}
	if err := tokenizer.Err(); err != nil {
		t.Fatalf("error tokenizing sigScript: %v", err)
	}

	// If there was a sigScript set but there was no 33-byte
	// compressed key push, we skip the input.
	return true
}

func isNUMSWitness(witnessBytes []byte) bool {
	return bytes.Contains(witnessBytes, BIP0341NUMSPoint)
}

func parseWitness(witnessBytes []byte) (wire.TxWitness, error) {
	if len(witnessBytes) == 0 {
		return nil, nil
	}

	witnessReader := bytes.NewReader(witnessBytes)
	witCount, err := wire.ReadVarInt(witnessReader, 0)
	if err != nil {
		return nil, err
	}

	result := make(wire.TxWitness, witCount)
	for j := uint64(0); j < witCount; j++ {
		wit, err := wire.ReadVarBytes(
			witnessReader, 0, txscript.MaxScriptSize, "witness",
		)
		if err != nil {
			return nil, err
		}
		result[j] = wit
	}

	return result, nil
}

// TestInputCompatible tests that InputCompatible correctly classifies the
// different input script types and only reports an input as compatible when the
// provided public key actually controls it.
func TestInputCompatible(t *testing.T) {
	t.Parallel()

	net := &chaincfg.MainNetParams

	// ownerKey is the public key that controls each of the inputs we
	// construct below, so every derived pkScript commits to it.
	ownerKey := pubKeyFromByte(t, 1)

	// otherKey is an unrelated key that doesn't control any of the inputs.
	// It's used to assert that an input is rejected when the provided key
	// doesn't match the script.
	otherKey := pubKeyFromByte(t, 2)

	testCases := []struct {
		name       string
		pkScript   []byte
		pubKey     *btcec.PublicKey
		class      txscript.ScriptClass
		compatible bool
		expectErr  bool
	}{{
		// A P2PKH input is compatible if the key hashes to the hash
		// committed to in the script.
		name:       "P2PKH with matching key",
		pkScript:   p2pkhScript(t, net, ownerKey),
		pubKey:     ownerKey,
		class:      txscript.PubKeyHashTy,
		compatible: true,
	}, {
		// A different key produces a different hash, so the input is
		// not ours to spend for shared secret derivation.
		name:       "P2PKH with non-matching key",
		pkScript:   p2pkhScript(t, net, ownerKey),
		pubKey:     otherKey,
		class:      txscript.PubKeyHashTy,
		compatible: false,
	}, {
		// The same matching/non-matching logic applies to P2WPKH.
		name:       "P2WPKH with matching key",
		pkScript:   p2wpkhScript(t, net, ownerKey),
		pubKey:     ownerKey,
		class:      txscript.WitnessV0PubKeyHashTy,
		compatible: true,
	}, {
		name:       "P2WPKH with non-matching key",
		pkScript:   p2wpkhScript(t, net, ownerKey),
		pubKey:     otherKey,
		class:      txscript.WitnessV0PubKeyHashTy,
		compatible: false,
	}, {
		// For P2TR the NUMS check can't be done from the pkScript
		// alone, so the input is always reported as compatible,
		// regardless of the key passed in.
		name:       "P2TR is always compatible regardless of key",
		pkScript:   p2trScript(t, net, ownerKey),
		pubKey:     otherKey,
		class:      txscript.WitnessV1TaprootTy,
		compatible: true,
	}, {
		// A nested P2SH-P2WPKH input is compatible if the key hashes
		// to the witness program wrapped by the P2SH output.
		name:       "nested P2SH-P2WPKH with matching key",
		pkScript:   p2shP2wpkhScript(t, net, ownerKey),
		pubKey:     ownerKey,
		class:      txscript.ScriptHashTy,
		compatible: true,
	}, {
		name:       "nested P2SH-P2WPKH with non-matching key",
		pkScript:   p2shP2wpkhScript(t, net, ownerKey),
		pubKey:     otherKey,
		class:      txscript.ScriptHashTy,
		compatible: false,
	}, {
		// Any other parsable script type (here P2WSH) is simply not
		// compatible with silent payments.
		name:       "P2WSH is not compatible",
		pkScript:   p2wshScript(t, net, ownerKey),
		pubKey:     ownerKey,
		class:      txscript.WitnessV0ScriptHashTy,
		compatible: false,
	}, {
		// A script type that ParsePkScript doesn't support (here P2PK)
		// surfaces as an error rather than just being incompatible.
		name:      "unsupported script type returns error",
		pkScript:  p2pkScript(t, net, ownerKey),
		pubKey:    ownerKey,
		expectErr: true,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			compatible, script, err := InputCompatible(
				tc.pkScript, tc.pubKey,
			)

			if tc.expectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.compatible, compatible)
			require.Equal(t, tc.class, script.Class())
		})
	}
}

// addrScript returns the pkScript that pays to the given address.
func addrScript(t *testing.T, addr address.Address) []byte {
	t.Helper()

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	return pkScript
}

// p2pkhScript derives a P2PKH address for the given key and returns its
// pkScript.
func p2pkhScript(t *testing.T, net *chaincfg.Params,
	pubKey *btcec.PublicKey) []byte {

	t.Helper()

	addr, err := address.NewAddressPubKeyHash(
		address.Hash160(pubKey.SerializeCompressed()), net,
	)
	require.NoError(t, err)

	return addrScript(t, addr)
}

// p2wpkhScript derives a P2WPKH address for the given key and returns its
// pkScript.
func p2wpkhScript(t *testing.T, net *chaincfg.Params,
	pubKey *btcec.PublicKey) []byte {

	t.Helper()

	addr, err := address.NewAddressWitnessPubKeyHash(
		address.Hash160(pubKey.SerializeCompressed()), net,
	)
	require.NoError(t, err)

	return addrScript(t, addr)
}

// p2trScript derives a P2TR address for the given key and returns its pkScript.
func p2trScript(t *testing.T, net *chaincfg.Params,
	pubKey *btcec.PublicKey) []byte {

	t.Helper()

	addr, err := address.NewAddressTaproot(
		schnorr.SerializePubKey(pubKey), net,
	)
	require.NoError(t, err)

	return addrScript(t, addr)
}

// p2shP2wpkhScript derives a nested P2SH-P2WPKH address for the given key and
// returns its pkScript. This is the only flavor of P2SH that's compatible with
// silent payments.
func p2shP2wpkhScript(t *testing.T, net *chaincfg.Params,
	pubKey *btcec.PublicKey) []byte {

	t.Helper()

	witnessProgram, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_0).
		AddData(address.Hash160(pubKey.SerializeCompressed())).
		Script()
	require.NoError(t, err)

	addr, err := address.NewAddressScriptHash(witnessProgram, net)
	require.NoError(t, err)

	return addrScript(t, addr)
}

// p2wshScript derives a P2WSH address (an unsupported input type) and returns
// its pkScript. The witness program content is irrelevant here, so we just use
// the hash of the key.
func p2wshScript(t *testing.T, net *chaincfg.Params,
	pubKey *btcec.PublicKey) []byte {

	t.Helper()

	witnessProgram := sha256.Sum256(pubKey.SerializeCompressed())
	addr, err := address.NewAddressWitnessScriptHash(witnessProgram[:], net)
	require.NoError(t, err)

	return addrScript(t, addr)
}

// p2pkScript derives a P2PK address for the given key and returns its pkScript.
// P2PK isn't a script type supported by ParsePkScript, so InputCompatible
// returns an error for it.
func p2pkScript(t *testing.T, net *chaincfg.Params,
	pubKey *btcec.PublicKey) []byte {

	t.Helper()

	addr, err := address.NewAddressPubKey(pubKey.SerializeCompressed(), net)
	require.NoError(t, err)

	return addrScript(t, addr)
}

// TestCalculateInputHashTweakOrdering tests that the smallest outpoint is
// selected by comparing the serialized form (32-byte little-endian txid
// followed by the little-endian uint32 vout), not by integer comparison:
// for the same txid, vout 256 serializes as 00 01 00 00 and therefore sorts
// before vout 1 (01 00 00 00). Getting this wrong produces a valid-looking
// but different input hash, silently breaking sender/receiver agreement.
func TestCalculateInputHashTweakOrdering(t *testing.T) {
	t.Parallel()

	sumKey := pubKeyFromByte(t, 1)

	var txid chainhash.Hash
	for i := range txid {
		txid[i] = 0x11
	}
	op1 := wire.OutPoint{Hash: txid, Index: 1}
	op256 := wire.OutPoint{Hash: txid, Index: 256}

	both, err := CalculateInputHashTweak(
		[]wire.OutPoint{op1, op256}, sumKey,
	)
	require.NoError(t, err)

	// The input order must not matter.
	reversed, err := CalculateInputHashTweak(
		[]wire.OutPoint{op256, op1}, sumKey,
	)
	require.NoError(t, err)
	require.Equal(t, both, reversed)

	// The hash must commit to vout 256, the lexicographically smallest
	// serialized outpoint, despite 1 < 256 as integers.
	only256, err := CalculateInputHashTweak([]wire.OutPoint{op256}, sumKey)
	require.NoError(t, err)
	require.Equal(t, only256, both)

	only1, err := CalculateInputHashTweak([]wire.OutPoint{op1}, sumKey)
	require.NoError(t, err)
	require.NotEqual(t, only1, both)
}

// TestMaxRecipientsPerGroup tests the K_max per-scan-key-group limit: a group
// of exactly MaxRecipientsPerGroup recipients is allowed, but one more is
// rejected.
func TestMaxRecipientsPerGroup(t *testing.T) {
	t.Parallel()

	scanPub := pubKeyFromByte(t, 1)
	spendPub := pubKeyFromByte(t, 2)
	recipient := *NewAddress(MainNetHRP, *scanPub, *spendPub, nil)

	// A non-zero key sum and input hash are enough; the K_max check runs
	// before any output is derived.
	var sumKey, inputHash btcec.ModNScalar
	sumKey.SetInt(42)
	inputHash.SetInt(7)

	group := func(n int) []Address {
		recipients := make([]Address, n)
		for i := range recipients {
			recipients[i] = recipient
		}

		return recipients
	}

	// Exactly K_max recipients sharing a scan key must succeed.
	atLimit, err := AddressOutputKeys(
		group(MaxRecipientsPerGroup), sumKey, inputHash,
	)
	require.NoError(t, err)
	require.Len(t, atLimit, MaxRecipientsPerGroup)

	// One past the limit must be rejected.
	_, err = AddressOutputKeys(
		group(MaxRecipientsPerGroup+1), sumKey, inputHash,
	)
	require.ErrorIs(t, err, ErrTooManyRecipientsPerGroup)
}

// resultsContained asserts that the produced outputs match (as a set, ignoring
// order) at least one of the expected output sets. This mirrors the BIP-352
// reference check `any(set(outputs) == set(lst) for lst in expected)`, which
// is strictly stronger than "some output matches": all outputs must match.
func resultsContained(t *testing.T, expected [][]string, results []string) {
	for _, expectedSet := range expected {
		if setEqual(expectedSet, results) {
			return
		}
	}

	require.Fail(t, "results do not match any expected output set")
}

// setEqual returns true if a and b contain exactly the same elements, ignoring
// order (multiset equality).
func setEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, x := range a {
		counts[x]++
	}
	for _, x := range b {
		counts[x]--
		if counts[x] < 0 {
			return false
		}
	}

	return true
}
