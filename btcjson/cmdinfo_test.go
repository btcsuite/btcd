// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
)

// TestCmdMethod tests the CmdMethod function to ensure it retunrs the expected
// methods and errors.
func TestCmdMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cmd    interface{}
		method string
		err    error
	}{
		{
			name: "unregistered type",
			cmd:  (*int)(nil),
			err:  btcjson.Error{ErrorCode: btcjson.ErrUnregisteredMethod},
		},
		{
			name:   "nil pointer of registered type",
			cmd:    (*btcjson.GetBlockCmd)(nil),
			method: "getblock",
		},
		{
			name:   "nil instance of registered type",
			cmd:    &btcjson.GetBlockCountCmd{},
			method: "getblockcount",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		method, err := btcjson.CmdMethod(test.cmd)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%[3]v), "+
				"want %T", i, test.name, err, test.err)
			continue
		}
		if err != nil {
			gotErrorCode := err.(btcjson.Error).ErrorCode
			if gotErrorCode != test.err.(btcjson.Error).ErrorCode {
				t.Errorf("Test #%d (%s) mismatched error code "+
					"- got %v (%v), want %v", i, test.name,
					gotErrorCode, err,
					test.err.(btcjson.Error).ErrorCode)
				continue
			}

			continue
		}

		// Ensure method matches the expected value.
		if method != test.method {
			t.Errorf("Test #%d (%s) mismatched method - got %v, "+
				"want %v", i, test.name, method, test.method)
			continue
		}
	}
}

// TestMethodUsageFlags tests the MethodUsage function ensure it returns the
// expected flags and errors.
func TestMethodUsageFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		err    error
		flags  btcjson.UsageFlag
	}{
		{
			name:   "unregistered type",
			method: "bogusmethod",
			err:    btcjson.Error{ErrorCode: btcjson.ErrUnregisteredMethod},
		},
		{
			name:   "getblock",
			method: "getblock",
			flags:  0,
		},
		{
			name:   "walletpassphrase",
			method: "walletpassphrase",
			flags:  btcjson.UFWalletOnly,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		flags, err := btcjson.MethodUsageFlags(test.method)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%[3]v), "+
				"want %T", i, test.name, err, test.err)
			continue
		}
		if err != nil {
			gotErrorCode := err.(btcjson.Error).ErrorCode
			if gotErrorCode != test.err.(btcjson.Error).ErrorCode {
				t.Errorf("Test #%d (%s) mismatched error code "+
					"- got %v (%v), want %v", i, test.name,
					gotErrorCode, err,
					test.err.(btcjson.Error).ErrorCode)
				continue
			}

			continue
		}

		// Ensure flags match the expected value.
		if flags != test.flags {
			t.Errorf("Test #%d (%s) mismatched flags - got %v, "+
				"want %v", i, test.name, flags, test.flags)
			continue
		}
	}
}

// TestMethodUsageText tests the MethodUsageText function ensure it returns the
// expected text.
func TestMethodUsageText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		err      error
		expected string
	}{
		{
			name:   "unregistered type",
			method: "bogusmethod",
			err:    btcjson.Error{ErrorCode: btcjson.ErrUnregisteredMethod},
		},
		{
			name:     "getblockcount",
			method:   "getblockcount",
			expected: "getblockcount",
		},
		{
			name:     "getblock",
			method:   "getblock",
			expected: `getblock "hash" (verbosity=1)`,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		usage, err := btcjson.MethodUsageText(test.method)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%[3]v), "+
				"want %T", i, test.name, err, test.err)
			continue
		}
		if err != nil {
			gotErrorCode := err.(btcjson.Error).ErrorCode
			if gotErrorCode != test.err.(btcjson.Error).ErrorCode {
				t.Errorf("Test #%d (%s) mismatched error code "+
					"- got %v (%v), want %v", i, test.name,
					gotErrorCode, err,
					test.err.(btcjson.Error).ErrorCode)
				continue
			}

			continue
		}

		// Ensure usage matches the expected value.
		if usage != test.expected {
			t.Errorf("Test #%d (%s) mismatched usage - got %v, "+
				"want %v", i, test.name, usage, test.expected)
			continue
		}

		// Get the usage again to exercise caching.
		usage, err = btcjson.MethodUsageText(test.method)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		// Ensure usage still matches the expected value.
		if usage != test.expected {
			t.Errorf("Test #%d (%s) mismatched usage - got %v, "+
				"want %v", i, test.name, usage, test.expected)
			continue
		}
	}
}

