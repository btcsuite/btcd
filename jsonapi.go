// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrIncorrectArgTypes describes an error where the wrong argument types
// are present.
var ErrIncorrectArgTypes = errors.New("incorrect arguement types")

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
//
// WARNING: This method is deprecated and may be removed in a future version.
// Do NOT use this as it does not work for all commands.  Instead, use one of
// the New<command>Cmd functions to create a specific command.
func CreateMessage(message string, args ...interface{}) ([]byte, error) {
	finalMessage, err := CreateMessageWithId(message, "btcd", args...)
	return finalMessage, err
}

// CreateMessageWithId takes a string, an id, and the optional arguments for
// it. Then, if it is a recognized bitcoin json message, generates the json
// message ready to send off to the daemon or server. It is capable of handling
// all of the commands from the standard client, described at:
// https://en.bitcoin.it/wiki/Original_Bitcoin_client/API_Calls_list
//
// WARNING: This method is deprecated and may be removed in a future version.
// Do NOT use this as it does not work for all commands.  Instead, use one of
// the New<command>Cmd functions to create a specific command.
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
		"stop", "walletlock", "getbestblockhash", "getblockchaininfo",
		"getnetworkinfo":
		if len(args) > 0 {
			err = fmt.Errorf("too many arguments for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional int
	case "listaccounts":
		if len(args) > 1 {
			err = fmt.Errorf("too many arguments for %s", message)
			return finalMessage, err
		}
		if len(args) == 1 {
			_, ok := args[0].(int)
			if !ok {
				err = fmt.Errorf("argument must be int for %s", message)
				return finalMessage, err
			}
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required int
	case "getblockhash", "estimatefee", "estimatepriority":
		if len(args) != 1 {
			err = fmt.Errorf("missing argument for %s", message)
			return finalMessage, err
		}
		_, ok := args[0].(int)
		if !ok {
			err = fmt.Errorf("argument must be int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required float
	case "settxfee":
		if len(args) != 1 {
			err = fmt.Errorf("missing argument for %s", message)
			return finalMessage, err
		}
		_, ok := args[0].(float64)
		if !ok {
			err = fmt.Errorf("argument must be float64 for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional string
	case "getmemorypool", "getnewaddress", "getwork", "help",
		"getrawchangeaddress":
		if len(args) > 1 {
			err = fmt.Errorf("too many arguments for %s", message)
			return finalMessage, err
		}
		if len(args) == 1 {
			_, ok := args[0].(string)
			if !ok {
				err = fmt.Errorf("optional argument must be string for %s", message)
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
			err = fmt.Errorf("argument must be string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two required strings
	case "setaccount", "signmessage", "walletpassphrasechange", "addnode":
		if len(args) != 2 {
			err = fmt.Errorf("missing arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			err = fmt.Errorf("arguments must be string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one required int
	case "walletpassphrase":
		if len(args) != 2 {
			err = fmt.Errorf("missing arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(int)
		if !ok1 || !ok2 {
			err = fmt.Errorf("arguments must be string and int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Three required strings
	case "verifymessage":
		if len(args) != 3 {
			err = fmt.Errorf("three arguments required for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		_, ok2 := args[1].(string)
		_, ok3 := args[2].(string)
		if !ok1 || !ok2 || !ok3 {
			err = fmt.Errorf("arguments must be string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required bool, one optional string
	case "getaddednodeinfo":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(bool)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(string)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("arguments must be bool and optionally string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required bool, one optional int
	case "setgenerate":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("wrong number of argument for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(bool)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(int)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("arguments must be bool and optionally int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional string, one optional int
	case "getbalance", "getreceivedbyaccount":
		if len(args) > 2 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("optional arguments must be string and int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one optional int
	case "getrawtransaction", "getreceivedbyaddress":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("wrong number of argument for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(int)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("arguments must be string and optionally int for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one optional string
	// Strictly, the optional arg for submit block is an object, but
	// bitcoind ignores it for now, so best to just allow string until
	// support for it is complete.
	case "submitblock":
		if len(args) > 2 || len(args) == 0 {
			err = fmt.Errorf("wrong number of argument for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		ok2 := true
		if len(args) == 2 {
			_, ok2 = args[1].(string)
		}
		if !ok1 || !ok2 {
			err = fmt.Errorf("arguments must be string and optionally string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional int, one optional bool
	case "listreceivedbyaccount", "listreceivedbyaddress":
		if len(args) > 2 {
			err = fmt.Errorf("wrong number of argument for %s", message)
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
			err = fmt.Errorf("optional arguments must be int and bool for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One optional string, two optional ints
	case "listtransactions":
		if len(args) > 3 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("optional arguments must be string  and up to two ints for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required string, one optional string, one optional bool
	case "importprivkey":
		if len(args) > 3 || len(args) == 0 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("arguments must be string and optionally string and bool for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two optional ints
	case "listunspent":
		if len(args) > 2 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("optional arguments must be ints for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two optional strings
	case "listsinceblock":
		if len(args) > 2 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("optional arguments must be strings for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)

	// Two required strings, one required float, one optional int,
	// two optional strings.
	case "sendfrom":
		if len(args) > 6 || len(args) < 3 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("arguments must be string, string, float64 and optionally int and two strings for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Two required strings, one required float, one optional int,
	// one optional string.
	case "move":
		if len(args) > 5 || len(args) < 3 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("arguments must be string, string, float64 and optionally int and string for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// One required strings, one required float, two optional strings
	case "sendtoaddress":
		if len(args) > 4 || len(args) < 2 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("arguments must be string, float64 and optionally two strings for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// required int, required pair of keys (string), optional string
	case "addmultisignaddress":
		if len(args) > 4 || len(args) < 3 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
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
			err = fmt.Errorf("arguments must be int, two string and optionally one for %s", message)
			return finalMessage, err
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// Must be a set of string, int, string, float (any number of those).
	case "createrawtransaction":
		if len(args)%4 != 0 || len(args) == 0 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
			return finalMessage, err
		}
		type vlist struct {
			Txid string `json:"txid"`
			Vout uint32 `json:"vout"`
		}
		vList := make([]vlist, len(args)/4)
		addresses := make(map[string]float64)
		for i := 0; i < len(args)/4; i++ {
			txid, ok1 := args[(i*4)+0].(string)
			vout, ok2 := args[(i*4)+1].(uint32)
			add, ok3 := args[(i*4)+2].(string)
			amt, ok4 := args[(i*4)+3].(float64)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				return finalMessage, ErrIncorrectArgTypes
			}
			vList[i].Txid = txid
			vList[i].Vout = vout
			addresses[add] = amt
		}
		finalMessage, err = jsonWithArgs(message, id, []interface{}{vList, addresses})
	// string, string/float pairs, optional int, and string
	case "sendmany":
		if len(args) < 3 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
			return finalMessage, err
		}
		var minconf int
		var comment string
		_, ok1 := args[0].(string)
		if !ok1 {
			return finalMessage, ErrIncorrectArgTypes
		}
		addresses := make(map[string]float64)
		for i := 1; i < len(args); i += 2 {
			add, ok1 := args[i].(string)
			if ok1 {
				if len(args) > i+1 {
					amt, ok2 := args[i+1].(float64)
					if !ok2 {
						return finalMessage, ErrIncorrectArgTypes
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
			err = fmt.Errorf("wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(bool)
		if !ok1 {
			return finalMessage, ErrIncorrectArgTypes
		}
		finalMessage, err = jsonWithArgs(message, id, args)
	// one required string (hex) and optional sets of one string, one int,
	// and one string along with another optional string.
	case "signrawtransaction":
		if len(args) < 1 {
			err = fmt.Errorf("wrong number of arguments for %s", message)
			return finalMessage, err
		}
		_, ok1 := args[0].(string)
		if !ok1 {
			return finalMessage, ErrIncorrectArgTypes
		}
		type txlist struct {
			Txid         string `json:"txid"`
			Vout         uint32 `json:"vout"`
			ScriptPubKey string `json:"scriptPubKey"`
		}
		txList := make([]txlist, 1)

		if len(args) > 1 {
			txid, ok2 := args[1].(string)
			vout, ok3 := args[2].(uint32)
			spkey, ok4 := args[3].(string)
			if !ok1 || !ok2 || !ok3 || !ok4 {
				return finalMessage, ErrIncorrectArgTypes
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
					return finalMessage, ErrIncorrectArgTypes
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
		err = fmt.Errorf("not a valid command: %s", message)
	}
	return finalMessage, err
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
		err := fmt.Errorf("error, message does not appear to be valid json: %v", err)
		return method, err
	}
	m := msg.(map[string]interface{})
	for k, v := range m {
		if k == "method" {
			method = v.(string)
		}
	}
	if method == "" {
		err := fmt.Errorf("error, no method specified")
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
		err := fmt.Errorf("error getting json reply: %v", err)
		return result, err
	}
	result, err = ReadResultCmd(method, body)
	if err != nil {
		err := fmt.Errorf("error reading json message: %v", err)
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
		err := fmt.Errorf("error, message does not appear to be valid json: %v", err)
		return result, err
	}
	resp, err := jsonRpcSend(user, password, server, message, https,
		certificates, skipverify)
	if err != nil {
		err := fmt.Errorf("error sending json message: " + err.Error())
		return result, err
	}
	result, err = GetRaw(resp.Body)
	if err != nil {
		err := fmt.Errorf("error getting json reply: %v", err)
		return result, err
	}
	return result, err
}

// RpcSend sends the passed command to the provided server using the provided
// authentication details, waits for a reply, and returns a Go struct with the
// result.
func RpcSend(user string, password string, server string, cmd Cmd) (Reply, error) {
	msg, err := cmd.MarshalJSON()
	if err != nil {
		return Reply{}, err
	}

	return RpcCommand(user, password, server, msg)
}

// TlsRpcSend sends the passed command to the provided server using the provided
// authentication details and PEM encoded certificate chain, waits for a reply,
// and returns a Go struct with the result.
func TlsRpcSend(user string, password string, server string, cmd Cmd,
	certificates []byte, skipVerify bool) (Reply, error) {

	msg, err := cmd.MarshalJSON()
	if err != nil {
		return Reply{}, err
	}

	return TlsRpcCommand(user, password, server, msg, certificates,
		skipVerify)
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
		err := fmt.Errorf("error %v is too large to convert", jsonAmount)
		return amount, err
	}
	if jsonAmount < -1.797693134862315708145274237317043567981e+300 {
		err := fmt.Errorf("error %v is too small to convert", jsonAmount)
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
