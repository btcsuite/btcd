package txscript

import (
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestP2ASpending tests that pay-to-anchor outputs can be spent correctly.
func TestP2ASpending(t *testing.T) {
	tests := []struct {
		name         string
		scriptPubKey []byte
		witness      wire.TxWitness
		sigScript    []byte
		shouldPass   bool
		errCode      ErrorCode
	}{
		{
			name: "valid P2A spend with empty witness and " +
				"sigscript",
			scriptPubKey: PayToAnchorScript,
			witness:      wire.TxWitness{},
			sigScript:    []byte{},
			shouldPass:   true,
		},
		{
			name: "P2A with non-empty witness should " +
				"succeed",
			scriptPubKey: PayToAnchorScript,
			witness:      wire.TxWitness{[]byte{0x01}},
			sigScript:    []byte{},
			shouldPass:   true,
		},
		{
			name:         "P2A with non-empty sigscript should fail",
			scriptPubKey: PayToAnchorScript,
			witness:      wire.TxWitness{},
			sigScript:    []byte{0x01, 0x02},
			shouldPass:   false,
			errCode:      ErrWitnessMalleated,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a transaction with the P2A output being spent.
			prevTx := wire.NewMsgTx(2)
			prevTx.AddTxOut(&wire.TxOut{
				Value:    1000,
				PkScript: test.scriptPubKey,
			})
			prevTxHash := prevTx.TxHash()

			// Create the spending transaction.
			tx := wire.NewMsgTx(2)
			tx.AddTxIn(&wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Hash:  prevTxHash,
					Index: 0,
				},
				SignatureScript: test.sigScript,
				Witness:         test.witness,
			})
			tx.AddTxOut(&wire.TxOut{
				Value:    900,
				PkScript: []byte{OP_TRUE},
			})

			// Create the script engine.
			vm, err := NewEngine(
				test.scriptPubKey, tx, 0,
				StandardVerifyFlags, nil,
				nil, 1000, nil,
			)
			if err != nil {
				if test.errCode != 0 {
					require.True(t, IsErrorCode(err, test.errCode))
					return
				} else {
					require.NoError(t, err)
				}
			}

			if test.shouldPass {
				if err != nil {
					t.Fatalf("NewEngine failed "+
						"unexpectedly: %v", err)
				}

				err = vm.Execute()
				if err != nil {
					t.Fatalf("Execute failed unexpectedly: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("Expected NewEngine to fail, but it succeeded")
				}

				require.True(t, IsErrorCode(err, test.errCode))
			}
		})
	}
}
