// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Message contains a message to be sent to the bitcoin client.
type Message struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// Reply is the general form of the reply from the bitcoin client.
// The form of the Result part varies from one command to the next so it
// is currently implemented as an interface.
type Reply struct {
	Result interface{} `json:"result"`
	Error  *Error      `json:"error"`
	// This has to be a pointer for go to put a null in it when empty.
	Id *interface{} `json:"id"`
}

// InfoResult contains the data returned by the getinfo command.
type InfoResult struct {
	Version         int     `json:"version,omitempty"`
	ProtocolVersion int     `json:"protocolversion,omitempty"`
	WalletVersion   int     `json:"walletversion,omitempty"`
	Balance         float64 `json:"balance,omitempty"`
	Blocks          int     `json:"blocks,omitempty"`
	TimeOffset      int64   `json:"timeoffset,omitempty"`
	Connections     int     `json:"connections,omitempty"`
	Proxy           string  `json:"proxy,omitempty"`
	Difficulty      float64 `json:"difficulty,omitempty"`
	TestNet         bool    `json:"testnet,omitempty"`
	KeypoolOldest   int64   `json:"keypoololdest,omitempty"`
	KeypoolSize     int     `json:"keypoolsize,omitempty"`
	PaytxFee        float64 `json:"paytxfee,omitempty"`
	RelayFee        float64 `json:"relayfee,omitempty"`
	Errors          string  `json:"errors,omitempty"`
}

// BlockResult models the data from the getblock command when the verbose flag
// is set.  When the verbose flag is not set, getblock return a hex-encoded
// string.
type BlockResult struct {
	Hash          string        `json:"hash"`
	Confirmations uint64        `json:"confirmations"`
	Size          int           `json:"size"`
	Height        int64         `json:"height"`
	Version       uint32        `json:"version"`
	MerkleRoot    string        `json:"merkleroot"`
	Tx            []string      `json:"tx,omitempty"`
	RawTx         []TxRawResult `json:"rawtx,omitempty"`
	Time          int64         `json:"time"`
	Nonce         uint32        `json:"nonce"`
	Bits          string        `json:"bits"`
	Difficulty    float64       `json:"difficulty"`
	PreviousHash  string        `json:"previousblockhash"`
	NextHash      string        `json:"nextblockhash"`
}

// DecodeScriptResult models the data returned from the decodescript command.
type DecodeScriptResult struct {
	Asm       string   `json:"asm"`
	ReqSigs   int      `json:"reqSigs,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
	P2sh      string   `json:"p2sh"`
}

// GetPeerInfoResult models the data returned from the getpeerinfo command.
type GetPeerInfoResult struct {
	Addr           string `json:"addr"`
	Services       string `json:"services"`
	LastSend       int64  `json:"lastsend"`
	LastRecv       int64  `json:"lastrecv"`
	BytesSent      uint64 `json:"bytessent"`
	BytesRecv      uint64 `json:"bytesrecv"`
	PingTime       int64  `json:"pingtime"`
	PingWait       int64  `json:"pingwait,omitempty"`
	ConnTime       int64  `json:"conntime"`
	Version        uint32 `json:"version"`
	SubVer         string `json:"subver"`
	Inbound        bool   `json:"inbound"`
	StartingHeight int32  `json:"startingheight"`
	BanScore       int    `json:"banscore,omitempty"`
	SyncNode       bool   `json:"syncnode"`
}

// GetRawMempoolResult models the data returned from the getrawmempool command.
type GetRawMempoolResult struct {
	Size             int      `json:"size"`
	Fee              float64  `json:"fee"`
	Time             int64    `json:"time"`
	Height           int64    `json:"height"`
	StartingPriority int      `json:"startingpriority"`
	CurrentPriority  int      `json:"currentpriority"`
	Depends          []string `json:"depends"`
}

// TxRawResult models the data from the getrawtransaction command.
type TxRawResult struct {
	Hex           string `json:"hex"`
	Txid          string `json:"txid"`
	Version       uint32 `json:"version"`
	LockTime      uint32 `json:"locktime"`
	Vin           []Vin  `json:"vin"`
	Vout          []Vout `json:"vout"`
	BlockHash     string `json:"blockhash,omitempty"`
	Confirmations uint64 `json:"confirmations,omitempty"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime,omitempty"`
}

