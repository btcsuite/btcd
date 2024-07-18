package txscript

import (
	"bytes"
	"testing"
)

// TestScriptTemplateLooksLikeInt tests the looksLikeInt function.
func TestScriptTemplateLooksLikeInt(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123", true},
		{"-123", true},
		{"+123", true},
		{"0", true},
		{"+0", true},
		{"-0", true},
		{"abc", false},
		{"12a", false},
		{"", false},
		{"+", false},
		{"-", false},
		{"++123", false},
		{"--123", false},
		{"+-123", false},
		{"1.23", false},
	}

	for _, test := range tests {
		result := looksLikeInt(test.input)
		if result != test.expected {
			t.Errorf("looksLikeInt(%q) = %v, want %v",
				test.input, result, test.expected)
		}
	}
}

// TestScriptTemplate tests the ScriptTemplate function.
func TestScriptTemplate(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		params     map[string]interface{}
		customFunc map[string]interface{}
		expected   string
		wantErr    bool
	}{
		{
			name: "simple P2PKH",
			template: "OP_DUP OP_HASH160 " +
				"0x14e8948c7afa71b6e6fad621256474b5959e0305 " +
				"OP_EQUALVERIFY OP_CHECKSIG",
			params: nil,
			expected: "OP_DUP OP_HASH160 " +
				"14e8948c7afa71b6e6fad621256474b5959e0305 " +
				"OP_EQUALVERIFY OP_CHECKSIG",
			wantErr: false,
		},
		{
			name:     "with positive integer",
			template: "123 OP_ADD",
			params:   nil,
			expected: "7b OP_ADD",
			wantErr:  false,
		},
		{
			name:     "with negative integer",
			template: "-42 OP_ADD",
			params:   nil,
			expected: "aa OP_ADD",
			wantErr:  false,
		},
		{
			name:     "with zero bytes for OP_CHECKSIG",
			template: "0x0000000000000000000000000000000000000000000000000000000000000000 OP_CHECKSIG",
			params:   nil,
			expected: "0000000000000000000000000000000000000000000000000000000000000000 OP_CHECKSIG",
			wantErr:  false,
		},
		{
			name:     "with hex template function for zero bytes",
			template: "{{ hex .ZeroSig }} OP_CHECKSIG",
			params:   map[string]interface{}{
				"ZeroSig": make([]byte, 32),
			},
			expected: "0000000000000000000000000000000000000000000000000000000000000000 OP_CHECKSIG",
			wantErr:  false,
		},
		{
			name:     "with hex data without 0x prefix",
			template: "abcdef OP_ADD",
			params:   nil,
			expected: "abcdef OP_ADD",
			wantErr:  false,
		},
		{
			name: "with template parameter",
			template: "OP_DUP OP_HASH160 {{ hex .Pubkey }} " +
				"OP_EQUALVERIFY OP_CHECKSIG",
			params: map[string]interface{}{
				"Pubkey": []byte{
					0x14, 0xe8, 0x94, 0x8c, 0x7a, 0xfa,
					0x71, 0xb6, 0xe6, 0xfa, 0xd6, 0x21,
					0x25, 0x64, 0x74, 0xb5, 0x95, 0x9e,
					0x03, 0x05,
				},
			},
			expected: "OP_DUP OP_HASH160 " +
				"14e8948c7afa71b6e6fad621256474b5959e0305 " +
				"OP_EQUALVERIFY OP_CHECKSIG",
			wantErr: false,
		},
		{
			name: "with range iteration",
			template: `
			{{ range $i := range_iter 1 4 }}
			OP_DUP OP_HASH160
			{{ if eq $i 1 }}
			    0x01
			{{ else if eq $i 2 }}
			    0x02
			{{ else }}
			   0x03
			{{ end }}
			OP_EQUALVERIFY {{ end }}
			OP_CHECKSIG`,
			params: nil,
			expected: "OP_DUP OP_HASH160 1 OP_EQUALVERIFY " +
				"OP_DUP OP_HASH160 2 OP_EQUALVERIFY " +
				"OP_DUP OP_HASH160 3 OP_EQUALVERIFY " +
				"OP_CHECKSIG",
			wantErr: false,
		},
		{
			name:     "with custom function",
			template: "{{ add 10 5 }} OP_DROP",
			params:   nil,
			customFunc: map[string]interface{}{
				"add": func(a, b int) int {
					return a + b
				},
			},
			expected: "15 OP_DROP",
			wantErr:  false,
		},
		{
			name:     "invalid opcode",
			template: "OP_UNKNOWN",
			params:   nil,
			wantErr:  true,
		},
		{
			name:     "invalid hex",
			template: "0xZZ",
			params:   nil,
			wantErr:  true,
		},
		{
			name:     "invalid integer",
			template: "9999999999999999999999999999",
			params:   nil,
			wantErr:  true,
		},
		{
			name:     "invalid token",
			template: "not_hex_or_op",
			params:   nil,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var opts []ScriptTemplateOption
			if test.params != nil {
				opts = append(
					opts,
					WithScriptTemplateParams(test.params),
				)
			}

			// Add custom functions if specified.
			for name, fn := range test.customFunc {
				opts = append(
					opts, WithCustomTemplateFunc(name, fn),
				)
			}

			script, err := ScriptTemplate(test.template, opts...)

			if test.wantErr {
				if err == nil {
					t.Errorf("ScriptTemplate(%q) expected "+
						"error, got nil", test.template)
				}
				return
			}

			if err != nil {
				t.Errorf("ScriptTemplate(%q) unexpected "+
					"error: %v", test.template, err)
				return
			}

			// Disassemble the script and compare the string
			// representation.
			disasm, err := DisasmString(script)
			if err != nil {
				t.Errorf("Failed to disassemble script: %v",
					err)
				return
			}

			if disasm != test.expected {
				t.Errorf("ScriptTemplate(%q):\ngot:  "+
					"%s\nwant: %s",
					test.template, disasm,
					test.expected)
			}
		})
	}
}

