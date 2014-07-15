// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// BlockResult models the data from the getblock command when the verbose flag
// is set.  When the verbose flag is not set, getblock return a hex-encoded
// string.
type BlockResult struct {
	Hash          string        `json:"hash"`
	Confirmations uint64        `json:"confirmations"`
	Size          int32         `json:"size"`
	Height        int64         `json:"height"`
	Version       int32         `json:"version"`
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
	ReqSigs   int32    `json:"reqSigs,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
	P2sh      string   `json:"p2sh"`
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

// GetBlockChainInfoResult models the data returned from the getblockchaininfo
// command.
type GetBlockChainInfoResult struct {
	Chain                string  `json:"chain"`
	Blocks               int32   `json:"blocks"`
	BestBlockHash        string  `json:"bestblockhash"`
	Difficulty           float64 `json:"difficulty"`
	VerificationProgress float64 `json:"verificationprogress"`
	ChainWork            string  `json:"chainwork"`
}

// GetBlockTemplateResultTx models the transactions field of the
// getblocktemplate command.
type GetBlockTemplateResultTx struct {
	Data    string  `json:"data"`
	Hash    string  `json:"hash"`
	Depends []int64 `json:"depends"`
	Fee     int64   `json:"fee"`
	SigOps  int64   `json:"sigops"`
}

// GetBlockTemplateResultAux models the coinbaseaux field of the
// getblocktemplate command.
type GetBlockTemplateResultAux struct {
	Flags string `json:"flags"`
}

// GetBlockTemplateResult models the data returned from the getblocktemplate
// command.
type GetBlockTemplateResult struct {
	// Base fields from BIP 0022.  CoinbaseAux is optional.  One of
	// CoinbaseTxn or CoinbaseValue must be specified, but not both.
	Bits          string                     `json:"bits"`
	CurTime       int64                      `json:"curtime"`
	Height        int64                      `json:"height"`
	PreviousHash  string                     `json:"previousblockhash"`
	SigOpLimit    int64                      `json:"sigoplimit,omitempty"`
	SizeLimit     int64                      `json:"sizelimit,omitempty"`
	Transactions  []GetBlockTemplateResultTx `json:"transactions"`
	Version       int32                      `json:"version"`
	CoinbaseAux   *GetBlockTemplateResultAux `json:"coinbaseaux,omitempty"`
	CoinbaseTxn   *GetBlockTemplateResultTx  `json:"coinbasetxn,omitempty"`
	CoinbaseValue *int64                     `json:"coinbasevalue,omitempty"`
	WorkID        string                     `json:"workid,omitempty"`

	// Optional long polling from BIP 0022.
	LongPollID  string `json:"longpollid,omitempty"`
	LongPollURI string `json:"longpolluri,omitempty"`
	SubmitOld   *bool  `json:"submitold,omitempty"`

	// Basic pool extension from BIP 0023.
	Target  string `json:"target,omitempty"`
	Expires int64  `json:"expires,omitempty"`

	// Mutations from BIP 0023.
	MaxTime    int64    `json:"maxtime,omitempty"`
	MinTime    int64    `json:"mintime,omitempty"`
	Mutable    []string `json:"mutable,omitempty"`
	NonceRange string   `json:"noncerange,omitempty"`

	// Block proposal from BIP 0023.
	Capabilities  []string `json:"capabilities,omitempty"`
	RejectReasion string   `json:"reject-reason,omitempty"`
}

// GetNetworkInfoResult models the data returned from the getnetworkinfo command.
type GetNetworkInfoResult struct {
	Version         int32                  `json:"version"`
	ProtocolVersion int32                  `json:"protocolversion"`
	TimeOffset      int64                  `json:"timeoffset"`
	Connections     int32                  `json:"connections"`
	Proxy           string                 `json:"proxy,omitempty"`
	RelayFee        float64                `json:"relayfee"`
	LocalAddresses  []LocalAddressesResult `json:"localaddresses"`
}

// GetPeerInfoResult models the data returned from the getpeerinfo command.
type GetPeerInfoResult struct {
	Addr           string  `json:"addr"`
	AddrLocal      string  `json:"addrlocal,omitempty"`
	Services       string  `json:"services"`
	LastSend       int64   `json:"lastsend"`
	LastRecv       int64   `json:"lastrecv"`
	BytesSent      uint64  `json:"bytessent"`
	BytesRecv      uint64  `json:"bytesrecv"`
	PingTime       float64 `json:"pingtime"`
	PingWait       float64 `json:"pingwait,omitempty"`
	ConnTime       int64   `json:"conntime"`
	Version        uint32  `json:"version"`
	SubVer         string  `json:"subver"`
	Inbound        bool    `json:"inbound"`
	StartingHeight int32   `json:"startingheight"`
	BanScore       int32   `json:"banscore,omitempty"`
	SyncNode       bool    `json:"syncnode"`
}

// GetRawMempoolResult models the data returned from the getrawmempool command.
type GetRawMempoolResult struct {
	Size             int32    `json:"size"`
	Fee              float64  `json:"fee"`
	Time             int64    `json:"time"`
	Height           int64    `json:"height"`
	StartingPriority float64  `json:"startingpriority"`
	CurrentPriority  float64  `json:"currentpriority"`
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

// GetTxOutResult models the data from the gettxout command.
type GetTxOutResult struct {
	BestBlock     string             `json:"bestblock"`
	Confirmations int64              `json:"confirmations"`
	Value         float64            `json:"value"`
	ScriptPubKey  ScriptPubKeyResult `json:"scriptPubKey"`
	Version       int32              `json:"version"`
	Coinbase      bool               `json:"coinbase"`
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
	Vout      uint32     `json:"vout"`
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
		Vout      uint32     `json:"vout"`
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

// ScriptPubKeyResult models the scriptPubKey data of a tx script.  It is
// defined separately since it is used by multiple commands.
type ScriptPubKeyResult struct {
	Asm       string   `json:"asm"`
	Hex       string   `json:"hex,omitempty"`
	ReqSigs   int32    `json:"reqSigs,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
}

