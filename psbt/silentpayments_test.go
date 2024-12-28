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