// TxRawDecodeResult models the data from the decoderawtransaction command.
type TxRawDecodeResult struct {
	Txid     string `json:"txid"`
	Version  uint32 `json:"version"`
	Locktime uint32 `json:"locktime"`
	Vin      []Vin  `json:"vin"`
	Vout     []Vout `json:"vout"`
}

// GetNetTotalsResult models the data returned from the getnettotals command.
type GetNetTotalsResult struct {
	TotalBytesRecv uint64 `json:"totalbytesrecv"`
	TotalBytesSent uint64 `json:"totalbytessent"`
	TimeMillis     int64  `json:"timemillis"`
}

// ScriptSig models a signature script.  It is defined seperately since it only
// applies to non-coinbase.  Therefore the field in the Vin structure needs
// to be a pointer.
type ScriptSig struct {
	Asm string `json:"asm"`
	Hex string `json:"hex"`
}

// Vin models parts of the tx data.  It is defined seperately since both
// getrawtransaction and decoderawtransaction use the same structure.
type Vin struct {
	Coinbase  string     `json:"coinbase"`
	Txid      string     `json:"txid"`
	Vout      int        `json:"vout"`
	ScriptSig *ScriptSig `json:"scriptSig"`
	Sequence  uint32     `json:"sequence"`
}

// IsCoinBase returns a bool to show if a Vin is a Coinbase one or not.
func (v *Vin) IsCoinBase() bool {
	return len(v.Coinbase) > 0
}

// MarshalJSON provides a custom Marshal method for Vin.
func (v *Vin) MarshalJSON() ([]byte, error) {
	if v.IsCoinBase() {
		coinbaseStruct := struct {
			Coinbase string `json:"coinbase"`
			Sequence uint32 `json:"sequence"`
		}{
			Coinbase: v.Coinbase,
			Sequence: v.Sequence,
		}
		return json.Marshal(coinbaseStruct)
	}

	txStruct := struct {
		Txid      string     `json:"txid"`
		Vout      int        `json:"vout"`
		ScriptSig *ScriptSig `json:"scriptSig"`
		Sequence  uint32     `json:"sequence"`
	}{
		Txid:      v.Txid,
		Vout:      v.Vout,
		ScriptSig: v.ScriptSig,
		Sequence:  v.Sequence,
	}
	return json.Marshal(txStruct)
}

// Vout models parts of the tx data.  It is defined seperately since both
// getrawtransaction and decoderawtransaction use the same structure.
type Vout struct {
	Value        float64 `json:"value"`
	N            int     `json:"n"`
	ScriptPubKey struct {
		Asm       string   `json:"asm"`
		Hex       string   `json:"hex"`
		ReqSigs   int      `json:"reqSigs,omitempty"`
		Type      string   `json:"type"`
		Addresses []string `json:"addresses,omitempty"`
	} `json:"scriptPubKey"`
}

// GetMiningInfoResult models the data from the getmininginfo command.
type GetMiningInfoResult struct {
	CurrentBlockSize float64 `json:"currentblocksize"`
	Difficulty       float64 `json:"difficulty"`
	Errors           string  `json:"errors"`
	Generate         bool    `json:"generate"`
	GenProcLimit     float64 `json:"genproclimit"`
	PooledTx         float64 `json:"pooledtx"`
	Testnet          bool    `json:"testnet"`
	Blocks           float64 `json:"blocks"`
	CurrentBlockTx   float64 `json:"currentblocktx"`
	HashesPerSec     float64 `json:"hashespersec"`
}

// GetWorkResult models the data from the getwork command.
type GetWorkResult struct {
	Data     string `json:"data"`
	Hash1    string `json:"hash1"`
	Midstate string `json:"midstate"`
	Target   string `json:"target"`
}

