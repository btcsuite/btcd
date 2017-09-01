// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2016 The Decred developers
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

// TestChainSvrCmds tests all of the chain server commands marshal and unmarshal
// into valid results include handling of optional fields being omitted in the
// marshalled command, while optional fields with defaults have the default
// assigned on unmarshalled commands.
func TestChainSvrCmds(t *testing.T) {
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
			name: "addnode",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("addnode", "127.0.0.1", dcrjson.ANRemove)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewAddNodeCmd("127.0.0.1", dcrjson.ANRemove)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"addnode","params":["127.0.0.1","remove"],"id":1}`,
			unmarshalled: &dcrjson.AddNodeCmd{Addr: "127.0.0.1", SubCmd: dcrjson.ANRemove},
		},
		{
			name: "createrawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("createrawtransaction", `[{"txid":"123","vout":1}]`,
					`{"456":0.0123}`)
			},
			staticCmd: func() interface{} {
				txInputs := []dcrjson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return dcrjson.NewCreateRawTransactionCmd(txInputs, amounts, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"txid":"123","vout":1,"tree":0}],{"456":0.0123}],"id":1}`,
			unmarshalled: &dcrjson.CreateRawTransactionCmd{
				Inputs:  []dcrjson.TransactionInput{{Txid: "123", Vout: 1}},
				Amounts: map[string]float64{"456": .0123},
			},
		},
		{
			name: "createrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("createrawtransaction", `[{"txid":"123","vout":1,"tree":0}]`,
					`{"456":0.0123}`, int64(12312333333))
			},
			staticCmd: func() interface{} {
				txInputs := []dcrjson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return dcrjson.NewCreateRawTransactionCmd(txInputs, amounts, dcrjson.Int64(12312333333))
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"txid":"123","vout":1,"tree":0}],{"456":0.0123},12312333333],"id":1}`,
			unmarshalled: &dcrjson.CreateRawTransactionCmd{
				Inputs:   []dcrjson.TransactionInput{{Txid: "123", Vout: 1}},
				Amounts:  map[string]float64{"456": .0123},
				LockTime: dcrjson.Int64(12312333333),
			},
		},
		{
			name: "decoderawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("decoderawtransaction", "123")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewDecodeRawTransactionCmd("123")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decoderawtransaction","params":["123"],"id":1}`,
			unmarshalled: &dcrjson.DecodeRawTransactionCmd{HexTx: "123"},
		},
		{
			name: "decodescript",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("decodescript", "00")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewDecodeScriptCmd("00")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decodescript","params":["00"],"id":1}`,
			unmarshalled: &dcrjson.DecodeScriptCmd{HexScript: "00"},
		},
		{
			name: "getaddednodeinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getaddednodeinfo", true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetAddedNodeInfoCmd(true, nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true],"id":1}`,
			unmarshalled: &dcrjson.GetAddedNodeInfoCmd{DNS: true, Node: nil},
		},
		{
			name: "getaddednodeinfo optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getaddednodeinfo", true, "127.0.0.1")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetAddedNodeInfoCmd(true, dcrjson.String("127.0.0.1"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true,"127.0.0.1"],"id":1}`,
			unmarshalled: &dcrjson.GetAddedNodeInfoCmd{
				DNS:  true,
				Node: dcrjson.String("127.0.0.1"),
			},
		},
		{
			name: "getbestblockhash",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getbestblockhash")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBestBlockHashCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getbestblockhash","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetBestBlockHashCmd{},
		},
		{
			name: "getblock",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblock", "123")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockCmd("123", nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123"],"id":1}`,
			unmarshalled: &dcrjson.GetBlockCmd{
				Hash:      "123",
				Verbose:   dcrjson.Bool(true),
				VerboseTx: dcrjson.Bool(false),
			},
		},
		{
			name: "getblock required optional1",
			newCmd: func() (interface{}, error) {
				// Intentionally use a source param that is
				// more pointers than the destination to
				// exercise that path.
				verbosePtr := dcrjson.Bool(true)
				return dcrjson.NewCmd("getblock", "123", &verbosePtr)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockCmd("123", dcrjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",true],"id":1}`,
			unmarshalled: &dcrjson.GetBlockCmd{
				Hash:      "123",
				Verbose:   dcrjson.Bool(true),
				VerboseTx: dcrjson.Bool(false),
			},
		},
		{
			name: "getblock required optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblock", "123", true, true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockCmd("123", dcrjson.Bool(true), dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",true,true],"id":1}`,
			unmarshalled: &dcrjson.GetBlockCmd{
				Hash:      "123",
				Verbose:   dcrjson.Bool(true),
				VerboseTx: dcrjson.Bool(true),
			},
		},
		{
			name: "getblockchaininfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblockchaininfo")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockChainInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockchaininfo","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetBlockChainInfoCmd{},
		},
		{
			name: "getblockcount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblockcount")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetBlockCountCmd{},
		},
		{
			name: "getblockhash",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblockhash", 123)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockHashCmd(123)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockhash","params":[123],"id":1}`,
			unmarshalled: &dcrjson.GetBlockHashCmd{Index: 123},
		},
		{
			name: "getblockheader",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblockheader", "123")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockHeaderCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockheader","params":["123"],"id":1}`,
			unmarshalled: &dcrjson.GetBlockHeaderCmd{
				Hash:    "123",
				Verbose: dcrjson.Bool(true),
			},
		},
		{
			name: "getblocksubsidy",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblocksubsidy", 123, 256)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockSubsidyCmd(123, 256)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocksubsidy","params":[123,256],"id":1}`,
			unmarshalled: &dcrjson.GetBlockSubsidyCmd{
				Height: 123,
				Voters: 256,
			},
		},
		{
			name: "getblocktemplate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblocktemplate")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBlockTemplateCmd(nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblocktemplate","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetBlockTemplateCmd{Request: nil},
		},
		{
			name: "getblocktemplate optional - template request",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"]}`)
			},
			staticCmd: func() interface{} {
				template := dcrjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
				}
				return dcrjson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"]}],"id":1}`,
			unmarshalled: &dcrjson.GetBlockTemplateCmd{
				Request: &dcrjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
				},
			},
		},
		{
			name: "getblocktemplate optional - template request with tweaks",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":500,"sizelimit":100000000,"maxversion":2}`)
			},
			staticCmd: func() interface{} {
				template := dcrjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   500,
					SizeLimit:    100000000,
					MaxVersion:   2,
				}
				return dcrjson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":500,"sizelimit":100000000,"maxversion":2}],"id":1}`,
			unmarshalled: &dcrjson.GetBlockTemplateCmd{
				Request: &dcrjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   int64(500),
					SizeLimit:    int64(100000000),
					MaxVersion:   2,
				},
			},
		},
		{
			name: "getblocktemplate optional - template request with tweaks 2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":true,"sizelimit":100000000,"maxversion":2}`)
			},
			staticCmd: func() interface{} {
				template := dcrjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   true,
					SizeLimit:    100000000,
					MaxVersion:   2,
				}
				return dcrjson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":true,"sizelimit":100000000,"maxversion":2}],"id":1}`,
			unmarshalled: &dcrjson.GetBlockTemplateCmd{
				Request: &dcrjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   true,
					SizeLimit:    int64(100000000),
					MaxVersion:   2,
				},
			},
		},
		{
			name: "getchaintips",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getchaintips")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetChainTipsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getchaintips","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetChainTipsCmd{},
		},
		{
			name: "getconnectioncount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getconnectioncount")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetConnectionCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getconnectioncount","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetConnectionCountCmd{},
		},
		{
			name: "getdifficulty",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getdifficulty")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetDifficultyCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getdifficulty","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetDifficultyCmd{},
		},
		{
			name: "getgenerate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getgenerate")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetGenerateCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getgenerate","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetGenerateCmd{},
		},
		{
			name: "gethashespersec",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("gethashespersec")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetHashesPerSecCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gethashespersec","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetHashesPerSecCmd{},
		},
		{
			name: "getinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getinfo")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getinfo","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetInfoCmd{},
		},
		{
			name: "getmempoolinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getmempoolinfo")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetMempoolInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmempoolinfo","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetMempoolInfoCmd{},
		},
		{
			name: "getmininginfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getmininginfo")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetMiningInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmininginfo","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetMiningInfoCmd{},
		},
		{
			name: "getnetworkinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getnetworkinfo")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetNetworkInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnetworkinfo","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetNetworkInfoCmd{},
		},
		{
			name: "getnettotals",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getnettotals")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetNetTotalsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnettotals","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetNetTotalsCmd{},
		},
		{
			name: "getnetworkhashps",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getnetworkhashps")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetNetworkHashPSCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetNetworkHashPSCmd{
				Blocks: dcrjson.Int(120),
				Height: dcrjson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getnetworkhashps", 200)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetNetworkHashPSCmd(dcrjson.Int(200), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200],"id":1}`,
			unmarshalled: &dcrjson.GetNetworkHashPSCmd{
				Blocks: dcrjson.Int(200),
				Height: dcrjson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getnetworkhashps", 200, 123)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetNetworkHashPSCmd(dcrjson.Int(200), dcrjson.Int(123))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200,123],"id":1}`,
			unmarshalled: &dcrjson.GetNetworkHashPSCmd{
				Blocks: dcrjson.Int(200),
				Height: dcrjson.Int(123),
			},
		},
		{
			name: "getpeerinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getpeerinfo")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetPeerInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getpeerinfo","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetPeerInfoCmd{},
		},
		{
			name: "getrawmempool",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getrawmempool")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetRawMempoolCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetRawMempoolCmd{
				Verbose: dcrjson.Bool(false),
			},
		},
		{
			name: "getrawmempool optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getrawmempool", false)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetRawMempoolCmd(dcrjson.Bool(false), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[false],"id":1}`,
			unmarshalled: &dcrjson.GetRawMempoolCmd{
				Verbose: dcrjson.Bool(false),
			},
		},
		{
			name: "getrawmempool optional 2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getrawmempool", false, "all")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetRawMempoolCmd(dcrjson.Bool(false), dcrjson.String("all"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[false,"all"],"id":1}`,
			unmarshalled: &dcrjson.GetRawMempoolCmd{
				Verbose: dcrjson.Bool(false),
				TxType:  dcrjson.String("all"),
			},
		},
		{
			name: "getrawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getrawtransaction", "123")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetRawTransactionCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123"],"id":1}`,
			unmarshalled: &dcrjson.GetRawTransactionCmd{
				Txid:    "123",
				Verbose: dcrjson.Int(0),
			},
		},
		{
			name: "getrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getrawtransaction", "123", 1)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetRawTransactionCmd("123", dcrjson.Int(1))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123",1],"id":1}`,
			unmarshalled: &dcrjson.GetRawTransactionCmd{
				Txid:    "123",
				Verbose: dcrjson.Int(1),
			},
		},
		{
			name: "gettxout",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("gettxout", "123", 1)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetTxOutCmd("123", 1, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1],"id":1}`,
			unmarshalled: &dcrjson.GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				IncludeMempool: dcrjson.Bool(true),
			},
		},
		{
			name: "gettxout optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("gettxout", "123", 1, true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetTxOutCmd("123", 1, dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1,true],"id":1}`,
			unmarshalled: &dcrjson.GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				IncludeMempool: dcrjson.Bool(true),
			},
		},
		{
			name: "gettxoutsetinfo",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("gettxoutsetinfo")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetTxOutSetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gettxoutsetinfo","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetTxOutSetInfoCmd{},
		},
		{
			name: "getwork",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getwork")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetWorkCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetWorkCmd{
				Data: nil,
			},
		},
		{
			name: "getwork optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getwork", "00112233")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetWorkCmd(dcrjson.String("00112233"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":["00112233"],"id":1}`,
			unmarshalled: &dcrjson.GetWorkCmd{
				Data: dcrjson.String("00112233"),
			},
		},
		{
			name: "help",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("help")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewHelpCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":[],"id":1}`,
			unmarshalled: &dcrjson.HelpCmd{
				Command: nil,
			},
		},
		{
			name: "help optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("help", "getblock")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewHelpCmd(dcrjson.String("getblock"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":["getblock"],"id":1}`,
			unmarshalled: &dcrjson.HelpCmd{
				Command: dcrjson.String("getblock"),
			},
		},
		{
			name: "ping",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("ping")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewPingCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"ping","params":[],"id":1}`,
			unmarshalled: &dcrjson.PingCmd{},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("searchrawtransactions", "1Address")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSearchRawTransactionsCmd("1Address", nil, nil, nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address"],"id":1}`,
			unmarshalled: &dcrjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     dcrjson.Int(1),
				Skip:        dcrjson.Int(0),
				Count:       dcrjson.Int(100),
				VinExtra:    dcrjson.Int(0),
				Reverse:     dcrjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("searchrawtransactions", "1Address", 0)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSearchRawTransactionsCmd("1Address",
					dcrjson.Int(0), nil, nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0],"id":1}`,
			unmarshalled: &dcrjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     dcrjson.Int(0),
				Skip:        dcrjson.Int(0),
				Count:       dcrjson.Int(100),
				VinExtra:    dcrjson.Int(0),
				Reverse:     dcrjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("searchrawtransactions", "1Address", 0, 5)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSearchRawTransactionsCmd("1Address",
					dcrjson.Int(0), dcrjson.Int(5), nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5],"id":1}`,
			unmarshalled: &dcrjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     dcrjson.Int(0),
				Skip:        dcrjson.Int(5),
				Count:       dcrjson.Int(100),
				VinExtra:    dcrjson.Int(0),
				Reverse:     dcrjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSearchRawTransactionsCmd("1Address",
					dcrjson.Int(0), dcrjson.Int(5), dcrjson.Int(10), nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10],"id":1}`,
			unmarshalled: &dcrjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     dcrjson.Int(0),
				Skip:        dcrjson.Int(5),
				Count:       dcrjson.Int(10),
				VinExtra:    dcrjson.Int(0),
				Reverse:     dcrjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSearchRawTransactionsCmd("1Address",
					dcrjson.Int(0), dcrjson.Int(5), dcrjson.Int(10), dcrjson.Int(1), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1],"id":1}`,
			unmarshalled: &dcrjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     dcrjson.Int(0),
				Skip:        dcrjson.Int(5),
				Count:       dcrjson.Int(10),
				VinExtra:    dcrjson.Int(1),
				Reverse:     dcrjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1, true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSearchRawTransactionsCmd("1Address",
					dcrjson.Int(0), dcrjson.Int(5), dcrjson.Int(10),
					dcrjson.Int(1), dcrjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1,true],"id":1}`,
			unmarshalled: &dcrjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     dcrjson.Int(0),
				Skip:        dcrjson.Int(5),
				Count:       dcrjson.Int(10),
				VinExtra:    dcrjson.Int(1),
				Reverse:     dcrjson.Bool(true),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1, true, []string{"1Address"})
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSearchRawTransactionsCmd("1Address",
					dcrjson.Int(0), dcrjson.Int(5), dcrjson.Int(10),
					dcrjson.Int(1), dcrjson.Bool(true), &[]string{"1Address"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1,true,["1Address"]],"id":1}`,
			unmarshalled: &dcrjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     dcrjson.Int(0),
				Skip:        dcrjson.Int(5),
				Count:       dcrjson.Int(10),
				VinExtra:    dcrjson.Int(1),
				Reverse:     dcrjson.Bool(true),
				FilterAddrs: &[]string{"1Address"},
			},
		},
		{
			name: "sendrawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendrawtransaction", "1122")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendRawTransactionCmd("1122", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122"],"id":1}`,
			unmarshalled: &dcrjson.SendRawTransactionCmd{
				HexTx:         "1122",
				AllowHighFees: dcrjson.Bool(false),
			},
		},
		{
			name: "sendrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendrawtransaction", "1122", false)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendRawTransactionCmd("1122", dcrjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",false],"id":1}`,
			unmarshalled: &dcrjson.SendRawTransactionCmd{
				HexTx:         "1122",
				AllowHighFees: dcrjson.Bool(false),
			},
		},
		{
			name: "setgenerate",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("setgenerate", true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSetGenerateCmd(true, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true],"id":1}`,
			unmarshalled: &dcrjson.SetGenerateCmd{
				Generate:     true,
				GenProcLimit: dcrjson.Int(-1),
			},
		},
		{
			name: "setgenerate optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("setgenerate", true, 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSetGenerateCmd(true, dcrjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true,6],"id":1}`,
			unmarshalled: &dcrjson.SetGenerateCmd{
				Generate:     true,
				GenProcLimit: dcrjson.Int(6),
			},
		},
		{
			name: "stop",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("stop")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewStopCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stop","params":[],"id":1}`,
			unmarshalled: &dcrjson.StopCmd{},
		},
		{
			name: "submitblock",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("submitblock", "112233")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSubmitBlockCmd("112233", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233"],"id":1}`,
			unmarshalled: &dcrjson.SubmitBlockCmd{
				HexBlock: "112233",
				Options:  nil,
			},
		},
		{
			name: "submitblock optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("submitblock", "112233", `{"workid":"12345"}`)
			},
			staticCmd: func() interface{} {
				options := dcrjson.SubmitBlockOptions{
					WorkID: "12345",
				}
				return dcrjson.NewSubmitBlockCmd("112233", &options)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233",{"workid":"12345"}],"id":1}`,
			unmarshalled: &dcrjson.SubmitBlockCmd{
				HexBlock: "112233",
				Options: &dcrjson.SubmitBlockOptions{
					WorkID: "12345",
				},
			},
		},
		{
			name: "validateaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("validateaddress", "1Address")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewValidateAddressCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"validateaddress","params":["1Address"],"id":1}`,
			unmarshalled: &dcrjson.ValidateAddressCmd{
				Address: "1Address",
			},
		},
		{
			name: "verifychain",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("verifychain")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewVerifyChainCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[],"id":1}`,
			unmarshalled: &dcrjson.VerifyChainCmd{
				CheckLevel: dcrjson.Int64(3),
				CheckDepth: dcrjson.Int64(288),
			},
		},
		{
			name: "verifychain optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("verifychain", 2)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewVerifyChainCmd(dcrjson.Int64(2), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2],"id":1}`,
			unmarshalled: &dcrjson.VerifyChainCmd{
				CheckLevel: dcrjson.Int64(2),
				CheckDepth: dcrjson.Int64(288),
			},
		},
		{
			name: "verifychain optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("verifychain", 2, 500)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewVerifyChainCmd(dcrjson.Int64(2), dcrjson.Int64(500))
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2,500],"id":1}`,
			unmarshalled: &dcrjson.VerifyChainCmd{
				CheckLevel: dcrjson.Int64(2),
				CheckDepth: dcrjson.Int64(500),
			},
		},
		{
			name: "verifymessage",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("verifymessage", "1Address", "301234", "test")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewVerifyMessageCmd("1Address", "301234", "test")
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifymessage","params":["1Address","301234","test"],"id":1}`,
			unmarshalled: &dcrjson.VerifyMessageCmd{
				Address:   "1Address",
				Signature: "301234",
				Message:   "test",
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
			t.Errorf("\n%s\n%s", marshalled, test.marshalled)
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

// TestChainSvrCmdErrors ensures any errors that occur in the command during
// custom mashal and unmarshal are as expected.
func TestChainSvrCmdErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     interface{}
		marshalled string
		err        error
	}{
		{
			name:       "template request with invalid type",
			result:     &dcrjson.TemplateRequest{},
			marshalled: `{"mode":1}`,
			err:        &json.UnmarshalTypeError{},
		},
		{
			name:       "invalid template request sigoplimit field",
			result:     &dcrjson.TemplateRequest{},
			marshalled: `{"sigoplimit":"invalid"}`,
			err:        dcrjson.Error{Code: dcrjson.ErrInvalidType},
		},
		{
			name:       "invalid template request sizelimit field",
			result:     &dcrjson.TemplateRequest{},
			marshalled: `{"sizelimit":"invalid"}`,
			err:        dcrjson.Error{Code: dcrjson.ErrInvalidType},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		err := json.Unmarshal([]byte(test.marshalled), &test.result)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error type - got `%T` (%v), got `%T`",
				i, test.name, err, err, test.err)
			continue
		}

		if terr, ok := test.err.(dcrjson.Error); ok {
			gotErrorCode := err.(dcrjson.Error).Code
			if gotErrorCode != terr.Code {
				t.Errorf("Test #%d (%s) mismatched error code "+
					"- got %v (%v), want %v", i, test.name,
					gotErrorCode, terr, terr.Code)
				continue
			}
		}
	}
}
