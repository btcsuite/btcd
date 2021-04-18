// Copyright (c) 2014-2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/txscript"
)

// CreateWalletResult models the result of the createwallet command.
type CreateWalletResult struct {
	Name    string `json:"name"`
	Warning string `json:"warning"`
}

// embeddedAddressInfo includes all getaddressinfo output fields, excluding
// metadata and relation to the wallet.
//
// It represents the non-metadata/non-wallet fields for GetAddressInfo, as well
// as the precise fields for an embedded P2SH or P2WSH address.
type embeddedAddressInfo struct {
	Address             string                `json:"address"`
	ScriptPubKey        string                `json:"scriptPubKey"`
	Solvable            bool                  `json:"solvable"`
	Descriptor          *string               `json:"desc,omitempty"`
	IsScript            bool                  `json:"isscript"`
	IsChange            bool                  `json:"ischange"`
	IsWitness           bool                  `json:"iswitness"`
	WitnessVersion      int                   `json:"witness_version,omitempty"`
	WitnessProgram      *string               `json:"witness_program,omitempty"`
	ScriptType          *txscript.ScriptClass `json:"script,omitempty"`
	Hex                 *string               `json:"hex,omitempty"`
	PubKeys             *[]string             `json:"pubkeys,omitempty"`
	SignaturesRequired  *int                  `json:"sigsrequired,omitempty"`
	PubKey              *string               `json:"pubkey,omitempty"`
	IsCompressed        *bool                 `json:"iscompressed,omitempty"`
	HDMasterFingerprint *string               `json:"hdmasterfingerprint,omitempty"`
	Labels              []string              `json:"labels"`
}

// GetAddressInfoResult models the result of the getaddressinfo command. It
// contains information about a bitcoin address.
//
// Reference: https://bitcoincore.org/en/doc/0.20.0/rpc/wallet/getaddressinfo
//
// The GetAddressInfoResult has three segments:
//   1. General information about the address.
//   2. Metadata (Timestamp, HDKeyPath, HDSeedID) and wallet fields
//      (IsMine, IsWatchOnly).
//   3. Information about the embedded address in case of P2SH or P2WSH.
//      Same structure as (1).
type GetAddressInfoResult struct {
	embeddedAddressInfo
	IsMine      bool                 `json:"ismine"`
	IsWatchOnly bool                 `json:"iswatchonly"`
	Timestamp   *int                 `json:"timestamp,omitempty"`
	HDKeyPath   *string              `json:"hdkeypath,omitempty"`
	HDSeedID    *string              `json:"hdseedid,omitempty"`
	Embedded    *embeddedAddressInfo `json:"embedded,omitempty"`
}