// ValidateAddressResult models the data from the validateaddress command.
type ValidateAddressResult struct {
	IsValid      bool   `json:"isvalid"`
	Address      string `json:"address,omitempty"`
	IsMine       bool   `json:"ismine,omitempty"`
	IsScript     bool   `json:"isscript,omitempty"`
	PubKey       string `json:"pubkey,omitempty"`
	IsCompressed bool   `json:"iscompressed,omitempty"`
	Account      string `json:"account,omitempty"`
}

// SignRawTransactionResult models the data from the signrawtransaction
// command.
type SignRawTransactionResult struct {
	Hex      string `json:"hex"`
	Complete bool   `json:"complete"`
}

// ListUnSpentResult models the data from the ListUnSpentResult command.
type ListUnSpentResult struct {
	TxId          string  `json:"txid"`
	Vout          float64 `json:"vout"`
	Address       string  `json:"address"`
	Account       string  `json:"account"`
	ScriptPubKey  string  `json:"scriptPubKey"`
	Amount        float64 `json:"amount"`
	Confirmations float64 `json:"confirmations"`
}

// GetAddedNodeInfoResultAddr models the data of the addresses portion of the
// getaddednodeinfo command.
type GetAddedNodeInfoResultAddr struct {
	Address   string `json:"address"`
	Connected string `json:"connected"`
}

// GetAddedNodeInfoResult models the data from the getaddednodeinfo command.
type GetAddedNodeInfoResult struct {
	AddedNode string                        `json:"addednode"`
	Connected *bool                         `json:"connected,omitempty"`
	Addresses *[]GetAddedNodeInfoResultAddr `json:"addresses,omitempty"`
}

// Error models the error field of the json returned by a bitcoin client.  When
// there is no error, this should be a nil pointer to produce the null in the
// json that bitcoind produces.
type Error struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Guarantee Error satisifies the builtin error interface
var _, _ error = Error{}, &Error{}