// Vout models parts of the tx data.  It is defined seperately since both
// getrawtransaction and decoderawtransaction use the same structure.
type Vout struct {
	Value        float64            `json:"value"`
	N            uint32             `json:"n"`
	ScriptPubKey ScriptPubKeyResult `json:"scriptPubKey"`
}

// GetMiningInfoResult models the data from the getmininginfo command.
type GetMiningInfoResult struct {
	Blocks           int64   `json:"blocks"`
	CurrentBlockSize uint64  `json:"currentblocksize"`
	CurrentBlockTx   uint64  `json:"currentblocktx"`
	Difficulty       float64 `json:"difficulty"`
	Errors           string  `json:"errors"`
	Generate         bool    `json:"generate"`
	GenProcLimit     int32   `json:"genproclimit"`
	HashesPerSec     int64   `json:"hashespersec"`
	NetworkHashPS    int64   `json:"networkhashps"`
	PooledTx         uint64  `json:"pooledtx"`
	TestNet          bool    `json:"testnet"`
}

// GetWorkResult models the data from the getwork command.
type GetWorkResult struct {
	Data     string `json:"data"`
	Hash1    string `json:"hash1"`
	Midstate string `json:"midstate"`
	Target   string `json:"target"`
}

// InfoResult contains the data returned by the getinfo command.
type InfoResult struct {
	Version         int32   `json:"version"`
	ProtocolVersion int32   `json:"protocolversion"`
	WalletVersion   int32   `json:"walletversion,omitempty"`
	Balance         float64 `json:"balance,omitempty"`
	Blocks          int32   `json:"blocks"`
	TimeOffset      int64   `json:"timeoffset"`
	Connections     int32   `json:"connections"`
	Proxy           string  `json:"proxy"`
	Difficulty      float64 `json:"difficulty"`
	TestNet         bool    `json:"testnet"`
	KeypoolOldest   int64   `json:"keypoololdest,omitempty"`
	KeypoolSize     int32   `json:"keypoolsize,omitempty"`
	UnlockedUntil   int64   `json:"unlocked_until,omitempty"`
	PaytxFee        float64 `json:"paytxfee,omitempty"`
	RelayFee        float64 `json:"relayfee"`
	Errors          string  `json:"errors"`
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

// LocalAddressesResult models the localaddresses data from the getnetworkinfo command.
type LocalAddressesResult struct {
	Address string `json:"address"`
	Port    uint16 `json:"port"`
	Score   int32  `json:"score"`
}

// ListReceivedByAccountResult models the data from the listreceivedbyaccount
// command.
type ListReceivedByAccountResult struct {
	Account       string  `json:"account"`
	Amount        float64 `json:"amount"`
	Confirmations uint64  `json:"confirmations"`
}

// ListReceivedByAddressResult models the data from the listreceivedbyaddress
// command.
type ListReceivedByAddressResult struct {
	Account       string  `json:"account"`
	Address       string  `json:"address"`
	Amount        float64 `json:"amount"`
	Confirmations uint64  `json:"confirmations"`
}

// ListSinceBlockResult models the data from the listsinceblock command.
type ListSinceBlockResult struct {
	Transactions []ListTransactionsResult `json:"transactions"`
	LastBlock    string                   `json:"lastblock"`
}

// ListUnspentResult models a successful response from the listunspent request.
type ListUnspentResult struct {
	TxId          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	Account       string  `json:"account"`
	ScriptPubKey  string  `json:"scriptPubKey"`
	RedeemScript  string  `json:"redeemScript,omitempty"`
	Amount        float64 `json:"amount"`
	Confirmations int64   `json:"confirmations"`
}

// SignRawTransactionResult models the data from the signrawtransaction
// command.
type SignRawTransactionResult struct {
	Hex      string `json:"hex"`
	Complete bool   `json:"complete"`
}

// TxRawResult models the data from the getrawtransaction command.
type TxRawResult struct {
	Hex           string `json:"hex"`
	Txid          string `json:"txid"`
	Version       int32  `json:"version"`
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
	Version  int32  `json:"version"`
	Locktime uint32 `json:"locktime"`
	Vin      []Vin  `json:"vin"`
	Vout     []Vout `json:"vout"`
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
	SigsRequired int32    `json:"sigsrequired,omitempty"`
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
			err = fmt.Errorf("authentication error")
		} else {
			err = fmt.Errorf("error unmarshalling json reply: %v", err)
		}
		return result, err
	}
	// Take care of the parts that are the same for all replies.
	var jsonErr Error
	var id interface{}
	err = json.Unmarshal(objmap["error"], &jsonErr)
	if err != nil {
		err = fmt.Errorf("error unmarshalling json reply: %v", err)
		return result, err
	}
	err = json.Unmarshal(objmap["id"], &id)
	if err != nil {
		err = fmt.Errorf("error unmarshalling json reply: %v", err)
		return result, err
	}

	// If it is a command where we have already worked out the reply,
	// generate put the results in the proper structure.
	// We handle the error condition after the switch statement.
	switch cmd {
	case "createmultisig":
		var res *CreateMultiSigResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "decodescript":
		var res *DecodeScriptResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getaddednodeinfo":
		// getaddednodeinfo can either return a JSON object or a
		// slice of strings depending on the verbose flag.  Choose the
		// right form accordingly.
		if bytes.IndexByte(objmap["result"], '{') > -1 {
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
		var res *InfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getblock":
		// getblock can either return a JSON object or a hex-encoded
		// string depending on the verbose flag.  Choose the right form
		// accordingly.
		if bytes.IndexByte(objmap["result"], '{') > -1 {
			var res *BlockResult
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
	case "getblockchaininfo":
		var res *GetBlockChainInfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getnettotals":
		var res *GetNetTotalsResult
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
		if bytes.IndexByte(objmap["result"], '{') > -1 {
			var res *TxRawResult
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
		var res *TxRawDecodeResult
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
		var res *GetMiningInfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "getnetworkinfo":
		var res *GetNetworkInfoResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			if res.LocalAddresses == nil {
				res.LocalAddresses = []LocalAddressesResult{}
			}
			result.Result = res
		}
	case "getrawmempool":
		// getrawmempool can either return a map of JSON objects or
		// an array of strings depending on the verbose flag.  Choose
		// the right form accordingly.
		if bytes.IndexByte(objmap["result"], '{') > -1 {
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
		var res *GetTransactionResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "gettxout":
		var res *GetTxOutResult
		err = json.Unmarshal(objmap["result"], &res)
		if res != nil && err == nil {
			if res.ScriptPubKey.Addresses == nil {
				res.ScriptPubKey.Addresses = []string{}
			}
			result.Result = res
		}
	case "getwork":
		// getwork can either return a JSON object or a boolean
		// depending on whether or not data was provided.  Choose the
		// right form accordingly.
		if bytes.IndexByte(objmap["result"], '{') > -1 {
			var res *GetWorkResult
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
		var res *ValidateAddressResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "signrawtransaction":
		var res *SignRawTransactionResult
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
	case "listreceivedbyaccount":
		var res []ListReceivedByAccountResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "listreceivedbyaddress":
		var res []ListReceivedByAddressResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	case "listsinceblock":
		var res *ListSinceBlockResult
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
		var res []ListUnspentResult
		err = json.Unmarshal(objmap["result"], &res)
		if err == nil {
			result.Result = res
		}
	// For commands that return a single item (or no items), we get it with
	// the correct concrete type for free (but treat them separately
	// for clarity).
	case "getblockcount", "getbalance", "getblockhash", "getgenerate",
		"getconnectioncount", "getdifficulty", "gethashespersec",
		"setgenerate", "stop", "settxfee", "getaccount",
		"getnewaddress", "sendtoaddress", "createrawtransaction",
		"sendrawtransaction", "getbestblockhash", "getrawchangeaddress",
		"sendfrom", "sendmany", "addmultisigaddress", "getunconfirmedbalance",
		"getaccountaddress", "estimatefee", "estimatepriority":
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
		err = fmt.Errorf("error unmarshalling json reply: %v", err)
		return result, err
	}
	// Only want the error field when there is an actual error to report.
	if jsonErr.Code != 0 {
		result.Error = &jsonErr
	}
	result.Id = &id
	return result, err
}