// TestFieldUsage tests the internal fieldUsage function ensure it returns the
// expected text.
func TestFieldUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		field    reflect.StructField
		defValue *reflect.Value
		expected string
	}{
		{
			name: "jsonrpcusage tag override",
			field: func() reflect.StructField {
				type s struct {
					Test int `jsonrpcusage:"testvalue"`
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: "testvalue",
		},
		{
			name: "generic interface",
			field: func() reflect.StructField {
				type s struct {
					Test interface{}
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `test`,
		},
		{
			name: "string without default value",
			field: func() reflect.StructField {
				type s struct {
					Test string
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `"test"`,
		},
		{
			name: "string with default value",
			field: func() reflect.StructField {
				type s struct {
					Test string
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: func() *reflect.Value {
				value := "default"
				rv := reflect.ValueOf(&value)
				return &rv
			}(),
			expected: `test="default"`,
		},
		{
			name: "array of strings",
			field: func() reflect.StructField {
				type s struct {
					Test []string
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `["test",...]`,
		},
		{
			name: "array of strings with plural field name 1",
			field: func() reflect.StructField {
				type s struct {
					Keys []string
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `["key",...]`,
		},
		{
			name: "array of strings with plural field name 2",
			field: func() reflect.StructField {
				type s struct {
					Addresses []string
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `["address",...]`,
		},
		{
			name: "array of strings with plural field name 3",
			field: func() reflect.StructField {
				type s struct {
					Capabilities []string
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `["capability",...]`,
		},
		{
			name: "array of structs",
			field: func() reflect.StructField {
				type s2 struct {
					Txid string
				}
				type s struct {
					Capabilities []s2
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `[{"txid":"value"},...]`,
		},
		{
			name: "array of ints",
			field: func() reflect.StructField {
				type s struct {
					Test []int
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `[test,...]`,
		},
		{
			name: "sub struct with jsonrpcusage tag override",
			field: func() reflect.StructField {
				type s2 struct {
					Test string `jsonrpcusage:"testusage"`
				}
				type s struct {
					Test s2
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `{testusage}`,
		},
		{
			name: "sub struct with string",
			field: func() reflect.StructField {
				type s2 struct {
					Txid string
				}
				type s struct {
					Test s2
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `{"txid":"value"}`,
		},
		{
			name: "sub struct with int",
			field: func() reflect.StructField {
				type s2 struct {
					Vout int
				}
				type s struct {
					Test s2
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `{"vout":n}`,
		},
		{
			name: "sub struct with float",
			field: func() reflect.StructField {
				type s2 struct {
					Amount float64
				}
				type s struct {
					Test s2
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `{"amount":n.nnn}`,
		},
		{
			name: "sub struct with sub struct",
			field: func() reflect.StructField {
				type s3 struct {
					Amount float64
				}
				type s2 struct {
					Template s3
				}
				type s struct {
					Test s2
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `{"template":{"amount":n.nnn}}`,
		},
		{
			name: "sub struct with slice",
			field: func() reflect.StructField {
				type s2 struct {
					Capabilities []string
				}
				type s struct {
					Test s2
				}
				return reflect.TypeOf((*s)(nil)).Elem().Field(0)
			}(),
			defValue: nil,
			expected: `{"capabilities":["capability",...]}`,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Ensure usage matches the expected value.
		usage := btcjson.TstFieldUsage(test.field, test.defValue)
		if usage != test.expected {
			t.Errorf("Test #%d (%s) mismatched usage - got %v, "+
				"want %v", i, test.name, usage, test.expected)
			continue
		}
	}
}
