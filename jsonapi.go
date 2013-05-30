// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"fmt"
)

// Message contains a message to be sent to the bitcoin client.
type Message struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id"`
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
	Connections     int     `json:"connections,omitempty"`
	Proxy           string  `json:"proxy,omitempty"`
	Difficulty      float64 `json:"difficulty,omitempty"`
	TestNet         bool    `json:"testnet,omitempty"`
	KeypoolOldest   int64   `json:"keypoololdest,omitempty"`
	KeypoolSize     int     `json:"keypoolsize,omitempty"`
	PaytxFee        float64 `json:"paytxfee,omitempty"`
	Errors          string  `json:"errors,omitempty"`
}

// BlockResult models the data from the getblock command.
type BlockResult struct {
	Hash          string   `json:"hash"`
	Confirmations uint64   `json:"confirmations"`
	Size          int      `json:"size"`
	Height        int64    `json:"height"`
	Version       uint32   `json:"version"`
	MerkleRoot    string   `json:"merkleroot"`
	Tx            []string `json:"tx"`
	Time          int64    `json:"time"`
	Nonce         uint32   `json:"nonce"`
	Bits          string   `json:"bits"`
	Difficulty    float64  `json:"difficulty"`
	PreviousHash  string   `json:"previousblockhash"`
	NextHash      string   `json:"nextblockhash"`
}

// TxRawResult models the data from the getrawtransaction command.
type TxRawResult struct {
	Hex           string `json:"hex"`
	Txid          string `json:"txid"`
	Version       uint32 `json:"version"`
	LockTime      uint32 `json:"locktime"`
	Vin           []Vin  `json:"vin"`
	Vout          []Vout `json:"vout"`
	BlockHash     string `json:"blockhash"`
	Confirmations uint64 `json:"confirmations"`
	Time          int64  `json:"time"`
	Blocktime     int64  `json:"blocktime"`
}

// TxRawDecodeResult models the data from the decoderawtransaction command.
type TxRawDecodeResult struct {
	Txid     string `json:"txid"`
	Version  uint32 `json:"version"`
	Locktime int    `json:"locktime"`
	Vin      []Vin  `json:"vin"`
	Vout     []Vout `json:"vout"`
}

// Vin models parts of the tx data.  It is defined seperately since both
// getrawtransaction and decoderawtransaction use the same structure.
type Vin struct {
	Coinbase  string `json:"coinbase,omitempty"`
	Vout      int    `json:"vout,omitempty"`
	ScriptSig struct {
		Txid string `json:"txid"`
		Asm  string `json:"asm"`
		Hex  string `json:"hex"`
	} `json:"scriptSig,omitempty"`
	Sequence float64 `json:"sequence"`
}

// Vout models parts of the tx data.  It is defined seperately since both
// getrawtransaction and decoderawtransaction use the same structure.
type Vout struct {
	Value        float64 `json:"value"`
	N            int     `json:"n"`
	ScriptPubKey struct {
		Asm       string   `json:"asm"`
		Hex       string   `json:"hex"`
		ReqSig    int      `json:"reqSig"`
		Type      string   `json:"type"`
		Addresses []string `json:"addresses"`
	} `json:"scriptPubKey"`
}

