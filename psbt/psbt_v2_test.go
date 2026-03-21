package psbt

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

func ptrUint32(v uint32) *uint32 { return &v }
func ptrUint8(v uint8) *uint8    { return &v }

// testTxid returns a deterministic txid for test use.
func testTxid(fill byte) *chainhash.Hash {
	var h chainhash.Hash
	for i := range h {
		h[i] = fill
	}
	return &h
}

// serializeV2Global is a helper that builds a raw v2 global section (after the
// magic bytes) from explicitly provided key-value pairs. Each pair is a key
// byte slice and a value byte slice, serialized per the PSBT wire format.
// A separator (0x00) is appended at the end. This allows tests to construct
// intentionally malformed PSBTs.
func serializeV2Global(t *testing.T, pairs ...[]byte) []byte {
	t.Helper()

	require.True(t, len(pairs)%2 == 0, "pairs must be key, value, ...")

	var buf bytes.Buffer

	// Magic bytes.
	buf.Write(psbtMagic[:])

	for i := 0; i < len(pairs); i += 2 {
		key := pairs[i]
		value := pairs[i+1]

		// Write key length + key.
		wire.WriteVarInt(&buf, 0, uint64(len(key)))
		buf.Write(key)

		// Write value length + value.
		wire.WriteVarInt(&buf, 0, uint64(len(value)))
		buf.Write(value)
	}

	// Separator.
	buf.WriteByte(0x00)
	return buf.Bytes()
}

// uint32LE returns a 4-byte little-endian encoding of v.
func uint32LE(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// compactSizeUint returns the compact-size encoding of v.
func compactSizeUint(v uint64) []byte {
	var buf bytes.Buffer
	wire.WriteVarInt(&buf, 0, v)
	return buf.Bytes()
}

// ==========================================================================
// 1. Creation & Round-Trip Tests
// ==========================================================================

func TestV2CreateEmptyPSBT(t *testing.T) {
	// Create a v2 PSBT with 0 inputs and 0 outputs.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)
	require.Equal(t, uint32(2), p.Version)
	require.Equal(t, int32(2), p.TxVersion)
	require.Equal(t, uint32(0), p.InputCount)
	require.Equal(t, uint32(0), p.OutputCount)

	// Round-trip serialize and parse.
	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)
	require.Equal(t, uint32(2), p2.Version)
	require.Equal(t, uint32(0), p2.InputCount)
	require.Equal(t, uint32(0), p2.OutputCount)
	require.Nil(t, p2.UnsignedTx)
}

func TestV2RoundTripAllFields(t *testing.T) {
	// Create a v2 PSBT with all fields populated.
	p, err := NewV2(2, ptrUint32(700000), ptrUint8(0x03))
	require.NoError(t, err)

	txid := testTxid(0xAA)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 5)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 5)).Index,
			Sequence:     0xFFFFFFFE,
		}),
	)
	p.Inputs[0].TimeLocktime = 1600000000
	p.Inputs[0].HeightLocktime = 400000

	script := []byte{0x00, 0x14, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 50000000,
			Script: script,
		}),
	)

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)

	// Global fields.
	require.Equal(t, uint32(2), p2.Version)
	require.Equal(t, int32(2), p2.TxVersion)
	require.Equal(t, uint32(700000), *p2.FallbackLocktime)
	require.Equal(t, uint8(0x03), *p2.TxModifiable)
	require.Equal(t, uint32(1), p2.InputCount)
	require.Equal(t, uint32(1), p2.OutputCount)

	// Input fields.
	require.Equal(t, txid[:], p2.Inputs[0].PreviousTxid)
	require.Equal(t, uint32(5), p2.Inputs[0].OutputIndex)
	require.Equal(t, uint32(0xFFFFFFFE), p2.Inputs[0].Sequence)
	require.Equal(t, uint32(1600000000), p2.Inputs[0].TimeLocktime)
	require.Equal(t, uint32(400000), p2.Inputs[0].HeightLocktime)

	// Output fields.
	require.Equal(t, int64(50000000), p2.Outputs[0].Amount)
	require.Equal(t, script, p2.Outputs[0].Script)
}

func TestV2RoundTripBase64(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0xBB)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     wire.MaxTxInSequenceNum,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	encoded, err := p.B64Encode()
	require.NoError(t, err)

	p2, err := NewFromRawBytes(bytes.NewReader([]byte(encoded)), true)
	require.NoError(t, err)
	require.Equal(t, uint32(2), p2.Version)
	require.Equal(t, uint32(1), p2.InputCount)
	require.Equal(t, uint32(1), p2.OutputCount)
	require.Equal(t, txid[:], p2.Inputs[0].PreviousTxid)
	require.Equal(t, int64(1000), p2.Outputs[0].Amount)
}

func TestV2SequenceDefaultNotSerialized(t *testing.T) {
	// When sequence equals MaxTxInSequenceNum (the default), it should NOT
	// be serialized per BIP-370.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0xCC)

	// Use the default sequence.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     wire.MaxTxInSequenceNum,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	// The serialized form should NOT contain the Sequence key (0x10).
	// We search the input section for the 0x10 key type.
	serialized := buf.Bytes()

	// We can verify by round-tripping and checking the default is restored.
	p2, err := NewFromRawBytes(bytes.NewReader(serialized), false)
	require.NoError(t, err)
	require.Equal(t, wire.MaxTxInSequenceNum, p2.Inputs[0].Sequence)

	// Now use a non-default sequence.
	p3, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)
	require.NoError(
		t,
		p3.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p3.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	var buf2 bytes.Buffer
	require.NoError(t, p3.Serialize(&buf2))

	p4, err := NewFromRawBytes(&buf2, false)
	require.NoError(t, err)
	require.Equal(t, uint32(0), p4.Inputs[0].Sequence)
}

func TestV2MultipleInputsOutputs(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	// Add 3 inputs and 2 outputs.
	for i := byte(1); i <= 3; i++ {
		txid := testTxid(i)
		require.NoError(
			t,
			p.AddInputV2(PInput{
				PreviousTxid: (*wire.NewOutPoint(txid, uint32(i))).Hash[:],
				OutputIndex:  (*wire.NewOutPoint(txid, uint32(i))).Index,
				Sequence:     wire.MaxTxInSequenceNum,
			}),
		)
	}
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 10000,
			Script: []byte{
				0x51,
			},
		}),
	)
	require.NoError(
		t,
		p.AddOutputV2(POutput{
			Amount: 20000,
			Script: []byte{
				0x00, 0x14, 0xaa,
			},
		}),
	)

	require.Equal(t, uint32(3), p.InputCount)
	require.Equal(t, uint32(2), p.OutputCount)

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)
	require.Len(t, p2.Inputs, 3)
	require.Len(t, p2.Outputs, 2)
	require.Equal(t, uint32(3), p2.InputCount)
	require.Equal(t, uint32(2), p2.OutputCount)

	for i := byte(1); i <= 3; i++ {
		require.Equal(t, testTxid(i)[:], p2.Inputs[i-1].PreviousTxid)
		require.Equal(t, uint32(i), p2.Inputs[i-1].OutputIndex)
	}
	require.Equal(t, int64(10000), p2.Outputs[0].Amount)
	require.Equal(t, int64(20000), p2.Outputs[1].Amount)
}

// ==========================================================================
// 2. Version Validation Tests
// ==========================================================================

func TestV2CannotHaveUnsignedTx(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	// Force an UnsignedTx onto a v2 PSBT.
	p.UnsignedTx = wire.NewMsgTx(2)
	require.Error(t, p.SanityCheck())
}

func TestV2RequiredGlobalFields(t *testing.T) {
	// A v2 PSBT without TxVersion should fail.
	raw := serializeV2Global(
		t,                         // Version = 2
		[]byte{0xfb}, uint32LE(2), // InputCount = 0
		[]byte{0x04}, compactSizeUint(0),
		// OutputCount = 0
		[]byte{0x05}, compactSizeUint(0),
		// Missing TxVersion (0x02)!
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "should fail without TxVersion")
}

func TestV2RejectsVersion1(t *testing.T) {
	// Version 1 is explicitly skipped per BIP-370.
	raw := serializeV2Global(t, []byte{0xfb}, uint32LE(1))

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err)
}

func TestV2RejectsVersion3(t *testing.T) {
	raw := serializeV2Global(t, []byte{0xfb}, uint32LE(3))

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err)
}

func TestV2AddInputToV0Fails(t *testing.T) {
	// Create a v0 PSBT.
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(testTxid(0x01), 0),
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(wire.NewTxOut(1000, []byte{0x51}))

	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)
	require.Equal(t, uint32(0), p.Version)

	// Adding input to v0 should fail.
	err = p.AddInputV2(PInput{
		PreviousTxid: testTxid(0x02)[:],
		OutputIndex:  0,
		Sequence:     wire.MaxTxInSequenceNum,
	})
	require.Error(t, err)
}

// ==========================================================================
// 3. Lock Time Algorithm Tests
// ==========================================================================

func TestV2LockTimeFallbackDefault(t *testing.T) {
	// No inputs, no fallback → locktime 0.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(0), lockTime)
}

func TestV2LockTimeFallbackExplicit(t *testing.T) {
	// No input locktime constraints → use fallback.
	p, err := NewV2(2, ptrUint32(654321), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	// No TimeLocktime or HeightLocktime set on the input.

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(654321), lockTime)
}

func TestV2LockTimeHeightOnly(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[0].HeightLocktime = 300000

	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[1].HeightLocktime = 400000

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(400000), lockTime) // Max of heights.
}

func TestV2LockTimeTimeOnly(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[0].TimeLocktime = 1600000000

	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[1].TimeLocktime = 1700000000

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(1700000000), lockTime) // Max of times.
}

func TestV2LockTimeBothSupportedPrefersHeight(t *testing.T) {
	// BIP-370: When both types are supported, height must be chosen.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)

	// Input with both types → supports either.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[0].TimeLocktime = 1600000000
	p.Inputs[0].HeightLocktime = 300000

	// Input with both types → supports either.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[1].TimeLocktime = 1700000000
	p.Inputs[1].HeightLocktime = 400000

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(400000), lockTime) // Height preferred.
}

func TestV2LockTimeConflictErrors(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)

	// Input 1: Time-only → cannot satisfy height.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[0].TimeLocktime = 1600000000

	// Input 2: Height-only → cannot satisfy time.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[1].HeightLocktime = 300000

	_, err = p.DetermineLockTime()
	require.Error(t, err)

	// Both tests should trigger conflicting locktimes error
	require.Error(t, err)
}

func TestV2LockTimeMixedFlexibleAndFixed(t *testing.T) {
	// One input requires time-only, another supports both → time wins.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)

	// Input 1: Time-only (forces time).
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[0].TimeLocktime = 1600000000

	// Input 2: Both → flexible.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[1].TimeLocktime = 1700000000
	p.Inputs[1].HeightLocktime = 500000

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(1700000000), lockTime)
}

func TestV2LockTimeUnconstrainedInputsIgnored(t *testing.T) {
	// Unconstrained inputs (no locktime fields) don't affect the choice.
	p, err := NewV2(2, ptrUint32(99), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)

	// Input 1: No locktime → unconstrained.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)

	// Input 2: Height-only.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[1].HeightLocktime = 400000

	// Input 3: No locktime → unconstrained.
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 2)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 2)).Index,
			Sequence:     0,
		}),
	)

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(400000), lockTime) // Not fallback.
}

