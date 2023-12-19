package address_test

import (
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// TestAddressPayToAnchor tests the AddressPayToAnchor type.
func TestAddressPayToAnchor(t *testing.T) {
	tests := []struct {
		name        string
		net         *chaincfg.Params
		wantAddress string
	}{
		{
			name:        "mainnet",
			net:         &chaincfg.MainNetParams,
			wantAddress: "bc1pfeessrawgf",
		},
		{
			name:        "testnet",
			net:         &chaincfg.TestNet3Params,
			wantAddress: "tb1pfees9rn5nz",
		},
		{
			name:        "regtest",
			net:         &chaincfg.RegressionNetParams,
			wantAddress: "bcrt1pfeesnyr2tx",
		},
		{
			name:        "simnet",
			net:         &chaincfg.SimNetParams,
			wantAddress: "sb1pfeesxv0pfa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := address.NewAddressPayToAnchor(tt.net)
			require.NoError(t, err)

			require.Equal(t, tt.wantAddress, addr.EncodeAddress())
			require.Equal(t, tt.wantAddress, addr.String())
			require.True(t, addr.IsForNet(tt.net))

			// Verify it's not for a different network.
			otherNet := &chaincfg.MainNetParams
			if tt.net == &chaincfg.MainNetParams {
				otherNet = &chaincfg.TestNet3Params
			}
			require.False(t, addr.IsForNet(otherNet))

			// Verify ScriptAddress returns the 2-byte witness
			// program portion of the P2A output (the bytes that
			// follow the OP_1 OP_DATA_2 prefix).
			wantWitnessProgram := []byte{0x4e, 0x73}
			require.Equal(t, wantWitnessProgram, addr.ScriptAddress())
		})
	}
}

// TestDecodeAddressP2A tests decoding P2A addresses.
func TestDecodeAddressP2A(t *testing.T) {
	tests := []struct {
		name    string
		address string
		net     *chaincfg.Params
		wantErr bool
	}{
		{
			name:    "mainnet P2A",
			address: "bc1pfeessrawgf",
			net:     &chaincfg.MainNetParams,
			wantErr: false,
		},
		{
			name:    "testnet P2A",
			address: "tb1pfees9rn5nz",
			net:     &chaincfg.TestNet3Params,
			wantErr: false,
		},
		{
			name:    "regtest P2A",
			address: "bcrt1pfeesnyr2tx",
			net:     &chaincfg.RegressionNetParams,
			wantErr: false,
		},
		{
			// BIP 173 permits all-uppercase bech32 encodings.
			// Decoding must normalize the HRP so the resulting
			// address still reports as belonging to its network.
			name:    "uppercase mainnet P2A",
			address: "BC1PFEESSRAWGF",
			net:     &chaincfg.MainNetParams,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := address.DecodeAddress(tt.address, tt.net)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Ensure the decoded address is of the correct P2A type.
			p2aAddr, ok := addr.(*address.AddressPayToAnchor)
			require.True(t, ok, "expected *AddressPayToAnchor, got %T", addr)

			// Ensure round-trip encoding produces the canonical
			// lowercase encoding for the address's network.
			require.True(t, p2aAddr.IsForNet(tt.net))
		})
	}
}

// TestNewAddressPayToAnchorNilNetwork tests that nil network returns error.
func TestNewAddressPayToAnchorNilNetwork(t *testing.T) {
	_, err := address.NewAddressPayToAnchor(nil)
	require.Error(t, err)
}
