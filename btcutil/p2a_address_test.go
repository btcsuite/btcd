package btcutil_test

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
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
			addr, err := btcutil.NewAddressPayToAnchor(tt.net)
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

			// Verify the script address matches the P2A script:
			// OP_1 OP_DATA_2 0x4e73.
			wantScript := []byte{0x51, 0x02, 0x4e, 0x73}
			require.Equal(t, wantScript, addr.ScriptAddress())
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := btcutil.DecodeAddress(tt.address, tt.net)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Ensure the decoded address is of the correct P2A type.
			p2aAddr, ok := addr.(*btcutil.AddressPayToAnchor)
			require.True(t, ok, "expected *AddressPayToAnchor, got %T", addr)

			// Ensure round-trip encoding preserves the original address.
			require.Equal(t, tt.address, p2aAddr.EncodeAddress())
		})
	}
}

// TestNewAddressPayToAnchorNilNetwork tests that nil network returns error.
func TestNewAddressPayToAnchorNilNetwork(t *testing.T) {
	_, err := btcutil.NewAddressPayToAnchor(nil)
	require.Error(t, err)
}