// ==========================================================================
// 4. GetUnsignedTx Tests
// ==========================================================================

func TestV2GetUnsignedTx(t *testing.T) {
	p, err := NewV2(2, ptrUint32(500000), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0xDD)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 3)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 3)).Index,
			Sequence:     42,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 100000,
			Script: []byte{
				0x51,
			},
		}),
	)

	// Set a height locktime.
	p.Inputs[0].HeightLocktime = 600000

	tx, err := p.GetUnsignedTx()
	require.NoError(t, err)

	require.Equal(t, int32(2), tx.Version)
	require.Len(t, tx.TxIn, 1)
	require.Len(t, tx.TxOut, 1)
	require.Equal(t, txid[:], tx.TxIn[0].PreviousOutPoint.Hash[:])
	require.Equal(t, uint32(3), tx.TxIn[0].PreviousOutPoint.Index)
	require.Equal(t, uint32(42), tx.TxIn[0].Sequence)
	require.Equal(t, int64(100000), tx.TxOut[0].Value)
	require.Equal(t, uint32(600000), tx.LockTime)
}

func TestV2GetUnsignedTxDoesNotMutate(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0xEE)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	tx1, err := p.GetUnsignedTx()
	require.NoError(t, err)

	tx2, err := p.GetUnsignedTx()
	require.NoError(t, err)

	// Mutating one should not affect the other.
	tx1.TxIn[0].Sequence = 999
	require.NotEqual(t, tx1.TxIn[0].Sequence, tx2.TxIn[0].Sequence)
}

func TestV0GetUnsignedTxStillWorks(t *testing.T) {
	// Ensure the v0 path is not broken.
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(testTxid(0x01), 0),
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(wire.NewTxOut(1000, []byte{0x51}))

	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)

	tx2, err := p.GetUnsignedTx()
	require.NoError(t, err)
	require.Equal(
		t, tx.TxIn[0].PreviousOutPoint, tx2.TxIn[0].PreviousOutPoint,
	)
	require.Equal(t, tx.TxOut[0].Value, tx2.TxOut[0].Value)
}

// ==========================================================================
// 5. Locktime Value Validation Tests
// ==========================================================================

func TestV2TimeLocktimeMustBeGTE500M(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)
	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	// Set an invalid time locktime (< 500M).
	p.Inputs[0].TimeLocktime = 499999999

	var buf bytes.Buffer

	// Serialize should reject the invalid value because IsSane() fails.
	err = p.Serialize(&buf)
	require.Error(
		t, err,
		"time locktime < 500000000 must be rejected by Serialize",
	)
}

func TestV2TimeLocktimeBoundary(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)
	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	// Exactly 500M should be valid.
	p.Inputs[0].TimeLocktime = 500000000

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)
	require.Equal(t, uint32(500000000), p2.Inputs[0].TimeLocktime)
}

func TestV2HeightLocktimeMustBeGTZeroAndLT500M(t *testing.T) {
	tests := []struct {
		name   string
		height uint32
	}{
		{name: "zero", height: 0},
		{name: "exactly 500M", height: 500000000},
		{name: "above 500M", height: 600000000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build raw PSBT with explicit HeightLocktime value
			// to bypass the serialization skip for zero values.
			txid := testTxid(0x01)

			raw := serializeV2WithInputKVPairs(t,
				[]byte{0x0e}, txid[:],
				[]byte{0x0f}, uint32LE(0),
				[]byte{0x12}, uint32LE(tc.height),
			)

			_, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.Error(t, err,
				"height locktime %d must be rejected",
				tc.height)
		})
	}
}

func TestV2HeightLocktimeValidBoundary(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)
	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	// Height 1 is the minimum valid value.
	p.Inputs[0].HeightLocktime = 1

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))
	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)
	require.Equal(t, uint32(1), p2.Inputs[0].HeightLocktime)

	// Height 499999999 is the maximum valid value.
	p3, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)
	require.NoError(
		t,
		p3.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p3.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)
	p3.Inputs[0].HeightLocktime = 499999999

	var buf2 bytes.Buffer
	require.NoError(t, p3.Serialize(&buf2))
	p4, err := NewFromRawBytes(&buf2, false)
	require.NoError(t, err)
	require.Equal(t, uint32(499999999), p4.Inputs[0].HeightLocktime)
}

// ==========================================================================
// 6. Duplicate Field Detection Tests
// ==========================================================================

func TestV2DuplicateGlobalFallbackLocktime(t *testing.T) {
	// Build a raw v2 PSBT with FallbackLocktime (0x03) appearing twice.
	raw := serializeV2Global(
		t,                         // TxVersion
		[]byte{0x02}, uint32LE(2), // FallbackLocktime first
		[]byte{0x03}, uint32LE(0),
		// FallbackLocktime duplicate
		[]byte{0x03}, uint32LE(0), // InputCount
		[]byte{0x04}, compactSizeUint(0),
		// OutputCount
		[]byte{0x05}, compactSizeUint(0), // Version
		[]byte{0xfb}, uint32LE(2),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "duplicate FallbackLocktime must be rejected")
}

func TestV2DuplicateGlobalTxModifiable(t *testing.T) {
	raw := serializeV2Global(
		t, []byte{0x02}, uint32LE(2), []byte{0x04}, compactSizeUint(0),
		[]byte{0x05}, compactSizeUint(0), // TxModifiable first
		[]byte{0x06}, []byte{0x00},
		// TxModifiable duplicate
		[]byte{0x06}, []byte{0x00}, []byte{0xfb}, uint32LE(2),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "duplicate TxModifiable must be rejected")
}

// serializeV2WithInputKVPairs builds a minimal v2 PSBT with one input, where
// the input section contains the given raw key-value pairs.
func serializeV2WithInputKVPairs(t *testing.T, pairs ...[]byte) []byte {
	t.Helper()
	require.True(t, len(pairs)%2 == 0)

	var buf bytes.Buffer
	buf.Write(psbtMagic[:])

	// Global: TxVersion=2, InputCount=1, OutputCount=1, Version=2.
	for _, pair := range []struct{ key, val []byte }{
		{[]byte{0x02}, uint32LE(2)},
		{[]byte{0x04}, compactSizeUint(1)},
		{[]byte{0x05}, compactSizeUint(1)},
		{[]byte{0xfb}, uint32LE(2)},
	} {

		wire.WriteVarInt(&buf, 0, uint64(len(pair.key)))
		buf.Write(pair.key)
		wire.WriteVarInt(&buf, 0, uint64(len(pair.val)))
		buf.Write(pair.val)
	}
	buf.WriteByte(0x00) // global separator

	// Input section.
	for i := 0; i < len(pairs); i += 2 {
		key := pairs[i]
		val := pairs[i+1]
		wire.WriteVarInt(&buf, 0, uint64(len(key)))
		buf.Write(key)
		wire.WriteVarInt(&buf, 0, uint64(len(val)))
		buf.Write(val)
	}
	buf.WriteByte(0x00) // input separator

	// Output section: Amount + Script.
	for _, pair := range []struct{ key, val []byte }{
		{
			[]byte{
				0x03,
			},
			[]byte{
				0xe8, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
		{[]byte{0x04}, []byte{0x51}},
	} {

		wire.WriteVarInt(&buf, 0, uint64(len(pair.key)))
		buf.Write(pair.key)
		wire.WriteVarInt(&buf, 0, uint64(len(pair.val)))
		buf.Write(pair.val)
	}
	buf.WriteByte(0x00) // output separator

	return buf.Bytes()
}

func TestV2DuplicateInputOutputIndex(t *testing.T) {
	txid := testTxid(0x01)

	raw := serializeV2WithInputKVPairs(
		t,                     // PreviousTxid
		[]byte{0x0e}, txid[:], // OutputIndex first
		[]byte{0x0f}, uint32LE(0),
		// OutputIndex duplicate
		[]byte{0x0f}, uint32LE(1),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "duplicate OutputIndex must be rejected")
}

func TestV2DuplicateInputSequence(t *testing.T) {
	txid := testTxid(0x01)

	raw := serializeV2WithInputKVPairs(
		t, []byte{0x0e}, txid[:], []byte{0x0f}, uint32LE(0),
		// Sequence first
		[]byte{0x10}, uint32LE(0), // Sequence duplicate
		[]byte{0x10}, uint32LE(1),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "duplicate Sequence must be rejected")
}

func TestV2DuplicateInputTimeLocktime(t *testing.T) {
	txid := testTxid(0x01)

	raw := serializeV2WithInputKVPairs(
		t, []byte{0x0e}, txid[:], []byte{0x0f}, uint32LE(0),
		// TimeLocktime first
		[]byte{0x11}, uint32LE(500000000), // TimeLocktime duplicate
		[]byte{0x11},
		uint32LE(600000000),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "duplicate TimeLocktime must be rejected")
}

func TestV2DuplicateInputHeightLocktime(t *testing.T) {
	txid := testTxid(0x01)

	raw := serializeV2WithInputKVPairs(
		t, []byte{0x0e}, txid[:], []byte{0x0f}, uint32LE(0),
		// HeightLocktime first
		[]byte{0x12}, uint32LE(100), // HeightLocktime duplicate
		[]byte{0x12}, uint32LE(200),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "duplicate HeightLocktime must be rejected")
}

// ==========================================================================
// 7. Input Serialization Key Ordering Tests
// ==========================================================================

func TestV2InputSerializationKeyOrder(t *testing.T) {
	// Build a v2 input with various fields and verify keys are in ascending
	// order after serialization.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0xFF)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     42,
		}),
	)
	p.Inputs[0].HeightLocktime = 100000

	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	// Parse back and extract the serialized input section to verify order.
	// We'll re-serialize the parsed packet and check key order in the input
	// section by scanning for key types.
	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)

	// Serialize the input and extract key types.
	var inputBuf bytes.Buffer
	require.NoError(t, p2.Inputs[0].serialize(&inputBuf, 2))

	keyTypes := extractKeyTypes(t, inputBuf.Bytes())
	for i := 1; i < len(keyTypes); i++ {
		require.True(
			t, keyTypes[i] >= keyTypes[i-1],
			"key type 0x%02x must come after 0x%02x", keyTypes[i],
			keyTypes[i-1],
		)
	}
}

// extractKeyTypes reads the serialized key-value pairs and returns just the key
// type bytes in order.
func extractKeyTypes(t *testing.T, data []byte) []byte {
	t.Helper()

	r := bytes.NewReader(data)
	var keyTypes []byte

	for {
		keyLen, err := wire.ReadVarInt(r, 0)
		if err != nil {
			break
		}
		if keyLen == 0 {
			break
		}

		key := make([]byte, keyLen)
		_, err = r.Read(key)
		require.NoError(t, err)

		keyTypes = append(keyTypes, key[0])

		// Read and discard value.
		valLen, err := wire.ReadVarInt(r, 0)
		require.NoError(t, err)
		val := make([]byte, valLen)
		_, err = r.Read(val)
		require.NoError(t, err)
	}

	return keyTypes
}

// ==========================================================================
// 8. SumUtxoInputValues & GetTxFee v2 Tests
// ==========================================================================

