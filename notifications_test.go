// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcws_test

import (
	"reflect"
	"testing"

	"github.com/conformal/btcjson"
	"github.com/conformal/btcws"
	"github.com/davecgh/go-spew/spew"
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
		name: "recvtx no block",
		f: func() btcjson.Cmd {
			return btcws.NewRecvTxNtfn("lalala the hex tx", nil)
		},
		result: &btcws.RecvTxNtfn{
			HexTx: "lalala the hex tx",
			Block: nil,
		},
	},
	{
		name: "recvtx with block",
		f: func() btcjson.Cmd {
			block := &btcws.BlockDetails{
				Height: 153469,
				Hash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				Index:  1,
				Time:   1386944019,
			}
			return btcws.NewRecvTxNtfn("lalala the hex tx", block)
		},
		result: &btcws.RecvTxNtfn{
			HexTx: "lalala the hex tx",
			Block: &btcws.BlockDetails{
				Height: 153469,
				Hash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				Index:  1,
				Time:   1386944019,
			},
		},
	},
	{
		name: "redeemingtx",
		f: func() btcjson.Cmd {
			return btcws.NewRedeemingTxNtfn("lalala the hex tx", nil)
		},
		result: &btcws.RedeemingTxNtfn{
			HexTx: "lalala the hex tx",
			Block: nil,
		},
	},
	{
		name: "redeemingtx with block",
		f: func() btcjson.Cmd {
			block := &btcws.BlockDetails{
				Height: 153469,
				Hash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				Index:  1,
				Time:   1386944019,
			}
			return btcws.NewRedeemingTxNtfn("lalala the hex tx", block)
		},
		result: &btcws.RedeemingTxNtfn{
			HexTx: "lalala the hex tx",
			Block: &btcws.BlockDetails{
				Height: 153469,
				Hash:   "000000004811dda1c320ad5d0ea184a20a53acd92292c5f1cb926c3ee82abf70",
				Index:  1,
				Time:   1386944019,
			},
		},
	},
	{
		name: "rescanfinished",
		f: func() btcjson.Cmd {
			return btcws.NewRescanFinishedNtfn(
				"00000000b8980ec1fe96bc1b4425788ddc88dd36699521a448ebca2020b38699",
				12345, 1240784732)
		},
		result: &btcws.RescanFinishedNtfn{
			Hash:   "00000000b8980ec1fe96bc1b4425788ddc88dd36699521a448ebca2020b38699",
			Height: 12345,
			Time:   1240784732,
		},
	},
	{
		name: "rescanprogress",
		f: func() btcjson.Cmd {
			return btcws.NewRescanProgressNtfn(
				"00000000b8980ec1fe96bc1b4425788ddc88dd36699521a448ebca2020b38699",
				12345, 1240784732)
		},
		result: &btcws.RescanProgressNtfn{
			Hash:   "00000000b8980ec1fe96bc1b4425788ddc88dd36699521a448ebca2020b38699",
			Height: 12345,
			Time:   1240784732,
		},
	},
	{
		name: "newtx",
		f: func() btcjson.Cmd {
			details := &btcjson.ListTransactionsResult{
				Account:         "original",
				Address:         "mnSsMBY8j4AhQzbR6XqawND7NPTECVdtLd",
				Category:        "receive",
				Amount:          100,
				Fee:             0.0,
				Confirmations:   6707,
				Generated:       false,
				BlockHash:       "000000000b20bf5fe8e25b19f9ec340744cda321a17ade12af9838530a75098b",
				BlockIndex:      2,
				BlockTime:       1397079345,
				TxID:            "cb082a63b29f446551829d03fa8bac02d3825a18994d5feec564f14101fc5fad",
				WalletConflicts: []string{},
				Time:            123123123,
				TimeReceived:    1397079169,
				Comment:         "comment",
				OtherAccount:    "",
			}
			return btcws.NewTxNtfn("abcde", details)
		},
		result: &btcws.TxNtfn{
			Account: "abcde",
			Details: &btcjson.ListTransactionsResult{
				Account:         "original",
				Address:         "mnSsMBY8j4AhQzbR6XqawND7NPTECVdtLd",
				Category:        "receive",
				Amount:          100,
				Fee:             0.0,
				Confirmations:   6707,
				Generated:       false,
				BlockHash:       "000000000b20bf5fe8e25b19f9ec340744cda321a17ade12af9838530a75098b",
				BlockIndex:      2,
				BlockTime:       1397079345,
				TxID:            "cb082a63b29f446551829d03fa8bac02d3825a18994d5feec564f14101fc5fad",
				WalletConflicts: []string{},
				Time:            123123123,
				TimeReceived:    1397079169,
				Comment:         "comment",
				OtherAccount:    "",
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
	{
		name: "txaccepted",
		f: func() btcjson.Cmd {
			return btcws.NewTxAcceptedNtfn(
				"062f2b5f7d28c787e0f3aee382132241cd590efb7b83bd2c7f506de5aa4ef275",
				34567765)
		},
		result: &btcws.TxAcceptedNtfn{
			TxID:   "062f2b5f7d28c787e0f3aee382132241cd590efb7b83bd2c7f506de5aa4ef275",
			Amount: 34567765,
		},
	},
	{
		name: "txacceptedverbose",
		f: func() btcjson.Cmd {
			return btcws.NewTxAcceptedVerboseNtfn(&btcjson.TxRawResult{
				Hex:      "01000000010cdf900074a3622499a2f28f44a94476f27a8900a2bdd60e042754b6cab09741000000008a473044022012e11012fad1eb21ba1c82deb8da98778b08e714b72f281293064528343fae0502204294d7520f469f9673087a55395de0ce0e9074dce236db9fe7f30013b5fd00b90141047b6ff7832b4a763666e5481a0bd9eedb656d9f882d215c16fe9563d7b191cd67b2a41601a853a9f9d92773ae6f912ef451a089148e510623759cf55c408efdefffffffff02f4063f00000000001976a914b269e0ceec5d5b5e192cf580ae42341e0f79b0b588aca8c84b02000000001976a91439233c0d43a1411e547c60bad8985bae3530b6af88ac00000000",
				Txid:     "0cfeb968fb5d0f6b9a2a1de37c0607a1964dd3e335f203377cec90e03b20869e",
				Version:  0x1,
				LockTime: 0x0,
			})
		},
		result: &btcws.TxAcceptedVerboseNtfn{
			RawTx: &btcjson.TxRawResult{
				Hex:      "01000000010cdf900074a3622499a2f28f44a94476f27a8900a2bdd60e042754b6cab09741000000008a473044022012e11012fad1eb21ba1c82deb8da98778b08e714b72f281293064528343fae0502204294d7520f469f9673087a55395de0ce0e9074dce236db9fe7f30013b5fd00b90141047b6ff7832b4a763666e5481a0bd9eedb656d9f882d215c16fe9563d7b191cd67b2a41601a853a9f9d92773ae6f912ef451a089148e510623759cf55c408efdefffffffff02f4063f00000000001976a914b269e0ceec5d5b5e192cf580ae42341e0f79b0b588aca8c84b02000000001976a91439233c0d43a1411e547c60bad8985bae3530b6af88ac00000000",
				Txid:     "0cfeb968fb5d0f6b9a2a1de37c0607a1964dd3e335f203377cec90e03b20869e",
				Version:  0x1,
				LockTime: 0x0,
			},
		},
	},
}

func TestNtfns(t *testing.T) {
	for _, test := range ntfntests {
		// create notification.
		n := test.f()

		// verify that id is nil.
		if n.Id() != nil {
			t.Errorf("%s: notification should not have non-nil id %v",
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
		if err := n.UnmarshalJSON(mn); err != nil {
			t.Errorf("%s: unmarshal failed: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(test.result, n) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", test.name, spew.Sdump(n),
				spew.Sdump(test.result))
		}
	}
}