// Error models the error field of the json returned by a bitcoin client.  When
// there is no error, this should be a nil pointer to produce the null in the
// json that bitcoind produces.
type Error struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// jsonWithArgs takes a command and an interface which contains an array
// of the arguments for that command.  It knows NOTHING about the commands so
// all error checking of the arguments must happen before it is called.
func jsonWithArgs(command string, args interface{}) ([]byte, error) {
	rawMessage := Message{"1.0", "btcd", command, args}
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
		"stop", "walletlock":
		if len(args) > 0 {
			err = fmt.Errorf("Too many arguments for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
	// One optional string
	case "getmemorypool", "getnewaddress", "getwork", "help":
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
		finalMessage, err = jsonWithArgs(message, args)
	// One required string
	case "backupwallet", "decoderawtransaction", "dumpprivkey",
		"encryptwallet", "getaccount", "getaccountaddress",
		"getaddressbyaccount", "getblock",
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
		finalMessage, err = jsonWithArgs(message, args)
	// Two required strings
	case "listsinceblock", "setaccount", "signmessage", "walletpassphrase",
		"walletpassphrasechange":
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
	// One required string, one optional int
	case "addnode", "getrawtransaction", "getreceivedbyaddress":
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
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
		finalMessage, err = jsonWithArgs(message, args)
	// Must be a set of 3 strings and a float (any number of those)
	case "createrawtransaction":
		if len(args)%4 != 0 || len(args) == 0 {
			err = fmt.Errorf("Wrong number of arguments for %s", message)
			return finalMessage, err
		}
		type vlist struct {
			Vin  string `json:"vin"`
			Vout string `json:"vout"`
		}
		vList := make([]vlist, len(args)/4)
		addresses := make(map[string]float64)
		for i := 0; i < len(args)/4; i += 1 {
			vin, ok1 := args[(i*4)+0].(string)
			vout, ok2 := args[(i*4)+1].(string)
			add, ok3 := args[(i*4)+2].(string)
			amt, ok4 := args[(i*4)+3].(float64)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				err = fmt.Errorf("Incorrect arguement types.")
				return finalMessage, err
			}
			vList[i].Vin = vin
			vList[i].Vout = vout
			addresses[add] = amt
		}
		finalMessage, err = jsonWithArgs(message, []interface{}{vList, addresses})
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
		finalMessage, err = jsonWithArgs(message, []interface{}{args[0].(string), addresses, minconf, comment})
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
		finalMessage, err = jsonWithArgs(message, args)
	// one required string (hex) and at least one set of 4 other strings.
	case "signrawtransaction":
		if (len(args)-1)%4 != 0 || len(args) < 5 {
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
			Vout         string `json:"vout"`
			ScriptPubKey string `json:"scriptPubKey"`
		}
		txList := make([]txlist, (len(args)-1)/4)
		pkeyList := make([]string, (len(args)-1)/4)
		for i := 0; i < len(args)/4; i += 1 {
			txid, ok1 := args[(i*4)+0].(string)
			vout, ok2 := args[(i*4)+1].(string)
			spkey, ok3 := args[(i*4)+2].(string)
			pkey, ok4 := args[(i*4)+3].(string)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				err = fmt.Errorf("Incorrect arguement types.")
				return finalMessage, err
			}
			txList[i].Txid = txid
			txList[i].Vout = vout
			txList[i].ScriptPubKey = spkey
			pkeyList[i] = pkey
		}
		finalMessage, err = jsonWithArgs(message, []interface{}{args[0].(string), txList, pkeyList})
	// Any other message
	default:
		err = fmt.Errorf("Not a valid command: %s", message)
	}
	return finalMessage, err
}

// readResultCmd unmarshalls the json reply with data struct for specific
// commands or an interface if it is not a command where we already have a
// struct ready.
func readResultCmd(cmd string, message []byte) (Reply, error) {
	var result Reply
	var err error
	var objmap map[string]json.RawMessage
	err = json.Unmarshal(message, &objmap)
	if err != nil {
		err = fmt.Errorf("Error unmarshalling json reply: %v", err)
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
	switch cmd {
	case "getinfo":
		var res InfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err != nil {
			err = fmt.Errorf("Error unmarshalling json reply: %v", err)
			return result, err
		}
		result.Result = res
	case "getblock":
		var res BlockResult
		err = json.Unmarshal(objmap["result"], &res)
		if err != nil {
			err = fmt.Errorf("Error unmarshalling json reply: %v", err)
			return result, err
		}
		result.Result = res
	case "getrawtransaction":
		var res TxRawResult
		err = json.Unmarshal(objmap["result"], &res)
		if err != nil {
			err = fmt.Errorf("Error unmarshalling json reply: %v", err)
			return result, err
		}
		result.Result = res
	case "decoderawtransaction":
		var res TxRawDecodeResult
		err = json.Unmarshal(objmap["result"], &res)
		if err != nil {
			err = fmt.Errorf("Error unmarshalling json reply: %v", err)
			return result, err
		}
		result.Result = res
	// For anything else put it in an interface.  All the data is still
	// there, just a little less convenient to deal with.
	default:
		err = json.Unmarshal(message, &result)
		if err != nil {
			err = fmt.Errorf("Error unmarshalling json reply: %v", err)
			return result, err
		}
	}
	// Only want the error field when there is an actual error to report.
	if jsonErr.Code != 0 {
		result.Error = &jsonErr
	}
	result.Id = &id
	return result, err
}

// RpcCommand takes a message generated from one of the routines above
// along with the login/server info, sends it, and gets a reply, returning
// a go struct with the result.
func RpcCommand(user string, password string, server string, message []byte) (Reply, error) {
	var result Reply
	// Need this so we can tell what kind of message we are sending
	// so we can unmarshal it properly.
	var method string
	var msg interface{}
	err := json.Unmarshal(message, &msg)
	if err != nil {
		err := fmt.Errorf("Error, message does not appear to be valid json: %v", err)
		return result, err
	}
	m := msg.(map[string]interface{})
	for k, v := range m {
		if k == "method" {
			method = v.(string)
		}
	}
	if method == "" {
		err := fmt.Errorf("Error, no method specified.")
		return result, err
	}
	resp, err := jsonRpcSend(user, password, server, message)
	if err != nil {
		err := fmt.Errorf("Error Sending json message.")
		return result, err
	}
	body, err := GetRaw(resp.Body)
	if err != nil {
		err := fmt.Errorf("Error getting json reply: %v", err)
		return result, err
	}
	result, err = readResultCmd(method, body)
	if err != nil {
		err := fmt.Errorf("Error reading json message: %v", err)
		return result, err
	}
	return result, err
}