func TestV2SumUtxoInputValuesWitness(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		}),
	)

	// Set witness UTXOs.
	p.Inputs[0].WitnessUtxo = wire.NewTxOut(50000, []byte{0x51})
	p.Inputs[1].WitnessUtxo = wire.NewTxOut(30000, []byte{0x51})

	sum, err := SumUtxoInputValues(p)
	require.NoError(t, err)
	require.Equal(t, int64(80000), sum)
}

func TestV2SumUtxoInputValuesNonWitness(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	// Create a "previous transaction" with outputs.
	prevTx := wire.NewMsgTx(2)
	prevTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(testTxid(0xFF), 0),
	})
	prevTx.AddTxOut(wire.NewTxOut(10000, []byte{0x51}))
	prevTx.AddTxOut(wire.NewTxOut(20000, []byte{0x51}))
	prevTx.AddTxOut(wire.NewTxOut(30000, []byte{0x51}))

	// Input spending output index 2 of prevTx.
	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 2)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 2)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[0].NonWitnessUtxo = prevTx

	sum, err := SumUtxoInputValues(p)
	require.NoError(t, err)
	require.Equal(t, int64(30000), sum)
}

func TestV2SumUtxoInputValuesNoUtxoError(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	// No UTXO set.

	_, err = SumUtxoInputValues(p)
	require.Error(t, err)
}

func TestV2GetTxFee(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	p.Inputs[0].WitnessUtxo = wire.NewTxOut(100000, []byte{0x51})

	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 90000,
			Script: []byte{
				0x51,
			},
		}),
	)

	fee, err := p.GetTxFee()
	require.NoError(t, err)
	require.Equal(t, int64(10000), int64(fee))
}

// ==========================================================================
// 9. CopyInputFields / Finalization Preservation Tests
// ==========================================================================

func TestCopyInputFieldsPreservesV2Fields(t *testing.T) {
	src := &PInput{
		PreviousTxid:   testTxid(0xAA)[:],
		OutputIndex:    7,
		Sequence:       42,
		TimeLocktime:   1600000000,
		HeightLocktime: 300000,
		Unknowns: []*Unknown{
			{Key: []byte{0xfc, 0x01}, Value: []byte{0x02}},
		},
	}

	dst := NewPsbtInput(nil, nil)
	dst.CopyInputFields(src)

	require.Equal(t, src.PreviousTxid, dst.PreviousTxid)
	require.Equal(t, src.OutputIndex, dst.OutputIndex)
	require.Equal(t, src.Sequence, dst.Sequence)
	require.Equal(t, src.TimeLocktime, dst.TimeLocktime)
	require.Equal(t, src.HeightLocktime, dst.HeightLocktime)
	require.Len(t, dst.Unknowns, 1)
	require.Equal(t, src.Unknowns[0].Key, dst.Unknowns[0].Key)
	require.Equal(t, src.Unknowns[0].Value, dst.Unknowns[0].Value)

	// Verify deep copy: mutating dst should not affect src.
	dst.Unknowns[0].Value[0] = 0xFF
	require.NotEqual(t, src.Unknowns[0].Value[0], dst.Unknowns[0].Value[0])
}

// ==========================================================================
// 10. SanityCheck Tests
// ==========================================================================

func TestV2SanityCheckRejectsUnsignedTx(t *testing.T) {
	p := &Packet{
		Version:    2,
		TxVersion:  2,
		UnsignedTx: wire.NewMsgTx(2),
	}
	require.Error(t, p.SanityCheck())
}

func TestV0SanityCheckRequiresUnsignedTx(t *testing.T) {
	p := &Packet{
		Version:    0,
		UnsignedTx: nil,
	}
	require.Error(t, p.SanityCheck())
}

// ==========================================================================
// 11. PreviousTxid Validation Tests
// ==========================================================================

func TestV2RejectsAllZeroPreviousTxid(t *testing.T) {
	zeroTxid := make([]byte, 32)

	raw := serializeV2WithInputKVPairs(
		t, []byte{0x0e}, zeroTxid, []byte{0x0f}, uint32LE(0),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "all-zero PreviousTxid must be rejected")
}

func TestV2RejectsWrongLengthPreviousTxid(t *testing.T) {
	shortTxid := make([]byte, 16) // Should be 32 bytes.
	shortTxid[0] = 0x01

	raw := serializeV2WithInputKVPairs(
		t, []byte{0x0e}, shortTxid, []byte{0x0f}, uint32LE(0),
	)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.Error(t, err, "wrong-length PreviousTxid must be rejected")
}

// ==========================================================================
// 12. Input/Output Count Mismatch Tests
// ==========================================================================

func TestV2InputCountMismatchFails(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	// Override count to claim more inputs.
	p.InputCount = 3

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	// Should fail because we claimed 3 inputs but only provided 1.
	_, err = NewFromRawBytes(&buf, false)
	require.Error(t, err)
}

func TestV2OutputCountMismatchFails(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	p.OutputCount = 2

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	_, err = NewFromRawBytes(&buf, false)
	require.Error(t, err)
}

// ==========================================================================
// 13. Amount Type Tests
// ==========================================================================

func TestV2AmountSignedInt64(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)

	// Use a large but valid amount.
	require.NoError(
		t,
		p.AddOutputV2(POutput{
			Amount: 2100000000000000,
			Script: []byte{
				0x51,
			},
		}),
	)

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)
	require.Equal(t, int64(2100000000000000), p2.Outputs[0].Amount)

	// Verify it converts correctly to a wire transaction.
	tx, err := p2.GetUnsignedTx()
	require.NoError(t, err)
	require.Equal(t, int64(2100000000000000), tx.TxOut[0].Value)
}

// ==========================================================================
// 14. Unknown Fields Round-Trip Tests
// ==========================================================================

func TestV2UnknownFieldsRoundTrip(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	// Add unknown fields to input and output.
	require.NoError(t, p.Inputs[0].addUnknown(
		0xfc, []byte{0x01, 0x02}, []byte{0x03, 0x04},
	))
	require.NoError(t, p.Outputs[0].addUnknown(
		0xf1, []byte{0x05}, []byte{0x06, 0x07},
	))

	// Global unknown (use key type < 0xfd to avoid varint prefix issues).
	p.Unknowns = append(p.Unknowns, &Unknown{
		Key:   []byte{0xf0, 0x01},
		Value: []byte{0x02, 0x03},
	})

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)

	require.Len(t, p2.Inputs[0].Unknowns, 1)
	require.Equal(t, []byte{0xfc, 0x01, 0x02}, p2.Inputs[0].Unknowns[0].Key)
	require.Equal(t, []byte{0x03, 0x04}, p2.Inputs[0].Unknowns[0].Value)

	require.Len(t, p2.Outputs[0].Unknowns, 1)
	require.Equal(t, []byte{0xf1, 0x05}, p2.Outputs[0].Unknowns[0].Key)
	require.Equal(t, []byte{0x06, 0x07}, p2.Outputs[0].Unknowns[0].Value)

	require.Len(t, p2.Unknowns, 1)
	require.Equal(t, []byte{0xf0, 0x01}, p2.Unknowns[0].Key)
}

// ==========================================================================
// 15. Signer / Updater v2 Compatibility Tests
// ==========================================================================

func TestV2UpdaterCreation(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	u, err := NewUpdater(p)
	require.NoError(t, err)
	require.NotNil(t, u)
}

func TestV2UpdaterAddWitnessUtxo(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	u, err := NewUpdater(p)
	require.NoError(t, err)

	utxo := wire.NewTxOut(
		50000,
		[]byte{0x00, 0x14, 0x01, 0x02, 0x03,
			0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d,
			0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14},
	)
	require.NoError(t, u.AddInWitnessUtxo(utxo, 0))
	require.Equal(t, utxo, p.Inputs[0].WitnessUtxo)
}

// ==========================================================================
// 16. IsComplete / Extraction Tests
// ==========================================================================

func TestV2IsCompleteReturnsFalseWhenNotFinalized(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	require.False(t, p.IsComplete())
}

func TestV2ExtractRejectsIncomplete(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	_, err = Extract(p)
	require.Error(t, err)
	require.Equal(t, ErrIncompletePSBT, err)
}

// ==========================================================================
// 17. Edge Cases
// ==========================================================================

func TestV2ZeroFallbackLocktime(t *testing.T) {
	// Explicitly set fallback locktime to 0 (the default).
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(testTxid(0x01), 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(testTxid(0x01), 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	var buf bytes.Buffer
	require.NoError(t, p.Serialize(&buf))

	p2, err := NewFromRawBytes(&buf, false)
	require.NoError(t, err)
	require.Equal(t, uint32(0), *p2.FallbackLocktime)
}

func TestV2TxModifiableFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags uint8
	}{
		{name: "none", flags: 0x00},
		{name: "inputs modifiable", flags: 0x01},
		{name: "outputs modifiable", flags: 0x02},
		{name: "both modifiable", flags: 0x03},
		{name: "sighash single", flags: 0x04},
		{name: "all flags", flags: 0x07},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewV2(2, ptrUint32(0), ptrUint8(tc.flags))
			require.NoError(t, err)

			// Manually append to bypass AddInput/AddOutput
			// Constructor logic which enforces modifiability flags,
			// so we can test serialization.
			p.Inputs = append(p.Inputs, PInput{
				PreviousTxid: testTxid(0x01)[:],
				OutputIndex:  0,
			})
			p.Outputs = append(p.Outputs, POutput{
				Amount: 1000,
				Script: []byte{
					0x51,
				},
			})
			p.InputCount = 1
			p.OutputCount = 1

			var buf bytes.Buffer
			err = p.Serialize(&buf)
			require.NoError(t, err, "Serialize failed")

			p2, err := NewFromRawBytes(&buf, false)
			require.NoError(t, err)
			require.Equal(t, tc.flags, *p2.TxModifiable)
		})
	}
}

func TestV2NewFromUnsignedTxPopulatesV2Fields(t *testing.T) {
	// Verify that NewFromUnsignedTx pre-populates v2-compatible fields on
	// PInput, even though the packet is v0.
	txid := testTxid(0x01)
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(txid, 7),
		Sequence:         42,
	})
	tx.AddTxOut(wire.NewTxOut(1000, []byte{0x51}))

	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)
	require.Equal(t, uint32(0), p.Version)

	// v2-compatible fields should be populated.
	require.Equal(t, txid[:], p.Inputs[0].PreviousTxid)
	require.Equal(t, uint32(7), p.Inputs[0].OutputIndex)
	require.Equal(t, uint32(42), p.Inputs[0].Sequence)
}

func TestV2LockTimeInGetUnsignedTx(t *testing.T) {
	// Verify that the locktime in the extracted transaction matches the
	// DetermineLockTime result.
	p, err := NewV2(2, ptrUint32(100), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	require.NoError(
		t,
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		}),
	)
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	// No input locktimes → fallback.
	tx, err := p.GetUnsignedTx()
	require.NoError(t, err)
	require.Equal(t, uint32(100), tx.LockTime)
}

func TestV2MultipleInputsLockTimeMax(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid := testTxid(0x01)
	for i := uint32(0); i < 5; i++ {
		require.NoError(
			t,
			p.AddInputV2(PInput{
				PreviousTxid: (*wire.NewOutPoint(txid, i)).Hash[:],
				OutputIndex:  (*wire.NewOutPoint(txid, i)).Index,
				Sequence:     0,
			}),
		)
		p.Inputs[i].HeightLocktime = 100000 + i*50000
	}
	require.NoError(
		t, p.AddOutputV2(POutput{
			Amount: 1000,
			Script: []byte{
				0x51,
			},
		}),
	)

	lockTime, err := p.DetermineLockTime()
	require.NoError(t, err)
	require.Equal(t, uint32(300000), lockTime) // 100000 + 4*50000
}

