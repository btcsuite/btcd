package psbt

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

func TestSumUtxoInputValues(t *testing.T) {
	// Expect sum to fail for packet with non-matching txIn and PInputs.
	tx := wire.NewMsgTx(2)
	badPacket, err := NewFromUnsignedTx(tx)
	if err != nil {
		t.Fatalf("could not create packet from TX: %v", err)
	}
	badPacket.Inputs = append(badPacket.Inputs, PInput{})

	_, err = SumUtxoInputValues(badPacket)
	if err == nil {
		t.Fatalf("expected sum of bad packet to fail")
	}

	// Expect sum to fail if any inputs don't have UTXO information added.
	op := []*wire.OutPoint{{}, {}}
	noUtxoInfoPacket, err := New(op, nil, 2, 0, []uint32{0, 0})
	if err != nil {
		t.Fatalf("could not create new packet: %v", err)
	}

	_, err = SumUtxoInputValues(noUtxoInfoPacket)
	if err == nil {
		t.Fatalf("expected sum of missing UTXO info to fail")
	}

	// Create a packet that is OK and contains both witness and non-witness
	// UTXO information.
	okPacket, err := New(op, nil, 2, 0, []uint32{0, 0})
	if err != nil {
		t.Fatalf("could not create new packet: %v", err)
	}
	okPacket.Inputs[0].WitnessUtxo = &wire.TxOut{Value: 1234}
	okPacket.Inputs[1].NonWitnessUtxo = &wire.MsgTx{
		TxOut: []*wire.TxOut{{Value: 6543}},
	}

	sum, err := SumUtxoInputValues(okPacket)
	if err != nil {
		t.Fatalf("could not sum input: %v", err)
	}
	if sum != (1234 + 6543) {
		t.Fatalf("unexpected sum, got %d wanted %d", sum, 1234+6543)
	}

	// Create a malformed packet where NonWitnessUtxo.TxOut does not
	// contain the index specified by the PreviousOutPoint in the
	// packet's Unsigned.TxIn field.
	badOp := []*wire.OutPoint{{}, {Index: 500}}
	malformedPacket, err := New(badOp, nil, 2, 0, []uint32{0, 0})
	if err != nil {
		t.Fatalf("could not create malformed packet: %v", err)
	}
	malformedPacket.Inputs[0].WitnessUtxo = &wire.TxOut{Value: 1234}
	malformedPacket.Inputs[1].NonWitnessUtxo = &wire.MsgTx{
		TxOut: []*wire.TxOut{{Value: 6543}},
	}

	_, err = SumUtxoInputValues(malformedPacket)
	if err == nil {
		t.Fatalf("expected sum of malformed packet to fail")
	}
}

func TestTxOutsEqual(t *testing.T) {
	testCases := []struct {
		name        string
		out1        *wire.TxOut
		out2        *wire.TxOut
		expectEqual bool
	}{{
		name:        "both nil",
		out1:        nil,
		out2:        nil,
		expectEqual: true,
	}, {
		name:        "one nil",
		out1:        nil,
		out2:        &wire.TxOut{},
		expectEqual: false,
	}, {
		name:        "both empty",
		out1:        &wire.TxOut{},
		out2:        &wire.TxOut{},
		expectEqual: true,
	}, {
		name: "one pk script set",
		out1: &wire.TxOut{},
		out2: &wire.TxOut{
			PkScript: []byte("foo"),
		},
		expectEqual: false,
	}, {
		name: "both fully set",
		out1: &wire.TxOut{
			Value:    1234,
			PkScript: []byte("bar"),
		},
		out2: &wire.TxOut{
			Value:    1234,
			PkScript: []byte("bar"),
		},
		expectEqual: true,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := TxOutsEqual(tc.out1, tc.out2)
			if result != tc.expectEqual {
				t.Fatalf("unexpected result, got %v wanted %v",
					result, tc.expectEqual)
			}
		})
	}
}

func TestVerifyOutputsEqual(t *testing.T) {
	testCases := []struct {
		name      string
		outs1     []*wire.TxOut
		outs2     []*wire.TxOut
		expectErr bool
	}{{
		name:      "both nil",
		outs1:     nil,
		outs2:     nil,
		expectErr: false,
	}, {
		name:      "one nil",
		outs1:     nil,
		outs2:     []*wire.TxOut{{}},
		expectErr: true,
	}, {
		name:      "both empty",
		outs1:     []*wire.TxOut{{}},
		outs2:     []*wire.TxOut{{}},
		expectErr: false,
	}, {
		name:  "one pk script set",
		outs1: []*wire.TxOut{{}},
		outs2: []*wire.TxOut{{
			PkScript: []byte("foo"),
		}},
		expectErr: true,
	}, {
		name: "both fully set",
		outs1: []*wire.TxOut{{
			Value:    1234,
			PkScript: []byte("bar"),
		}, {}},
		outs2: []*wire.TxOut{{
			Value:    1234,
			PkScript: []byte("bar"),
		}, {}},
		expectErr: false,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyOutputsEqual(tc.outs1, tc.outs2)
			if (tc.expectErr && err == nil) ||
				(!tc.expectErr && err != nil) {

				t.Fatalf("got error '%v' but wanted it to be "+
					"nil: %v", err, tc.expectErr)
			}
		})
	}
}

