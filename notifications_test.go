// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcws_test

import (
	"github.com/conformal/btcjson"
	"github.com/conformal/btcws"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

var ntfntests = []struct {
	name   string
	f      func() btcjson.Cmd
	result btcjson.Cmd // after marshal and unmarshal
}{
	{
		name: "accountbalance",
		f: func() btcjson.Cmd {
			return btcws.NewAccountBalanceNtfn("abcde", 1.2345, true)
		},
		result: &btcws.AccountBalanceNtfn{
			Account:   "abcde",
			Balance:   1.2345,
			Confirmed: true,
		},
	},
	{
		name: "blockconnected",
		f: func() btcjson.Cmd {
			return btcws.NewBlockConnectedNtfn(
				"000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				153469)
		},
		result: &btcws.BlockConnectedNtfn{
			Hash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
			Height: 153469,
		},
	},
	{
		name: "blockdisconnected",
		f: func() btcjson.Cmd {
			return btcws.NewBlockDisconnectedNtfn(
				"000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				153469)
		},
		result: &btcws.BlockDisconnectedNtfn{
			Hash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
			Height: 153469,
		},
	},
	{
		name: "btcdconnected",
		f: func() btcjson.Cmd {
			return btcws.NewBtcdConnectedNtfn(true)
		},
		result: &btcws.BtcdConnectedNtfn{
			Connected: true,
		},
	},
	{
		name: "processedtx",
		f: func() btcjson.Cmd {
			cmd := &btcws.ProcessedTxNtfn{
				Receiver:    "miFxiuApPo3KBqtMnPUjasZmHoVnoH3Eoc",
				Amount:      200000000,
				TxID:        "851f5c0652e785c5ed80aafaf2d918e5cbe5c307dbba3680808ada1d01f36886",
				TxOutIndex:  1,
				PkScript:    "76a9141e127eda7cd71b9724085f588840a3e9d697ae9888ac",
				BlockHash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				BlockHeight: 153469,
				BlockIndex:  1,
				BlockTime:   1386944019,
				Spent:       true,
			}
			return cmd
		},
		result: &btcws.ProcessedTxNtfn{
			Receiver:    "miFxiuApPo3KBqtMnPUjasZmHoVnoH3Eoc",
			Amount:      200000000,
			TxID:        "851f5c0652e785c5ed80aafaf2d918e5cbe5c307dbba3680808ada1d01f36886",
			TxOutIndex:  1,
			PkScript:    "76a9141e127eda7cd71b9724085f588840a3e9d697ae9888ac",
			BlockHash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
			BlockHeight: 153469,
			BlockIndex:  1,
			BlockTime:   1386944019,
			Spent:       true,
		},
	},
	{
		name: "txmined",
		f: func() btcjson.Cmd {
			return btcws.NewTxMinedNtfn(
				"062f2b5f7d28c787e0f3aee382132241cd590efb7b83bd2c7f506de5aa4ef275",
				"000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				153469,
				1386944019,
				0)
		},
		result: &btcws.TxMinedNtfn{
			TxID:        "062f2b5f7d28c787e0f3aee382132241cd590efb7b83bd2c7f506de5aa4ef275",
			BlockHash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
			BlockHeight: 153469,
			BlockTime:   1386944019,
			Index:       0,
		},
	},
	{
		name: "newtx",
		f: func() btcjson.Cmd {
			details := map[string]interface{}{
				"key1": float64(12345),
				"key2": true,
				"key3": "lalala",
				"key4": []interface{}{"abcde", float64(12345)},
			}
			return btcws.NewTxNtfn("abcde", details)
		},
		result: &btcws.TxNtfn{
			Account: "abcde",
			Details: map[string]interface{}{
				"key1": float64(12345),
				"key2": true,
				"key3": "lalala",
				"key4": []interface{}{"abcde", float64(12345)},
			},
		},
	},
	{
		name: "walletlockstate",
		f: func() btcjson.Cmd {
			return btcws.NewWalletLockStateNtfn("abcde", true)
		},
		result: &btcws.WalletLockStateNtfn{
			Account: "abcde",
			Locked:  true,
		},
	},
}

func TestNtfns(t *testing.T) {
	for _, test := range ntfntests {
		// create notification.
		n := test.f()

		// verify that id is nil.
		if n.Id() != nil {
			t.Error("%s: notification should not have non-nil id %v",
				test.name, n.Id())
			continue
		}

		mn, err := n.MarshalJSON()
		if err != nil {
			t.Errorf("%s: failed to marshal notification: %v",
				test.name, err)
			continue
		}

		n2, err := btcjson.ParseMarshaledCmd(mn)
		if err != nil {
			t.Errorf("%s: failed to ummarshal cmd: %v",
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(test.result, n2) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", test.name, spew.Sdump(n2),
				spew.Sdump(test.result))
		}
		if !reflect.DeepEqual(n, n2) {
			t.Errorf("%s: unmarshal not as we started with. "+
				"got %v wanted %v", test.name, spew.Sdump(n2),
				spew.Sdump(n))
		}

		// Read marshaled notification back into n.  Should still
		// match result.
		n.UnmarshalJSON(mn)
		if !reflect.DeepEqual(test.result, n) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", test.name, spew.Sdump(n),
				spew.Sdump(test.result))
		}
	}
}