// Error returns a string describing the btcjson error.  This
// satisifies the builtin error interface.
func (e Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// jsonWithArgs takes a command, an id,  and an interface which contains an
// array of the arguments for that command.  It knows NOTHING about the commands
//  so all error checking of the arguments must happen before it is called.
func jsonWithArgs(command string, id interface{}, args interface{}) ([]byte, error) {
	rawMessage := Message{"1.0", id, command, args}
	finalMessage, err := json.Marshal(rawMessage)
	if err != nil {
		return nil, err
	}
	return finalMessage, nil
}

// CreateMessage takes a string and the optional arguments for it.  Then,
// if it is a recognized bitcoin json message, generates the json message ready
// to send off to the daemon or server.
// It is capable of handeling all of the commands from the standard client,
// described at:
// https://en.bitcoin.it/wiki/Original_Bitcoin_client/API_Calls_list
func CreateMessage(message string, args ...interface{}) ([]byte, error) {
	finalMessage, err := CreateMessageWithId(message, "btcd", args...)
	return finalMessage, err
}

// CreateMessageWithId takes a string, an id, and the optional arguments for
// it. Then, if it is a recognized bitcoin json message, generates the json
// message ready to send off to the daemon or server. It is capable of handling
// all of the commands from the standard client, described at:
// https://en.bitcoin.it/wiki/Original_Bitcoin_client/API_Calls_list
func CreateMessageWithId(message string, id interface{}, args ...interface{}) ([]byte, error) {
	var finalMessage []byte
	var err error
	// Different commands have different required and optional arguments.
	// Need to handle them based on that.
	switch message {
	// No args
	case "getblockcount", "getblocknumber", "getconnectioncount",
		"getdifficulty", "getgenerate", "gethashespersec", "getinfo",
		"getmininginfo", "getpeerinfo", "getrawmempool",
		"keypoolrefill", "listaddressgroupings", "listlockunspent",
		"stop", "walletlock", "getbestblockhash":
		if len(args) > 0 {
			err = fmt.Errorf("Too many arguments for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional int
	case "listaccounts":
		if len(args) > 1 {
			err = fmt.Errorf("Too many arguments for %s", message)
			return finalMessage, err
		}
		if len(args) == 1 {
			_, ok := args[0].(int)
			if !ok {
				err = fmt.Errorf("Argument must be int for %s", message)
				return finalMessage, err
			}
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required int
	case "getblockhash":
		if len(args) != 1 {
			err = fmt.Errorf("Missing argument for %s", message)
			return finalMessage, err
		}
		_, ok := args[0].(int)
		if !ok {
			err = fmt.Errorf("Argument must be int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required float
	case "settxfee":
		if len(args) != 1 {
			err = fmt.Errorf("Missing argument for %s", message)
			return finalMessage, err
		}
		_, ok := args[0].(float64)
		if !ok {
			err = fmt.Errorf("Argument must be float64 for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional string
	case "getmemorypool", "getnewaddress", "getwork", "help",
		"getrawchangeaddress":
		if len(args) > 1 {
			err = fmt.Errorf("Too many arguments for %s", message)
			return finalMessage, err
		}
		if len(args) == 1 {
			_, ok := args[0].(string)
			if !ok {
				err = fmt.Errorf("Optional argument must be string for %s", message)
				return finalMessage, err
			}
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string
	case "backupwallet", "decoderawtransaction", "dumpprivkey",
		"encryptwallet", "getaccount", "getaccountaddress",
		"getaddressesbyaccount", "getblock",
		"gettransaction", "sendrawtransaction", "validateaddress":
		if len(args) != 1 {
			err = fmt.Errorf("%s requires one argument", message)
			return finalMessage, err
		}
		_, ok := args[0].(string)
		if !ok {
			err = fmt.Errorf("Argument must be string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two required strings
	case "setaccount", "signmessage", "walletpassphrasechange", "addnode":
		if len(args) != 2 {
			err = fmt.Errorf("Missing arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			err = fmt.Errorf("Arguments must be string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one required int
	case "walletpassphrase":
		if len(args) != 2 {
			err = fmt.Errorf("Missing arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(int)
		if !ok1 || !ok2 {
			err = fmt.Errorf("Arguments must be string and int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Three required strings
	case "verifymessage":
		if len(args) != 3 {
			err = fmt.Errorf("Three arguments required for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(string)
		_, ok3 := args[2].(string)
		if !ok1 || !ok2 || !ok3 {
			err = fmt.Errorf("Arguments must be string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required bool, one optional string
	case "getaddednodeinfo":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(bool)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(string)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Arguments must be bool and optionally string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required bool, one optional int
	case "setgenerate":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("Wrong number of argument for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(bool)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(int)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Arguments must be bool and optionally int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional string, one optional int
	case "getbalance", "getreceivedbyaccount":
		if len(args) > 2 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		ok1 := true
		ok2 := true
		if len(args) >= 1 {
			_, ok1 = args[0].(string)
		}
		if len(args) == 2 {
			_, ok2 = args[1].(int)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Optional arguments must be string and int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one optional int
	case "getrawtransaction", "getreceivedbyaddress":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("Wrong number of argument for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(int)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Arguments must be string and optionally int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one optional string
	// Strictly, the optional arg for submit block is an object, but
	// bitcoind ignores it for now, so best to just allow string until
	// support for it is complete.
	case "submitblock":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("Wrong number of argument for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(string)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Arguments must be string and optionally string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional int, one optional bool
	case "listreceivedbyaccount", "listreceivedbyaddress":
		if len(args) > 2 {
			err = fmt.Errorf("Wrong number of argument for %s", message)
			return finalMessage, err
		}
		ok1 := true
		ok2 := true
		if len(args) >= 1 {
			_, ok1 = args[0].(int)
		}
		if len(args) == 2 {
			_, ok2 = args[1].(bool)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Optional arguments must be int and bool for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional string, two optional ints
	case "listtransactions":
		if len(args) > 3 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		ok1 := true
		ok2 := true
		ok3 := true
		if len(args) >= 1 {
			_, ok1 = args[0].(string)
		}
		if len(args) > 1 {
			_, ok2 = args[1].(int)
		}
		if len(args) == 3 {
			_, ok3 = args[2].(int)
		}
		if !ok1 || !ok2 || !ok3 {
			err = fmt.Errorf("Optional arguments must be string  and up to two ints for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one optional string, one optional bool
	case "importprivkey":
		if len(args) > 3 || len(args) == 0 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		ok2 := true
		ok3 := true
		if len(args) > 1 {
			_, ok2 = args[1].(string)
		}
		if len(args) == 3 {
			_, ok3 = args[2].(bool)
		}
		if !ok1 || !ok2 || !ok3 {
			err = fmt.Errorf("Arguments must be string and optionally string and bool for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two optional ints
	case "listunspent":
		if len(args) > 2 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		ok1 := true
		ok2 := true
		if len(args) >= 1 {
			_, ok1 = args[0].(int)
		}
		if len(args) == 2 {
			_, ok2 = args[1].(int)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Optional arguments must be ints for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two optional strings
	case "listsinceblock":
		if len(args) > 2 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		ok1 := true
		ok2 := true
		if len(args) >= 1 {
			_, ok1 = args[0].(string)
		}
		if len(args) == 2 {
			_, ok2 = args[1].(string)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("Optional arguments must be strings for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)

	// Two required strings, one required float, one optional int,
	// two optional strings.
	case "sendfrom":
		if len(args) > 6 || len(args) < 3 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(string)
		_, ok3 := args[2].(float64)
		ok4 := true
		ok5 := true
		ok6 := true
		if len(args) >= 4 {
			_, ok4 = args[3].(int)
		}
		if len(args) >= 5 {
			_, ok5 = args[4].(string)
		}
		if len(args) == 6 {
			_, ok6 = args[5].(string)
		}
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
			err = fmt.Errorf("Arguments must be string, string, float64 and optionally int and two strings for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two required strings, one required float, one optional int,
	// one optional string.
	case "move":
		if len(args) > 5 || len(args) < 3 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(string)
		_, ok3 := args[2].(float64)
		ok4 := true
		ok5 := true
		if len(args) >= 4 {
			_, ok4 = args[3].(int)
		}
		if len(args) == 5 {
			_, ok5 = args[4].(string)
		}
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
			err = fmt.Errorf("Arguments must be string, string, float64 and optionally int and string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required strings, one required float, two optional strings
	case "sendtoaddress":
		if len(args) > 4 || len(args) < 2 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(float64)
		ok3 := true
		ok4 := true
		if len(args) >= 3 {
			_, ok3 = args[2].(string)
		}
		if len(args) == 4 {
			_, ok4 = args[3].(string)
		}
		if !ok1 || !ok2 || !ok3 || !ok4 {
			err = fmt.Errorf("Arguments must be string, float64 and optionally two strings for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// required int, required pair of keys (string), optional string
	case "addmultisignaddress":
		if len(args) > 4 || len(args) < 3 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(int)
		_, ok2 := args[1].(string)
		_, ok3 := args[2].(string)
		ok4 := true
		if len(args) == 4 {
			_, ok4 = args[2].(string)
		}
		if !ok1 || !ok2 || !ok3 || !ok4 {
			err = fmt.Errorf("Arguments must be int, two string and optionally one for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Must be a set of string, int, string, float (any number of those).
	case "createrawtransaction":
		if len(args)%4 != 0 || len(args) == 0 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		type vlist struct {
			Txid string `json:"txid"`
			Vout int    `json:"vout"`
		}
		vList := make([]vlist, len(args)/4)
		addresses := make(map[string]float64)
		for i := 0; i < len(args)/4; i += 1 {
			txid, ok1 := args[(i*4)+0].(string)
			vout, ok2 := args[(i*4)+1].(int)
			add, ok3 := args[(i*4)+2].(string)
			amt, ok4 := args[(i*4)+3].(float64)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				err = fmt.Errorf("Incorrect arguement types.")
				return finalMessage, err
			}
			vList[i].Txid = txid
			vList[i].Vout = vout
			addresses[add] = amt
		}
		finalMessage, err = jsonWithArgs(message, id, []interface{}{vList, addresses})
	// string, string/float pairs, optional int, and string
	case "sendmany":
		if len(args) < 3 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		var minconf int
		var comment string
		_, ok1 := args[0].(string)
		if !ok1 {
			err = fmt.Errorf("Incorrect arguement types.")
			return finalMessage, err
		}
		addresses := make(map[string]float64)
		for i := 1; i < len(args); i += 2 {
			add, ok1 := args[i].(string)
			if ok1 {
				if len(args) > i+1 {
					amt, ok2 := args[i+1].(float64)
					if !ok2 {
						err = fmt.Errorf("Incorrect arguement types.")
						return finalMessage, err
					}
					// Put a single pair into addresses
					addresses[add] = amt
				} else {
					comment = add
				}
			} else {
				if _, ok := args[i].(int); ok {
					minconf = args[i].(int)
				}
				if len(args)-1 > i {
					if _, ok := args[i+1].(string); ok {
						comment = args[i+1].(string)
					}
				}
			}
		}
		finalMessage, err = jsonWithArgs(message, id, []interface{}{args[0].(string), addresses, minconf, comment})
	// bool and an array of stuff
	case "lockunspent":
		if len(args) < 2 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(bool)
		if !ok1 {
			err = fmt.Errorf("Incorrect arguement types.")
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// one required string (hex) and optional sets of one string, one int,
	// and one string along with another optional string.
	case "signrawtransaction":
		if len(args) < 1 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		if !ok1 {
			err = fmt.Errorf("Incorrect arguement types.")
			return finalMessage, err
		}
		type txlist struct {
			Txid         string `json:"txid"`
			Vout         int    `json:"vout"`
			ScriptPubKey string `json:"scriptPubKey"`
		}
		txList := make([]txlist, 1)

		if len(args) > 1 {
			txid, ok2 := args[1].(string)
			vout, ok3 := args[2].(int)
			spkey, ok4 := args[3].(string)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				err = fmt.Errorf("Incorrect arguement types.")
				return finalMessage, err
			}
			txList[0].Txid = txid
			txList[0].Vout = vout
			txList[0].ScriptPubKey = spkey
		}
		/*
			pkeyList := make([]string, (len(args)-1)/4)
			for i := 0; i < len(args)/4; i += 1 {
				fmt.Println(args[(i*4)+4])
				txid, ok1 := args[(i*4)+1].(string)
				vout, ok2 := args[(i*4)+2].(int)
				spkey, ok3 := args[(i*4)+3].(string)
				pkey, ok4 := args[(i*4)+4].(string)
				if !ok1 || !ok2 || !ok3 || !ok4 {
					err = fmt.Errorf("Incorrect arguement types.")
					return finalMessage, err
				}
				txList[i].Txid = txid
				txList[i].Vout = vout
				txList[i].ScriptPubKey = spkey
				pkeyList[i] = pkey
			}
		*/
		finalMessage, err = jsonWithArgs(message, id, []interface{}{args[0].(string), txList})
	// Any other message
	default:
		err = fmt.Errorf("Not a valid command: %s", message)
	}
	return finalMessage, err
}

// ReadResultCmd unmarshalls the json reply with data struct for specific
// commands or an interface if it is not a command where we already have a
// struct ready.
func ReadResultCmd(cmd string, message []byte) (Reply, error) {
	var result Reply
	var err error
	var objmap map[string]json.RawMessage
	err = json.Unmarshal(message, &objmap)
	if err != nil {
		if strings.Contains(string(message), "401 Unauthorized.") {
			err = fmt.Errorf("Authentication error.")
		} else {
			err = fmt.Errorf("Error unmarshalling json reply: %v", err)
		}
		return result, err
	}
	// Take care of the parts that are the same for all replies.
	var jsonErr Error
	var id interface{}
	err = json.Unmarshal(objmap["error"], &jsonErr)
	if err != nil {
		err = fmt.Errorf("Error unmarshalling json reply: %v", err)
		return result, err
	}
	err = json.Unmarshal(objmap["id"], &id)
	if err != nil {
		err = fmt.Errorf("Error unmarshalling json reply: %v", err)
		return result, err
	}

	// If it is a command where we have already worked out the reply,
	// generate put the results in the proper structure.
	// We handle the error condition after the switch statement.
	switch cmd {
	case "getaddednodeinfo":
		// getaddednodeinfo can either return a JSON object or a
		// slice of strings depending on the verbose flag.  Choose the
		// right form accordingly.
		var res interface{}
		if strings.Contains(string(objmap["result"]), "{") {
			res = []GetAddedNodeInfoResult{}
		} else {
			res = []string{}
		}
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getinfo":
		var res InfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getblock":
		// getblock can either return a JSON object or a hex-encoded
		// string depending on the verbose flag.  Choose the right form
		// accordingly.
		if strings.Contains(string(objmap["result"]), "{") {
			var res BlockResult
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		} else {
			var res string
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		}
	case "getnettotals":
		var res GetNetTotalsResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getnetworkhashps":
		var res int64
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getpeerinfo":
		var res []GetPeerInfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getrawtransaction":
		// getrawtransaction can either return a JSON object or a
		// hex-encoded string depending on the verbose flag.  Choose the
		// right form accordingly.
		if strings.Contains(string(objmap["result"]), "{") {
			var res TxRawResult
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		} else {
			var res string
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		}
	case "decoderawtransaction":
		var res TxRawDecodeResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getaddressesbyaccount":
		var res []string
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getmininginfo":
		var res GetMiningInfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getrawmempool":
		// getrawmempool can either return a map of JSON objects or
		// an array of strings depending on the verbose flag.  Choose
		// the right form accordingly.
		if strings.Contains(string(objmap["result"]), "{") {
			var res map[string]GetRawMempoolResult
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		} else {
			var res []string
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		}
	case "getwork":
		// getwork can either return a JSON object or a boolean
		// depending on whether or not data was provided.  Choose the
		// right form accordingly.
		if strings.Contains(string(objmap["result"]), "{") {
			var res GetWorkResult
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		} else {
			var res bool
			err = json.Unmarshal(objmap["result"], &res)
			if err == nil {
				result.Result = res
			}
		}
	case "validateaddress":
		var res ValidateAddressResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "signrawtransaction":
		var res SignRawTransactionResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "listunspent":
		var res []ListUnSpentResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	// For commands that return a single item (or no items), we get it with
	// the correct concrete type for free (but treat them separately
	// for clarity).
	case "getblockcount", "getbalance", "getblocknumber", "getgenerate",
		"getconnetioncount", "getdifficulty", "gethashespersec",
		"setgenerate", "stop", "settxfee", "getaccount",
		"getnewaddress", "sendtoaddress", "createrawtransaction",
		"sendrawtransaction", "getbestblockhash", "getrawchangeaddress":
		err = json.Unmarshal(message, &result)
	// For anything else put it in an interface.  All the data is still
	// there, just a little less convenient to deal with.
	default:
		err = json.Unmarshal(message, &result)
	}
	if err != nil {
		err = fmt.Errorf("Error unmarshalling json reply: %v", err)
		return result, err
	}
	// Only want the error field when there is an actual error to report.
	if jsonErr.Code != 0 {
		result.Error = &jsonErr
	}
	result.Id = &id
	return result, err
}

// JSONGetMethod takes a message and tries to find the bitcoin command that it
// is in reply to so it can be processed further.
func JSONGetMethod(message []byte) (string, error) {
	// Need this so we can tell what kind of message we are sending
	// so we can unmarshal it properly.
	var method string
	var msg interface{}
	err := json.Unmarshal(message, &msg)
	if err != nil {
		err := fmt.Errorf("Error, message does not appear to be valid json: %v", err)
		return method, err
	}
	m := msg.(map[string]interface{})
	for k, v := range m {
		if k == "method" {
			method = v.(string)
		}
	}
	if method == "" {
		err := fmt.Errorf("Error, no method specified.")
		return method, err
	}
	return method, err
}

// TlsRpcCommand takes a message generated from one of the routines above
// along with the login/server information and any relavent PEM encoded
// certificates chains. It sends the command via https and returns a go struct
// with the result.
func TlsRpcCommand(user string, password string, server string, message []byte,
	certificates []byte, skipverify bool) (Reply, error) {
	return rpcCommand(user, password, server, message, true, certificates,
		skipverify)
}

// RpcCommand takes a message generated from one of the routines above
// along with the login/server info, sends it, and gets a reply, returning
// a go struct with the result.
func RpcCommand(user string, password string, server string, message []byte) (Reply, error) {
	return rpcCommand(user, password, server, message, false, nil, false)
}

func rpcCommand(user string, password string, server string, message []byte,
	https bool, certificates []byte, skipverify bool) (Reply, error) {
	var result Reply
	method, err := JSONGetMethod(message)
	if err != nil {
		return result, err
	}
	body, err := rpcRawCommand(user, password, server, message, https,
		certificates, skipverify)
	if err != nil {
		err := fmt.Errorf("Error getting json reply: %v", err)
		return result, err
	}
	result, err = ReadResultCmd(method, body)
	if err != nil {
		err := fmt.Errorf("Error reading json message: %v", err)
		return result, err
	}
	return result, err
}

// TlsRpcRawCommand takes a message generated from one of the routines above
// along with the login,server info and PEM encoded certificate chains for the
// server  sends it, and gets a reply, returning
// the raw []byte response for use with ReadResultCmd.
func TlsRpcRawCommand(user string, password string, server string,
	message []byte, certificates []byte, skipverify bool) ([]byte, error) {
	return rpcRawCommand(user, password, server, message, true,
		certificates, skipverify)
}

// RpcRawCommand takes a message generated from one of the routines above
// along with the login/server info, sends it, and gets a reply, returning
// the raw []byte response for use with ReadResultCmd.
func RpcRawCommand(user string, password string, server string, message []byte) ([]byte, error) {
	return rpcRawCommand(user, password, server, message, false, nil, false)
}

// rpcRawCommand is a helper function for the above two functions.
func rpcRawCommand(user string, password string, server string,
	message []byte, https bool, certificates []byte, skipverify bool) ([]byte, error) {
	var result []byte
	var msg interface{}
	err := json.Unmarshal(message, &msg)
	if err != nil {
		err := fmt.Errorf("Error, message does not appear to be valid json: %v", err)
		return result, err
	}
	resp, err := jsonRpcSend(user, password, server, message, https,
		certificates, skipverify)
	if err != nil {
		err := fmt.Errorf("Error sending json message: " + err.Error())
		return result, err
	}
	result, err = GetRaw(resp.Body)
	if err != nil {
		err := fmt.Errorf("Error getting json reply: %v", err)
		return result, err
	}
	return result, err
}

// IsValidIdType checks that the Id field (which can go in any of the json
// messages) is valid.  json rpc 1.0 allows any (json) type, but we still need
// to prevent values that cannot be marshalled from going in.  json rpc 2.0
// (which bitcoind follows for some parts) only allows string, number, or null,
// so we restrict to that list.  Ths is only necessary if you manually marshal
// a message.  The normal btcjson functions only use string ids.
func IsValidIdType(id interface{}) bool {
	switch id.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		string,
		nil:
		return true
	default:
		return false
	}
}

// JSONToAmount Safely converts a floating point value to an int.
// Clearly not all floating point numbers can be converted to ints (there
// is no one-to-one mapping), but bitcoin's json api returns most numbers as
// floats which are not safe to use when handling money.  Since bitcoins can
// only be divided in a limited way, this methods works for the amounts returned
// by the json api.  It is not for general use.
// This follows the method described at:
// https://en.bitcoin.it/wiki/Proper_Money_Handling_%28JSON-RPC%29
func JSONToAmount(jsonAmount float64) (int64, error) {
	var amount int64
	var err error
	if jsonAmount > 1.797693134862315708145274237317043567981e+300 {
		err := fmt.Errorf("Error %v is too large to convert", jsonAmount)
		return amount, err
	}
	if jsonAmount < -1.797693134862315708145274237317043567981e+300 {
		err := fmt.Errorf("Error %v is too small to convert", jsonAmount)
		return amount, err
	}
	tempVal := 1e8 * jsonAmount
	// So we round properly.  float won't == 0 and if it did, that
	// would be converted fine anyway.
	if tempVal < 0 {
		tempVal = tempVal - 0.5
	}
	if tempVal > 0 {
		tempVal = tempVal + 0.5
	}
	// Then just rely on the integer truncating
	amount = int64(tempVal)
	return amount, err
}
