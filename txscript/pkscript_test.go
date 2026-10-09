package txscript

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/wire/v2"
)

// TestParsePkScript ensures that the supported script types can be parsed
// correctly and re-derived into its raw byte representation.
func TestParsePkScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pkScript []byte
		valid    bool
	}{
		{
			name:     "empty output script",
			pkScript: []byte{},
			valid:    false,
		},
		{
			name: "valid P2PKH",
			pkScript: []byte{
				// OP_DUP
				0x76,
				// OP_HASH160
				0xa9,
				// OP_DATA_20
				0x14,
				// <20-byte pubkey hash>
				0xf0, 0x7a, 0xb8, 0xce, 0x72, 0xda, 0x4e, 0x76,
				0x0b, 0x74, 0x7d, 0x48, 0xd6, 0x65, 0xec, 0x96,
				0xad, 0xf0, 0x24, 0xf5,
				// OP_EQUALVERIFY
				0x88,
				// OP_CHECKSIG
				0xac,
			},
			valid: true,
		},
		// Invalid P2PKH - same as above but replaced OP_CHECKSIG with
		// OP_CHECKSIGVERIFY.
		{
			name: "invalid P2PKH",
			pkScript: []byte{
				// OP_DUP
				0x76,
				// OP_HASH160
				0xa9,
				// OP_DATA_20
				0x14,
				// <20-byte pubkey hash>
				0xf0, 0x7a, 0xb8, 0xce, 0x72, 0xda, 0x4e, 0x76,
				0x0b, 0x74, 0x7d, 0x48, 0xd6, 0x65, 0xec, 0x96,
				0xad, 0xf0, 0x24, 0xf5,
				// OP_EQUALVERIFY
				0x88,
				// OP_CHECKSIGVERIFY
				0xad,
			},
			valid: false,
		},
		{
			name: "valid P2SH",
			pkScript: []byte{
				// OP_HASH160
				0xA9,
				// OP_DATA_20
				0x14,
				// <20-byte script hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96, 0xcd,
				// OP_EQUAL
				0x87,
			},
			valid: true,
		},
		// Invalid P2SH - same as above but replaced OP_EQUAL with
		// OP_EQUALVERIFY.
		{
			name: "invalid P2SH",
			pkScript: []byte{
				// OP_HASH160
				0xA9,
				// OP_DATA_20
				0x14,
				// <20-byte script hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96, 0xcd,
				// OP_EQUALVERIFY
				0x88,
			},
			valid: false,
		},
		{
			name: "valid v0 P2WSH",
			pkScript: []byte{
				// OP_0
				0x00,
				// OP_DATA_32
				0x20,
				// <32-byte script hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
			},
			valid: true,
		},
		// Invalid v0 P2WSH - same as above but missing one byte.
		{
			name: "invalid v0 P2WSH",
			pkScript: []byte{
				// OP_0
				0x00,
				// OP_DATA_32
				0x20,
				// <32-byte script hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96, 0xcd,
				0x06, 0xf6, 0x96, 0xcd, 0x06, 0xf6, 0x96,
			},
			valid: false,
		},
		{
			name: "valid v0 P2WPKH",
			pkScript: []byte{
				// OP_0
				0x00,
				// OP_DATA_20
				0x14,
				// <20-byte pubkey hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96, 0xcd,
			},
			valid: true,
		},
		// Invalid v0 P2WPKH - same as above but missing one byte.
		{
			name: "invalid v0 P2WPKH",
			pkScript: []byte{
				// OP_0
				0x00,
				// OP_DATA_20
				0x14,
				// <20-byte pubkey hash>
				0xec, 0x6f, 0x7a, 0x5a, 0xa8, 0xf2, 0xb1, 0x0c,
				0xa5, 0x15, 0x04, 0x52, 0x3a, 0x60, 0xd4, 0x03,
				0x06, 0xf6, 0x96,
			},
			valid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pkScript, err := ParsePkScript(test.pkScript)
			switch {
			case err != nil && test.valid:
				t.Fatalf("unable to parse valid pkScript=%x: %v",
					test.pkScript, err)
			case err == nil && !test.valid:
				t.Fatalf("successfully parsed invalid pkScript=%x",
					test.pkScript)
			}

			if !test.valid {
				return
			}

			if !bytes.Equal(pkScript.Script(), test.pkScript) {
				t.Fatalf("expected to re-derive pkScript=%x, "+
					"got pkScript=%x", test.pkScript,
					pkScript.Script())
			}
		})
	}
}

