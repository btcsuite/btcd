package btcutil_test

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
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
			if err != nil {
				t.Fatalf("NewAddressPayToAnchor() error = %v",
					err)
			}

			got := addr.EncodeAddress()
			if got != tt.wantAddress {
				t.Errorf("EncodeAddress() = %v, want %v",
					got, tt.wantAddress)
			}

			if addr.String() != tt.wantAddress {
				t.Errorf("String() = %v, want %v",
					addr.String(), tt.wantAddress)
			}

			if !addr.IsForNet(tt.net) {
				t.Errorf("IsForNet() = false, want true")
			}

			otherNet := &chaincfg.MainNetParams
			if tt.net == &chaincfg.MainNetParams {
				otherNet = &chaincfg.TestNet3Params
			}
			if addr.IsForNet(otherNet) {
				t.Errorf("IsForNet(other) = true, want false")
			}

			expectedScript := []byte{0x51, 0x02, 0x4e, 0x73}
			script := addr.ScriptAddress()
			if !bytes.Equal(script, expectedScript) {

				t.Errorf("ScriptAddress() = %x, want %x",
					script, expectedScript)
			}
		})
	}
}

// TestDecodeAddress_P2A tests decoding P2A addresses.
func TestDecodeAddress_P2A(t *testing.T) {
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
			// Address will decode but won't be for the right
			// network.
			name:    "wrong network",
			address: "bc1pfeessrawgf",
			net:     &chaincfg.TestNet3Params,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := btcutil.DecodeAddress(tt.address, tt.net)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DecodeAddress() error = %v, "+
					"wantErr %v", err, tt.wantErr)
			}

			if err == nil {
				// Ensure the decoded address is of the correct
				// P2A type.
				_, ok := addr.(*btcutil.AddressPayToAnchor)
				if !ok {
					t.Errorf("DecodeAddress() returned "+
						"%T, want *AddressPayToAnchor",
						addr)
				}

				// Ensure round-trip encoding preserves the
				// original address.
				if addr.EncodeAddress() != tt.address {
					t.Errorf("Round-trip encoding "+
						"failed: got %v, want %v",
						addr.EncodeAddress(),
						tt.address)
				}
			}
		})
	}
}

// TestNewAddressPayToAnchor_NilNetwork tests that nil network returns error.
func TestNewAddressPayToAnchor_NilNetwork(t *testing.T) {
	_, err := btcutil.NewAddressPayToAnchor(nil)
	if err == nil {
		t.Error("NewAddressPayToAnchor(nil) should return error")
	}
}