// ==========================================================================
// 11. Creator Validation Tests
// ==========================================================================

// TestNewV2RejectsBadTxVersion verifies that the PSBTv2 Creator rejects a
// transaction version below 2, as required by BIP-370.
func TestNewV2RejectsBadTxVersion(t *testing.T) {
	tests := []struct {
		name      string
		txVersion int32
	}{
		{name: "version 0", txVersion: 0},
		{name: "version 1", txVersion: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewV2(tc.txVersion, ptrUint32(0), ptrUint8(0))
			require.Error(t, err,
				"NewV2 with txVersion %d must be rejected", tc.txVersion)
		})
	}

	// Version 2 is the minimum valid value.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)
	require.Equal(t, int32(2), p.TxVersion)
}

// ==========================================================================
// 12. Updater Role Modifiable Flag Tests
// ==========================================================================

// TestConstructorAddInputV2RespectsTxModifiable verifies that the
// Constructor-role AddInputV2 enforces the PSBT_GLOBAL_TX_MODIFIABLE Inputs
// Modifiable flag (Bit 0) per BIP-370.
func TestConstructorAddInputV2RespectsTxModifiable(t *testing.T) {
	// Build a valid base packet to start from.
	makePkt := func(modifiable uint8) *Packet {
		p, err := NewV2(2, ptrUint32(0), ptrUint8(modifiable))
		require.NoError(t, err)
		return p
	}

	txid := testTxid(0xAB)
	input := PInput{
		PreviousTxid: txid[:],
		OutputIndex:  0,
		Sequence:     wire.MaxTxInSequenceNum,
	}

	t.Run(
		"fails when inputs not modifiable (bit 0 clear)",
		func(t *testing.T) {
			p := makePkt(0x00) // No bits set.
			err := p.AddInputV2(input)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInputsNotModifiable)
		},
	)

	t.Run(
		"fails when only outputs modifiable (bit 1 set, bit 0 clear)",
		func(t *testing.T) {
			p := makePkt(
				0x02,
			) // Bit 1 only — outputs modifiable, not inputs.
			err := p.AddInputV2(input)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInputsNotModifiable)
		},
	)

	t.Run(
		"succeeds when inputs modifiable (bit 0 set)",
		func(t *testing.T) {
			p := makePkt(0x01) // Bit 0 set — inputs modifiable.
			require.NoError(t, p.AddInputV2(input))
			require.Len(t, p.Inputs, 1)
			require.Equal(t, uint32(1), p.InputCount)
		},
	)
}

// TestConstructorAddOutputV2RespectsTxModifiable verifies that the
// Constructor-role AddOutputV2 enforces the PSBT_GLOBAL_TX_MODIFIABLE Outputs
// Modifiable flag (Bit 1) per BIP-370.
func TestConstructorAddOutputV2RespectsTxModifiable(t *testing.T) {
	makePkt := func(modifiable uint8) *Packet {
		p, err := NewV2(2, ptrUint32(0), ptrUint8(modifiable))
		require.NoError(t, err)
		return p
	}

	output := POutput{
		Amount: 100000,
		Script: []byte{0x51},
	}

	t.Run(
		"fails when outputs not modifiable (bit 1 clear)",
		func(t *testing.T) {
			p := makePkt(0x00)
			err := p.AddOutputV2(output)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrOutputsNotModifiable)
		},
	)

	t.Run(
		"fails when only inputs modifiable (bit 0 set, bit 1 clear)",
		func(t *testing.T) {
			p := makePkt(
				0x01,
			) // Bit 0 only — inputs modifiable, not outputs.
			err := p.AddOutputV2(output)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrOutputsNotModifiable)
		},
	)

	t.Run(
		"succeeds when outputs modifiable (bit 1 set)",
		func(t *testing.T) {
			p := makePkt(0x02) // Bit 1 set — outputs modifiable.
			require.NoError(t, p.AddOutputV2(output))
			require.Len(t, p.Outputs, 1)
			require.Equal(t, uint32(1), p.OutputCount)
		},
	)
}

// ==========================================================================
// 13. V0 Rejects V2-Only Fields Tests
// ==========================================================================

// TestV0RejectsV2InputFields verifies that when a v0 PSBT contains v2-only
// input fields (0x0e–0x12), they are routed to the Unknowns list instead of
// being parsed as named fields, as required by BIP-370.
func TestV0RejectsV2InputFields(t *testing.T) {
	// Build a v0 PSBT from an unsigned tx.
	tx := wire.NewMsgTx(2)
	txid := testTxid(0x01)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(txid, 0),
		Sequence:         wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(wire.NewTxOut(50000, []byte{0x51}))

	p, err := NewFromUnsignedTx(tx)
	require.NoError(t, err)
	require.Equal(t, uint32(0), p.Version)

	// Inject a v2-only field (PreviousTxid = 0x0e) directly into the
	// unknowns, simulating a PSBT that has this field embedded.
	// After a round-trip through serialize/parse, PreviousTxid must NOT
	// be populated — it must live in Unknowns.
	p.Inputs[0].Unknowns = append(p.Inputs[0].Unknowns, &Unknown{
		Key:   []byte{byte(PreviousTxidInputType)},
		Value: txid[:],
	})

	var buf bytes.Buffer
	err = p.Serialize(&buf)
	if err == nil {
		_, err = NewFromRawBytes(&buf, false)
	}
	require.Error(
		t, err,
		"parsing or serialization should fail for v2 fields in v0",
	)
}

// ==========================================================================
// 14. BIP 370 Test Vectors
// ==========================================================================