// TestComputePkScript ensures that we can correctly re-derive an output's
// pkScript by looking at the input's signature script/witness attempting to
// spend it.
func TestComputePkScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sigScript []byte
		witness   wire.TxWitness
		class     ScriptClass
		pkScript  []byte
	}{
		{
			name:      "empty sigScript and witness",
			sigScript: nil,
			witness:   nil,
			class:     NonStandardTy,
			pkScript:  nil,
		},
		{
			name: "P2PKH sigScript",
			sigScript: []byte{
				// OP_DATA_73,
				0x49,
				// <73-byte sig>
				0x30, 0x44, 0x02, 0x20, 0x65, 0x92, 0xd8, 0x8e,
				0x1d, 0x0a, 0x4a, 0x3c, 0xc5, 0x9f, 0x92, 0xae,
				0xfe, 0x62, 0x54, 0x74, 0xa9, 0x4d, 0x13, 0xa5,
				0x9f, 0x84, 0x97, 0x78, 0xfc, 0xe7, 0xdf, 0x4b,
				0xe0, 0xc2, 0x28, 0xd8, 0x02, 0x20, 0x2d, 0xea,
				0x36, 0x96, 0x19, 0x1f, 0xb7, 0x00, 0xc5, 0xa7,
				0x7e, 0x22, 0xd9, 0xfb, 0x6b, 0x42, 0x67, 0x42,
				0xa4, 0x2c, 0xac, 0xdb, 0x74, 0xa2, 0x7c, 0x43,
				0xcd, 0x89, 0xa0, 0xf9, 0x44, 0x54, 0x12, 0x74,
				0x01,
				// OP_DATA_33
				0x21,
				// <33-byte compressed pubkey>
				0x02, 0x7d, 0x56, 0x12, 0x09, 0x75, 0x31, 0xc2,
				0x17, 0xfd, 0xd4, 0xd2, 0xe1, 0x7a, 0x35, 0x4b,
				0x17, 0xf2, 0x7a, 0xef, 0x30, 0x9f, 0xb2, 0x7f,
				0x1f, 0x1f, 0x7b, 0x73, 0x7d, 0x9a, 0x24, 0x49,
				0x90,
			},
			witness: nil,
			class:   PubKeyHashTy,
			pkScript: []byte{
				// OP_DUP
				0x76,
				// OP_HASH160
				0xa9,
				// OP_DATA_20
				0x14,
				// <20-byte pubkey hash>
				0xf0, 0x7a, 0xb8, 0xce, 0x72, 0xda, 0x4e, 0x76,
				0x0b, 0x74, 0x7d, 0x48, 0xd6, 0x65, 0xec, 0x96,
				0xad, 0xf0, 0x24, 0xf5,
				// OP_EQUALVERIFY
				0x88,
				// OP_CHECKSIG
				0xac,
			},
		},
		{
			name: "NP2WPKH sigScript",
			// Since this is a NP2PKH output, the sigScript is a
			// data push of a serialized v0 P2WPKH script.
			sigScript: []byte{
				// OP_DATA_16
				0x16,
				// <22-byte redeem script>
				0x00, 0x14, 0x1d, 0x7c, 0xd6, 0xc7, 0x5c, 0x2e,
				0x86, 0xf4, 0xcb, 0xf9, 0x8e, 0xae, 0xd2, 0x21,
				0xb3, 0x0b, 0xd9, 0xa0, 0xb9, 0x28,
			},
			// NP2PKH outputs include a witness, but it is not
			// needed to reconstruct the pkScript.
			witness: nil,
			class:   ScriptHashTy,
			pkScript: []byte{
				// OP_HASH160
				0xa9,
				// OP_DATA_20
				0x14,
				// <20-byte script hash>
				0x90, 0x1c, 0x86, 0x94, 0xc0, 0x3f, 0xaf, 0xd5,
				0x52, 0x28, 0x10, 0xe0, 0x33, 0x0f, 0x26, 0xe6,
				0x7a, 0x85, 0x33, 0xcd,
				// OP_EQUAL
				0x87,
			},
		},
		{
			name: "P2SH sigScript",
			sigScript: []byte{
				0x00, 0x49, 0x30, 0x46, 0x02, 0x21, 0x00, 0xda,
				0xe6, 0xb6, 0x14, 0x1b, 0xa7, 0x24, 0x4f, 0x54,
				0x62, 0xb6, 0x2a, 0x3b, 0x27, 0x59, 0xde, 0xe4,
				0x46, 0x76, 0x19, 0x4e, 0x6c, 0x56, 0x8d, 0x5b,
				0x1c, 0xda, 0x96, 0x2d, 0x4f, 0x6d, 0x79, 0x02,
				0x21, 0x00, 0xa6, 0x6f, 0x60, 0x34, 0x46, 0x09,
				0x0a, 0x22, 0x3c, 0xec, 0x30, 0x33, 0xd9, 0x86,
				0x24, 0xd2, 0x73, 0xa8, 0x91, 0x55, 0xa5, 0xe6,
				0x96, 0x66, 0x0b, 0x6a, 0x50, 0xa3, 0x46, 0x45,
				0xbb, 0x67, 0x01, 0x48, 0x30, 0x45, 0x02, 0x21,
				0x00, 0xe2, 0x73, 0x49, 0xdb, 0x93, 0x82, 0xe1,
				0xf8, 0x8d, 0xae, 0x97, 0x5c, 0x71, 0x19, 0xb7,
				0x79, 0xb6, 0xda, 0x43, 0xa8, 0x4f, 0x16, 0x05,
				0x87, 0x11, 0x9f, 0xe8, 0x12, 0x1d, 0x85, 0xae,
				0xee, 0x02, 0x20, 0x6f, 0x23, 0x2d, 0x0a, 0x7b,
				0x4b, 0xfa, 0xcd, 0x56, 0xa0, 0x72, 0xcc, 0x2a,
				0x44, 0x81, 0x31, 0xd1, 0x0d, 0x73, 0x35, 0xf9,
				0xa7, 0x54, 0x8b, 0xee, 0x1f, 0x70, 0xc5, 0x71,
				0x0b, 0x37, 0x9e, 0x01, 0x47, 0x52, 0x21, 0x03,
				0xab, 0x11, 0x5d, 0xa6, 0xdf, 0x4f, 0x54, 0x0b,
				0xd6, 0xc9, 0xc4, 0xbe, 0x5f, 0xdd, 0xcc, 0x24,
				0x58, 0x8e, 0x7c, 0x2c, 0xaf, 0x13, 0x82, 0x28,
				0xdd, 0x0f, 0xce, 0x29, 0xfd, 0x65, 0xb8, 0x7c,
				0x21, 0x02, 0x15, 0xe8, 0xb7, 0xbf, 0xfe, 0x8d,
				0x9b, 0xbd, 0x45, 0x81, 0xf9, 0xc3, 0xb6, 0xf1,
				0x6d, 0x67, 0x08, 0x36, 0xc3, 0x0b, 0xb2, 0xe0,
				0x3e, 0xfd, 0x9d, 0x41, 0x03, 0xb5, 0x59, 0xeb,
				0x67, 0xcd, 0x52, 0xae,
			},
			witness: nil,
			class:   ScriptHashTy,
			pkScript: []byte{
				// OP_HASH160
				0xA9,
				// OP_DATA_20
				0x14,
				// <20-byte script hash>
				0x12, 0xd6, 0x9c, 0xd3, 0x38, 0xa3, 0x8d, 0x0d,
				0x77, 0x83, 0xcf, 0x22, 0x64, 0x97, 0x63, 0x3d,
				0x3c, 0x20, 0x79, 0xea,
				// OP_EQUAL
				0x87,
			},
		},
		// Invalid P2SH (non push-data only script).
		{
			name:      "invalid P2SH sigScript",
			sigScript: []byte{0x6b, 0x65, 0x6b}, // kek
			witness:   nil,
			class:     NonStandardTy,
			pkScript:  nil,
		},
		{
			name:      "P2WSH witness",
			sigScript: nil,
			witness: [][]byte{
				{},
				// Witness script.
				{
					0x21, 0x03, 0x82, 0x62, 0xa6, 0xc6,
					0xce, 0xc9, 0x3c, 0x2d, 0x3e, 0xcd,
					0x6c, 0x60, 0x72, 0xef, 0xea, 0x86,
					0xd0, 0x2f, 0xf8, 0xe3, 0x32, 0x8b,
					0xbd, 0x02, 0x42, 0xb2, 0x0a, 0xf3,
					0x42, 0x59, 0x90, 0xac, 0xac,
				},
			},
			class: WitnessV0ScriptHashTy,
			pkScript: []byte{
				// OP_0
				0x00,
				// OP_DATA_32
				0x20,
				// <32-byte script hash>
				0x01, 0xd5, 0xd9, 0x2e, 0xff, 0xa6, 0xff, 0xba,
				0x3e, 0xfa, 0x37, 0x9f, 0x98, 0x30, 0xd0, 0xf7,
				0x56, 0x18, 0xb1, 0x33, 0x93, 0x82, 0x71, 0x52,
				0xd2, 0x6e, 0x43, 0x09, 0x00, 0x0e, 0x88, 0xb1,
			},
		},
		{
			name:      "P2WPKH witness",
			sigScript: nil,
			witness: [][]byte{
				// Signature is not needed to re-derive the
				// pkScript.
				{},
				// Compressed pubkey.
				{
					0x03, 0x82, 0x62, 0xa6, 0xc6, 0xce,
					0xc9, 0x3c, 0x2d, 0x3e, 0xcd, 0x6c,
					0x60, 0x72, 0xef, 0xea, 0x86, 0xd0,
					0x2f, 0xf8, 0xe3, 0x32, 0x8b, 0xbd,
					0x02, 0x42, 0xb2, 0x0a, 0xf3, 0x42,
					0x59, 0x90, 0xac,
				},
			},
			class: WitnessV0PubKeyHashTy,
			pkScript: []byte{
				// OP_0
				0x00,
				// OP_DATA_20
				0x14,
				// <20-byte pubkey hash>
				0x1d, 0x7c, 0xd6, 0xc7, 0x5c, 0x2e, 0x86, 0xf4,
				0xcb, 0xf9, 0x8e, 0xae, 0xd2, 0x21, 0xb3, 0x0b,
				0xd9, 0xa0, 0xb9, 0x28,
			},
		},
		// Invalid v0 P2WPKH - same as above but missing a byte on the
		// public key.
		{
			name:      "invalid P2WPKH witness",
			sigScript: nil,
			witness: [][]byte{
				// Signature is not needed to re-derive the
				// pkScript.
				{},
				// Malformed compressed pubkey.
				{
					0x03, 0x82, 0x62, 0xa6, 0xc6, 0xce,
					0xc9, 0x3c, 0x2d, 0x3e, 0xcd, 0x6c,
					0x60, 0x72, 0xef, 0xea, 0x86, 0xd0,
					0x2f, 0xf8, 0xe3, 0x32, 0x8b, 0xbd,
					0x02, 0x42, 0xb2, 0x0a, 0xf3, 0x42,
					0x59, 0x90,
				},
			},
			class:    WitnessV0PubKeyHashTy,
			pkScript: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid := test.pkScript != nil
			pkScript, err := ComputePkScript(
				test.sigScript, test.witness,
			)
			if err != nil && valid {
				t.Fatalf("unable to compute pkScript: %v", err)
			}

			if !valid {
				return
			}

			if pkScript.Class() != test.class {
				t.Fatalf("%s: expected pkScript of type %v, got %v",
					test.name, test.class, pkScript.Class())
			}
			if !bytes.Equal(pkScript.Script(), test.pkScript) {
				t.Fatalf("%s: expected pkScript=%x, got pkScript=%x",
					test.name, test.pkScript, pkScript.Script())
			}
		})
	}
}

