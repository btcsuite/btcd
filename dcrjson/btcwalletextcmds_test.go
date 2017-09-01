// Copyright (c) 2014 The btcsuite developers
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

// TestBtcWalletExtCmds tests all of the btcwallet extended commands marshal and
// unmarshal into valid results include handling of optional fields being
// omitted in the marshalled command, while optional fields with defaults have
// the default assigned on unmarshalled commands.
func TestBtcWalletExtCmds(t *testing.T) {
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
			name: "createnewaccount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("createnewaccount", "acct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewCreateNewAccountCmd("acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"createnewaccount","params":["acct"],"id":1}`,
			unmarshalled: &dcrjson.CreateNewAccountCmd{
				Account: "acct",
			},
		},
		{
			name: "importaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importaddress", "1Address")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportAddressCmd("1Address", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importaddress","params":["1Address"],"id":1}`,
			unmarshalled: &dcrjson.ImportAddressCmd{
				Address: "1Address",
				Rescan:  dcrjson.Bool(true),
			},
		},
		{
			name: "importaddress optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importaddress", "1Address", false)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportAddressCmd("1Address", dcrjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"importaddress","params":["1Address",false],"id":1}`,
			unmarshalled: &dcrjson.ImportAddressCmd{
				Address: "1Address",
				Rescan:  dcrjson.Bool(false),
			},
		},
		{
			name: "importpubkey",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importpubkey", "031234")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportPubKeyCmd("031234", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importpubkey","params":["031234"],"id":1}`,
			unmarshalled: &dcrjson.ImportPubKeyCmd{
				PubKey: "031234",
				Rescan: dcrjson.Bool(true),
			},
		},
		{
			name: "importpubkey optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importpubkey", "031234", false)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportPubKeyCmd("031234", dcrjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"importpubkey","params":["031234",false],"id":1}`,
			unmarshalled: &dcrjson.ImportPubKeyCmd{
				PubKey: "031234",
				Rescan: dcrjson.Bool(false),
			},
		},
		{
			name: "renameaccount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("renameaccount", "oldacct", "newacct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewRenameAccountCmd("oldacct", "newacct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"renameaccount","params":["oldacct","newacct"],"id":1}`,
			unmarshalled: &dcrjson.RenameAccountCmd{
				OldAccount: "oldacct",
				NewAccount: "newacct",
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