// TestScriptTemplateOptions tests the ScriptTemplate option functions.
func TestScriptTemplateOptions(t *testing.T) {
	t.Run("WithScriptTemplateParams", func(t *testing.T) {
		template := "{{ .Value }} OP_DROP"
		params := map[string]interface{}{
			"Value": 42,
		}

		script, err := ScriptTemplate(
			template, WithScriptTemplateParams(params),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		disasm, err := DisasmString(script)
		if err != nil {
			t.Errorf("Failed to disassemble script: %v", err)
			return
		}

		expected := "2a OP_DROP"
		if disasm != expected {
			t.Errorf("ScriptTemplate(%q):\ngot:  %s\nwant: %s",
				template, disasm, expected)
		}
	})

	t.Run("WithCustomTemplateFunc", func(t *testing.T) {
		template := "{{ multiply 6 7 }} OP_DROP"
		script, err := ScriptTemplate(
			template,
			WithCustomTemplateFunc("multiply", func(a, b int) int {
				return a * b
			}),
		)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		disasm, err := DisasmString(script)
		if err != nil {
			t.Errorf("Failed to disassemble script: %v", err)
			return
		}

		expected := "2a OP_DROP"
		if disasm != expected {
			t.Errorf("ScriptTemplate(%q):\ngot:  %s\nwant: %s",
				template, disasm, expected)
		}
	})
}

// TestScriptTemplateHelperFunctions tests the helper functions used in
// templates.
func TestScriptTemplateHelperFunctions(t *testing.T) {
	t.Run("rangeIter", func(t *testing.T) {
		result := rangeIter(2, 5)
		expected := []int{2, 3, 4}

		if len(result) != len(expected) {
			t.Fatalf("rangeIter(2, 5) has length %d, want %d",
				len(result), len(expected))
		}

		for i, v := range expected {
			if result[i] != v {
				t.Errorf("rangeIter(2, 5)[%d] = %d, want %d",
					i, result[i], v)
			}
		}
	})

	t.Run("hexEncode", func(t *testing.T) {
		input := []byte{0x12, 0x34, 0x56}
		result := hexEncode(input)
		expected := "0x123456"

		if result != expected {
			t.Errorf("hexEncode(%v) = %q, want %q",
				input, result, expected)
		}
	})
	
	t.Run("hexStr", func(t *testing.T) {
		input := []byte{0x12, 0x34, 0x56}
		result := hexStr(input)
		expected := "123456"

		if result != expected {
			t.Errorf("hexStr(%v) = %q, want %q",
				input, result, expected)
		}
	})

	t.Run("hexDecode", func(t *testing.T) {
		tests := []struct {
			input    string
			expected []byte
			wantErr  bool
		}{
			{"123456", []byte{0x12, 0x34, 0x56}, false},
			{"0x123456", []byte{0x12, 0x34, 0x56}, false},
			{"zz", nil, true},
		}

		for _, test := range tests {
			result, err := hexDecode(test.input)

			if test.wantErr {
				if err == nil {
					t.Errorf("hexDecode(%q) "+
						"expected error, got nil",
						test.input)
				}
				continue
			}

			if err != nil {
				t.Errorf("hexDecode(%q) unexpected error: %v",
					test.input, err)
				continue
			}

			if !bytes.Equal(result, test.expected) {
				t.Errorf("hexDecode(%q) = %v, want %v",
					test.input, result, test.expected)
			}
		}
	})
}

