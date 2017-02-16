// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"reflect"
	"testing"

	"github.com/decred/dcrd/dcrjson"
)

// TestHelpReflectInternals ensures the various help functions which deal with
// reflect types work as expected for various Go types.
func TestHelpReflectInternals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reflectType reflect.Type
		indentLevel int
		key         string
		examples    []string
		isComplex   bool
		help        string
		isInvalid   bool
	}{
		{
			name:        "int",
			reflectType: reflect.TypeOf(int(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "*int",
			reflectType: reflect.TypeOf((*int)(nil)),
			key:         "json-type-value",
			examples:    []string{"n"},
			help:        "n (json-type-value) fdk",
			isInvalid:   true,
		},
		{
			name:        "int8",
			reflectType: reflect.TypeOf(int8(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "int16",
			reflectType: reflect.TypeOf(int16(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "int32",
			reflectType: reflect.TypeOf(int32(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "int64",
			reflectType: reflect.TypeOf(int64(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "uint",
			reflectType: reflect.TypeOf(uint(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "uint8",
			reflectType: reflect.TypeOf(uint8(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "uint16",
			reflectType: reflect.TypeOf(uint16(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "uint32",
			reflectType: reflect.TypeOf(uint32(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "uint64",
			reflectType: reflect.TypeOf(uint64(0)),
			key:         "json-type-numeric",
			examples:    []string{"n"},
			help:        "n (json-type-numeric) fdk",
		},
		{
			name:        "float32",
			reflectType: reflect.TypeOf(float32(0)),
			key:         "json-type-numeric",
			examples:    []string{"n.nnn"},
			help:        "n.nnn (json-type-numeric) fdk",
		},
		{
			name:        "float64",
			reflectType: reflect.TypeOf(float64(0)),
			key:         "json-type-numeric",
			examples:    []string{"n.nnn"},
			help:        "n.nnn (json-type-numeric) fdk",
		},
		{
			name:        "string",
			reflectType: reflect.TypeOf(""),
			key:         "json-type-string",
			examples:    []string{`"json-example-string"`},
			help:        "\"json-example-string\" (json-type-string) fdk",
		},
		{
			name:        "bool",
			reflectType: reflect.TypeOf(true),
			key:         "json-type-bool",
			examples:    []string{"json-example-bool"},
			help:        "json-example-bool (json-type-bool) fdk",
		},
		{
			name:        "array of int",
			reflectType: reflect.TypeOf([1]int{0}),
			key:         "json-type-arrayjson-type-numeric",
			examples:    []string{"[n,...]"},
			help:        "[n,...] (json-type-arrayjson-type-numeric) fdk",
		},
		{
			name:        "slice of int",
			reflectType: reflect.TypeOf([]int{0}),
			key:         "json-type-arrayjson-type-numeric",
			examples:    []string{"[n,...]"},
			help:        "[n,...] (json-type-arrayjson-type-numeric) fdk",
		},
		{
			name:        "struct",
			reflectType: reflect.TypeOf(struct{}{}),
			key:         "json-type-object",
			examples:    []string{"{", "}\t\t"},
			isComplex:   true,
			help:        "{\n} ",
		},
		{
			name:        "struct indent level 1",
			reflectType: reflect.TypeOf(struct{ field int }{}),
			indentLevel: 1,
			key:         "json-type-object",
			examples: []string{
				"  \"field\": n,\t(json-type-numeric)\t-field",
				" },\t\t",
			},
			help: "{\n" +
				" \"field\": n, (json-type-numeric) -field\n" +
				"}            ",
			isComplex: true,
		},
		{
			name: "array of struct indent level 0",
			reflectType: func() reflect.Type {
				type s struct {
					field int
				}
				return reflect.TypeOf([]s{})
			}(),
			key: "json-type-arrayjson-type-object",
			examples: []string{
				"[{",
				" \"field\": n,\t(json-type-numeric)\ts-field",
				"},...]",
			},
			help: "[{\n" +
				" \"field\": n, (json-type-numeric) s-field\n" +
				"},...]",
			isComplex: true,
		},
		{
			name: "array of struct indent level 1",
			reflectType: func() reflect.Type {
				type s struct {
					field int
				}
				return reflect.TypeOf([]s{})
			}(),
			indentLevel: 1,
			key:         "json-type-arrayjson-type-object",
			examples: []string{
				"  \"field\": n,\t(json-type-numeric)\ts-field",
				" },...],\t\t",
			},
			help: "[{\n" +
				" \"field\": n, (json-type-numeric) s-field\n" +
				"},...]",
			isComplex: true,
		},
		{
			name:        "map",
			reflectType: reflect.TypeOf(map[string]string{}),
			key:         "json-type-object",
			examples: []string{"{",
				" \"fdk--key\": fdk--value, (json-type-object) fdk--desc",
				" ...", "}",
			},
			help: "{\n" +
				" \"fdk--key\": fdk--value, (json-type-object) fdk--desc\n" +
				" ...\n" +
				"}",
			isComplex: true,
		},
		{
			name:        "complex",
			reflectType: reflect.TypeOf(complex64(0)),
			key:         "json-type-value",
			examples:    []string{"json-example-unknown"},
			help:        "json-example-unknown (json-type-value) fdk",
			isInvalid:   true,
		},
	}

	xT := func(key string) string {
		return key
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Ensure the description key is the expected value.
		key := dcrjson.TstReflectTypeToJSONType(xT, test.reflectType)
		if key != test.key {
			t.Errorf("Test #%d (%s) unexpected key - got: %v, "+
				"want: %v", i, test.name, key, test.key)
			continue
		}

		// Ensure the generated example is as expected.
		examples, isComplex := dcrjson.TstReflectTypeToJSONExample(xT,
			test.reflectType, test.indentLevel, "fdk")
		if isComplex != test.isComplex {
			t.Errorf("Test #%d (%s) unexpected isComplex - got: %v, "+
				"want: %v", i, test.name, isComplex,
				test.isComplex)
			continue
		}
		if len(examples) != len(test.examples) {
			t.Errorf("Test #%d (%s) unexpected result length - "+
				"got: %v, want: %v", i, test.name, len(examples),
				len(test.examples))
			continue
		}
		for j, example := range examples {
			if example != test.examples[j] {
				t.Errorf("Test #%d (%s) example #%d unexpected "+
					"example - got: %v, want: %v", i,
					test.name, j, example, test.examples[j])
				continue
			}
		}

		// Ensure the generated result type help is as expected.
		helpText := dcrjson.TstResultTypeHelp(xT, test.reflectType, "fdk")
		if helpText != test.help {
			t.Errorf("Test #%d (%s) unexpected result help - "+
				"got: %v, want: %v", i, test.name, helpText,
				test.help)
			continue
		}

		isValid := dcrjson.TstIsValidResultType(test.reflectType.Kind())
		if isValid != !test.isInvalid {
			t.Errorf("Test #%d (%s) unexpected result type validity "+
				"- got: %v", i, test.name, isValid)
			continue
		}
	}
}

// TestResultStructHelp ensures the expected help text format is returned for
// various Go struct types.
func TestResultStructHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reflectType reflect.Type
		expected    []string
	}{
		{
			name: "empty struct",
			reflectType: func() reflect.Type {
				type s struct{}
				return reflect.TypeOf(s{})
			}(),
			expected: nil,
		},
		{
			name: "struct with primitive field",
			reflectType: func() reflect.Type {
				type s struct {
					field int
				}
				return reflect.TypeOf(s{})
			}(),
			expected: []string{
				"\"field\": n,\t(json-type-numeric)\ts-field",
			},
		},
		{
			name: "struct with primitive field and json tag",
			reflectType: func() reflect.Type {
				type s struct {
					Field int `json:"f"`
				}
				return reflect.TypeOf(s{})
			}(),
			expected: []string{
				"\"f\": n,\t(json-type-numeric)\ts-f",
			},
		},
		{
			name: "struct with array of primitive field",
			reflectType: func() reflect.Type {
				type s struct {
					field []int
				}
				return reflect.TypeOf(s{})
			}(),
			expected: []string{
				"\"field\": [n,...],\t(json-type-arrayjson-type-numeric)\ts-field",
			},
		},
		{
			name: "struct with sub-struct field",
			reflectType: func() reflect.Type {
				type s2 struct {
					subField int
				}
				type s struct {
					field s2
				}
				return reflect.TypeOf(s{})
			}(),
			expected: []string{
				"\"field\": {\t(json-type-object)\ts-field",
				"{",
				" \"subfield\": n,\t(json-type-numeric)\ts2-subfield",
				"}\t\t",
			},
		},
		{
			name: "struct with sub-struct field pointer",
			reflectType: func() reflect.Type {
				type s2 struct {
					subField int
				}
				type s struct {
					field *s2
				}
				return reflect.TypeOf(s{})
			}(),
			expected: []string{
				"\"field\": {\t(json-type-object)\ts-field",
				"{",
				" \"subfield\": n,\t(json-type-numeric)\ts2-subfield",
				"}\t\t",
			},
		},
		{
			name: "struct with array of structs field",
			reflectType: func() reflect.Type {
				type s2 struct {
					subField int
				}
				type s struct {
					field []s2
				}
				return reflect.TypeOf(s{})
			}(),
			expected: []string{
				"\"field\": [{\t(json-type-arrayjson-type-object)\ts-field",
				"[{",
				" \"subfield\": n,\t(json-type-numeric)\ts2-subfield",
				"},...]",
			},
		},
	}

	xT := func(key string) string {
		return key
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		results := dcrjson.TstResultStructHelp(xT, test.reflectType, 0)
		if len(results) != len(test.expected) {
			t.Errorf("Test #%d (%s) unexpected result length - "+
				"got: %v, want: %v", i, test.name, len(results),
				len(test.expected))
			continue
		}
		for j, result := range results {
			if result != test.expected[j] {
				t.Errorf("Test #%d (%s) result #%d unexpected "+
					"result - got: %v, want: %v", i,
					test.name, j, result, test.expected[j])
				continue
			}
		}
	}
}

// TestHelpArgInternals ensures the various help functions which deal with
// arguments work as expected for various argument types.
func TestHelpArgInternals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		reflectType reflect.Type
		defaults    map[int]reflect.Value
		help        string
	}{
		{
			name:   "command with no args",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct{}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: nil,
			help:     "",
		},
		{
			name:   "command with one required arg",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct {
					Field int
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: nil,
			help:     "1. field (json-type-numeric, help-required) test-field\n",
		},
		{
			name:   "command with one optional arg, no default",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct {
					Optional *int
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: nil,
			help:     "1. optional (json-type-numeric, help-optional) test-optional\n",
		},
		{
			name:   "command with one optional arg with default",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct {
					Optional *string
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: func() map[int]reflect.Value {
				defVal := "test"
				return map[int]reflect.Value{
					0: reflect.ValueOf(&defVal),
				}
			}(),
			help: "1. optional (json-type-string, help-optional, help-default=\"test\") test-optional\n",
		},
		{
			name:   "command with struct field",
			method: "test",
			reflectType: func() reflect.Type {
				type s2 struct {
					F int8
				}
				type s struct {
					Field s2
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: nil,
			help: "1. field (json-type-object, help-required) test-field\n" +
				"{\n" +
				" \"f\": n, (json-type-numeric) s2-f\n" +
				"}        \n",
		},
		{
			name:   "command with map field",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct {
					Field map[string]float64
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: nil,
			help: "1. field (json-type-object, help-required) test-field\n" +
				"{\n" +
				" \"test-field--key\": test-field--value, (json-type-object) test-field--desc\n" +
				" ...\n" +
				"}\n",
		},
		{
			name:   "command with slice of primitives field",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct {
					Field []int64
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: nil,
			help:     "1. field (json-type-arrayjson-type-numeric, help-required) test-field\n",
		},
		{
			name:   "command with slice of structs field",
			method: "test",
			reflectType: func() reflect.Type {
				type s2 struct {
					F int64
				}
				type s struct {
					Field []s2
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			defaults: nil,
			help: "1. field (json-type-arrayjson-type-object, help-required) test-field\n" +
				"[{\n" +
				" \"f\": n, (json-type-numeric) s2-f\n" +
				"},...]\n",
		},
	}

	xT := func(key string) string {
		return key
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		help := dcrjson.TstArgHelp(xT, test.reflectType, test.defaults,
			test.method)
		if help != test.help {
			t.Errorf("Test #%d (%s) unexpected help - got:\n%v\n"+
				"want:\n%v", i, test.name, help, test.help)
			continue
		}
	}
}

// TestMethodHelp ensures the method help function works as expected for various
// command structs.
func TestMethodHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		reflectType reflect.Type
		defaults    map[int]reflect.Value
		resultTypes []interface{}
		help        string
	}{
		{
			name:   "command with no args or results",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct{}
				return reflect.TypeOf((*s)(nil))
			}(),
			help: "test\n\ntest--synopsis\n\n" +
				"help-arguments:\nhelp-arguments-none\n\n" +
				"help-result:\nhelp-result-nothing\n",
		},
		{
			name:   "command with no args and one primitive result",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct{}
				return reflect.TypeOf((*s)(nil))
			}(),
			resultTypes: []interface{}{(*int64)(nil)},
			help: "test\n\ntest--synopsis\n\n" +
				"help-arguments:\nhelp-arguments-none\n\n" +
				"help-result:\nn (json-type-numeric) test--result0\n",
		},
		{
			name:   "command with no args and two results",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct{}
				return reflect.TypeOf((*s)(nil))
			}(),
			resultTypes: []interface{}{(*int64)(nil), nil},
			help: "test\n\ntest--synopsis\n\n" +
				"help-arguments:\nhelp-arguments-none\n\n" +
				"help-result (test--condition0):\nn (json-type-numeric) test--result0\n\n" +
				"help-result (test--condition1):\nhelp-result-nothing\n",
		},
		{
			name:   "command with primitive arg and no results",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct {
					Field bool
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			help: "test field\n\ntest--synopsis\n\n" +
				"help-arguments:\n1. field (json-type-bool, help-required) test-field\n\n" +
				"help-result:\nhelp-result-nothing\n",
		},
		{
			name:   "command with primitive optional and no results",
			method: "test",
			reflectType: func() reflect.Type {
				type s struct {
					Field *bool
				}
				return reflect.TypeOf((*s)(nil))
			}(),
			help: "test (field)\n\ntest--synopsis\n\n" +
				"help-arguments:\n1. field (json-type-bool, help-optional) test-field\n\n" +
				"help-result:\nhelp-result-nothing\n",
		},
	}

	xT := func(key string) string {
		return key
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		help := dcrjson.TestMethodHelp(xT, test.reflectType,
			test.defaults, test.method, test.resultTypes)
		if help != test.help {
			t.Errorf("Test #%d (%s) unexpected help - got:\n%v\n"+
				"want:\n%v", i, test.name, help, test.help)
			continue
		}
	}
}

// TestGenerateHelpErrors ensures the GenerateHelp function returns the expected
// errors.
func TestGenerateHelpErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		resultTypes []interface{}
		err         dcrjson.Error
	}{
		{
			name:   "unregistered command",
			method: "boguscommand",
			err:    dcrjson.Error{Code: dcrjson.ErrUnregisteredMethod},
		},
		{
			name:        "non-pointer result type",
			method:      "help",
			resultTypes: []interface{}{0},
			err:         dcrjson.Error{Code: dcrjson.ErrInvalidType},
		},
		{
			name:        "invalid result type",
			method:      "help",
			resultTypes: []interface{}{(*complex64)(nil)},
			err:         dcrjson.Error{Code: dcrjson.ErrInvalidType},
		},
		{
			name:        "missing description",
			method:      "help",
			resultTypes: []interface{}{(*string)(nil), nil},
			err:         dcrjson.Error{Code: dcrjson.ErrMissingDescription},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		_, err := dcrjson.GenerateHelp(test.method, nil,
			test.resultTypes...)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error type - got `%T` (%v), want `%T`",
				i, test.name, err, err, test.err)
			continue
		}
		gotErrorCode := err.(dcrjson.Error).Code
		if gotErrorCode != test.err.Code {
			t.Errorf("Test #%d (%s) mismatched error code - got "+
				"%v (%v), want %v", i, test.name, gotErrorCode,
				err, test.err.Code)
			continue
		}
	}
}

// TestGenerateHelp performs a very basic test to ensure GenerateHelp is working
// as expected.  The internal are testd much more thoroughly in other tests, so
// there is no need to add more tests here.
func TestGenerateHelp(t *testing.T) {
	t.Parallel()

	descs := map[string]string{
		"help--synopsis": "test",
		"help-command":   "test",
	}
	help, err := dcrjson.GenerateHelp("help", descs)
	if err != nil {
		t.Fatalf("GenerateHelp: unexpected error: %v", err)
	}
	wantHelp := "help (\"command\")\n\n" +
		"test\n\nArguments:\n1. command (string, optional) test\n\n" +
		"Result:\nNothing\n"
	if help != wantHelp {
		t.Fatalf("GenerateHelp: unexpected help - got\n%v\nwant\n%v",
			help, wantHelp)
	}
}
