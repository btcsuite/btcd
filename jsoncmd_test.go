// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// this has to be in the real json subpackage so we can mock up structs
package btcjson

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

var jsoncmdtests = []struct {
	name   string
	f      func() (Cmd, error)
	result Cmd // after marshal and unmarshal
}{
	{
		name: "basic addmultisigaddress",
		f: func() (Cmd, error) {
			return NewAddMultisigAddressCmd(float64(1), 1,
				[]string{"foo", "bar"})
		},
		result: &AddMultisigAddressCmd{
			id:        float64(1),
			NRequired: 1,
			Keys:      []string{"foo", "bar"},
			Account:   "",
		},
	},
	{
		name: "addmultisigaddress + optional",
		f: func() (Cmd, error) {
			return NewAddMultisigAddressCmd(float64(1), 1,
				[]string{"foo", "bar"}, "address")
		},
		result: &AddMultisigAddressCmd{
			id:        float64(1),
			NRequired: 1,
			Keys:      []string{"foo", "bar"},
			Account:   "address",
		},
	},
	// TODO(oga) Too many arguments to newaddmultisigaddress
	{
		name: "basic addnode add",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(float64(1), "address",
				"add")
		},
		result: &AddNodeCmd{
			id:     float64(1),
			Addr:   "address",
			SubCmd: "add",
		},
	},
	{
		name: "basic addnode remove",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(float64(1), "address",
				"remove")
		},
		result: &AddNodeCmd{
			id:     float64(1),
			Addr:   "address",
			SubCmd: "remove",
		},
	},
	{
		name: "basic addnode onetry",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(float64(1), "address",
				"onetry")
		},
		result: &AddNodeCmd{
			id:     float64(1),
			Addr:   "address",
			SubCmd: "onetry",
		},
	},
	// TODO(oga) try invalid subcmds
	{
		name: "basic backupwallet",
		f: func() (Cmd, error) {
			return NewBackupWalletCmd(float64(1), "destination")
		},
		result: &BackupWalletCmd{
			id:          float64(1),
			Destination: "destination",
		},
	},
	{
		name: "basic createmultisig",
		f: func() (Cmd, error) {
			return NewCreateMultisigCmd(float64(1), 1,
				[]string{"key1", "key2", "key3"})
		},
		result: &CreateMultisigCmd{
			id:        float64(1),
			NRequired: 1,
			Keys:      []string{"key1", "key2", "key3"},
		},
	},
	{
		name: "basic createrawtransaction",
		f: func() (Cmd, error) {
			return NewCreateRawTransactionCmd(float64(1),
				[]TransactionInput{
					TransactionInput{Txid: "tx1", Vout: 1},
					TransactionInput{Txid: "tx2", Vout: 3}},
				map[string]int64{"bob": 1, "bill": 2})
		},
		result: &CreateRawTransactionCmd{
			id: float64(1),
			Inputs: []TransactionInput{
				TransactionInput{Txid: "tx1", Vout: 1},
				TransactionInput{Txid: "tx2", Vout: 3},
			},
			Amounts: map[string]int64{
				"bob":  1,
				"bill": 2,
			},
		},
	},
	{
		name: "basic decoderawtransaction",
		f: func() (Cmd, error) {
			return NewDecodeRawTransactionCmd(float64(1),
				"thisisahexidecimaltransaction")
		},
		result: &DecodeRawTransactionCmd{
			id:    float64(1),
			HexTx: "thisisahexidecimaltransaction",
		},
	},
	{
		name: "basic dumpprivkey",
		f: func() (Cmd, error) {
			return NewDumpPrivKeyCmd(float64(1),
				"address")
		},
		result: &DumpPrivKeyCmd{
			id:      float64(1),
			Address: "address",
		},
	},
	{
		name: "basic dumpwallet",
		f: func() (Cmd, error) {
			return NewDumpWalletCmd(float64(1),
				"filename")
		},
		result: &DumpWalletCmd{
			id:       float64(1),
			Filename: "filename",
		},
	},
	{
		name: "basic encryptwallet",
		f: func() (Cmd, error) {
			return NewEncryptWalletCmd(float64(1),
				"passphrase")
		},
		result: &EncryptWalletCmd{
			id:         float64(1),
			Passphrase: "passphrase",
		},
	},
	{
		name: "basic getaccount",
		f: func() (Cmd, error) {
			return NewGetAccountCmd(float64(1),
				"address")
		},
		result: &GetAccountCmd{
			id:      float64(1),
			Address: "address",
		},
	},
	{
		name: "basic getaccountaddress",
		f: func() (Cmd, error) {
			return NewGetAccountAddressCmd(float64(1),
				"account")
		},
		result: &GetAccountAddressCmd{
			id:      float64(1),
			Account: "account",
		},
	},
	{
		name: "basic getaddednodeinfo true",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(float64(1), true)
		},
		result: &GetAddedNodeInfoCmd{
			id:  float64(1),
			Dns: true,
		},
	},
	{
		name: "basic getaddednodeinfo false",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(float64(1), false)
		},
		result: &GetAddedNodeInfoCmd{
			id:  float64(1),
			Dns: false,
		},
	},
	{
		name: "basic getaddednodeinfo withnode",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(float64(1), true,
				"thisisanode")
		},
		result: &GetAddedNodeInfoCmd{
			id:   float64(1),
			Dns:  true,
			Node: "thisisanode",
		},
	},
	{
		name: "basic getaddressesbyaccount",
		f: func() (Cmd, error) {
			return NewGetAddressesByAccountCmd(float64(1),
				"account")
		},
		result: &GetAddressesByAccountCmd{
			id:      float64(1),
			Account: "account",
		},
	},
	{
		name: "basic getbalance",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(float64(1))
		},
		result: &GetBalanceCmd{
			id:      float64(1),
			Minconf: 1, // the default
		},
	},
	{
		name: "basic getbalance + account",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(float64(1), "account")
		},
		result: &GetBalanceCmd{
			id:      float64(1),
			Account: "account",
			Minconf: 1, // the default
		},
	},
	{
		name: "basic getbalance + minconf",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(float64(1), "", 2)
		},
		result: &GetBalanceCmd{
			id:      float64(1),
			Minconf: 2,
		},
	},
	{
		name: "basic getbalance + account + minconf",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(float64(1), "account", 2)
		},
		result: &GetBalanceCmd{
			id:      float64(1),
			Account: "account",
			Minconf: 2,
		},
	},
	{
		name: "basic getbestblockhash",
		f: func() (Cmd, error) {
			return NewGetBestBlockHashCmd(float64(1))
		},
		result: &GetBestBlockHashCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getblock",
		f: func() (Cmd, error) {
			return NewGetBlockCmd(float64(1),
				"somehash")
		},
		result: &GetBlockCmd{
			id:   float64(1),
			Hash: "somehash",
		},
	},
	{
		name: "basic getblockcount",
		f: func() (Cmd, error) {
			return NewGetBlockCountCmd(float64(1))
		},
		result: &GetBlockCountCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getblockhash",
		f: func() (Cmd, error) {
			return NewGetBlockHashCmd(float64(1), 1234)
		},
		result: &GetBlockHashCmd{
			id:    float64(1),
			Index: 1234,
		},
	},
	{
		name: "basic getblocktemplate",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(float64(1))
		},
		result: &GetBlockTemplateCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getblocktemplate + request",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(float64(1),
				&TemplateRequest{Mode: "mode",
					Capabilities: []string{"one", "two", "three"}})
		},
		result: &GetBlockTemplateCmd{
			id: float64(1),
			Request: &TemplateRequest{
				Mode: "mode",
				Capabilities: []string{
					"one",
					"two",
					"three",
				},
			},
		},
	},
	{
		name: "basic getblocktemplate + request no mode",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(float64(1),
				&TemplateRequest{
					Capabilities: []string{"one", "two", "three"}})
		},
		result: &GetBlockTemplateCmd{
			id: float64(1),
			Request: &TemplateRequest{
				Capabilities: []string{
					"one",
					"two",
					"three",
				},
			},
		},
	},
	{
		name: "basic getconectioncount",
		f: func() (Cmd, error) {
			return NewGetConnectionCountCmd(float64(1))
		},
		result: &GetConnectionCountCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getdifficulty",
		f: func() (Cmd, error) {
			return NewGetDifficultyCmd(float64(1))
		},
		result: &GetDifficultyCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getgeneratecmd",
		f: func() (Cmd, error) {
			return NewGetGenerateCmd(float64(1))
		},
		result: &GetGenerateCmd{
			id: float64(1),
		},
	},
	{
		name: "basic gethashespersec",
		f: func() (Cmd, error) {
			return NewGetHashesPerSecCmd(float64(1))
		},
		result: &GetHashesPerSecCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getinfo",
		f: func() (Cmd, error) {
			return NewGetInfoCmd(float64(1))
		},
		result: &GetInfoCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getinfo",
		f: func() (Cmd, error) {
			return NewGetInfoCmd(float64(1))
		},
		result: &GetInfoCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getmininginfo",
		f: func() (Cmd, error) {
			return NewGetMiningInfoCmd(float64(1))
		},
		result: &GetMiningInfoCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getnettotals",
		f: func() (Cmd, error) {
			return NewGetNetTotalsCmd(float64(1))
		},
		result: &GetNetTotalsCmd{
			id: float64(1),
		},
	},
	{
		name: "basic getnetworkhashps",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(float64(1))
		},
		result: &GetNetworkHashPSCmd{
			id:     float64(1),
			Blocks: 120,
			Height: -1,
		},
	},
	{
		name: "basic getnetworkhashps + blocks",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(float64(1), 5000)
		},
		result: &GetNetworkHashPSCmd{
			id:     float64(1),
			Blocks: 5000,
			Height: -1,
		},
	},
	{
		name: "basic getnetworkhashps + blocks + height",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(float64(1), 5000, 1000)
		},
		result: &GetNetworkHashPSCmd{
			id:     float64(1),
			Blocks: 5000,
			Height: 1000,
		},
	},
	{
		name: "basic getnewaddress",
		f: func() (Cmd, error) {
			return NewGetNewAddressCmd(float64(1), "address")
		},
		result: &GetNewAddressCmd{
			id: float64(1),
			Address: "address",
		},
	},
	{
		name: "basic getpeerinfo",
		f: func() (Cmd, error) {
			return NewGetPeerInfoCmd(float64(1)) 
		},
		result: &GetPeerInfoCmd{
			id: float64(1),
		},
	},
	{
		name: "basic ping",
		f: func() (Cmd, error) {
			return NewPingCmd(float64(1))
		},
		result: &PingCmd{
			id: float64(1),
		},
	},
	{
		name: "basic verifymessage",
		f: func() (Cmd, error) {
			return NewVerifyMessageCmd(float64(1),
				"someaddress",
				"somesig",
				"a message")
		},
		result: &VerifyMessageCmd{
			id:        float64(1),
			Address:   "someaddress",
			Signature: "somesig",
			Message:   "a message",
		},
	},
	{
		name: "basic walletlock",
		f: func() (Cmd, error) {
			return NewWalletLockCmd(float64(1))
		},
		result: &WalletLockCmd{
			id: float64(1),
		},
	},
	{
		name: "basic walletpassphrase",
		f: func() (Cmd, error) {
			return NewWalletPassphraseCmd(float64(1),
				"phrase1",
				10)
		},
		result: &WalletPassphraseCmd{
			id:         float64(1),
			Passphrase: "phrase1",
			Timeout:    10,
		},
	},
	{
		name: "basic walletpassphrasechange",
		f: func() (Cmd, error) {
			return NewWalletPassphraseChangeCmd(float64(1),
				"phrase1", "phrase2")
		},
		result: &WalletPassphraseChangeCmd{
			id:            float64(1),
			OldPassphrase: "phrase1",
			NewPassphrase: "phrase2",
		},
	},
}

func TestCmds(t *testing.T) {
	for _, test := range jsoncmdtests {
		c, err := test.f()
		if err != nil {
			t.Errorf("%s: failed to run func: %v",
				test.name, err)
			continue
		}

		msg, err := json.Marshal(c)
		if err != nil {
			t.Errorf("%s: failed to marshal cmd: %v",
				test.name, err)
			continue
		}

		c2, err := ParseMarshaledCmd(msg)
		if err != nil {
			t.Errorf("%s: failed to ummarshal cmd: %v",
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(test.result, c2) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", test.name, spew.Sdump(c2),
				spew.Sdump(test.result))
		}
		if !reflect.DeepEqual(c, c2) {
			t.Errorf("%s: unmarshal not as we started with. "+
				"got %v wanted %v", test.name, spew.Sdump(c2),
				spew.Sdump(c))
		}

	}
}
