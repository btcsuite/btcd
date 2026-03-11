package txscript

import (
	"encoding/hex"
	"testing"
)

// TestIsPayToAnchorScript tests the IsPayToAnchorScript function.
func TestIsPayToAnchorScript(t *testing.T) {
	tests := []struct {
		name   string
		script []byte
		want   bool
	}{
		{
			name:   "valid P2A script",
			script: PayToAnchorScript,
			want:   true,
		},
		{
			name:   "invalid - wrong data",
			script: []byte{OP_1, OP_DATA_2, 0x4e, 0x74},
			want:   false,
		},
		{
			name:   "invalid - wrong length",
			script: []byte{OP_1, OP_DATA_2, 0x4e},
			want:   false,
		},
		{
			name:   "invalid - wrong version",
			script: []byte{OP_0, OP_DATA_2, 0x4e, 0x73},
			want:   false,
		},
		{
			name: "invalid - P2WPKH",
			script: []byte{
				OP_0, OP_DATA_20, 0x00, 0x01, 0x02, 0x03, 0x04,
				0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c,
				0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13,
			},
			want: false,
		},
		{
			name:   "empty script",
			script: []byte{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPayToAnchorScript(tt.script)
			if got != tt.want {
				t.Errorf(
					"IsPayToAnchorScript() = %v, want %v",
					got, tt.want,
				)
			}
		})
	}
}

// TestGetScriptClassP2A tests that P2A scripts are properly classified.
func TestGetScriptClassP2A(t *testing.T) {
	p2aScript := PayToAnchorScript

	class := GetScriptClass(p2aScript)
	if class != PayToAnchorTy {
		t.Errorf("GetScriptClass() = %v, want %v", class, PayToAnchorTy)
	}

	if class.String() != "anchor" {
		t.Errorf("PayToAnchorTy.String() = %v, want 'anchor'",
			class.String())
	}

	// Test that regular taproot scripts are still recognized as
	// WitnessV1TaprootTy.
	taprootScript := []byte{
		OP_1, OP_DATA_32,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
		0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
		0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
	}
	class = GetScriptClass(taprootScript)
	if class != WitnessV1TaprootTy {
		t.Errorf("GetScriptClass(taproot) = %v, want %v",
			class, WitnessV1TaprootTy)
	}
}

// TestExtractPkScriptAddrs_P2A tests ExtractPkScriptAddrs with P2A scripts.
func TestExtractPkScriptAddrs_P2A(t *testing.T) {
	p2aScript := PayToAnchorScript

	class, addrs, reqSigs, err := ExtractPkScriptAddrs(p2aScript, nil)
	if err != nil {
		t.Fatalf("ExtractPkScriptAddrs() error = %v", err)
	}

	if class != PayToAnchorTy {
		t.Errorf("ExtractPkScriptAddrs() class = %v, want %v",
			class, PayToAnchorTy)
	}

	if len(addrs) != 0 {
		t.Errorf("ExtractPkScriptAddrs() addrs = %v, want empty", addrs)
	}

	if reqSigs != 0 {
		t.Errorf("ExtractPkScriptAddrs() reqSigs = %v, want 0", reqSigs)
	}
}

// TestExpectedInputs_P2A tests that P2A scripts require no inputs.
func TestExpectedInputs_P2A(t *testing.T) {
	p2aScript := PayToAnchorScript

	inputs := expectedInputs(p2aScript, PayToAnchorTy)
	if inputs != 0 {
		t.Errorf("expectedInputs() = %v, want 0", inputs)
	}
}

// TestP2AScriptHex tests P2A script in hex format.
func TestP2AScriptHex(t *testing.T) {
	// The P2A script in hex should be "51024e73".
	expectedHex := "51024e73"
	p2aScript := PayToAnchorScript

	actualHex := hex.EncodeToString(p2aScript)
	if actualHex != expectedHex {
		t.Errorf("P2A script hex = %v, want %v", actualHex, expectedHex)
	}

	// Test decoding.
	decoded, err := hex.DecodeString(expectedHex)
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	if !IsPayToAnchorScript(decoded) {
		t.Error("Decoded hex not recognized as P2A script")
	}
}