// UnmarshalJSON provides a custom unmarshaller for GetAddressInfoResult.
// It is adapted to avoid creating a duplicate raw struct for unmarshalling
// the JSON bytes into.
//
// Reference: http://choly.ca/post/go-json-marshalling
func (e *GetAddressInfoResult) UnmarshalJSON(data []byte) error {
	// Step 1: Create type aliases of the original struct, including the
	// embedded one.
	type Alias GetAddressInfoResult
	type EmbeddedAlias embeddedAddressInfo

	// Step 2: Create an anonymous struct with raw replacements for the special
	// fields.
	aux := &struct {
		ScriptType *string `json:"script,omitempty"`
		Embedded   *struct {
			ScriptType *string `json:"script,omitempty"`
			*EmbeddedAlias
		} `json:"embedded,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(e),
	}

	// Step 3: Unmarshal the data into the anonymous struct.
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Step 4: Convert the raw fields to the desired types
	var (
		sc  *txscript.ScriptClass
		err error
	)

	if aux.ScriptType != nil {
		sc, err = txscript.NewScriptClass(*aux.ScriptType)
		if err != nil {
			return err
		}
	}

	e.ScriptType = sc

	if aux.Embedded != nil {
		var (
			embeddedSc *txscript.ScriptClass
			err        error
		)

		if aux.Embedded.ScriptType != nil {
			embeddedSc, err = txscript.NewScriptClass(*aux.Embedded.ScriptType)
			if err != nil {
				return err
			}
		}

		e.Embedded = (*embeddedAddressInfo)(aux.Embedded.EmbeddedAlias)
		e.Embedded.ScriptType = embeddedSc
	}

	return nil
}

// GetTransactionDetailsResult models the details data from the gettransaction command.
//
// This models the "short" version of the ListTransactionsResult type, which
// excludes fields common to the transaction.  These common fields are instead
// part of the GetTransactionResult.
type GetTransactionDetailsResult struct {
	Account           string   `json:"account"`
	Address           string   `json:"address,omitempty"`
	Amount            float64  `json:"amount"`
	Category          string   `json:"category"`
	InvolvesWatchOnly bool     `json:"involveswatchonly,omitempty"`
	Fee               *float64 `json:"fee,omitempty"`
	Vout              uint32   `json:"vout"`
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

type ScanningOrFalse struct {
	Value interface{}
}

type ScanProgress struct {
	Duration int     `json:"duration"`
	Progress float64 `json:"progress"`
}

// MarshalJSON implements the json.Marshaler interface
func (h ScanningOrFalse) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.Value)
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (h *ScanningOrFalse) UnmarshalJSON(data []byte) error {
	var unmarshalled interface{}
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		return err
	}

	switch v := unmarshalled.(type) {
	case bool:
		h.Value = v
	case map[string]interface{}:
		h.Value = ScanProgress{
			Duration: int(v["duration"].(float64)),
			Progress: v["progress"].(float64),
		}
	default:
		return fmt.Errorf("invalid scanning value: %v", unmarshalled)
	}

	return nil
}

// GetWalletInfoResult models the result of the getwalletinfo command.
type GetWalletInfoResult struct {
	WalletName            string          `json:"walletname"`
	WalletVersion         int             `json:"walletversion"`
	TransactionCount      int             `json:"txcount"`
	KeyPoolOldest         int             `json:"keypoololdest"`
	KeyPoolSize           int             `json:"keypoolsize"`
	KeyPoolSizeHDInternal *int            `json:"keypoolsize_hd_internal,omitempty"`
	UnlockedUntil         *int            `json:"unlocked_until,omitempty"`
	PayTransactionFee     float64         `json:"paytxfee"`
	HDSeedID              *string         `json:"hdseedid,omitempty"`
	PrivateKeysEnabled    bool            `json:"private_keys_enabled"`
	AvoidReuse            bool            `json:"avoid_reuse"`
	Scanning              ScanningOrFalse `json:"scanning"`
}

// InfoWalletResult models the data returned by the wallet server getinfo
// command.
type InfoWalletResult struct {
	Version         int32   `json:"version"`
	ProtocolVersion int32   `json:"protocolversion"`
	WalletVersion   int32   `json:"walletversion"`
	Balance         float64 `json:"balance"`
	Blocks          int32   `json:"blocks"`
	TimeOffset      int64   `json:"timeoffset"`
	Connections     int32   `json:"connections"`
	Proxy           string  `json:"proxy"`
	Difficulty      float64 `json:"difficulty"`
	TestNet         bool    `json:"testnet"`
	KeypoolOldest   int64   `json:"keypoololdest"`
	KeypoolSize     int32   `json:"keypoolsize"`
	UnlockedUntil   int64   `json:"unlocked_until"`
	PaytxFee        float64 `json:"paytxfee"`
	RelayFee        float64 `json:"relayfee"`
	Errors          string  `json:"errors"`
}

// ListTransactionsResult models the data from the listtransactions command.
type ListTransactionsResult struct {
	Abandoned         bool     `json:"abandoned"`
	Account           string   `json:"account"`
	Address           string   `json:"address,omitempty"`
	Amount            float64  `json:"amount"`
	BIP125Replaceable string   `json:"bip125-replaceable,omitempty"`
	BlockHash         string   `json:"blockhash,omitempty"`
	BlockHeight       *int32   `json:"blockheight,omitempty"`
	BlockIndex        *int64   `json:"blockindex,omitempty"`
	BlockTime         int64    `json:"blocktime,omitempty"`
	Category          string   `json:"category"`
	Confirmations     int64    `json:"confirmations"`
	Fee               *float64 `json:"fee,omitempty"`
	Generated         bool     `json:"generated,omitempty"`
	InvolvesWatchOnly bool     `json:"involveswatchonly,omitempty"`
	Label             *string  `json:"label,omitempty"`
	Time              int64    `json:"time"`
	TimeReceived      int64    `json:"timereceived"`
	Trusted           bool     `json:"trusted"`
	TxID              string   `json:"txid"`
	Vout              uint32   `json:"vout"`
	WalletConflicts   []string `json:"walletconflicts"`
	Comment           string   `json:"comment,omitempty"`
	OtherAccount      string   `json:"otheraccount,omitempty"`
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
	Account           string   `json:"account"`
	Address           string   `json:"address"`
	Amount            float64  `json:"amount"`
	Confirmations     uint64   `json:"confirmations"`
	TxIDs             []string `json:"txids,omitempty"`
	InvolvesWatchonly bool     `json:"involvesWatchonly,omitempty"`
}

// ListSinceBlockResult models the data from the listsinceblock command.
type ListSinceBlockResult struct {
	Transactions []ListTransactionsResult `json:"transactions"`
	LastBlock    string                   `json:"lastblock"`
}

// ListUnspentResult models a successful response from the listunspent request.
type ListUnspentResult struct {
	TxID          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	Account       string  `json:"account"`
	ScriptPubKey  string  `json:"scriptPubKey"`
	RedeemScript  string  `json:"redeemScript,omitempty"`
	Amount        float64 `json:"amount"`
	Confirmations int64   `json:"confirmations"`
	Spendable     bool    `json:"spendable"`
}

// SignRawTransactionError models the data that contains script verification
// errors from the signrawtransaction request.
type SignRawTransactionError struct {
	TxID      string `json:"txid"`
	Vout      uint32 `json:"vout"`
	ScriptSig string `json:"scriptSig"`
	Sequence  uint32 `json:"sequence"`
	Error     string `json:"error"`
}

// SignRawTransactionResult models the data from the signrawtransaction
// command.
type SignRawTransactionResult struct {
	Hex      string                    `json:"hex"`
	Complete bool                      `json:"complete"`
	Errors   []SignRawTransactionError `json:"errors,omitempty"`
}

// SignRawTransactionWithWalletResult models the data from the
// signrawtransactionwithwallet command.
type SignRawTransactionWithWalletResult struct {
	Hex      string                    `json:"hex"`
	Complete bool                      `json:"complete"`
	Errors   []SignRawTransactionError `json:"errors,omitempty"`
}

// ValidateAddressWalletResult models the data returned by the wallet server
// validateaddress command.
type ValidateAddressWalletResult struct {
	IsValid      bool     `json:"isvalid"`
	Address      string   `json:"address,omitempty"`
	IsMine       bool     `json:"ismine,omitempty"`
	IsWatchOnly  bool     `json:"iswatchonly,omitempty"`
	IsScript     bool     `json:"isscript,omitempty"`
	PubKey       string   `json:"pubkey,omitempty"`
	IsCompressed bool     `json:"iscompressed,omitempty"`
	Account      string   `json:"account,omitempty"`
	Addresses    []string `json:"addresses,omitempty"`
	Hex          string   `json:"hex,omitempty"`
	Script       string   `json:"script,omitempty"`
	SigsRequired int32    `json:"sigsrequired,omitempty"`
}

// GetBestBlockResult models the data from the getbestblock command.
type GetBestBlockResult struct {
	Hash   string `json:"hash"`
	Height int32  `json:"height"`
}

// BalanceDetailsResult models the details data from the `getbalances` command.
type BalanceDetailsResult struct {
	Trusted          float64  `json:"trusted"`
	UntrustedPending float64  `json:"untrusted_pending"`
	Immature         float64  `json:"immature"`
	Used             *float64 `json:"used"`
}

// GetBalancesResult models the data returned from the getbalances command.
type GetBalancesResult struct {
	Mine      BalanceDetailsResult  `json:"mine"`
	WatchOnly *BalanceDetailsResult `json:"watchonly"`
}

// ImportMultiResults is a slice that models the result of the importmulti command.
//
// Each item in the slice contains the execution result corresponding to the input
// requests of type btcjson.ImportMultiRequest, passed to the ImportMulti[Async]
// function.
type ImportMultiResults []struct {
	Success  bool      `json:"success"`
	Error    *RPCError `json:"error,omitempty"`
	Warnings *[]string `json:"warnings,omitempty"`
}

// WalletCreateFundedPsbtResult models the data returned from the
// walletcreatefundedpsbtresult command.
type WalletCreateFundedPsbtResult struct {
	Psbt      string  `json:"psbt"`
	Fee       float64 `json:"fee"`
	ChangePos int64   `json:"changepos"`
}

// WalletProcessPsbtResult models the data returned from the
// walletprocesspsbtresult command.
type WalletProcessPsbtResult struct {
	Psbt     string `json:"psbt"`
	Complete bool   `json:"complete"`
}
