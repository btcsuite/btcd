package silentpayments

import (
	"encoding/hex"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestCreateOutputs tests the generation of silent payment outputs.
func TestCreateOutputs(t *testing.T) {
	vectors, err := ReadTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
		vector := vector
		t.Run(vector.Comment, func(tt *testing.T) {
			runCreateOutputTest(tt, vector)
		})
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

			privKeyBytes, err := hex.DecodeString(
				vin.PrivateKey,
			)
			require.NoError(t, err)
			privKey, _ := btcec.PrivKeyFromBytes(
				privKeyBytes,
			)

			inputs = append(inputs, Input{
				OutPoint: *outpoint,
				Utxo:     *utxo,
				PrivKey:  *privKey,
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
		require.NoError(t, err)

		if len(result) == 0 {
			require.Empty(t, sending.Expected.Outputs[0])

			continue
		}

		require.Len(t, result, len(sending.Expected.Outputs[0]))

		for idx, output := range result {
			resultBytes := schnorr.SerializePubKey(
				output.OutputKey,
			)

			require.Equal(
				t, sending.Expected.Outputs[0][idx],
				hex.EncodeToString(resultBytes),
			)
		}
	}
}