// hexBytes decodes the concatenation of the given hex strings.
func hexBytes(parts ...string) []byte {
	b, err := hex.DecodeString(strings.Join(parts, ""))
	if err != nil {
		panic(err)
	}
	return b
}

// TestComputePkScriptTaproot ensures that the pkScript of a P2TR output is
// derived from a script path spend, and that a key path spend, which reveals
// no script, is refused.
func TestComputePkScriptTaproot(t *testing.T) {
	t.Parallel()

	// The script path rows are the leaves of the BIP 341 wallet test
	// vectors. A witness is the leaf script and its control block.
	scriptPaths := []struct {
		name         string
		script       []byte
		controlBlock []byte
		pkScript     []byte
	}{
		{
			name: "BIP341 vector 1 leaf 0",
			script: hexBytes(
				"20d85a959b0290bf19bb89ed43c916be835475d013da",
				"4b362117393e25a48229b8ac",
			),
			controlBlock: hexBytes(
				"c1187791b6f712a8ea41c8ecdd0ee77fab3e85263b37",
				"e1ec18a3651926b3a6cf27",
			),
			pkScript: hexBytes(
				"5120147c9c57132f6e7ecddba9800bb0c4449251c92a",
				"1e60371ee77557b6620f3ea3",
			),
		},
		{
			name: "BIP341 vector 2 leaf 0",
			script: hexBytes(
				"20b617298552a72ade070667e86ca63b8f5789a9fe87",
				"31ef91202a91c9f3459007ac",
			),
			controlBlock: hexBytes(
				"c093478e9488f956df2396be2ce6c5cced75f900dfa1",
				"8e7dabd2428aae78451820",
			),
			pkScript: hexBytes(
				"5120e4d810fd50586274face62b8a807eb9719cef49c",
				"04177cc6b76a9a4251d5450e",
			),
		},
		{
			name: "BIP341 vector 3 leaf 0",
			script: hexBytes(
				"20387671353e273264c495656e27e39ba899ea8fee3b",
				"b69fb2a680e22093447d48ac",
			),
			controlBlock: hexBytes(
				"c0ee4fe085983462a184015d1f782d6a5f8b9c2b6013",
				"0aff050ce221ecf3786592f224a923cd0021ab202ab1",
				"39cc56802ddb92dcfc172b9212261a539df79a112a",
			),
			pkScript: hexBytes(
				"5120712447206d7a5238acc7ff53fbe94a3b64539ad2",
				"91c7cdbc490b7577e4b17df5",
			),
		},
		{
			name:   "BIP341 vector 3 leaf 1",
			script: hexBytes("06424950333431"),
			controlBlock: hexBytes(
				"faee4fe085983462a184015d1f782d6a5f8b9c2b6013",
				"0aff050ce221ecf37865928ad69ec7cf41c2a4001fd1",
				"f738bf1e505ce2277acdcaa63fe4765192497f47a7",
			),
			pkScript: hexBytes(
				"5120712447206d7a5238acc7ff53fbe94a3b64539ad2",
				"91c7cdbc490b7577e4b17df5",
			),
		},
		{
			name: "BIP341 vector 4 leaf 0",
			script: hexBytes(
				"2044b178d64c32c4a05cc4f4d1407268f764c940d20c",
				"e97abfd44db5c3592b72fdac",
			),
			controlBlock: hexBytes(
				"c1f9f400803e683727b14f463836e1e78e1c64417638",
				"aa066919291a225f0e8dd82cb2b90daa543b54416153",
				"0c925f285b06196940d6085ca9474d41dc3822c5cb",
			),
			pkScript: hexBytes(
				"512077e30a5522dd9f894c3f8b8bd4c4b2cf82ca7da8",
				"a3ea6a239655c39c050ab220",
			),
		},
		{
			name:   "BIP341 vector 4 leaf 1",
			script: hexBytes("07546170726f6f74"),
			controlBlock: hexBytes(
				"c1f9f400803e683727b14f463836e1e78e1c64417638",
				"aa066919291a225f0e8dd864512fecdb5afa04f98839",
				"b50e6f0cb7b1e539bf6f205f67934083cdcc3c8d89",
			),
			pkScript: hexBytes(
				"512077e30a5522dd9f894c3f8b8bd4c4b2cf82ca7da8",
				"a3ea6a239655c39c050ab220",
			),
		},
		{
			name: "BIP341 vector 5 leaf 0",
			script: hexBytes(
				"2072ea6adcf1d371dea8fba1035a09f3d24ed5a05979",
				"9bae114084130ee5898e69ac",
			),
			controlBlock: hexBytes(
				"c0e0dfe2300b0dd746a3f8674dfd4525623639042569",
				"d829c7f0eed9602d263e6fffe578e9ea769027e4f5a3",
				"de40732f75a88a6353a09d767ddeb66accef85e553",
			),
			pkScript: hexBytes(
				"512091b64d5324723a985170e4dc5a0f84c041804f2c",
				"d12660fa5dec09fc21783605",
			),
		},
		{
			name: "BIP341 vector 5 leaf 1",
			script: hexBytes(
				"202352d137f2f3ab38d1eaa976758873377fa5ebb817",
				"372c71e2c542313d4abda8ac",
			),
			controlBlock: hexBytes(
				"c0e0dfe2300b0dd746a3f8674dfd4525623639042569",
				"d829c7f0eed9602d263e6f9e31407bffa15fefbf5090",
				"b149d53959ecdf3f62b1246780238c24501d5ceaf626",
				"45a02e0aac1fe69d69755733a9b7621b694bb5b5cde2",
				"bbfc94066ed62b9817",
			),
			pkScript: hexBytes(
				"512091b64d5324723a985170e4dc5a0f84c041804f2c",
				"d12660fa5dec09fc21783605",
			),
		},
		{
			name: "BIP341 vector 5 leaf 2",
			script: hexBytes(
				"207337c0dd4253cb86f2c43a2351aadd82cccb12a172",
				"cd120452b9bb8324f2186aac",
			),
			controlBlock: hexBytes(
				"c0e0dfe2300b0dd746a3f8674dfd4525623639042569",
				"d829c7f0eed9602d263e6fba982a91d4fc552163cb1c",
				"0da03676102d5b7a014304c01f0c77b2b8e888de1c26",
				"45a02e0aac1fe69d69755733a9b7621b694bb5b5cde2",
				"bbfc94066ed62b9817",
			),
			pkScript: hexBytes(
				"512091b64d5324723a985170e4dc5a0f84c041804f2c",
				"d12660fa5dec09fc21783605",
			),
		},
		{
			name: "BIP341 vector 6 leaf 0",
			script: hexBytes(
				"2071981521ad9fc9036687364118fb6ccd2035b96a42",
				"3c59c5430e98310a11abe2ac",
			),
			controlBlock: hexBytes(
				"c155adf4e8967fbd2e29f20ac896e60c3b0f1d5b0efa",
				"9d34941b5958c7b0a0312d3cd369a528b326bc9d2133",
				"cbd2ac21451acb31681a410434672c8e34fe757e91",
			),
			pkScript: hexBytes(
				"512075169f4001aa68f15bbed28b218df1d0a62cbbcf",
				"1188c6665110c293c907b831",
			),
		},
		{
			name: "BIP341 vector 6 leaf 1",
			script: hexBytes(
				"20d5094d2dbe9b76e2c245a2b89b6006888952e2faa6",
				"a149ae318d69e520617748ac",
			),
			controlBlock: hexBytes(
				"c155adf4e8967fbd2e29f20ac896e60c3b0f1d5b0efa",
				"9d34941b5958c7b0a0312dd7485025fceb78b9ed667d",
				"b36ed8b8dc7b1f0b307ac167fa516fe4352b9f4ef7f1",
				"54e8e8e17c31d3462d7132589ed29353c6fafdb884c5",
				"a6e04ea938834f0d9d",
			),
			pkScript: hexBytes(
				"512075169f4001aa68f15bbed28b218df1d0a62cbbcf",
				"1188c6665110c293c907b831",
			),
		},
		{
			name: "BIP341 vector 6 leaf 2",
			script: hexBytes(
				"20c440b462ad48c7a77f94cd4532d8f2119dcebbd7c9",
				"764557e62726419b08ad4cac",
			),
			controlBlock: hexBytes(
				"c155adf4e8967fbd2e29f20ac896e60c3b0f1d5b0efa",
				"9d34941b5958c7b0a0312d737ed1fe30bc42b8022d71",
				"7b44f0d93516617af64a64753b7a06bf16b26cd711f1",
				"54e8e8e17c31d3462d7132589ed29353c6fafdb884c5",
				"a6e04ea938834f0d9d",
			),
			pkScript: hexBytes(
				"512075169f4001aa68f15bbed28b218df1d0a62cbbcf",
				"1188c6665110c293c907b831",
			),
		},
	}

	for _, test := range scriptPaths {
		t.Run(test.name, func(t *testing.T) {
			witness := wire.TxWitness{
				test.script, test.controlBlock,
			}
			pkScript, err := ComputePkScript(nil, witness)
			if err != nil {
				t.Fatalf("unable to compute pkScript: %v", err)
			}
			if pkScript.Class() != WitnessV1TaprootTy {
				t.Fatalf("expected class %v, got %v",
					WitnessV1TaprootTy, pkScript.Class())
			}
			if !bytes.Equal(pkScript.Script(), test.pkScript) {
				t.Fatalf("expected pkScript=%x, got %x",
					test.pkScript, pkScript.Script())
			}

			// An annex must not change the result.
			annex := []byte{TaprootAnnexTag, 0x01}
			witness = append(witness, annex)
			pkScript, err = ComputePkScript(nil, witness)
			if err != nil {
				t.Fatalf("unable to compute pkScript: %v", err)
			}
			if !bytes.Equal(pkScript.Script(), test.pkScript) {
				t.Fatalf("with annex, expected %x, got %x",
					test.pkScript, pkScript.Script())
			}
		})
	}

	sig64 := bytes.Repeat([]byte{0x01}, 64)
	sig65 := bytes.Repeat([]byte{0x01}, 65)
	annex := []byte{TaprootAnnexTag, 0x01}

	unsupported := []struct {
		name    string
		witness wire.TxWitness
	}{
		{"key path, 64 byte signature", wire.TxWitness{sig64}},
		{"key path, 65 byte signature", wire.TxWitness{sig65}},
		{"key path with annex", wire.TxWitness{sig64, annex}},
	}
	for _, test := range unsupported {
		t.Run(test.name, func(t *testing.T) {
			_, err := ComputePkScript(nil, test.witness)
			if err != ErrUnsupportedScriptType {
				t.Fatalf("expected %v, got %v",
					ErrUnsupportedScriptType, err)
			}
		})
	}

	// A control block that does not hold a valid internal key is an error,
	// not a pkScript.
	t.Run("invalid internal key", func(t *testing.T) {
		controlBlock := append(
			[]byte{byte(BaseLeafVersion)}, bytes.Repeat(
				[]byte{0xff}, 32,
			)...,
		)
		witness := wire.TxWitness{{OP_TRUE}, controlBlock}
		if _, err := ComputePkScript(nil, witness); err == nil {
			t.Fatal("expected an error")
		}
	})
}