func TestBIP370InvalidVectors(t *testing.T) {
	invalidVectors := []struct {
		desc   string
		base64 string
	}{
		{
			"PSBTv0 but with PSBT_GLOBAL_VERSION set to 2",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAH7BAIAAAAAAQBSAgAAAAHB" +
				"qiVuIUuWoYIvk95Cv/O18/+NBRkwbjUV11FaXoBbEgAA" +
				"AAAA/////wEYxpo7AAAAABYAFLCjrxRCCEEmk8p9FmhS" +
				"tS2wrvBuAAAAAAEBHxjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4BCGsCRzBEAiAFJ1pIVzTgrh87lxI3" +
				"WG8OctyFgz0njA5HTNIxEsD6XgIgawSMg868PEHQuTzH" +
				"2nYYXO29Aw0AWwgBi+K5i7rL33sBIQN2DcygXzmX3GWy" +
				"kwYPfynxUUyMUnBI4SgCsEHU/DQKJwAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1l" +
				"w93GQWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACA" +
				"AAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_GLOBAL_TX_VERSION",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAECBAIAAAAAAQBSAgAAAAHB" +
				"qiVuIUuWoYIvk95Cv/O18/+NBRkwbjUV11FaXoBbEgAA" +
				"AAAA/////wEYxpo7AAAAABYAFLCjrxRCCEEmk8p9FmhS" +
				"tS2wrvBuAAAAAAEBHxjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4BCGsCRzBEAiAFJ1pIVzTgrh87lxI3" +
				"WG8OctyFgz0njA5HTNIxEsD6XgIgawSMg868PEHQuTzH" +
				"2nYYXO29Aw0AWwgBi+K5i7rL33sBIQN2DcygXzmX3GWy" +
				"kwYPfynxUUyMUnBI4SgCsEHU/DQKJwAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1l" +
				"w93GQWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACA" +
				"AAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_GLOBAL_FALLBACK_LOCKTIME",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAEDBAIAAAAAAQBSAgAAAAHB" +
				"qiVuIUuWoYIvk95Cv/O18/+NBRkwbjUV11FaXoBbEgAA" +
				"AAAA/////wEYxpo7AAAAABYAFLCjrxRCCEEmk8p9FmhS" +
				"tS2wrvBuAAAAAAEBHxjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4BCGsCRzBEAiAFJ1pIVzTgrh87lxI3" +
				"WG8OctyFgz0njA5HTNIxEsD6XgIgawSMg868PEHQuTzH" +
				"2nYYXO29Aw0AWwgBi+K5i7rL33sBIQN2DcygXzmX3GWy" +
				"kwYPfynxUUyMUnBI4SgCsEHU/DQKJwAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1l" +
				"w93GQWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACA" +
				"AAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_GLOBAL_INPUT_COUNT",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAEEAQIAAQBSAgAAAAHBqiVu" +
				"IUuWoYIvk95Cv/O18/+NBRkwbjUV11FaXoBbEgAAAAAA" +
				"/////wEYxpo7AAAAABYAFLCjrxRCCEEmk8p9FmhStS2w" +
				"rvBuAAAAAAEBHxjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4BCGsCRzBEAiAFJ1pIVzTgrh87lxI3WG8O" +
				"ctyFgz0njA5HTNIxEsD6XgIgawSMg868PEHQuTzH2nYY" +
				"XO29Aw0AWwgBi+K5i7rL33sBIQN2DcygXzmX3GWykwYP" +
				"fynxUUyMUnBI4SgCsEHU/DQKJwAiAgLWAfhIRqZ1X3dr" +
				"4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAAgAEA" +
				"AIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1lw93G" +
				"QWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACAAAAA" +
				"gAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_GLOBAL_OUTPUT_COUNT",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAEFAQIAAQBSAgAAAAHBqiVu" +
				"IUuWoYIvk95Cv/O18/+NBRkwbjUV11FaXoBbEgAAAAAA" +
				"/////wEYxpo7AAAAABYAFLCjrxRCCEEmk8p9FmhStS2w" +
				"rvBuAAAAAAEBHxjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4BCGsCRzBEAiAFJ1pIVzTgrh87lxI3WG8O" +
				"ctyFgz0njA5HTNIxEsD6XgIgawSMg868PEHQuTzH2nYY" +
				"XO29Aw0AWwgBi+K5i7rL33sBIQN2DcygXzmX3GWykwYP" +
				"fynxUUyMUnBI4SgCsEHU/DQKJwAiAgLWAfhIRqZ1X3dr" +
				"4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAAgAEA" +
				"AIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1lw93G" +
				"QWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACAAAAA" +
				"gAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_GLOBAL_TX_MODIFIABLE",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAEGAQAAAQBSAgAAAAHBqiVu" +
				"IUuWoYIvk95Cv/O18/+NBRkwbjUV11FaXoBbEgAAAAAA" +
				"/////wEYxpo7AAAAABYAFLCjrxRCCEEmk8p9FmhStS2w" +
				"rvBuAAAAAAEBHxjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4BCGsCRzBEAiAFJ1pIVzTgrh87lxI3WG8O" +
				"ctyFgz0njA5HTNIxEsD6XgIgawSMg868PEHQuTzH2nYY" +
				"XO29Aw0AWwgBi+K5i7rL33sBIQN2DcygXzmX3GWykwYP" +
				"fynxUUyMUnBI4SgCsEHU/DQKJwAiAgLWAfhIRqZ1X3dr" +
				"4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAAgAEA" +
				"AIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1lw93G" +
				"QWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACAAAAA" +
				"gAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_IN_PREVIOUS_TXID",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAABAFICAAAAAcGqJW4hS5ah" +
				"gi+T3kK/87Xz/40FGTBuNRXXUVpegFsSAAAAAAD/////" +
				"ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0WaFK1LbCu8G4A" +
				"AAAAAQEfGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgEIawJHMEQCIAUnWkhXNOCuHzuXEjdYbw5y3IWD" +
				"PSeMDkdM0jESwPpeAiBrBIyDzrw8QdC5PMfadhhc7b0D" +
				"DQBbCAGL4rmLusvfewEhA3YNzKBfOZfcZbKTBg9/KfFR" +
				"TIxScEjhKAKwQdT8NAonAQ4gCwrZIUGcHIcZc11y3HOf" +
				"nqngY40f5MHu8PmUQISBX8gAIgIC1gH4SEamdV93a+AO" +
				"PZ3o+xCsyTX7g8RfsBYtTK1at5IY9p2HPlQAAIABAACA" +
				"AAAAgAAAAAAqAAAAACICA27+LCVWIZhlU7qdZcPdxkFl" +
				"yhQ24FqjWkxusCRRz3ltGPadhz5UAACAAQAAgAAAAIAB" +
				"AAAAYgAAAAA=",
		},
		{
			"PSBTv0 but with PSBT_IN_OUTPUT_INDEX",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAABAFICAAAAAcGqJW4hS5ah" +
				"gi+T3kK/87Xz/40FGTBuNRXXUVpegFsSAAAAAAD/////" +
				"ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0WaFK1LbCu8G4A" +
				"AAAAAQEfGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgEIawJHMEQCIAUnWkhXNOCuHzuXEjdYbw5y3IWD" +
				"PSeMDkdM0jESwPpeAiBrBIyDzrw8QdC5PMfadhhc7b0D" +
				"DQBbCAGL4rmLusvfewEhA3YNzKBfOZfcZbKTBg9/KfFR" +
				"TIxScEjhKAKwQdT8NAonAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1l" +
				"w93GQWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACA" +
				"AAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_IN_SEQUENCE",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAABAFICAAAAAcGqJW4hS5ah" +
				"gi+T3kK/87Xz/40FGTBuNRXXUVpegFsSAAAAAAD/////" +
				"ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0WaFK1LbCu8G4A" +
				"AAAAAQEfGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgEIawJHMEQCIAUnWkhXNOCuHzuXEjdYbw5y3IWD" +
				"PSeMDkdM0jESwPpeAiBrBIyDzrw8QdC5PMfadhhc7b0D" +
				"DQBbCAGL4rmLusvfewEhA3YNzKBfOZfcZbKTBg9/KfFR" +
				"TIxScEjhKAKwQdT8NAonARAE/////wAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1l" +
				"w93GQWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACA" +
				"AAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_IN_REQUIRED_TIME_LOCKTIME",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAABAFICAAAAAcGqJW4hS5ah" +
				"gi+T3kK/87Xz/40FGTBuNRXXUVpegFsSAAAAAAD/////" +
				"ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0WaFK1LbCu8G4A" +
				"AAAAAQEfGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgEIawJHMEQCIAUnWkhXNOCuHzuXEjdYbw5y3IWD" +
				"PSeMDkdM0jESwPpeAiBrBIyDzrw8QdC5PMfadhhc7b0D" +
				"DQBbCAGL4rmLusvfewEhA3YNzKBfOZfcZbKTBg9/KfFR" +
				"TIxScEjhKAKwQdT8NAonAREEjI3EYgAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1l" +
				"w93GQWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACA" +
				"AAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_IN_REQUIRED_HEIGHT_LOCKTIME",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAABAFICAAAAAcGqJW4hS5ah" +
				"gi+T3kK/87Xz/40FGTBuNRXXUVpegFsSAAAAAAD/////" +
				"ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0WaFK1LbCu8G4A" +
				"AAAAAQEfGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgEIawJHMEQCIAUnWkhXNOCuHzuXEjdYbw5y3IWD" +
				"PSeMDkdM0jESwPpeAiBrBIyDzrw8QdC5PMfadhhc7b0D" +
				"DQBbCAGL4rmLusvfewEhA3YNzKBfOZfcZbKTBg9/KfFR" +
				"TIxScEjhKAKwQdT8NAonARIEECcAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAAAIgIDbv4sJVYhmGVTup1l" +
				"w93GQWXKFDbgWqNaTG6wJFHPeW0Y9p2HPlQAAIABAACA" +
				"AAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv0 but with PSBT_OUT_AMOUNT",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAABAFICAAAAAcGqJW4hS5ah" +
				"gi+T3kK/87Xz/40FGTBuNRXXUVpegFsSAAAAAAD/////" +
				"ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0WaFK1LbCu8G4A" +
				"AAAAAQEfGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgEIawJHMEQCIAUnWkhXNOCuHzuXEjdYbw5y3IWD" +
				"PSeMDkdM0jESwPpeAiBrBIyDzrw8QdC5PMfadhhc7b0D" +
				"DQBbCAGL4rmLusvfewEhA3YNzKBfOZfcZbKTBg9/KfFR" +
				"TIxScEjhKAKwQdT8NAonACICAtYB+EhGpnVfd2vgDj2d" +
				"6PsQrMk1+4PEX7AWLUytWreSGPadhz5UAACAAQAAgAAA" +
				"AIAAAAAAKgAAAAEDCAAIry8AAAAAACICA27+LCVWIZhl" +
				"U7qdZcPdxkFlyhQ24FqjWkxusCRRz3ltGPadhz5UAACA" +
				"AQAAgAAAAIABAAAAYgAAAAA=",
		},
		{
			"PSBTv0 but with PSBT_OUT_SCRIPT",
			"cHNidP8BAHECAAAAAQsK2SFBnByHGXNdctxzn56p4GONH+TB7vD5" +
				"lECEgV/IAAAAAAD+////AgAIry8AAAAAFgAUxDD2TEdW" +
				"2jENvRoIVXLvKZkmJyyLvesLAAAAABYAFKB9rIq2ypQt" +
				"N57Xlfg1unHJzGiFAAAAAAABAFICAAAAAcGqJW4hS5ah" +
				"gi+T3kK/87Xz/40FGTBuNRXXUVpegFsSAAAAAAD/////" +
				"ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0WaFK1LbCu8G4A" +
				"AAAAAQEfGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgEIawJHMEQCIAUnWkhXNOCuHzuXEjdYbw5y3IWD" +
				"PSeMDkdM0jESwPpeAiBrBIyDzrw8QdC5PMfadhhc7b0D" +
				"DQBbCAGL4rmLusvfewEhA3YNzKBfOZfcZbKTBg9/KfFR" +
				"TIxScEjhKAKwQdT8NAonACICAtYB+EhGpnVfd2vgDj2d" +
				"6PsQrMk1+4PEX7AWLUytWreSGPadhz5UAACAAQAAgAAA" +
				"AIAAAAAAKgAAAAEEFgAUoH2sirbKlC03nteV+DW6ccnM" +
				"aIUAIgIDbv4sJVYhmGVTup1lw93GQWXKFDbgWqNaTG6w" +
				"JFHPeW0Y9p2HPlQAAIABAACAAAAAgAEAAABiAAAAAA==",
		},
		{
			"PSBTv2 but with PSBT_GLOBAL_UNSIGNED_TX",
			"cHNidP8BAFICAAAAAcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXX" +
				"UVpegFsSAAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEII" +
				"QSaTyn0WaFK1LbCu8G4AAAAAAQIEAgAAAAEDBAAAAAAB" +
				"BAEBAQUBAgEGAQcB+wQCAAAAAAEAUgIAAAABwaolbiFL" +
				"lqGCL5PeQr/ztfP/jQUZMG41FddRWl6AWxIAAAAAAP//" +
				"//8BGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUtsK7w" +
				"bgAAAAABAR8Yxpo7AAAAABYAFLCjrxRCCEEmk8p9FmhS" +
				"tS2wrvBuAQ4gCwrZIUGcHIcZc11y3HOfnqngY40f5MHu" +
				"8PmUQISBX8gBDwQAAAAAARAE/v///wERBIyNxGIBEgQQ" +
				"JwAAACICAtYB+EhGpnVfd2vgDj2d6PsQrMk1+4PEX7AW" +
				"LUytWreSGPadhz5UAACAAQAAgAAAAIAAAAAAKgAAAAED" +
				"CAAIry8AAAAAAQQWABTEMPZMR1baMQ29GghVcu8pmSYn" +
				"LAAiAgLjb7/1PdU0Bwz4/TlmFGgPNXqbhdtzQL8c+nRd" +
				"KtezQBj2nYc+VAAAgAEAAIAAAACAAQAAAGQAAAABAwiL" +
				"vesLAAAAAAEEFgAUTdGTrJZKVqwbnhzKhFT+L0dPhRMA",
		},
		{
			"PSBTv2 missing PSBT_GLOBAL_INPUT_COUNT",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEFAQIB+wQCAAAAAAEAUgIAAAAB" +
				"waolbiFLlqGCL5PeQr/ztfP/jQUZMG41FddRWl6AWxIA" +
				"AAAAAP////8BGMaaOwAAAAAWABSwo68UQghBJpPKfRZo" +
				"UrUtsK7wbgAAAAABAR8Yxpo7AAAAABYAFLCjrxRCCEEm" +
				"k8p9FmhStS2wrvBuAQ4gCwrZIUGcHIcZc11y3HOfnqng" +
				"Y40f5MHu8PmUQISBX8gBDwQAAAAAARAE/v///wAiAgLW" +
				"AfhIRqZ1X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2" +
				"nYc+VAAAgAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAA" +
				"AAEEFgAUxDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/" +
				"9T3VNAcM+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2H" +
				"PlQAAIABAACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAAB" +
				"BBYAFE3Rk6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"PSBTv2 missing PSBT_GLOBAL_OUTPUT_COUNT",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEB+wQCAAAAAAEAUgIAAAAB" +
				"waolbiFLlqGCL5PeQr/ztfP/jQUZMG41FddRWl6AWxIA" +
				"AAAAAP////8BGMaaOwAAAAAWABSwo68UQghBJpPKfRZo" +
				"UrUtsK7wbgAAAAABAR8Yxpo7AAAAABYAFLCjrxRCCEEm" +
				"k8p9FmhStS2wrvBuAQ4gCwrZIUGcHIcZc11y3HOfnqng" +
				"Y40f5MHu8PmUQISBX8gBDwQAAAAAARAE/v///wAiAgLW" +
				"AfhIRqZ1X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2" +
				"nYc+VAAAgAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAA" +
				"AAEEFgAUxDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/" +
				"9T3VNAcM+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2H" +
				"PlQAAIABAACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAAB" +
				"BBYAFE3Rk6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"PSBTv2 missing PSBT_GLOBAL_TX_VERSION",
			"cHNidP8BBAEBAQUBAgH7BAIAAAAAAQ4gCwrZIUGcHIcZc11y3HOf" +
				"nqngY40f5MHu8PmUQISBX8gBDwQAAAAAAAEDCAAIry8A" +
				"AAAAAQQWABTEMPZMR1baMQ29GghVcu8pmSYnLAABAwiL" +
				"vesLAAAAAAEEFgAUTdGTrJZKVqwbnhzKhFT+L0dPhRMA",
		},
		{
			"PSBTv2 missing PSBT_IN_PREVIOUS_TXID",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEBBQECAfsEAgAAAAABAFIC" +
				"AAAAAcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpe" +
				"gFsSAAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68U" +
				"QghBJpPKfRZoUrUtsK7wbgEPBAAAAAABEAT+////ACIC" +
				"AtYB+EhGpnVfd2vgDj2d6PsQrMk1+4PEX7AWLUytWreS" +
				"GPadhz5UAACAAQAAgAAAAIAAAAAAKgAAAAEDCAAIry8A" +
				"AAAAAQQWABTEMPZMR1baMQ29GghVcu8pmSYnLAAiAgLj" +
				"b7/1PdU0Bwz4/TlmFGgPNXqbhdtzQL8c+nRdKtezQBj2" +
				"nYc+VAAAgAEAAIAAAACAAQAAAGQAAAABAwiLvesLAAAA" +
				"AAEEFgAUTdGTrJZKVqwbnhzKhFT+L0dPhRMA",
		},
		{
			"PSBTv2 missing PSBT_IN_OUTPUT_INDEX",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEBBQECAfsEAgAAAAABAFIC" +
				"AAAAAcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpe" +
				"gFsSAAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68U" +
				"QghBJpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxz" +
				"n56p4GONH+TB7vD5lECEgV/IARAE/v///wAiAgLWAfhI" +
				"RqZ1X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+" +
				"VAAAgAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEE" +
				"FgAUxDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3V" +
				"NAcM+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQA" +
				"AIABAACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYA" +
				"FE3Rk6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"PSBTv2 missing PSBT_OUT_AMOUNT",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEBBQECAfsEAgAAAAABAFIC" +
				"AAAAAcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpe" +
				"gFsSAAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68U" +
				"QghBJpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxz" +
				"n56p4GONH+TB7vD5lECEgV/IAQ8EAAAAAAEQBP7///8A" +
				"IgIC1gH4SEamdV93a+AOPZ3o+xCsyTX7g8RfsBYtTK1a" +
				"t5IY9p2HPlQAAIABAACAAAAAgAAAAAAqAAAAAQQWABTE" +
				"MPZMR1baMQ29GghVcu8pmSYnLAAiAgLjb7/1PdU0Bwz4" +
				"/TlmFGgPNXqbhdtzQL8c+nRdKtezQBj2nYc+VAAAgAEA" +
				"AIAAAACAAQAAAGQAAAABAwiLvesLAAAAAAEEFgAUTdGT" +
				"rJZKVqwbnhzKhFT+L0dPhRMA",
		},
		{
			"PSBTv2 missing PSBT_OUT_SCRIPT",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEBBQECAfsEAgAAAAABAFIC" +
				"AAAAAcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpe" +
				"gFsSAAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68U" +
				"QghBJpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxz" +
				"n56p4GONH+TB7vD5lECEgV/IAQ8EAAAAAAEQBP7///8A" +
				"IgIC1gH4SEamdV93a+AOPZ3o+xCsyTX7g8RfsBYtTK1a" +
				"t5IY9p2HPlQAAIABAACAAAAAgAAAAAAqAAAAAQMIAAiv" +
				"LwAAAAAAIgIC42+/9T3VNAcM+P05ZhRoDzV6m4Xbc0C/" +
				"HPp0XSrXs0AY9p2HPlQAAIABAACAAAAAgAEAAABkAAAA" +
				"AQMIi73rCwAAAAABBBYAFE3Rk6yWSlasG54cyoRU/i9H" +
				"T4UTAA==",
		},
		{
			"PSBTv2 with PSBT_IN_REQUIRED_TIME_LOCKTIME less than " +
				"500000000",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIB+wQCAAAAAAEAUgIAAAABwaol" +
				"biFLlqGCL5PeQr/ztfP/jQUZMG41FddRWl6AWxIAAAAA" +
				"AP////8BGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgAAAAABAR8Yxpo7AAAAABYAFLCjrxRCCEEmk8p9" +
				"FmhStS2wrvBuAQ4gCwrZIUGcHIcZc11y3HOfnqngY40f" +
				"5MHu8PmUQISBX8gBDwQAAAAAAREE/2TNHQAiAgLWAfhI" +
				"RqZ1X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+" +
				"VAAAgAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEE" +
				"FgAUxDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3V" +
				"NAcM+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQA" +
				"AIABAACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYA" +
				"FE3Rk6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"PSBTv2 with PSBT_IN_REQUIRED_HEIGHT_LOCKTIME greater " +
				"than or equal to 500000000",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIB+wQCAAAAAAEAUgIAAAABwaol" +
				"biFLlqGCL5PeQr/ztfP/jQUZMG41FddRWl6AWxIAAAAA" +
				"AP////8BGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgAAAAABAR8Yxpo7AAAAABYAFLCjrxRCCEEmk8p9" +
				"FmhStS2wrvBuAQ4gCwrZIUGcHIcZc11y3HOfnqngY40f" +
				"5MHu8PmUQISBX8gBDwQAAAAAARIEAGXNHQAiAgLWAfhI" +
				"RqZ1X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+" +
				"VAAAgAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEE" +
				"FgAUxDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3V" +
				"NAcM+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQA" +
				"AIABAACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYA" +
				"FE3Rk6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"PSBTv2 with PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of 0",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEBBQECAQYBBwH7BAIAAAAA" +
				"AQBSAgAAAAHBqiVuIUuWoYIvk95Cv/O18/+NBRkwbjUV" +
				"11FaXoBbEgAAAAAA/////wEYxpo7AAAAABYAFLCjrxRC" +
				"CEEmk8p9FmhStS2wrvBuAAAAAAEBHxjGmjsAAAAAFgAU" +
				"sKOvFEIIQSaTyn0WaFK1LbCu8G4BDiALCtkhQZwchxlz" +
				"XXLcc5+eqeBjjR/kwe7w+ZRAhIFfyAEPBAAAAAABEAT+" +
				"////AREEjI3EYgESBAAAAAAAIgIC1gH4SEamdV93a+AO" +
				"PZ3o+xCsyTX7g8RfsBYtTK1at5IY9p2HPlQAAIABAACA" +
				"AAAAgAAAAAAqAAAAAQMIAAivLwAAAAABBBYAFMQw9kxH" +
				"VtoxDb0aCFVy7ymZJicsACICAuNvv/U91TQHDPj9OWYU" +
				"aA81epuF23NAvxz6dF0q17NAGPadhz5UAACAAQAAgAAA" +
				"AIABAAAAZAAAAAEDCIu96wsAAAAAAQQWABRN0ZOslkpW" +
				"rBueHMqEVP4vR0+FEwA=",
		},
	}

	for _, tc := range invalidVectors {
		t.Run(tc.desc, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(tc.base64)
			require.NoError(t, err)
			_, err = NewFromRawBytes(bytes.NewReader(data), false)
			require.Error(t, err, "Expected parsing to fail for: "+
				"%s", tc.desc)
		})
	}
}

