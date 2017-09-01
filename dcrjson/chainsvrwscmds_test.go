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

// TestChainSvrWsCmds tests all of the chain server websocket-specific commands
// marshal and unmarshal into valid results include handling of optional fields
// being omitted in the marshalled command, while optional fields with defaults
// have the default assigned on unmarshalled commands.
func TestChainSvrWsCmds(t *testing.T) {
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
			name: "authenticate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("authenticate", "user", "pass")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewAuthenticateCmd("user", "pass")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"authenticate","params":["user","pass"],"id":1}`,
			unmarshalled: &dcrjson.AuthenticateCmd{Username: "user", Passphrase: "pass"},
		},
		{
			name: "notifywinningtickets",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("notifywinningtickets")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewNotifyWinningTicketsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifywinningtickets","params":[],"id":1}`,
			unmarshalled: &dcrjson.NotifyWinningTicketsCmd{},
		},
		{
			name: "notifyspentandmissedtickets",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("notifyspentandmissedtickets")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewNotifySpentAndMissedTicketsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifyspentandmissedtickets","params":[],"id":1}`,
			unmarshalled: &dcrjson.NotifySpentAndMissedTicketsCmd{},
		},
		{
			name: "notifynewtickets",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("notifynewtickets")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewNotifyNewTicketsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifynewtickets","params":[],"id":1}`,
			unmarshalled: &dcrjson.NotifyNewTicketsCmd{},
		},
		{
			name: "notifystakedifficulty",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("notifystakedifficulty")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewNotifyStakeDifficultyCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifystakedifficulty","params":[],"id":1}`,
			unmarshalled: &dcrjson.NotifyStakeDifficultyCmd{},
		},
		{
			name: "notifyblocks",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("notifyblocks")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewNotifyBlocksCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"notifyblocks","params":[],"id":1}`,
			unmarshalled: &dcrjson.NotifyBlocksCmd{},
		},
		{
			name: "stopnotifyblocks",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("stopnotifyblocks")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewStopNotifyBlocksCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stopnotifyblocks","params":[],"id":1}`,
			unmarshalled: &dcrjson.StopNotifyBlocksCmd{},
		},
		{
			name: "notifynewtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("notifynewtransactions")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewNotifyNewTransactionsCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"notifynewtransactions","params":[],"id":1}`,
			unmarshalled: &dcrjson.NotifyNewTransactionsCmd{
				Verbose: dcrjson.Bool(false),
			},
		},
		{
			name: "notifynewtransactions optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("notifynewtransactions", true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewNotifyNewTransactionsCmd(dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"notifynewtransactions","params":[true],"id":1}`,
			unmarshalled: &dcrjson.NotifyNewTransactionsCmd{
				Verbose: dcrjson.Bool(true),
			},
		},
		{
			name: "stopnotifynewtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("stopnotifynewtransactions")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewStopNotifyNewTransactionsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stopnotifynewtransactions","params":[],"id":1}`,
			unmarshalled: &dcrjson.StopNotifyNewTransactionsCmd{},
		},
		{
			name: "rescan",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("rescan", "0000000000000000000000000000000000000000000000000000000000000123")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewRescanCmd("0000000000000000000000000000000000000000000000000000000000000123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"rescan","params":["0000000000000000000000000000000000000000000000000000000000000123"],"id":1}`,
			unmarshalled: &dcrjson.RescanCmd{
				BlockHashes: "0000000000000000000000000000000000000000000000000000000000000123",
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
