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

// TestWalletSvrWsNtfns tests all of the chain server websocket-specific
// notifications marshal and unmarshal into valid results include handling of
// optional fields being omitted in the marshalled command, while optional
// fields with defaults have the default assigned on unmarshalled commands.
func TestWalletSvrWsNtfns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		newNtfn      func() (interface{}, error)
		staticNtfn   func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "accountbalance",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("accountbalance", "acct", 1.25, true)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewAccountBalanceNtfn("acct", 1.25, true)
			},
			marshalled: `{"jsonrpc":"1.0","method":"accountbalance","params":["acct",1.25,true],"id":null}`,
			unmarshalled: &dcrjson.AccountBalanceNtfn{
				Account:   "acct",
				Balance:   1.25,
				Confirmed: true,
			},
		},
		{
			name: "dcrdconnected",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("dcrdconnected", true)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewDcrdConnectedNtfn(true)
			},
			marshalled: `{"jsonrpc":"1.0","method":"dcrdconnected","params":[true],"id":null}`,
			unmarshalled: &dcrjson.DcrdConnectedNtfn{
				Connected: true,
			},
		},
		{
			name: "newtickets",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("newtickets", "123", 100, 3, []string{"a", "b"})
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewNewTicketsNtfn("123", 100, 3, []string{"a", "b"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"newtickets","params":["123",100,3,["a","b"]],"id":null}`,
			unmarshalled: &dcrjson.NewTicketsNtfn{
				Hash:      "123",
				Height:    100,
				StakeDiff: 3,
				Tickets:   []string{"a", "b"},
			},
		},
		{
			name: "newtx",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("newtx", "acct", `{"account":"acct","address":"1Address","category":"send","amount":1.5,"fee":0.0001,"confirmations":1,"txid":"456","walletconflicts":[],"time":12345678,"timereceived":12345876,"vout":789,"otheraccount":"otheracct"}`)
			},
			staticNtfn: func() interface{} {
				result := dcrjson.ListTransactionsResult{
					Account:         "acct",
					Address:         "1Address",
					Category:        "send",
					Amount:          1.5,
					Fee:             dcrjson.Float64(0.0001),
					Confirmations:   1,
					TxID:            "456",
					WalletConflicts: []string{},
					Time:            12345678,
					TimeReceived:    12345876,
					Vout:            789,
					OtherAccount:    "otheracct",
				}
				return dcrjson.NewNewTxNtfn("acct", result)
			},
			marshalled: `{"jsonrpc":"1.0","method":"newtx","params":["acct",{"account":"acct","address":"1Address","amount":1.5,"category":"send","confirmations":1,"fee":0.0001,"time":12345678,"timereceived":12345876,"txid":"456","vout":789,"walletconflicts":[],"otheraccount":"otheracct"}],"id":null}`,
			unmarshalled: &dcrjson.NewTxNtfn{
				Account: "acct",
				Details: dcrjson.ListTransactionsResult{
					Account:         "acct",
					Address:         "1Address",
					Category:        "send",
					Amount:          1.5,
					Fee:             dcrjson.Float64(0.0001),
					Confirmations:   1,
					TxID:            "456",
					WalletConflicts: []string{},
					Time:            12345678,
					TimeReceived:    12345876,
					Vout:            789,
					OtherAccount:    "otheracct",
				},
			},
		},
		{
			name: "revocationcreated",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("revocationcreated", "123", "1234")
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewRevocationCreatedNtfn("123", "1234")
			},
			marshalled: `{"jsonrpc":"1.0","method":"revocationcreated","params":["123","1234"],"id":null}`,
			unmarshalled: &dcrjson.RevocationCreatedNtfn{
				TxHash: "123",
				SStxIn: "1234",
			},
		},
		{
			name: "spentandmissedtickets",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("spentandmissedtickets", "123", 100, 3, map[string]string{"a": "b"})
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewSpentAndMissedTicketsNtfn("123", 100, 3, map[string]string{"a": "b"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"spentandmissedtickets","params":["123",100,3,{"a":"b"}],"id":null}`,
			unmarshalled: &dcrjson.SpentAndMissedTicketsNtfn{
				Hash:      "123",
				Height:    100,
				StakeDiff: 3,
				Tickets:   map[string]string{"a": "b"},
			},
		},
		{
			name: "ticketpurchase",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("ticketpurchased", "123", 5)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewTicketPurchasedNtfn("123", 5)
			},
			marshalled: `{"jsonrpc":"1.0","method":"ticketpurchased","params":["123",5],"id":null}`,
			unmarshalled: &dcrjson.TicketPurchasedNtfn{
				TxHash: "123",
				Amount: 5,
			},
		},
		{
			name: "votecreated",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("votecreated", "123", "1234", 100, "12345", 1)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewVoteCreatedNtfn("123", "1234", 100, "12345", 1)
			},
			marshalled: `{"jsonrpc":"1.0","method":"votecreated","params":["123","1234",100,"12345",1],"id":null}`,
			unmarshalled: &dcrjson.VoteCreatedNtfn{
				TxHash:    "123",
				BlockHash: "1234",
				Height:    100,
				SStxIn:    "12345",
				VoteBits:  1,
			},
		},
		{
			name: "walletlockstate",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("walletlockstate", true)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewWalletLockStateNtfn(true)
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletlockstate","params":[true],"id":null}`,
			unmarshalled: &dcrjson.WalletLockStateNtfn{
				Locked: true,
			},
		},
		{
			name: "winningtickets",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("winningtickets", "123", 100, map[string]string{"a": "b"})
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewWinningTicketsNtfn("123", 100, map[string]string{"a": "b"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"winningtickets","params":["123",100,{"a":"b"}],"id":null}`,
			unmarshalled: &dcrjson.WinningTicketsNtfn{
				BlockHash:   "123",
				BlockHeight: 100,
				Tickets:     map[string]string{"a": "b"},
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the notification as created by the new static
		// creation function.  The ID is nil for notifications.
		marshalled, err := dcrjson.MarshalCmd("1.0", nil, test.staticNtfn())
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

		// Ensure the notification is created without error via the
		// generic new notification creation function.
		cmd, err := test.newNtfn()
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected NewCmd error: %v ",
				i, test.name, err)
		}

		// Marshal the notification as created by the generic new
		// notification creation function.    The ID is nil for
		// notifications.
		marshalled, err = dcrjson.MarshalCmd("1.0", nil, cmd)
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
