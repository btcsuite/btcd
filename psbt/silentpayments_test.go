package psbt

import (
	"bytes"
	crand "crypto/rand"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// TestSilentPaymentsPacket tests the serialization and deserialization of
// SilentPaymentShare data in a PSBT packet.
func TestSilentPaymentsPacket(t *testing.T) {
	randomPrivKey1, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	randomPrivKey2, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	randomPrivKey3, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	randomData1 := make([]byte, 64)
	_, err = crand.Read(randomData1)
	require.NoError(t, err)
	randomData2 := make([]byte, 64)
	_, err = crand.Read(randomData2)
	require.NoError(t, err)

	share1 := SilentPaymentShare{
		ScanKey: randomPrivKey1.PubKey().SerializeCompressed(),
		Share:   randomPrivKey2.PubKey().SerializeCompressed(),
	}
	share2 := SilentPaymentShare{
		ScanKey: randomPrivKey3.PubKey().SerializeCompressed(),
		Share:   randomPrivKey2.PubKey().SerializeCompressed(),
	}

	proof1 := SilentPaymentDLEQ{
		ScanKey: share1.ScanKey,
		Proof:   randomData1,
	}
	proof2 := SilentPaymentDLEQ{
		ScanKey: share2.ScanKey,
		Proof:   randomData2,
	}

	info := SilentPaymentInfo{
		ScanKey:  share1.ScanKey,
		SpendKey: share2.ScanKey,
	}
	label := uint32(1234567890)

	// Start with a valid packet from our test vectors (number 9, with the
	// PSBT_GLOBAL_XPUB field set).
	originalBase64 := validPsbtBase64[8]

	// Decode the packet.
	testPacket, err := NewFromRawBytes(
		strings.NewReader(originalBase64), true,
	)
	require.NoError(t, err)

	// We serialize our packet before making any changes to it, so we can
	// compare it to the one we get back after adding our test data.
	originalBytes := serialize(t, testPacket)

	// Now add our test data to the packet.
	testPacket.SilentPaymentShares = append(
		testPacket.SilentPaymentShares, share1,
	)
	testPacket.SilentPaymentDLEQs = append(
		testPacket.SilentPaymentDLEQs, proof1,
	)
	testPacket.Inputs[0].SilentPaymentShares = append(
		testPacket.Inputs[0].SilentPaymentShares, share2,
	)
	testPacket.Inputs[0].SilentPaymentDLEQs = append(
		testPacket.Inputs[0].SilentPaymentDLEQs, proof2,
	)
	testPacket.Outputs[0].SilentPaymentInfo = &info
	testPacket.Outputs[0].SilentPaymentLabel = &label

	// And do another full encoding/decoding round trip.
	newBytes := serialize(t, testPacket)
	newPacket, err := NewFromRawBytes(bytes.NewReader(newBytes), false)
	require.NoError(t, err)

	// Check that the packet we got back is the same as the one we got after
	// adding test data.
	require.Equal(t, testPacket, newPacket)

	// We want to make sure that our new data is much larger than the
	// original packet.
	require.Greater(t, len(newBytes), len(originalBytes))
}

func serialize(t *testing.T, packet *Packet) []byte {
	var buf bytes.Buffer
	err := packet.Serialize(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}

// TestReadSilentPaymentFieldsMalformed verifies that the silent payment field
// readers reject malformed inputs: wrong lengths and (for the pubkey-bearing
// fields) values that aren't valid points on the curve. The global and
// per-input shares/DLEQs share these readers, so this covers both scopes.
func TestReadSilentPaymentFieldsMalformed(t *testing.T) {
	t.Parallel()

	priv, _ := btcec.PrivKeyFromBytes([]byte{0x01})
	validKey := priv.PubKey().SerializeCompressed()
	validProof := make([]byte, 64)
	validInfo := append(append([]byte{}, validKey...), validKey...)

	// A correctly-sized but off-curve compressed public key (x >= p).
	badKey := append([]byte{0x02}, bytes.Repeat([]byte{0xff}, 32)...)

	t.Run("share", func(t *testing.T) {
		t.Parallel()

		_, err := ReadSilentPaymentShare(validKey, validKey)
		require.NoError(t, err)

		_, err = ReadSilentPaymentShare(validKey[:32], validKey)
		require.ErrorIs(t, err, ErrInvalidKeyData)
		_, err = ReadSilentPaymentShare(badKey, validKey)
		require.ErrorIs(t, err, ErrInvalidKeyData)

		_, err = ReadSilentPaymentShare(validKey, validKey[:32])
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
		_, err = ReadSilentPaymentShare(validKey, badKey)
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})

	t.Run("dleq", func(t *testing.T) {
		t.Parallel()

		_, err := ReadSilentPaymentDLEQ(validKey, validProof)
		require.NoError(t, err)

		_, err = ReadSilentPaymentDLEQ(validKey[:32], validProof)
		require.ErrorIs(t, err, ErrInvalidKeyData)
		_, err = ReadSilentPaymentDLEQ(badKey, validProof)
		require.ErrorIs(t, err, ErrInvalidKeyData)

		_, err = ReadSilentPaymentDLEQ(validKey, validProof[:63])
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})

	t.Run("info", func(t *testing.T) {
		t.Parallel()

		_, err := ReadSilentPaymentInfo(validInfo)
		require.NoError(t, err)

		_, err = ReadSilentPaymentInfo(validInfo[:65])
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)

		badScan := append(append([]byte{}, badKey...), validKey...)
		badSpend := append(append([]byte{}, validKey...), badKey...)
		_, err = ReadSilentPaymentInfo(badScan)
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
		_, err = ReadSilentPaymentInfo(badSpend)
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})

	t.Run("label", func(t *testing.T) {
		t.Parallel()

		label, err := ReadSilentPaymentLabel([]byte{1, 0, 0, 0})
		require.NoError(t, err)
		require.Equal(t, uint32(1), label)

		_, err = ReadSilentPaymentLabel([]byte{0, 0, 0})
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})
}

// TestSilentPaymentDuplicateKeys verifies that decoding a PSBT with two silent
// payment shares or DLEQ proofs that share the same scan key is rejected, at
// both the global and per-input scopes.
func TestSilentPaymentDuplicateKeys(t *testing.T) {
	t.Parallel()

	priv, _ := btcec.PrivKeyFromBytes([]byte{0x01})
	scanKey := priv.PubKey().SerializeCompressed()
	share := SilentPaymentShare{ScanKey: scanKey, Share: scanKey}
	dleq := SilentPaymentDLEQ{ScanKey: scanKey, Proof: make([]byte, 64)}

	dupShares := []SilentPaymentShare{share, share}
	dupDLEQs := []SilentPaymentDLEQ{dleq, dleq}

	newPacket := func(t *testing.T) *Packet {
		t.Helper()

		pkt, err := NewFromRawBytes(
			strings.NewReader(validPsbtBase64[8]), true,
		)
		require.NoError(t, err)

		return pkt
	}

	// assertDupErr serializes the packet and asserts that decoding it again
	// fails with ErrDuplicateKey.
	assertDupErr := func(t *testing.T, pkt *Packet) {
		t.Helper()

		raw := serialize(t, pkt)
		_, err := NewFromRawBytes(bytes.NewReader(raw), false)
		require.ErrorIs(t, err, ErrDuplicateKey)
	}

	t.Run("global shares", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.SilentPaymentShares = dupShares
		assertDupErr(t, pkt)
	})

	t.Run("global DLEQs", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.SilentPaymentDLEQs = dupDLEQs
		assertDupErr(t, pkt)
	})

	t.Run("input shares", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.Inputs[0].SilentPaymentShares = dupShares
		assertDupErr(t, pkt)
	})

	t.Run("input DLEQs", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.Inputs[0].SilentPaymentDLEQs = dupDLEQs
		assertDupErr(t, pkt)
	})
}
