package psbt

import (
	"bytes"
	"slices"
	"testing"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestSerializeDoesNotMutate asserts that serializing a packet has no side
// effect on it. Serialization sorts several input and output fields (partial
// sigs, BIP32 derivations, taproot data) to produce deterministic output; it
// must do so on copies, leaving the caller's packet untouched.
func TestSerializeDoesNotMutate(t *testing.T) {
	t.Parallel()

	// buildPacket returns a packet whose every sortable input and output
	// field is keyed by the given bytes, in the given order.
	buildPacket := func(order []byte) *Packet {
		tx := wire.NewMsgTx(2)
		tx.AddTxIn(&wire.TxIn{})
		tx.AddTxOut(&wire.TxOut{
			Value: 1000, PkScript: []byte{txscript.OP_1},
		})

		var (
			partialSigs []*PartialSig
			inBip32     []*Bip32Derivation
			inTapBip32  []*TaprootBip32Derivation
			outBip32    []*Bip32Derivation
		)
		for _, b := range order {
			compressed := bytes.Repeat([]byte{b}, 33)
			xOnly := bytes.Repeat([]byte{b}, 32)

			partialSigs = append(partialSigs, &PartialSig{
				PubKey: compressed, Signature: []byte{b},
			})
			inBip32 = append(inBip32, &Bip32Derivation{
				PubKey: compressed, Bip32Path: []uint32{0},
			})
			inTapBip32 = append(inTapBip32, &TaprootBip32Derivation{
				XOnlyPubKey: xOnly, Bip32Path: []uint32{0},
			})
			outBip32 = append(outBip32, &Bip32Derivation{
				PubKey: compressed, Bip32Path: []uint32{0},
			})
		}

		return &Packet{
			UnsignedTx: tx,
			Inputs: []PInput{{
				PartialSigs:            partialSigs,
				Bip32Derivation:        inBip32,
				TaprootBip32Derivation: inTapBip32,
			}},
			Outputs: []POutput{{Bip32Derivation: outBip32}},
		}
	}

	// The fields are deliberately in descending key order, the opposite of
	// the ascending order that serialization sorts them into.
	packet := buildPacket([]byte{0x03, 0x02, 0x01})

	// Snapshot the field orderings before serializing. slices.Clone copies
	// the slice headers into fresh backing arrays, so an in-place sort of
	// the packet's own slices would show up as a difference below.
	wantPartialSigs := slices.Clone(packet.Inputs[0].PartialSigs)
	wantInBip32 := slices.Clone(packet.Inputs[0].Bip32Derivation)
	wantInTapBip32 := slices.Clone(packet.Inputs[0].TaprootBip32Derivation)
	wantOutBip32 := slices.Clone(packet.Outputs[0].Bip32Derivation)

	var buf bytes.Buffer
	require.NoError(t, packet.Serialize(&buf))

	// Serialization must not have reordered any of the packet's fields.
	require.Equal(t, wantPartialSigs, packet.Inputs[0].PartialSigs)
	require.Equal(t, wantInBip32, packet.Inputs[0].Bip32Derivation)
	require.Equal(
		t, wantInTapBip32, packet.Inputs[0].TaprootBip32Derivation,
	)
	require.Equal(t, wantOutBip32, packet.Outputs[0].Bip32Derivation)

	// It must still produce deterministic, sorted output: the same data
	// already in ascending order must serialize to identical bytes.
	sorted := buildPacket([]byte{0x01, 0x02, 0x03})
	var sortedBuf bytes.Buffer
	require.NoError(t, sorted.Serialize(&sortedBuf))
	require.Equal(t, sortedBuf.Bytes(), buf.Bytes())
}
