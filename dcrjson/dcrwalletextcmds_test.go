// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/decred/dcrd/dcrjson"
)

// TestDcrWalletExtCmds tests all of the btcwallet extended commands marshal and
// unmarshal into valid results include handling of optional fields being
// omitted in the marshalled command, while optional fields with defaults have
// the default assigned on unmarshalled commands.
func TestDcrWalletExtCmds(t *testing.T) {
	t.Parallel()

	testID := int(1)
	tests := []struct {
		name         string
		newCmd       func() (interface{}, error)
		staticCmd    func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "sweepaccount - optionals provided",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sweepaccount", "default", "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu", 6, 0.05)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSweepAccountCmd("default", "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
					func(i uint32) *uint32 { return &i }(6),
					func(i float64) *float64 { return &i }(0.05))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sweepaccount","params":["default","DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",6,0.05],"id":1}`,
			unmarshalled: &dcrjson.SweepAccountCmd{
				SourceAccount:         "default",
				DestinationAddress:    "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
				RequiredConfirmations: func(i uint32) *uint32 { return &i }(6),
				FeePerKb:              func(i float64) *float64 { return &i }(0.05),
			},
		},
		{
			name: "sweepaccount - optionals omitted",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sweepaccount", "default", "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSweepAccountCmd("default", "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu", nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sweepaccount","params":["default","DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu"],"id":1}`,
			unmarshalled: &dcrjson.SweepAccountCmd{
				SourceAccount:      "default",
				DestinationAddress: "DsUZxxoHJSty8DCfwfartwTYbuhmVct7tJu",
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the command as created by the new static command
		// creation function.
		marshalled, err := dcrjson.MarshalCmd("1.0", testID, test.staticCmd())
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			continue
		}

		// Ensure the command is created without error via the generic
		// new command creation function.
		cmd, err := test.newCmd()
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected NewCmd error: %v ",
				i, test.name, err)
		}

		// Marshal the command as created by the generic new command
		// creation function.
		marshalled, err = dcrjson.MarshalCmd("1.0", testID, cmd)
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			continue
		}

		var request dcrjson.Request
		if err := json.Unmarshal(marshalled, &request); err != nil {
			t.Errorf("Test #%d (%s) unexpected error while "+
				"unmarshalling JSON-RPC request: %v", i,
				test.name, err)
			continue
		}

		cmd, err = dcrjson.UnmarshalCmd(&request)
		if err != nil {
			t.Errorf("UnmarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(cmd, test.unmarshalled) {
			t.Errorf("Test #%d (%s) unexpected unmarshalled command "+
				"- got %s, want %s", i, test.name,
				fmt.Sprintf("(%T) %+[1]v", cmd),
				fmt.Sprintf("(%T) %+[1]v\n", test.unmarshalled))
			continue
		}
	}
}