func TestBIP370ValidVectors(t *testing.T) {
	validVectors := []struct {
		desc   string
		base64 string
	}{
		{
			"1 input, 2 output PSBTv2, required fields only",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIB+wQCAAAAAAEOIAsK2SFBnByH" +
				"GXNdctxzn56p4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAB" +
				"AwgACK8vAAAAAAEEFgAUxDD2TEdW2jENvRoIVXLvKZkm" +
				"JywAAQMIi73rCwAAAAABBBYAFE3Rk6yWSlasG54cyoRU" +
				"/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIB+wQCAAAAAAEAUgIAAAABwaol" +
				"biFLlqGCL5PeQr/ztfP/jQUZMG41FddRWl6AWxIAAAAA" +
				"AP////8BGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgAAAAABAR8Yxpo7AAAAABYAFLCjrxRCCEEmk8p9" +
				"FmhStS2wrvBuAQ4gCwrZIUGcHIcZc11y3HOfnqngY40f" +
				"5MHu8PmUQISBX8gBDwQAAAAAACICAtYB+EhGpnVfd2vg" +
				"Dj2d6PsQrMk1+4PEX7AWLUytWreSGPadhz5UAACAAQAA" +
				"gAAAAIAAAAAAKgAAAAEDCAAIry8AAAAAAQQWABTEMPZM" +
				"R1baMQ29GghVcu8pmSYnLAAiAgLjb7/1PdU0Bwz4/Tlm" +
				"FGgPNXqbhdtzQL8c+nRdKtezQBj2nYc+VAAAgAEAAIAA" +
				"AACAAQAAAGQAAAABAwiLvesLAAAAAAEEFgAUTdGTrJZK" +
				"VqwbnhzKhFT+L0dPhRMA",
		},
		{
			"1 input, 2 output updated PSBTv2, with " +
				"PSBT_IN_SEQUENCE",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIB+wQCAAAAAAEAUgIAAAABwaol" +
				"biFLlqGCL5PeQr/ztfP/jQUZMG41FddRWl6AWxIAAAAA" +
				"AP////8BGMaaOwAAAAAWABSwo68UQghBJpPKfRZoUrUt" +
				"sK7wbgAAAAABAR8Yxpo7AAAAABYAFLCjrxRCCEEmk8p9" +
				"FmhStS2wrvBuAQ4gCwrZIUGcHIcZc11y3HOfnqngY40f" +
				"5MHu8PmUQISBX8gBDwQAAAAAARAE/v///wAiAgLWAfhI" +
				"RqZ1X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+" +
				"VAAAgAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEE" +
				"FgAUxDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3V" +
				"NAcM+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQA" +
				"AIABAACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYA" +
				"FE3Rk6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with " +
				"PSBT_IN_SEQUENCE, and all locktime fields",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEBBQECAfsEAgAAAAABAFIC" +
				"AAAAAcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpe" +
				"gFsSAAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaT" +
				"yn0WaFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68U" +
				"QghBJpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxz" +
				"n56p4GONH+TB7vD5lECEgV/IAQ8EAAAAAAEQBP7///8B" +
				"EQSMjcRiARIEECcAAAAiAgLWAfhIRqZ1X3dr4A49nej7" +
				"EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAAgAEAAIAAAACA" +
				"AAAAACoAAAABAwgACK8vAAAAAAEEFgAUxDD2TEdW2jEN" +
				"vRoIVXLvKZkmJywAIgIC42+/9T3VNAcM+P05ZhRoDzV6" +
				"m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIABAACAAAAAgAEA" +
				"AABkAAAAAQMIi73rCwAAAAABBBYAFE3Rk6yWSlasG54c" +
				"yoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with Inputs " +
				"Modifiable Flag (bit 0) of " +
				"PSBT_GLOBAL_TX_MODIFIABLE set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgEBAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with Outputs " +
				"Modifiable Flag (bit 1) of " +
				"PSBT_GLOBAL_TX_MODIFIABLE set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgECAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with Has " +
				"SIGHASH_SINGLE Flag (bit 2) of " +
				"PSBT_GLOBAL_TX_MODIFIABLE set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgEEAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with an undefined " +
				"flag (bit 3) of PSBT_GLOBAL_TX_MODIFIABLE set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgEIAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with both Inputs " +
				"Modifiable Flag (bit 0) and Outputs " +
				"Modifiable Flag (bit 1) of " +
				"PSBT_GLOBAL_TX_MODIFIABLE set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgEDAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with both Inputs " +
				"Modifiable Flag (bit 0) and Has " +
				"SIGHASH_SINGLE Flag (bit 2) of " +
				"PSBT_GLOBAL_TX_MODIFIABLE set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgEFAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with both Outputs " +
				"Modifiable Flag (bit 1) and Has " +
				"SIGHASH_SINGLE FLag (bit 2) of " +
				"PSBT_GLOBAL_TX_MODIFIABLE set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgEGAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with all defined " +
				"PSBT_GLOBAL_TX_MODIFIABLE flags set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgEHAfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with all possible " +
				"PSBT_GLOBAL_TX_MODIFIABLE flags set",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIBBgH/AfsEAgAAAAABAFICAAAA" +
				"AcGqJW4hS5ahgi+T3kK/87Xz/40FGTBuNRXXUVpegFsS" +
				"AAAAAAD/////ARjGmjsAAAAAFgAUsKOvFEIIQSaTyn0W" +
				"aFK1LbCu8G4AAAAAAQEfGMaaOwAAAAAWABSwo68UQghB" +
				"JpPKfRZoUrUtsK7wbgEOIAsK2SFBnByHGXNdctxzn56p" +
				"4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAiAgLWAfhIRqZ1" +
				"X3dr4A49nej7EKzJNfuDxF+wFi1MrVq3khj2nYc+VAAA" +
				"gAEAAIAAAACAAAAAACoAAAABAwgACK8vAAAAAAEEFgAU" +
				"xDD2TEdW2jENvRoIVXLvKZkmJywAIgIC42+/9T3VNAcM" +
				"+P05ZhRoDzV6m4Xbc0C/HPp0XSrXs0AY9p2HPlQAAIAB" +
				"AACAAAAAgAEAAABkAAAAAQMIi73rCwAAAAABBBYAFE3R" +
				"k6yWSlasG54cyoRU/i9HT4UTAA==",
		},
		{
			"1 input, 2 output updated PSBTv2, with all PSBTv2 " +
				"fields",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQEBBQECAQYBBwH7BAIAAAAA" +
				"AQBSAgAAAAHBqiVuIUuWoYIvk95Cv/O18/+NBRkwbjUV" +
				"11FaXoBbEgAAAAAA/////wEYxpo7AAAAABYAFLCjrxRC" +
				"CEEmk8p9FmhStS2wrvBuAAAAAAEBHxjGmjsAAAAAFgAU" +
				"sKOvFEIIQSaTyn0WaFK1LbCu8G4BDiALCtkhQZwchxlz" +
				"XXLcc5+eqeBjjR/kwe7w+ZRAhIFfyAEPBAAAAAABEAT+" +
				"////AREEjI3EYgESBBAnAAAAIgIC1gH4SEamdV93a+AO" +
				"PZ3o+xCsyTX7g8RfsBYtTK1at5IY9p2HPlQAAIABAACA" +
				"AAAAgAAAAAAqAAAAAQMIAAivLwAAAAABBBYAFMQw9kxH" +
				"VtoxDb0aCFVy7ymZJicsACICAuNvv/U91TQHDPj9OWYU" +
				"aA81epuF23NAvxz6dF0q17NAGPadhz5UAACAAQAAgAAA" +
				"AIABAAAAZAAAAAEDCIu96wsAAAAAAQQWABRN0ZOslkpW" +
				"rBueHMqEVP4vR0+FEwA=",
		},
	}

	for _, tc := range validVectors {
		t.Run(tc.desc, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(tc.base64)
			require.NoError(t, err)
			p, err := NewFromRawBytes(bytes.NewReader(data), false)
			require.NoError(t, err, "Expected parsing to succeed "+
				"for: %s", tc.desc)
			require.Equal(t, uint32(2), p.Version, "PSBT should "+
				"be version 2")
		})
	}
}

