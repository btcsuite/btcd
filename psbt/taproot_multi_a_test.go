package psbt

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

func TestTaprootMultiAFinalizerOrdersSignatures(t *testing.T) {
	keyA := bytes.Repeat([]byte{0x02}, 32)
	keyB := bytes.Repeat([]byte{0x03}, 32)
	sigA := bytes.Repeat([]byte{0xaa}, 64)
	sigB := bytes.Repeat([]byte{0xbb}, 64)

	script, err := txscript.NewScriptBuilder().
		AddData(keyA).AddOp(txscript.OP_CHECKSIG).
		AddData(keyB).AddOp(txscript.OP_CHECKSIGADD).
		AddInt64(2).AddOp(txscript.OP_NUMEQUAL).Script()
	require.NoError(t, err)

	testCases := []struct {
		name string
		sigs []*TaprootScriptSpendSig
	}{
		{
			name: "script order",
			sigs: []*TaprootScriptSpendSig{
				{XOnlyPubKey: keyA, Signature: sigA},
				{XOnlyPubKey: keyB, Signature: sigB},
			},
		},
		{
			name: "reverse script order",
			sigs: []*TaprootScriptSpendSig{
				{XOnlyPubKey: keyB, Signature: sigB},
				{XOnlyPubKey: keyA, Signature: sigA},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			packet := taprootMultiATestPacket(
				t, script, testCase.sigs,
			)

			require.NoError(t, MaybeFinalizeAll(packet))
			finalTx, err := Extract(packet)
			require.NoError(t, err)
			require.Equal(t, wire.TxWitness{
				sigB,
				sigA,
				script,
				make([]byte, 33),
			}, finalTx.TxIn[0].Witness)
		})
	}
}

func TestTaprootMultiAFinalizerAddsPlaceholders(t *testing.T) {
	keyA := bytes.Repeat([]byte{0x02}, 32)
	keyB := bytes.Repeat([]byte{0x03}, 32)
	keyC := bytes.Repeat([]byte{0x04}, 32)
	sigA := bytes.Repeat([]byte{0xaa}, 64)
	sigC := bytes.Repeat([]byte{0xcc}, 64)

	script, err := txscript.NewScriptBuilder().
		AddData(keyA).AddOp(txscript.OP_CHECKSIG).
		AddData(keyB).AddOp(txscript.OP_CHECKSIGADD).
		AddData(keyC).AddOp(txscript.OP_CHECKSIGADD).
		AddInt64(2).AddOp(txscript.OP_NUMEQUAL).Script()
	require.NoError(t, err)

	packet := taprootMultiATestPacket(t, script, []*TaprootScriptSpendSig{
		{XOnlyPubKey: keyA, Signature: sigA},
		{XOnlyPubKey: keyC, Signature: sigC},
	})

	require.NoError(t, MaybeFinalizeAll(packet))
	finalTx, err := Extract(packet)
	require.NoError(t, err)
	require.Equal(t, wire.TxWitness{
		sigC,
		[]byte{},
		sigA,
		script,
		make([]byte, 33),
	}, finalTx.TxIn[0].Witness)
}

func TestTaprootMultiAFinalizerIgnoresExcessSignatures(t *testing.T) {
	keyA := bytes.Repeat([]byte{0x02}, 32)
	keyB := bytes.Repeat([]byte{0x03}, 32)
	keyC := bytes.Repeat([]byte{0x04}, 32)
	sigA := bytes.Repeat([]byte{0xaa}, 64)
	sigB := bytes.Repeat([]byte{0xbb}, 64)
	sigC := bytes.Repeat([]byte{0xcc}, 64)

	script, err := txscript.NewScriptBuilder().
		AddData(keyA).AddOp(txscript.OP_CHECKSIG).
		AddData(keyB).AddOp(txscript.OP_CHECKSIGADD).
		AddData(keyC).AddOp(txscript.OP_CHECKSIGADD).
		AddInt64(2).AddOp(txscript.OP_NUMEQUAL).Script()
	require.NoError(t, err)

	packet := taprootMultiATestPacket(t, script, []*TaprootScriptSpendSig{
		{XOnlyPubKey: keyA, Signature: sigA},
		{XOnlyPubKey: keyB, Signature: sigB},
		{XOnlyPubKey: keyC, Signature: sigC},
	})

	require.NoError(t, MaybeFinalizeAll(packet))
	finalTx, err := Extract(packet)
	require.NoError(t, err)
	require.Equal(t, wire.TxWitness{
		[]byte{},
		sigB,
		sigA,
		script,
		make([]byte, 33),
	}, finalTx.TxIn[0].Witness)
}

func TestTaprootMultiAFinalizerRejectsInsufficientSignatures(t *testing.T) {
	keyA := bytes.Repeat([]byte{0x02}, 32)
	keyB := bytes.Repeat([]byte{0x03}, 32)
	keyC := bytes.Repeat([]byte{0x04}, 32)
	sigA := bytes.Repeat([]byte{0xaa}, 64)

	script, err := txscript.NewScriptBuilder().
		AddData(keyA).AddOp(txscript.OP_CHECKSIG).
		AddData(keyB).AddOp(txscript.OP_CHECKSIGADD).
		AddData(keyC).AddOp(txscript.OP_CHECKSIGADD).
		AddInt64(2).AddOp(txscript.OP_NUMEQUAL).Script()
	require.NoError(t, err)

	packet := taprootMultiATestPacket(t, script, []*TaprootScriptSpendSig{
		{XOnlyPubKey: keyA, Signature: sigA},
	})

	_, err = MaybeFinalize(packet, 0)
	require.ErrorIs(t, err, ErrNotFinalizable)
}

func taprootMultiATestPacket(t *testing.T, script []byte,
	sigs []*TaprootScriptSpendSig) *Packet {

	t.Helper()

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{}, nil, nil))
	tx.AddTxOut(wire.NewTxOut(0, nil))

	packet, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)

	pkScript := append(
		[]byte{txscript.OP_1, txscript.OP_DATA_32}, make([]byte, 32)...,
	)
	packet.Inputs[0].WitnessUtxo = wire.NewTxOut(1, pkScript)

	leafHash := txscript.NewBaseTapLeaf(script).TapHash()
	for _, sig := range sigs {
		sig.LeafHash = append([]byte{}, leafHash[:]...)
	}

	packet.Inputs[0].TaprootLeafScript = []*TaprootTapLeafScript{{
		ControlBlock: make([]byte, 33),
		Script:       script,
		LeafVersion:  txscript.BaseLeafVersion,
	}}
	packet.Inputs[0].TaprootScriptSpendSig = sigs

	return packet
}
