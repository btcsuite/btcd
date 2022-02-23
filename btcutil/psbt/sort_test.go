package psbt

import (
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

func TestInPlaceSort(t *testing.T) {
	testCases := []struct {
		name          string
		packet        *Packet
		expectedTxIn  []*wire.TxIn
		expectedTxOut []*wire.TxOut
		expectedPIn   []PInput
		expectedPOut  []POutput
		expectErr     bool
	}{{
		name:      "packet nil",
		packet:    nil,
		expectErr: true,
	}, {
		name:      "no inputs or outputs",
		packet:    &Packet{UnsignedTx: &wire.MsgTx{}},
		expectErr: false,
	}, {
		name: "inputs only",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{99, 88},
						Index: 7,
					},
				}, {
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{77, 88},
						Index: 12,
					},
				}, {
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{77, 88},
						Index: 7,
					},
				}},
			},
			// Abuse the SighashType as an index to make sure the
			// partial inputs are also sorted together with the wire
			// inputs.
			Inputs: []PInput{{
				SighashType: 0,
			}, {
				SighashType: 1,
			}, {
				SighashType: 2,
			}},
		},
		expectedTxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{77, 88},
				Index: 7,
			},
		}, {
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{77, 88},
				Index: 12,
			},
		}, {
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{99, 88},
				Index: 7,
			},
		}},
		expectedPIn: []PInput{{
			SighashType: 2,
		}, {
			SighashType: 1,
		}, {
			SighashType: 0,
		}},
		expectErr: false,
	}, {
		name: "outputs only",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{
				TxOut: []*wire.TxOut{{
					PkScript: []byte{99, 88},
					Value:    7,
				}, {
					PkScript: []byte{77, 88},
					Value:    12,
				}, {
					PkScript: []byte{77, 88},
					Value:    7,
				}},
			},
			// Abuse the RedeemScript as an index to make sure the
			// partial inputs are also sorted together with the wire
			// inputs.
			Outputs: []POutput{{
				RedeemScript: []byte{0},
			}, {
				RedeemScript: []byte{1},
			}, {
				RedeemScript: []byte{2},
			}},
		},
		expectedTxOut: []*wire.TxOut{{
			PkScript: []byte{77, 88},
			Value:    7,
		}, {
			PkScript: []byte{99, 88},
			Value:    7,
		}, {
			PkScript: []byte{77, 88},
			Value:    12,
		}},
		expectedPOut: []POutput{{
			RedeemScript: []byte{2},
		}, {
			RedeemScript: []byte{0},
		}, {
			RedeemScript: []byte{1},
		}},
		expectErr: false,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			p := tc.packet
			err := InPlaceSort(p)
			if (tc.expectErr && err == nil) ||
				(!tc.expectErr && err != nil) {

				t.Fatalf("got error '%v' but wanted it to be "+
					"nil: %v", err, tc.expectErr)
			}

			// Don't continue on this special test case.
			if p == nil {
				return
			}

			tx := p.UnsignedTx
			if !reflect.DeepEqual(tx.TxIn, tc.expectedTxIn) {
				t.Fatalf("unexpected txin, got %#v wanted %#v",
					tx.TxIn, tc.expectedTxIn)
			}
			if !reflect.DeepEqual(tx.TxOut, tc.expectedTxOut) {
				t.Fatalf("unexpected txout, got %#v wanted %#v",
					tx.TxOut, tc.expectedTxOut)
			}

			if !reflect.DeepEqual(p.Inputs, tc.expectedPIn) {
				t.Fatalf("unexpected pin, got %#v wanted %#v",
					p.Inputs, tc.expectedPIn)
			}
			if !reflect.DeepEqual(p.Outputs, tc.expectedPOut) {
				t.Fatalf("unexpected pout, got %#v wanted %#v",
					p.Inputs, tc.expectedPOut)
			}
		})
	}
}
