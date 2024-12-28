package psbt

import (
	"bytes"
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

	share1 := SilentPaymentShare{
		ScanKey: randomPrivKey1.PubKey().SerializeCompressed(),
		Share:   randomPrivKey2.PubKey().SerializeCompressed(),
	}
	share2 := SilentPaymentShare{
		ScanKey: randomPrivKey3.PubKey().SerializeCompressed(),
		Share:   randomPrivKey2.PubKey().SerializeCompressed(),
	}

	// Start with a valid packet from our test vectors (number 9, with the
	// PSBT_GLOBAL_XPUB field set).
	originalBase64 := validPsbtBase64[8]

	// Decode the packet.
	testPacket, err := NewFromRawBytes(
		strings.NewReader(originalBase64), true,
	)
	require.NoError(t, err)

	// Now add our test data to the packet.
	testPacket.SilentPaymentShares = append(
		testPacket.SilentPaymentShares, share1,
	)
	testPacket.Inputs[0].SilentPaymentShares = append(
		testPacket.Inputs[0].SilentPaymentShares, share2,
	)

	// And do another full encoding/decoding round trip.
	newBytes := serialize(t, testPacket)
	newPacket, err := NewFromRawBytes(bytes.NewReader(newBytes), false)
	require.NoError(t, err)

	// Check that the packet we got back is the same as the one we got after
	// adding test data.
	require.Equal(t, testPacket, newPacket)
}

func serialize(t *testing.T, packet *Packet) []byte {
	var buf bytes.Buffer
	err := packet.Serialize(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}
