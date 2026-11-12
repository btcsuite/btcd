package rpcclient

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestMempoolAcceptPreservesTransactions checks that local validation leaves
// caller-owned transactions unchanged on both success and failure.
func TestMempoolAcceptPreservesTransactions(t *testing.T) {
	for _, valid := range []bool{false, true} {
		name := "invalid"
		if valid {
			name = "valid"
		}
		t.Run(name, func(t *testing.T) {
			client := &Client{
				config:         &ConnConfig{HTTPPostMode: true},
				backendVersion: BitcoindPost25,
				sendPostChan:   make(chan *jsonRequest, 1),
				shutdown:       make(chan struct{}),
			}
			tx := wire.NewMsgTx(2)
			if valid {
				tx.AddTxIn(wire.NewTxIn(
					&wire.OutPoint{}, []byte{0x01},
					wire.TxWitness{{0x02, 0x03}},
				))
			}
			tx.AddTxOut(wire.NewTxOut(1, []byte{0x04, 0x05}))
			tx.LockTime = 123
			before := tx.Copy()
			inputs := tx.TxIn
			output := tx.TxOut[0]
			var serialized bytes.Buffer
			require.NoError(t, tx.Serialize(&serialized))

			future := client.TestMempoolAcceptAsync([]*wire.MsgTx{tx}, 0.1)
			require.Equal(t, before, tx)
			require.Same(t, output, tx.TxOut[0])
			if !valid {
				require.Empty(t, client.sendPostChan)
				require.Len(t, future, 1)
				_, err := future.Receive()
				require.ErrorIs(t, err, ErrInvalidParam)
				return
			}

			require.Same(t, inputs[0], tx.TxIn[0])
			require.Len(t, client.sendPostChan, 1)
			request := <-client.sendPostChan
			require.Equal(t, "testmempoolaccept", request.method)
			command := request.cmd.(*btcjson.TestMempoolAcceptCmd)
			require.Equal(t, []string{hex.EncodeToString(serialized.Bytes())}, command.RawTxns)
			require.Equal(t, btcjson.BTCPerkvB(0.1), command.MaxFeeRate)
			request.responseChan <- &Response{result: []byte("[]")}
			_, err := future.Receive()
			require.NoError(t, err)
		})
	}
}

// TestSignRawTransactionWithKey checks which optional parameters
// SignRawTransactionWithKeyAsync sends and that the result is decoded.
func TestSignRawTransactionWithKey(t *testing.T) {
	keys := []string{"test-wif-key"}
	inputs := []btcjson.RawTxWitnessInput{{
		Txid:         "00",
		Vout:         1,
		ScriptPubKey: "0014",
	}}

	tests := []struct {
		name         string
		inputs       []btcjson.RawTxWitnessInput
		hashType     SigHashType
		wantInputs   *[]btcjson.RawTxWitnessInput
		wantHashType *string
	}{{
		name: "keys only",
	}, {
		name:       "inputs",
		inputs:     inputs,
		wantInputs: &inputs,
	}, {
		name:         "hash type without inputs",
		hashType:     SigHashAllAnyoneCanPay,
		wantInputs:   &[]btcjson.RawTxWitnessInput{},
		wantHashType: btcjson.String("ALL|ANYONECANPAY"),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &Client{
				config:       &ConnConfig{HTTPPostMode: true},
				sendPostChan: make(chan *jsonRequest, 1),
				shutdown:     make(chan struct{}),
			}
			tx := wire.NewMsgTx(2)
			tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: 1}, nil, nil))
			tx.AddTxOut(wire.NewTxOut(1000, []byte{0x51}))
			var serialized bytes.Buffer
			require.NoError(t, tx.Serialize(&serialized))
			txHex := hex.EncodeToString(serialized.Bytes())

			future := client.SignRawTransactionWithKeyAsync(
				tx, keys, tc.inputs, tc.hashType,
			)

			request := <-client.sendPostChan
			require.Equal(t, "signrawtransactionwithkey", request.method)
			cmd := request.cmd.(*btcjson.SignRawTransactionWithKeyCmd)
			require.Equal(t, txHex, cmd.RawTx)
			require.Equal(t, keys, cmd.PrivKeys)
			require.Equal(t, tc.wantInputs, cmd.Inputs)
			require.Equal(t, tc.wantHashType, cmd.SigHashType)

			request.responseChan <- &Response{result: []byte(
				`{"hex":"` + txHex + `","complete":true}`,
			)}
			signed, complete, err := future.Receive()
			require.NoError(t, err)
			require.True(t, complete)
			require.Equal(t, tx.TxHash(), signed.TxHash())
		})
	}
}