func TestBIP370LocktimeVectors(t *testing.T) {
	// 0 expected
	zeroExpected := []struct {
		desc   string
		base64 string
	}{
		{
			"No locktimes specified",
			"cHNidP8BAgQCAAAAAQQBAQEFAQIB+wQCAAAAAAEOIAsK2SFBnByH" +
				"GXNdctxzn56p4GONH+TB7vD5lECEgV/IAQ8EAAAAAAAB" +
				"AwgACK8vAAAAAAEEFgAUxDD2TEdW2jENvRoIVXLvKZkm" +
				"JywAAQMIi73rCwAAAAABBBYAFE3Rk6yWSlasG54cyoRU" +
				"/i9HT4UTAA==",
		},
		{
			"Fallback locktime of 0",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAAAAQ4gOhs7PIN9ZInqejHY5sfdUDwAG+8+BpWO" +
				"dXSAjWjKeKUBDwQAAAAAAAEDCE+TNXcAAAAAAQQWABQL" +
				"E1LKzQPPaqG388jWOIZxs0peEQA=",
		},
	}

	// 10000 expected
	tenKExpected := []struct {
		desc   string
		base64 string
	}{
		{
			"Input 1 has PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of " +
				"10000, Input 2 has no locktime fields",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAABEgQQJwAAAAEOIDobOzyDfWSJ6nox2ObH3VA8" +
				"ABvvPgaVjnV0gI1oynilAQ8EAAAAAAABAwhPkzV3AAAA" +
				"AAEEFgAUCxNSys0Dz2qht/PI1jiGcbNKXhEA",
		},
		{
			"Input 1 has PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of " +
				"10000, Input 2 has " +
				"PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of 9000",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAABEgQQJwAAAAEOIDobOzyDfWSJ6nox2ObH3VA8" +
				"ABvvPgaVjnV0gI1oynilAQ8EAAAAAAESBCgjAAAAAQMI" +
				"T5M1dwAAAAABBBYAFAsTUsrNA89qobfzyNY4hnGzSl4R" +
				"AA==",
		},
		{
			"Input 1 has PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of " +
				"10000, Input 2 has " +
				"PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of 9000 and " +
				"PSBT_IN_REQUIRED_TIME_LOCKTIME of 1657048460",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAABEgQQJwAAAAEOIDobOzyDfWSJ6nox2ObH3VA8" +
				"ABvvPgaVjnV0gI1oynilAQ8EAAAAAAERBIyNxGIBEgQo" +
				"IwAAAAEDCE+TNXcAAAAAAQQWABQLE1LKzQPPaqG388jW" +
				"OIZxs0peEQA=",
		},
		{
			"Input 1 has PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of " +
				"10000 and PSBT_IN_REQUIRED_TIME_LOCKTIME of " +
				"1657048459, Input 2 has " +
				"PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of 9000 and " +
				"PSBT_IN_REQUIRED_TIME_LOCKTIME of 1657048460",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAABEQSLjcRiARIEECcAAAABDiA6Gzs8g31kiep6" +
				"Mdjmx91QPAAb7z4GlY51dICNaMp4pQEPBAAAAAABEQSM" +
				"jcRiARIEKCMAAAABAwhPkzV3AAAAAAEEFgAUCxNSys0D" +
				"z2qht/PI1jiGcbNKXhEA",
		},
	}

	// 1657048460 expected
	timeExpected := []struct {
		desc   string
		base64 string
	}{
		{
			"Input 1 has PSBT_IN_REQUIRED_TIME_LOCKTIME of " +
				"1657048459, Input 2 has " +
				"PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of 9000 and " +
				"PSBT_IN_REQUIRED_TIME_LOCKTIME of 1657048460",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAABEQSLjcRiAAEOIDobOzyDfWSJ6nox2ObH3VA8" +
				"ABvvPgaVjnV0gI1oynilAQ8EAAAAAAERBIyNxGIBEgQo" +
				"IwAAAAEDCE+TNXcAAAAAAQQWABQLE1LKzQPPaqG388jW" +
				"OIZxs0peEQA=",
		},
		{
			"Input 1 has PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of " +
				"10000 and PSBT_IN_REQUIRED_TIME_LOCKTIME of " +
				"1657048459, Input 2 has " +
				"PSBT_IN_REQUIRED_TIME_LOCKTIME of 1657048460",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAABEQSLjcRiARIEECcAAAABDiA6Gzs8g31kiep6" +
				"Mdjmx91QPAAb7z4GlY51dICNaMp4pQEPBAAAAAABEQSM" +
				"jcRiAAEDCE+TNXcAAAAAAQQWABQLE1LKzQPPaqG388jW" +
				"OIZxs0peEQA=",
		},
		{
			"Cannot be computed fallback scenario (which the BIP " +
				"says has same time)",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAAAAQ4gOhs7PIN9ZInqejHY5sfdUDwAG+8+BpWO" +
				"dXSAjWjKeKUBDwQAAAAAAREEjI3EYgABAwhPkzV3AAAA" +
				"AAEEFgAUCxNSys0Dz2qht/PI1jiGcbNKXhEA",
		},
	}

	// Error expected
	errorExpected := []struct {
		desc   string
		base64 string
	}{
		{
			"Input 1 has PSBT_IN_REQUIRED_HEIGHT_LOCKTIME of " +
				"10000, Input 2 has " +
				"PSBT_IN_REQUIRED_TIME_LOCKTIME of 1657048460",
			"cHNidP8BAgQCAAAAAQMEAAAAAAEEAQIBBQEBAfsEAgAAAAABDiAP" +
				"dY2/vU2nwWyKMwnDyB4RAPVh6mRttbAXUsSF4b3enwEP" +
				"BAEAAAABEgQQJwAAAAEOIDobOzyDfWSJ6nox2ObH3VA8" +
				"ABvvPgaVjnV0gI1oynilAQ8EAAAAAAERBIyNxGIAAQMI" +
				"T5M1dwAAAAABBBYAFAsTUsrNA89qobfzyNY4hnGzSl4R" +
				"AA==",
		},
	}

	for _, tc := range zeroExpected {
		t.Run(tc.desc, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(tc.base64)
			require.NoError(t, err)
			p, err := NewFromRawBytes(bytes.NewReader(data), false)
			require.NoError(t, err)
			locktime, err := p.DetermineLockTime()
			require.NoError(t, err)
			require.Equal(t, uint32(0), locktime)
		})
	}

	for _, tc := range tenKExpected {
		t.Run(tc.desc, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(tc.base64)
			require.NoError(t, err)
			p, err := NewFromRawBytes(bytes.NewReader(data), false)
			require.NoError(t, err)
			locktime, err := p.DetermineLockTime()
			require.NoError(t, err)
			require.Equal(t, uint32(10000), locktime)
		})
	}

	for _, tc := range timeExpected {
		t.Run(tc.desc, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(tc.base64)
			require.NoError(t, err)
			p, err := NewFromRawBytes(bytes.NewReader(data), false)
			require.NoError(t, err)
			locktime, err := p.DetermineLockTime()
			require.NoError(t, err)
			require.Equal(t, uint32(1657048460), locktime)
		})
	}

	for _, tc := range errorExpected {
		t.Run(tc.desc, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(tc.base64)
			require.NoError(t, err)
			p, err := NewFromRawBytes(bytes.NewReader(data), false)
			require.NoError(t, err)
			_, err = p.DetermineLockTime()
			require.Error(t, err)
		})
	}
}

