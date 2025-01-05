package silentpayments

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/btcsuite/btcd/txscript"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

var (
	// BIP0341NUMSPoint is an example NUMS point as defined in BIP-0341.
	BIP0341NUMSPoint = []byte{
		0x50, 0x92, 0x9b, 0x74, 0xc1, 0xa0, 0x49, 0x54,
		0xb7, 0x8b, 0x4b, 0x60, 0x35, 0xe9, 0x7a, 0x5e,
		0x07, 0x8a, 0x5a, 0x0f, 0x28, 0xec, 0x96, 0xd5,
		0x47, 0xbf, 0xee, 0x9a, 0xce, 0x80, 0x3a, 0xc0,
	}
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
		inputs := make([]Input, 0, len(sending.Given.Vin))
		for _, vin := range sending.Given.Vin {
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

			privKeyBytes, err := hex.DecodeString(
				vin.PrivateKey,
			)
			require.NoError(t, err)
			privKey, _ := btcec.PrivKeyFromBytes(
				privKeyBytes,
			)

			inputs = append(inputs, Input{
				OutPoint:  *outpoint,
				Utxo:      *utxo,
				PrivKey:   *privKey,
				SkipInput: skip,
			})
		}

		recipients := make(
			[]Address, 0, len(sending.Given.Recipients),
		)
		for _, recipient := range sending.Given.Recipients {
			addr, err := DecodeAddress(recipient)
			require.NoError(t, err)

			recipients = append(recipients, *addr)
		}

		result, err := CreateOutputs(inputs, recipients)
		
		// Special case for when the input keys add up to zero.
		if errors.Is(err, ErrInputKeyZero) {
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

func resultsContained(t *testing.T, expected [][]string, results []string) {
	for _, expectedSet := range expected {
		contained := false
		for _, e := range expectedSet {
			for _, r := range results {
				if e == r {
					contained = true
					break
				}
			}
		}

		if contained {
			return
		}
	}

	require.Fail(t, "no expected output found in results")
}