func TestVerifyInputPrevOutpointsEqual(t *testing.T) {
	testCases := []struct {
		name      string
		ins1      []*wire.TxIn
		ins2      []*wire.TxIn
		expectErr bool
	}{{
		name:      "both nil",
		ins1:      nil,
		ins2:      nil,
		expectErr: false,
	}, {
		name:      "one nil",
		ins1:      nil,
		ins2:      []*wire.TxIn{{}},
		expectErr: true,
	}, {
		name:      "both empty",
		ins1:      []*wire.TxIn{{}},
		ins2:      []*wire.TxIn{{}},
		expectErr: false,
	}, {
		name: "one previous output set",
		ins1: []*wire.TxIn{{}},
		ins2: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{11, 22, 33},
				Index: 7,
			},
		}},
		expectErr: true,
	}, {
		name: "both fully set",
		ins1: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{11, 22, 33},
				Index: 7,
			},
		}, {}},
		ins2: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{11, 22, 33},
				Index: 7,
			},
		}, {}},
		expectErr: false,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyInputPrevOutpointsEqual(tc.ins1, tc.ins2)
			if (tc.expectErr && err == nil) ||
				(!tc.expectErr && err != nil) {

				t.Fatalf("got error '%v' but wanted it to be "+
					"nil: %v", err, tc.expectErr)
			}
		})
	}
}

func TestVerifyInputOutputLen(t *testing.T) {
	testCases := []struct {
		name        string
		packet      *Packet
		needInputs  bool
		needOutputs bool
		expectErr   bool
	}{{
		name:      "packet nil",
		packet:    nil,
		expectErr: true,
	}, {
		name:      "wire tx nil",
		packet:    &Packet{},
		expectErr: true,
	}, {
		name: "both empty don't need outputs",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{},
		},
		expectErr: false,
	}, {
		name: "both empty but need outputs",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{},
		},
		needOutputs: true,
		expectErr:   true,
	}, {
		name: "both empty but need inputs",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{},
		},
		needInputs: true,
		expectErr:  true,
	}, {
		name: "input len mismatch",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{
				TxIn: []*wire.TxIn{{}},
			},
		},
		needInputs: true,
		expectErr:  true,
	}, {
		name: "output len mismatch",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{
				TxOut: []*wire.TxOut{{}},
			},
		},
		needOutputs: true,
		expectErr:   true,
	}, {
		name: "all fully set",
		packet: &Packet{
			UnsignedTx: &wire.MsgTx{
				TxIn:  []*wire.TxIn{{}},
				TxOut: []*wire.TxOut{{}},
			},
			Inputs:  []PInput{{}},
			Outputs: []POutput{{}},
		},
		needInputs:  true,
		needOutputs: true,
		expectErr:   false,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyInputOutputLen(
				tc.packet, tc.needInputs, tc.needOutputs,
			)
			if (tc.expectErr && err == nil) ||
				(!tc.expectErr && err != nil) {

				t.Fatalf("got error '%v' but wanted it to be "+
					"nil: %v", err, tc.expectErr)
			}
		})
	}
}

func TestNewFromSignedTx(t *testing.T) {
	orig := &wire.MsgTx{
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{},
			SignatureScript:  []byte("script"),
			Witness:          [][]byte{[]byte("witness")},
			Sequence:         1234,
		}},
		TxOut: []*wire.TxOut{{
			PkScript: []byte{77, 88},
			Value:    99,
		}},
	}

	packet, scripts, witnesses, err := NewFromSignedTx(orig)
	if err != nil {
		t.Fatalf("could not create packet from signed TX: %v", err)
	}

	tx := packet.UnsignedTx
	expectedTxIn := []*wire.TxIn{{
		PreviousOutPoint: wire.OutPoint{},
		Sequence:         1234,
	}}
	if !reflect.DeepEqual(tx.TxIn, expectedTxIn) {
		t.Fatalf("unexpected txin, got %#v wanted %#v",
			tx.TxIn, expectedTxIn)
	}
	if !reflect.DeepEqual(tx.TxOut, orig.TxOut) {
		t.Fatalf("unexpected txout, got %#v wanted %#v",
			tx.TxOut, orig.TxOut)
	}
	if len(scripts) != 1 || !bytes.Equal(scripts[0], []byte("script")) {
		t.Fatalf("script not extracted correctly")
	}
	if len(witnesses) != 1 ||
		!bytes.Equal(witnesses[0][0], []byte("witness")) {

		t.Fatalf("witness not extracted correctly")
	}
}