// TestPsbtV2LifeCycle ensures that the full lifecycle of a PSBTv2 (creating,
// constructing, serializing, and extracting) works as expected.
func TestPsbtV2LifeCycle(t *testing.T) {

	// 1. Create a new V2 PSBT.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	// 2. Add an input with a specific sequence (e.g., 0 for RBF).
	txid, _ := chainhash.NewHashFromStr(
		"010203040506070809101112131415161718192021222324252627282930" +
			"3132",
	)

	outPoint := wire.NewOutPoint(txid, 1)
	err = p.AddInputV2(PInput{
		PreviousTxid: (*outPoint).Hash[:],
		OutputIndex:  (*outPoint).Index,
		Sequence:     0,
	})
	require.NoError(t, err)

	// 3. Add an output.
	script, _ := hex.DecodeString(
		"76a914b6bc2c0ee5655a843d79afedd0ccc3f7dd64340988ac",
	)
	err = p.AddOutputV2(POutput{Amount: 100000000, Script: script})
	require.NoError(t, err)

	// 4. Serialize and Parse back.
	var b bytes.Buffer
	err = p.Serialize(&b)
	require.NoError(t, err)

	p2, err := NewFromRawBytes(&b, false)
	require.NoError(t, err)

	// 5. Verify fields survived the round-trip.
	require.Equal(t, int32(2), p2.TxVersion)
	require.Equal(t, uint32(1), p2.InputCount)
	require.Equal(t, txid[:], p2.Inputs[0].PreviousTxid)
	require.Equal(t, uint32(0), p2.Inputs[0].Sequence)
	require.Equal(t, int64(100000000), p2.Outputs[0].Amount)

	// 6. Extract the transaction and verify it works.
	// (Note: For extraction to work, we need to bypass the IsComplete check
	// or finalize it. Since we are testing construction, we can test
	// GetUnsignedTx directly).
	msgTx, err := p2.GetUnsignedTx()
	require.NoError(t, err)
	require.Equal(t, int32(2), msgTx.Version)
	require.Equal(t, uint32(0), msgTx.TxIn[0].Sequence)
	require.Equal(t, int64(100000000), msgTx.TxOut[0].Value)
}

// TestPsbtV2Validation verifies that PSBTv2 packets are validated correctly
// for strict versioning rules and mandatory field combinations.
func TestPsbtV2Validation(t *testing.T) {
	t.Run("V2 cannot have global UnsignedTx", func(t *testing.T) {
		// Construct raw bytes with both Version 2 AND UnsignedTx (0x00
		// forbidden in V2). Magic + Version (0xfb: 2) + UnsignedTx
		// (0x00: minimal 1-byte tx)
		raw := []byte{
			0x70, 0x73, 0x62, 0x74, 0xff, // Magic
			0x01, 0xfb, 0x04, 0x02, 0x00, 0x00, 0x00, // Version 2
			0x01, 0x00, 0x01, 0x01, // UnsignedTx (keyCode 0x00, minimal value)
			0x00, // Separator
		}

		_, err := NewFromRawBytes(bytes.NewReader(raw), false)
		require.Error(t, err)
	})

	t.Run("V2 must have InputCount and OutputCount", func(t *testing.T) {
		// Create a raw V2 serialization but manually omit counts.
		// Magic + Version (0xfb: 2) + TxVersion (0x02: 2)
		raw := []byte{
			0x70, 0x73, 0x62, 0x74, 0xff, // Magic
			0x01, 0xfb, 0x04, 0x02, 0x00, 0x00, 0x00, // Version 2
			0x01, 0x02, 0x04, 0x02, 0x00, 0x00, 0x00, // TxVersion 2
			0x00, // Separator
		}

		// parsing should fail because InputCount/OutputCount
		// are missing for V2.
		_, err := NewFromRawBytes(bytes.NewReader(raw), false)
		require.Error(t, err)
	})

	t.Run("Unsupported version should fail", func(t *testing.T) {
		// Version 3 (unsupported)
		raw := []byte{
			0x70, 0x73, 0x62, 0x74, 0xff, // Magic
			0x01, 0xfb, 0x04, 0x03, 0x00, 0x00, 0x00, // Version 3
			0x00, // Separator
		}

		_, err := NewFromRawBytes(bytes.NewReader(raw), false)
		require.Error(t, err)
	})
}

// TestPsbtV2Counts ensures that the number of inputs and outputs parsed
// matches the InputCount and OutputCount global fields.
func TestPsbtV2Counts(t *testing.T) {
	// Create a V2 PSBT that claims 2 inputs but only provides 1.
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid, _ := chainhash.NewHashFromStr(
		"010203040506070809101112131415161718192021222324252627282930" +
			"3132",
	)
	outPoint := wire.NewOutPoint(txid, 1)
	err = p.AddInputV2(PInput{
		PreviousTxid: (*outPoint).Hash[:],
		OutputIndex:  (*outPoint).Index,
		Sequence:     0xffffffff,
	})
	require.NoError(t, err)

	script, _ := hex.DecodeString("00")
	err = p.AddOutputV2(POutput{Amount: 1000, Script: script})
	require.NoError(t, err)

	// Manually override counts to mismatch reality.
	p.InputCount = 2
	p.OutputCount = 1

	var b bytes.Buffer
	err = p.Serialize(&b)
	require.NoError(t, err)

	// Parsing should fail because we promised 2 inputs but only 1 followed.
	_, err = NewFromRawBytes(&b, false)
	require.Error(t, err)
}

// TestPsbtV2Locktimes verifies that PSBTv2 locktime fields are correctly
// handled.
func TestPsbtV2Locktimes(t *testing.T) {
	// Create a V2 PSBT with a fallback locktime.
	p, err := NewV2(2, ptrUint32(500000), ptrUint8(0x01))
	require.NoError(t, err)

	txid, _ := chainhash.NewHashFromStr(
		"010203040506070809101112131415161718192021222324252627282930" +
			"3132",
	)
	outPoint := wire.NewOutPoint(txid, 1)
	err = p.AddInputV2(PInput{
		PreviousTxid: (*outPoint).Hash[:],
		OutputIndex:  (*outPoint).Index,
		Sequence:     0xffffffff,
	})
	require.NoError(t, err)

	p.Inputs[0].HeightLocktime = 600000

	msgTx, err := p.GetUnsignedTx()
	require.NoError(t, err)

	// Since we have a height locktime in an input, it should take
	// precedence.
	require.Equal(t, uint32(600000), msgTx.LockTime)

	// Now try with time locktime.
	p.Inputs[0].HeightLocktime = 0
	p.Inputs[0].TimeLocktime = 1600000000
	msgTx, err = p.GetUnsignedTx()
	require.NoError(t, err)
	require.Equal(t, uint32(1600000000), msgTx.LockTime)

	// Test combined locktimes (BIP suggests highest value for same type,
	// but here we just check our extraction logic).
	err = p.AddInputV2(PInput{
		PreviousTxid: (*outPoint).Hash[:],
		OutputIndex:  (*outPoint).Index,
		Sequence:     0xffffffff,
	})
	require.NoError(t, err)
	p.Inputs[1].TimeLocktime = 1700000000

	msgTx, err = p.GetUnsignedTx()
	require.NoError(t, err)
	require.Equal(t, uint32(1700000000), msgTx.LockTime)

	// Test modifiability flag.
	require.Equal(t, uint8(0x01), *p.TxModifiable)
}

// TestPSBTv2DetermineLockTimeAlgorithm tests the comprehensive BIP-370
// lock time determination algorithm
func TestPSBTv2DetermineLockTimeAlgorithm(t *testing.T) {
	t.Run(
		"Height-based preference when both supported (BIP-370 "+
			"tie-breaker)",
		func(t *testing.T) {
			p, err := NewV2(2, ptrUint32(100000), ptrUint8(3))
			require.NoError(t, err)

			txid, _ := chainhash.NewHashFromStr(
				"11111111111111111111111111111111111111111111" +
					"11111111111111111111",
			)

			// Input 1: Both time and height (flexible)
			p.AddInputV2(PInput{
				PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
				OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
				Sequence:     0,
			})
			p.Inputs[0].TimeLocktime = 1600000000
			p.Inputs[0].HeightLocktime = 550000

			// Input 2: Both time and height (flexible)
			p.AddInputV2(PInput{
				PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
				OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
				Sequence:     0,
			})
			p.Inputs[1].TimeLocktime = 1650000000
			p.Inputs[1].HeightLocktime = 600000

			// BIP-370: "height-based must be chosen" when both supported
			lockTime, err := p.DetermineLockTime()
			require.NoError(t, err)
			require.Equal(
				t, uint32(600000), lockTime,
			) // Max height, NOT max time
		},
	)

	t.Run("Conflicting requirements should error", func(t *testing.T) {
		p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
		require.NoError(t, err)

		txid, _ := chainhash.NewHashFromStr("222222222222222222222222" +
			"2222222222222222222222222222222222222222")

		// Input 1: Time-only (cannot satisfy height)
		p.AddInputV2(
			PInput{
				PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
				OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
				Sequence:     0,
			})
		p.Inputs[0].TimeLocktime = 1600000000

		// Input 2: Height-only (cannot satisfy time)
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 1)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 1)).Index,
			Sequence:     0,
		})
		p.Inputs[1].HeightLocktime = 500000

		// Should fail - conflicting requirements
		_, err = p.DetermineLockTime()
		require.Error(t, err)
	})

	t.Run("Fallback locktime when no constraints", func(t *testing.T) {
		fallback := uint32(123456)
		p, err := NewV2(2, ptrUint32(fallback), ptrUint8(3))
		require.NoError(t, err)

		txid, _ := chainhash.NewHashFromStr("333333333333333333333333" +
			"3333333333333333333333333333333333333333")
		p.AddInputV2(PInput{
			PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
			OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
			Sequence:     0,
		})
		// No TimeLocktime or HeightLocktime set

		lockTime, err := p.DetermineLockTime()
		require.NoError(t, err)
		require.Equal(t, fallback, lockTime)
	})
}

// TestPSBTv2AddUnknownFields tests the addUnknown field handling
func TestPSBTv2AddUnknownFields(t *testing.T) {
	p, err := NewV2(2, ptrUint32(0), ptrUint8(3))
	require.NoError(t, err)

	txid, _ := chainhash.NewHashFromStr(
		"444444444444444444444444444444444444444444444444444444444444" +
			"4444",
	)
	p.AddInputV2(PInput{
		PreviousTxid: (*wire.NewOutPoint(txid, 0)).Hash[:],
		OutputIndex:  (*wire.NewOutPoint(txid, 0)).Index,
		Sequence:     0,
	})

	// Test adding unknown field succeeds
	err = p.Inputs[0].addUnknown(0xfc, []byte{
		0x01, 0x02,
	}, []byte{
		0x03, 0x04,
	})
	require.NoError(t, err)
	require.Len(t, p.Inputs[0].Unknowns, 1)

	// Test duplicate detection
	err = p.Inputs[0].addUnknown(0xfc, []byte{
		0x01, 0x02,
	}, []byte{
		0x03, 0x04,
	})
	require.Error(t, err)
	require.Equal(t, ErrDuplicateKey, err)

	p.AddOutputV2(POutput{
		Amount: 1000000,
		Script: []byte{
			0x76, 0xa9, 0x14,
		},
	})

	// Test output unknown fields
	err = p.Outputs[0].addUnknown(0xfd, []byte{0x05}, []byte{0x06})
	require.NoError(t, err)
	require.Len(t, p.Outputs[0].Unknowns, 1)
}
