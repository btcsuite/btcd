package address

import (
	"testing"
)

// TestSegWitAddressBIP350Vectors runs a subset of the official BIP-173/BIP-350
// reference segwit address vectors against decodeSegWitAddress, which implements
// the reference decode() function from those BIPs. It specifically guards the
// rule that witness versions 1 through 16 MUST use the bech32m encoding (BIP-350
// line "Addresses for segregated witness outputs version 1 through 16 use
// Bech32m"), not just version 1.
func TestSegWitAddressBIP350Vectors(t *testing.T) {
	// Valid Bech32m vectors (BIP-350). These MUST decode without error. The HRP
	// is validated at a higher layer; here we only validate segwit-encoding
	// rules, so the HRP value is irrelevant.
	valid := []string{
		"bc1pw508d6qejxtdg4y5r3zarvary0c5xw7kw508d6qejxtdg4y5r3zarvary0c5xw7kt5nd6y",
		"BC1SW50QGDZ25J",
		"bc1zw508d6qejxtdg4y5r3zarvaryvaxxpcs",
		"bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0",
		// Valid Bech32 v0 vectors.
		"BC1QW508D6QEJXTDG4Y5R3ZARVARY0C5XW7KV8F3T4",
		"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7",
	}
	for _, addr := range valid {
		if _, _, err := decodeSegWitAddress(addr); err != nil {
			t.Errorf("BIP350 valid vector rejected: %q -> %v", addr, err)
		}
	}

	// Invalid vectors from the BIP-350 INVALID_ADDRESS list whose defect lies in
	// the segwit-encoding rules (not merely the HRP). Each MUST be rejected.
	invalid := []struct {
		name string
		addr string
	}{
		// Witness version 1, Bech32 instead of Bech32m.
		{"v1-bech32-not-bech32m", "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqh2y7hd"},
		// Witness version 2, Bech32 instead of Bech32m. This is the vector that
		// previously decoded successfully because only v0/v1 were checked.
		{"v2-bech32-not-bech32m", "tb1z0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqglt7rf"},
		// Witness version 16, Bech32 instead of Bech32m.
		{"v16-bech32-not-bech32m", "BC1S0XLXVLHEMJA6C4DQV22UAPCTQUPFHLXM9H8Z3K2E72Q4K9HCZ7VQ54WELL"},
		// Witness version 0, Bech32m instead of Bech32.
		{"v0-bech32m-not-bech32", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kemeawh"},
		// Invalid program length (1 byte).
		{"program-length-1", "bc1pw5dgrnzv"},
		// Invalid program length (41 bytes).
		{"program-length-41", "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7v8n0nx0muaewav253zgeav"},
		// Invalid program length for witness version 0 (per BIP-141).
		{"v0-program-length-16", "BC1QR508D6QEJXTDG4Y5R3ZARVARYV98GJ9P"},
	}
	for _, tc := range invalid {
		if _, _, err := decodeSegWitAddress(tc.addr); err == nil {
			t.Errorf("BIP350 invalid vector %s accepted: %q (expected error)",
				tc.name, tc.addr)
		}
	}
}
