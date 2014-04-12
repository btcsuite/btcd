// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
	UnlockedUntil   int64   `json:"unlocked_until,omitempty"`
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

// CreateMultiSigResult models the data returned from the createmultisig command.
type CreateMultiSigResult struct {
	Address      string `json:"address"`
	RedeemScript string `json:"redeemScript"`
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
	AddrLocal      string `json:"addrlocal,omitempty"`
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

// GetTransactionDetailsResult models the details data from the gettransaction command.
type GetTransactionDetailsResult struct {
	Account  string  `json:"account"`
	Address  string  `json:"address,omitempty"`
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
	Fee      float64 `json:"fee,omitempty"`
}

// GetTransactionResult models the data from the gettransaction command.
type GetTransactionResult struct {
	Amount          float64                       `json:"amount"`
	Fee             float64                       `json:"fee,omitempty"`
	Confirmations   int64                         `json:"confirmations"`
	BlockHash       string                        `json:"blockhash"`
	BlockIndex      int64                         `json:"blockindex"`
	BlockTime       int64                         `json:"blocktime"`
	TxID            string                        `json:"txid"`
	WalletConflicts []string                      `json:"walletconflicts"`
	Time            int64                         `json:"time"`
	TimeReceived    int64                         `json:"timereceived"`
	Details         []GetTransactionDetailsResult `json:"details"`
	Hex             string                        `json:"hex"`
}

// ListTransactionsResult models the data from the listtransactions command.
type ListTransactionsResult struct {
	Account         string   `json:"account"`
	Address         string   `json:"address,omitempty"`
	Category        string   `json:"category"`
	Amount          float64  `json:"amount"`
	Fee             float64  `json:"fee"`
	Confirmations   int64    `json:"confirmations"`
	Generated       bool     `json:"generated,omitempty"`
	BlockHash       string   `json:"blockhash,omitempty"`
	BlockIndex      int64    `json:"blockindex,omitempty"`
	BlockTime       int64    `json:"blocktime,omitempty"`
	TxID            string   `json:"txid"`
	WalletConflicts []string `json:"walletconflicts"`
	Time            int64    `json:"time"`
	TimeReceived    int64    `json:"timereceived"`
	Comment         string   `json:"comment,omitempty"`
	OtherAccount    string   `json:"otheraccount"`
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
	IsValid      bool     `json:"isvalid"`
	Address      string   `json:"address,omitempty"`
	IsMine       bool     `json:"ismine,omitempty"`
	IsScript     bool     `json:"isscript,omitempty"`
	PubKey       string   `json:"pubkey,omitempty"`
	IsCompressed bool     `json:"iscompressed,omitempty"`
	Account      string   `json:"account,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
	Hex          string   `json:"hex,omitempty"`
	Script       string   `json:"script,omitempty"`
	SigsRequired int      `json:"sigsrequired,omitempty"`
}

// SignRawTransactionResult models the data from the signrawtransaction
// command.
type SignRawTransactionResult struct {
	Hex      string `json:"hex"`
	Complete bool   `json:"complete"`
}

// ListSinceBlockResult models the data from the listsinceblock command.
type ListSinceBlockResult struct {
	Transactions []ListTransactionsResult `json:"transactions"`
	LastBlock    string                   `json:"lastblock"`
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
	case "createmultisig":
		var res CreateMultiSigResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getaddednodeinfo":
		// getaddednodeinfo can either return a JSON object or a
		// slice of strings depending on the verbose flag.  Choose the
		// right form accordingly.
		if strings.Contains(string(objmap["result"]), "{") {
			var res []GetAddedNodeInfoResult
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
	case "gettransaction":
		var res GetTransactionResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
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
	case "listaccounts":
		var res map[string]float64
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "listsinceblock":
		var res ListSinceBlockResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			if res.Transactions == nil {
				res.Transactions = []ListTransactionsResult{}
			}
			result.Result = res
		}
	case "listtransactions":
		var res []ListTransactionsResult
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
	default:
		// None of the standard Bitcoin RPC methods matched.  Try
		// registered custom command reply parsers.
		if c, ok := customCmds[cmd]; ok && c.replyParser != nil {
			var res interface{}
			res, err = c.replyParser(objmap["result"])
			if err == nil {
				result.Result = res
			}
		} else {
			// For anything else put it in an interface.  All the
			// data is still there, just a little less convenient
			// to deal with.
			err = json.Unmarshal(message, &result)
		}
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
