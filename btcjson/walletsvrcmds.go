// Copyright (c) 2014-2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a wallet server.

package btcjson

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcutil"
)

// AddMultisigAddressCmd defines the addmutisigaddress JSON-RPC command.
type AddMultisigAddressCmd struct {
	NRequired int
	Keys      []string
	Account   *string
}

// NewAddMultisigAddressCmd returns a new instance which can be used to issue a
// addmultisigaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewAddMultisigAddressCmd(nRequired int, keys []string, account *string) *AddMultisigAddressCmd {
	return &AddMultisigAddressCmd{
		NRequired: nRequired,
		Keys:      keys,
		Account:   account,
	}
}

// AddWitnessAddressCmd defines the addwitnessaddress JSON-RPC command.
type AddWitnessAddressCmd struct {
	Address string
}

// NewAddWitnessAddressCmd returns a new instance which can be used to issue a
// addwitnessaddress JSON-RPC command.
func NewAddWitnessAddressCmd(address string) *AddWitnessAddressCmd {
	return &AddWitnessAddressCmd{
		Address: address,
	}
}

// CreateMultisigCmd defines the createmultisig JSON-RPC command.
type CreateMultisigCmd struct {
	NRequired int
	Keys      []string
}

// NewCreateMultisigCmd returns a new instance which can be used to issue a
// createmultisig JSON-RPC command.
func NewCreateMultisigCmd(nRequired int, keys []string) *CreateMultisigCmd {
	return &CreateMultisigCmd{
		NRequired: nRequired,
		Keys:      keys,
	}
}

// CreateWalletCmd defines the createwallet JSON-RPC command.
type CreateWalletCmd struct {
	WalletName         string
	DisablePrivateKeys *bool   `jsonrpcdefault:"false"`
	Blank              *bool   `jsonrpcdefault:"false"`
	Passphrase         *string `jsonrpcdefault:"\"\""`
	AvoidReuse         *bool   `jsonrpcdefault:"false"`
}

// NewCreateWalletCmd returns a new instance which can be used to issue a
// createwallet JSON-RPC command.
func NewCreateWalletCmd(walletName string, disablePrivateKeys *bool,
	blank *bool, passphrase *string, avoidReuse *bool) *CreateWalletCmd {
	return &CreateWalletCmd{
		WalletName:         walletName,
		DisablePrivateKeys: disablePrivateKeys,
		Blank:              blank,
		Passphrase:         passphrase,
		AvoidReuse:         avoidReuse,
	}
}

// DumpPrivKeyCmd defines the dumpprivkey JSON-RPC command.
type DumpPrivKeyCmd struct {
	Address string
}

// NewDumpPrivKeyCmd returns a new instance which can be used to issue a
// dumpprivkey JSON-RPC command.
func NewDumpPrivKeyCmd(address string) *DumpPrivKeyCmd {
	return &DumpPrivKeyCmd{
		Address: address,
	}
}

// EncryptWalletCmd defines the encryptwallet JSON-RPC command.
type EncryptWalletCmd struct {
	Passphrase string
}

// NewEncryptWalletCmd returns a new instance which can be used to issue a
// encryptwallet JSON-RPC command.
func NewEncryptWalletCmd(passphrase string) *EncryptWalletCmd {
	return &EncryptWalletCmd{
		Passphrase: passphrase,
	}
}

// EstimateSmartFeeMode defines the different fee estimation modes available
// for the estimatesmartfee JSON-RPC command.
type EstimateSmartFeeMode string

var (
	EstimateModeUnset        EstimateSmartFeeMode = "UNSET"
	EstimateModeEconomical   EstimateSmartFeeMode = "ECONOMICAL"
	EstimateModeConservative EstimateSmartFeeMode = "CONSERVATIVE"
)

// EstimateSmartFeeCmd defines the estimatesmartfee JSON-RPC command.
type EstimateSmartFeeCmd struct {
	ConfTarget   int64
	EstimateMode *EstimateSmartFeeMode `jsonrpcdefault:"\"CONSERVATIVE\""`
}

// NewEstimateSmartFeeCmd returns a new instance which can be used to issue a
// estimatesmartfee JSON-RPC command.
func NewEstimateSmartFeeCmd(confTarget int64, mode *EstimateSmartFeeMode) *EstimateSmartFeeCmd {
	return &EstimateSmartFeeCmd{
		ConfTarget: confTarget, EstimateMode: mode,
	}
}

// EstimateFeeCmd defines the estimatefee JSON-RPC command.
type EstimateFeeCmd struct {
	NumBlocks int64
}

// NewEstimateFeeCmd returns a new instance which can be used to issue a
// estimatefee JSON-RPC command.
func NewEstimateFeeCmd(numBlocks int64) *EstimateFeeCmd {
	return &EstimateFeeCmd{
		NumBlocks: numBlocks,
	}
}

// EstimatePriorityCmd defines the estimatepriority JSON-RPC command.
type EstimatePriorityCmd struct {
	NumBlocks int64
}

// NewEstimatePriorityCmd returns a new instance which can be used to issue a
// estimatepriority JSON-RPC command.
func NewEstimatePriorityCmd(numBlocks int64) *EstimatePriorityCmd {
	return &EstimatePriorityCmd{
		NumBlocks: numBlocks,
	}
}

// GetAccountCmd defines the getaccount JSON-RPC command.
type GetAccountCmd struct {
	Address string
}

// NewGetAccountCmd returns a new instance which can be used to issue a
// getaccount JSON-RPC command.
func NewGetAccountCmd(address string) *GetAccountCmd {
	return &GetAccountCmd{
		Address: address,
	}
}

// GetAccountAddressCmd defines the getaccountaddress JSON-RPC command.
type GetAccountAddressCmd struct {
	Account string
}

// NewGetAccountAddressCmd returns a new instance which can be used to issue a
// getaccountaddress JSON-RPC command.
func NewGetAccountAddressCmd(account string) *GetAccountAddressCmd {
	return &GetAccountAddressCmd{
		Account: account,
	}
}

// GetAddressesByAccountCmd defines the getaddressesbyaccount JSON-RPC command.
type GetAddressesByAccountCmd struct {
	Account string
}

// NewGetAddressesByAccountCmd returns a new instance which can be used to issue
// a getaddressesbyaccount JSON-RPC command.
func NewGetAddressesByAccountCmd(account string) *GetAddressesByAccountCmd {
	return &GetAddressesByAccountCmd{
		Account: account,
	}
}

// GetAddressInfoCmd defines the getaddressinfo JSON-RPC command.
type GetAddressInfoCmd struct {
	Address string
}

// NewGetAddressInfoCmd returns a new instance which can be used to issue a
// getaddressinfo JSON-RPC command.
func NewGetAddressInfoCmd(address string) *GetAddressInfoCmd {
	return &GetAddressInfoCmd{
		Address: address,
	}
}

// GetBalanceCmd defines the getbalance JSON-RPC command.
type GetBalanceCmd struct {
	Account *string
	MinConf *int `jsonrpcdefault:"1"`
}

// NewGetBalanceCmd returns a new instance which can be used to issue a
// getbalance JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetBalanceCmd(account *string, minConf *int) *GetBalanceCmd {
	return &GetBalanceCmd{
		Account: account,
		MinConf: minConf,
	}
}

// GetBalancesCmd defines the getbalances JSON-RPC command.
type GetBalancesCmd struct{}

// NewGetBalancesCmd returns a new instance which can be used to issue a
// getbalances JSON-RPC command.
func NewGetBalancesCmd() *GetBalancesCmd {
	return &GetBalancesCmd{}
}

// GetNewAddressCmd defines the getnewaddress JSON-RPC command.
type GetNewAddressCmd struct {
	Account *string
}

// NewGetNewAddressCmd returns a new instance which can be used to issue a
// getnewaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetNewAddressCmd(account *string) *GetNewAddressCmd {
	return &GetNewAddressCmd{
		Account: account,
	}
}

// GetRawChangeAddressCmd defines the getrawchangeaddress JSON-RPC command.
type GetRawChangeAddressCmd struct {
	Account *string
}

// NewGetRawChangeAddressCmd returns a new instance which can be used to issue a
// getrawchangeaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetRawChangeAddressCmd(account *string) *GetRawChangeAddressCmd {
	return &GetRawChangeAddressCmd{
		Account: account,
	}
}

// GetReceivedByAccountCmd defines the getreceivedbyaccount JSON-RPC command.
type GetReceivedByAccountCmd struct {
	Account string
	MinConf *int `jsonrpcdefault:"1"`
}

// NewGetReceivedByAccountCmd returns a new instance which can be used to issue
// a getreceivedbyaccount JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetReceivedByAccountCmd(account string, minConf *int) *GetReceivedByAccountCmd {
	return &GetReceivedByAccountCmd{
		Account: account,
		MinConf: minConf,
	}
}

// GetReceivedByAddressCmd defines the getreceivedbyaddress JSON-RPC command.
type GetReceivedByAddressCmd struct {
	Address string
	MinConf *int `jsonrpcdefault:"1"`
}

// NewGetReceivedByAddressCmd returns a new instance which can be used to issue
// a getreceivedbyaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetReceivedByAddressCmd(address string, minConf *int) *GetReceivedByAddressCmd {
	return &GetReceivedByAddressCmd{
		Address: address,
		MinConf: minConf,
	}
}

// GetTransactionCmd defines the gettransaction JSON-RPC command.
type GetTransactionCmd struct {
	Txid             string
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewGetTransactionCmd returns a new instance which can be used to issue a
// gettransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewGetTransactionCmd(txHash string, includeWatchOnly *bool) *GetTransactionCmd {
	return &GetTransactionCmd{
		Txid:             txHash,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// GetWalletInfoCmd defines the getwalletinfo JSON-RPC command.
type GetWalletInfoCmd struct{}

// NewGetWalletInfoCmd returns a new instance which can be used to issue a
// getwalletinfo JSON-RPC command.
func NewGetWalletInfoCmd() *GetWalletInfoCmd {
	return &GetWalletInfoCmd{}
}

// BackupWalletCmd defines the backupwallet JSON-RPC command
type BackupWalletCmd struct {
	Destination string
}

// NewBackupWalletCmd returns a new instance which can be used to issue a
// backupwallet JSON-RPC command
func NewBackupWalletCmd(destination string) *BackupWalletCmd {
	return &BackupWalletCmd{Destination: destination}
}

// UnloadWalletCmd defines the unloadwallet JSON-RPC command
type UnloadWalletCmd struct {
	WalletName *string
}

// NewUnloadWalletCmd returns a new instance which can be used to issue a
// unloadwallet JSON-RPC command.
func NewUnloadWalletCmd(walletName *string) *UnloadWalletCmd {
	return &UnloadWalletCmd{WalletName: walletName}
}

// LoadWalletCmd defines the loadwallet JSON-RPC command
type LoadWalletCmd struct {
	WalletName string
}

// NewLoadWalletCmd returns a new instance which can be used to issue a
// loadwallet JSON-RPC command
func NewLoadWalletCmd(walletName string) *LoadWalletCmd {
	return &LoadWalletCmd{WalletName: walletName}
}

// ImportPrivKeyCmd defines the importprivkey JSON-RPC command.
type ImportPrivKeyCmd struct {
	PrivKey string
	Label   *string
	Rescan  *bool `jsonrpcdefault:"true"`
}

// NewImportPrivKeyCmd returns a new instance which can be used to issue a
// importprivkey JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewImportPrivKeyCmd(privKey string, label *string, rescan *bool) *ImportPrivKeyCmd {
	return &ImportPrivKeyCmd{
		PrivKey: privKey,
		Label:   label,
		Rescan:  rescan,
	}
}

// KeyPoolRefillCmd defines the keypoolrefill JSON-RPC command.
type KeyPoolRefillCmd struct {
	NewSize *uint `jsonrpcdefault:"100"`
}

// NewKeyPoolRefillCmd returns a new instance which can be used to issue a
// keypoolrefill JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewKeyPoolRefillCmd(newSize *uint) *KeyPoolRefillCmd {
	return &KeyPoolRefillCmd{
		NewSize: newSize,
	}
}

// ListAccountsCmd defines the listaccounts JSON-RPC command.
type ListAccountsCmd struct {
	MinConf *int `jsonrpcdefault:"1"`
}

// NewListAccountsCmd returns a new instance which can be used to issue a
// listaccounts JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListAccountsCmd(minConf *int) *ListAccountsCmd {
	return &ListAccountsCmd{
		MinConf: minConf,
	}
}

// ListAddressGroupingsCmd defines the listaddressgroupings JSON-RPC command.
type ListAddressGroupingsCmd struct{}

// NewListAddressGroupingsCmd returns a new instance which can be used to issue
// a listaddressgroupoings JSON-RPC command.
func NewListAddressGroupingsCmd() *ListAddressGroupingsCmd {
	return &ListAddressGroupingsCmd{}
}

// ListLockUnspentCmd defines the listlockunspent JSON-RPC command.
type ListLockUnspentCmd struct{}

// NewListLockUnspentCmd returns a new instance which can be used to issue a
// listlockunspent JSON-RPC command.
func NewListLockUnspentCmd() *ListLockUnspentCmd {
	return &ListLockUnspentCmd{}
}

// ListReceivedByAccountCmd defines the listreceivedbyaccount JSON-RPC command.
type ListReceivedByAccountCmd struct {
	MinConf          *int  `jsonrpcdefault:"1"`
	IncludeEmpty     *bool `jsonrpcdefault:"false"`
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewListReceivedByAccountCmd returns a new instance which can be used to issue
// a listreceivedbyaccount JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListReceivedByAccountCmd(minConf *int, includeEmpty, includeWatchOnly *bool) *ListReceivedByAccountCmd {
	return &ListReceivedByAccountCmd{
		MinConf:          minConf,
		IncludeEmpty:     includeEmpty,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// ListReceivedByAddressCmd defines the listreceivedbyaddress JSON-RPC command.
type ListReceivedByAddressCmd struct {
	MinConf          *int  `jsonrpcdefault:"1"`
	IncludeEmpty     *bool `jsonrpcdefault:"false"`
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewListReceivedByAddressCmd returns a new instance which can be used to issue
// a listreceivedbyaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListReceivedByAddressCmd(minConf *int, includeEmpty, includeWatchOnly *bool) *ListReceivedByAddressCmd {
	return &ListReceivedByAddressCmd{
		MinConf:          minConf,
		IncludeEmpty:     includeEmpty,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// ListSinceBlockCmd defines the listsinceblock JSON-RPC command.
type ListSinceBlockCmd struct {
	BlockHash           *string
	TargetConfirmations *int  `jsonrpcdefault:"1"`
	IncludeWatchOnly    *bool `jsonrpcdefault:"false"`
}

// NewListSinceBlockCmd returns a new instance which can be used to issue a
// listsinceblock JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListSinceBlockCmd(blockHash *string, targetConfirms *int, includeWatchOnly *bool) *ListSinceBlockCmd {
	return &ListSinceBlockCmd{
		BlockHash:           blockHash,
		TargetConfirmations: targetConfirms,
		IncludeWatchOnly:    includeWatchOnly,
	}
}

// ListTransactionsCmd defines the listtransactions JSON-RPC command.
type ListTransactionsCmd struct {
	Account          *string
	Count            *int  `jsonrpcdefault:"10"`
	From             *int  `jsonrpcdefault:"0"`
	IncludeWatchOnly *bool `jsonrpcdefault:"false"`
}

// NewListTransactionsCmd returns a new instance which can be used to issue a
// listtransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListTransactionsCmd(account *string, count, from *int, includeWatchOnly *bool) *ListTransactionsCmd {
	return &ListTransactionsCmd{
		Account:          account,
		Count:            count,
		From:             from,
		IncludeWatchOnly: includeWatchOnly,
	}
}

// ListUnspentCmd defines the listunspent JSON-RPC command.
type ListUnspentCmd struct {
	MinConf   *int `jsonrpcdefault:"1"`
	MaxConf   *int `jsonrpcdefault:"9999999"`
	Addresses *[]string
}

// NewListUnspentCmd returns a new instance which can be used to issue a
// listunspent JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewListUnspentCmd(minConf, maxConf *int, addresses *[]string) *ListUnspentCmd {
	return &ListUnspentCmd{
		MinConf:   minConf,
		MaxConf:   maxConf,
		Addresses: addresses,
	}
}

// LockUnspentCmd defines the lockunspent JSON-RPC command.
type LockUnspentCmd struct {
	Unlock       bool
	Transactions []TransactionInput
}

// NewLockUnspentCmd returns a new instance which can be used to issue a
// lockunspent JSON-RPC command.
func NewLockUnspentCmd(unlock bool, transactions []TransactionInput) *LockUnspentCmd {
	return &LockUnspentCmd{
		Unlock:       unlock,
		Transactions: transactions,
	}
}

// MoveCmd defines the move JSON-RPC command.
type MoveCmd struct {
	FromAccount string
	ToAccount   string
	Amount      float64 // In BTC
	MinConf     *int    `jsonrpcdefault:"1"`
	Comment     *string
}

// NewMoveCmd returns a new instance which can be used to issue a move JSON-RPC
// command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewMoveCmd(fromAccount, toAccount string, amount float64, minConf *int, comment *string) *MoveCmd {
	return &MoveCmd{
		FromAccount: fromAccount,
		ToAccount:   toAccount,
		Amount:      amount,
		MinConf:     minConf,
		Comment:     comment,
	}
}

// SendFromCmd defines the sendfrom JSON-RPC command.
type SendFromCmd struct {
	FromAccount string
	ToAddress   string
	Amount      float64 // In BTC
	MinConf     *int    `jsonrpcdefault:"1"`
	Comment     *string
	CommentTo   *string
}

// NewSendFromCmd returns a new instance which can be used to issue a sendfrom
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendFromCmd(fromAccount, toAddress string, amount float64, minConf *int, comment, commentTo *string) *SendFromCmd {
	return &SendFromCmd{
		FromAccount: fromAccount,
		ToAddress:   toAddress,
		Amount:      amount,
		MinConf:     minConf,
		Comment:     comment,
		CommentTo:   commentTo,
	}
}

// SendManyCmd defines the sendmany JSON-RPC command.
type SendManyCmd struct {
	FromAccount string
	Amounts     map[string]float64 `jsonrpcusage:"{\"address\":amount,...}"` // In BTC
	MinConf     *int               `jsonrpcdefault:"1"`
	Comment     *string
}

// NewSendManyCmd returns a new instance which can be used to issue a sendmany
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendManyCmd(fromAccount string, amounts map[string]float64, minConf *int, comment *string) *SendManyCmd {
	return &SendManyCmd{
		FromAccount: fromAccount,
		Amounts:     amounts,
		MinConf:     minConf,
		Comment:     comment,
	}
}

// SendToAddressCmd defines the sendtoaddress JSON-RPC command.
type SendToAddressCmd struct {
	Address   string
	Amount    float64
	Comment   *string
	CommentTo *string
}

// NewSendToAddressCmd returns a new instance which can be used to issue a
// sendtoaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSendToAddressCmd(address string, amount float64, comment, commentTo *string) *SendToAddressCmd {
	return &SendToAddressCmd{
		Address:   address,
		Amount:    amount,
		Comment:   comment,
		CommentTo: commentTo,
	}
}

// SetAccountCmd defines the setaccount JSON-RPC command.
type SetAccountCmd struct {
	Address string
	Account string
}

// NewSetAccountCmd returns a new instance which can be used to issue a
// setaccount JSON-RPC command.
func NewSetAccountCmd(address, account string) *SetAccountCmd {
	return &SetAccountCmd{
		Address: address,
		Account: account,
	}
}

// SetTxFeeCmd defines the settxfee JSON-RPC command.
type SetTxFeeCmd struct {
	Amount float64 // In BTC
}

// NewSetTxFeeCmd returns a new instance which can be used to issue a settxfee
// JSON-RPC command.
func NewSetTxFeeCmd(amount float64) *SetTxFeeCmd {
	return &SetTxFeeCmd{
		Amount: amount,
	}
}

// SignMessageCmd defines the signmessage JSON-RPC command.
type SignMessageCmd struct {
	Address string
	Message string
}

// NewSignMessageCmd returns a new instance which can be used to issue a
// signmessage JSON-RPC command.
func NewSignMessageCmd(address, message string) *SignMessageCmd {
	return &SignMessageCmd{
		Address: address,
		Message: message,
	}
}

// RawTxInput models the data needed for raw transaction input that is used in
// the SignRawTransactionCmd struct.
type RawTxInput struct {
	Txid         string `json:"txid"`
	Vout         uint32 `json:"vout"`
	ScriptPubKey string `json:"scriptPubKey"`
	RedeemScript string `json:"redeemScript"`
}

// SignRawTransactionCmd defines the signrawtransaction JSON-RPC command.
type SignRawTransactionCmd struct {
	RawTx    string
	Inputs   *[]RawTxInput
	PrivKeys *[]string
	Flags    *string `jsonrpcdefault:"\"ALL\""`
}

// NewSignRawTransactionCmd returns a new instance which can be used to issue a
// signrawtransaction JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSignRawTransactionCmd(hexEncodedTx string, inputs *[]RawTxInput, privKeys *[]string, flags *string) *SignRawTransactionCmd {
	return &SignRawTransactionCmd{
		RawTx:    hexEncodedTx,
		Inputs:   inputs,
		PrivKeys: privKeys,
		Flags:    flags,
	}
}

// RawTxWitnessInput models the data needed for raw transaction input that is used in
// the SignRawTransactionWithWalletCmd struct. The RedeemScript is required for P2SH inputs,
// the WitnessScript is required for P2WSH or P2SH-P2WSH witness scripts, and the Amount is
// required for Segwit inputs. Otherwise, those fields can be left blank.
type RawTxWitnessInput struct {
	Txid          string   `json:"txid"`
	Vout          uint32   `json:"vout"`
	ScriptPubKey  string   `json:"scriptPubKey"`
	RedeemScript  *string  `json:"redeemScript,omitempty"`
	WitnessScript *string  `json:"witnessScript,omitempty"`
	Amount        *float64 `json:"amount,omitempty"` // In BTC
}

// SignRawTransactionWithWalletCmd defines the signrawtransactionwithwallet JSON-RPC command.
type SignRawTransactionWithWalletCmd struct {
	RawTx       string
	Inputs      *[]RawTxWitnessInput
	SigHashType *string `jsonrpcdefault:"\"ALL\""`
}

// NewSignRawTransactionWithWalletCmd returns a new instance which can be used to issue a
// signrawtransactionwithwallet JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewSignRawTransactionWithWalletCmd(hexEncodedTx string, inputs *[]RawTxWitnessInput, sigHashType *string) *SignRawTransactionWithWalletCmd {
	return &SignRawTransactionWithWalletCmd{
		RawTx:       hexEncodedTx,
		Inputs:      inputs,
		SigHashType: sigHashType,
	}
}

// WalletLockCmd defines the walletlock JSON-RPC command.
type WalletLockCmd struct{}

// NewWalletLockCmd returns a new instance which can be used to issue a
// walletlock JSON-RPC command.
func NewWalletLockCmd() *WalletLockCmd {
	return &WalletLockCmd{}
}

// WalletPassphraseCmd defines the walletpassphrase JSON-RPC command.
type WalletPassphraseCmd struct {
	Passphrase string
	Timeout    int64
}

// NewWalletPassphraseCmd returns a new instance which can be used to issue a
// walletpassphrase JSON-RPC command.
func NewWalletPassphraseCmd(passphrase string, timeout int64) *WalletPassphraseCmd {
	return &WalletPassphraseCmd{
		Passphrase: passphrase,
		Timeout:    timeout,
	}
}

// WalletPassphraseChangeCmd defines the walletpassphrase JSON-RPC command.
type WalletPassphraseChangeCmd struct {
	OldPassphrase string
	NewPassphrase string
}

// NewWalletPassphraseChangeCmd returns a new instance which can be used to
// issue a walletpassphrasechange JSON-RPC command.
func NewWalletPassphraseChangeCmd(oldPassphrase, newPassphrase string) *WalletPassphraseChangeCmd {
	return &WalletPassphraseChangeCmd{
		OldPassphrase: oldPassphrase,
		NewPassphrase: newPassphrase,
	}
}

// TimestampOrNow defines a type to represent a timestamp value in seconds,
// since epoch.
//
// The value can either be a integer, or the string "now".
//
// NOTE: Interpretation of the timestamp value depends upon the specific
// JSON-RPC command, where it is used.
type TimestampOrNow struct {
	Value interface{}
}

// MarshalJSON implements the json.Marshaler interface for TimestampOrNow
func (t TimestampOrNow) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Value)
}

// UnmarshalJSON implements the json.Unmarshaler interface for TimestampOrNow
func (t *TimestampOrNow) UnmarshalJSON(data []byte) error {
	var unmarshalled interface{}
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		return err
	}

	switch v := unmarshalled.(type) {
	case float64:
		t.Value = int(v)
	case string:
		if v != "now" {
			return fmt.Errorf("invalid timestamp value: %v", unmarshalled)
		}
		t.Value = v
	default:
		return fmt.Errorf("invalid timestamp value: %v", unmarshalled)
	}
	return nil
}

// ScriptPubKeyAddress represents an address, to be used in conjunction with
// ScriptPubKey.
type ScriptPubKeyAddress struct {
	Address string `json:"address"`
}

// ScriptPubKey represents a script (as a string) or an address
// (as a ScriptPubKeyAddress).
type ScriptPubKey struct {
	Value interface{}
}

// MarshalJSON implements the json.Marshaler interface for ScriptPubKey
func (s ScriptPubKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Value)
}

// UnmarshalJSON implements the json.Unmarshaler interface for ScriptPubKey
func (s *ScriptPubKey) UnmarshalJSON(data []byte) error {
	var unmarshalled interface{}
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		return err
	}

	switch v := unmarshalled.(type) {
	case string:
		s.Value = v
	case map[string]interface{}:
		s.Value = ScriptPubKeyAddress{Address: v["address"].(string)}
	default:
		return fmt.Errorf("invalid scriptPubKey value: %v", unmarshalled)
	}
	return nil
}

// DescriptorRange specifies the limits of a ranged Descriptor.
//
// Descriptors are typically ranged when specified in the form of generic HD
// chain paths.
//   Example of a ranged descriptor: pkh(tpub.../*)
//
// The value can be an int to specify the end of the range, or the range
// itself, as []int{begin, end}.
type DescriptorRange struct {
	Value interface{}
}

// MarshalJSON implements the json.Marshaler interface for DescriptorRange
func (r DescriptorRange) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Value)
}

// UnmarshalJSON implements the json.Unmarshaler interface for DescriptorRange
func (r *DescriptorRange) UnmarshalJSON(data []byte) error {
	var unmarshalled interface{}
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		return err
	}

	switch v := unmarshalled.(type) {
	case float64:
		r.Value = int(v)
	case []interface{}:
		if len(v) != 2 {
			return fmt.Errorf("expected [begin,end] integer range, got: %v", unmarshalled)
		}
		r.Value = []int{
			int(v[0].(float64)),
			int(v[1].(float64)),
		}
	default:
		return fmt.Errorf("invalid descriptor range value: %v", unmarshalled)
	}
	return nil
}

// ImportMultiRequest defines the request struct to be passed to the
// ImportMultiCmd, as an array.
type ImportMultiRequest struct {
	// Descriptor to import, in canonical form. If using Descriptor, do not
	// also provide ScriptPubKey, RedeemScript, WitnessScript, PubKeys, or Keys.
	Descriptor *string `json:"desc,omitempty"`

	// Script/address to import. Should not be provided if using Descriptor.
	ScriptPubKey *ScriptPubKey `json:"scriptPubKey,omitempty"`

	// Creation time of the key in seconds since epoch (Jan 1 1970 GMT), or
	// the string "now" to substitute the current synced blockchain time.
	//
	// The timestamp of the oldest key will determine how far back blockchain
	// rescans need to begin for missing wallet transactions.
	//
	// Specifying "now" bypasses scanning. Useful for keys that are known to
	// never have been used.
	//
	// Specifying 0 scans the entire blockchain.
	Timestamp TimestampOrNow `json:"timestamp"`

	// Allowed only if the ScriptPubKey is a P2SH or P2SH-P2WSH
	// address/scriptPubKey.
	RedeemScript *string `json:"redeemscript,omitempty"`

	// Allowed only if the ScriptPubKey is a P2SH-P2WSH or P2WSH
	// address/scriptPubKey.
	WitnessScript *string `json:"witnessscript,omitempty"`

	// Array of strings giving pubkeys to import. They must occur in P2PKH or
	// P2WPKH scripts. They are not required when the private key is also
	// provided (see Keys).
	PubKeys *[]string `json:"pubkeys,omitempty"`

	// Array of strings giving private keys to import. The corresponding
	// public keys must occur in the output or RedeemScript.
	Keys *[]string `json:"keys,omitempty"`

	// If the provided Descriptor is ranged, this specifies the end
	// (as an int) or the range (as []int{begin, end}) to import.
	Range *DescriptorRange `json:"range,omitempty"`

	// States whether matching outputs should be treated as not incoming
	// payments (also known as change).
	Internal *bool `json:"internal,omitempty"`

	// States whether matching outputs should be considered watchonly.
	//
	// If an address/script is imported without all of the private keys
	// required to spend from that address, set this field to true.
	//
	// If all the private keys are provided and the address/script is
	// spendable, set this field to false.
	WatchOnly *bool `json:"watchonly,omitempty"`

	// Label to assign to the address. Only allowed when Internal is false.
	Label *string `json:"label,omitempty"`

	// States whether imported public keys should be added to the keypool for
	// when users request new addresses. Only allowed when wallet private keys
	// are disabled.
	KeyPool *bool `json:"keypool,omitempty"`
}

// ImportMultiRequest defines the options struct, provided to the
// ImportMultiCmd as a pointer argument.
type ImportMultiOptions struct {
	Rescan bool `json:"rescan"` // Rescan the blockchain after all imports
}

// ImportMultiCmd defines the importmulti JSON-RPC command.
type ImportMultiCmd struct {
	Requests []ImportMultiRequest
	Options  *ImportMultiOptions
}

// NewImportMultiCmd returns a new instance which can be used to issue
// an importmulti JSON-RPC command.
//
// The parameters which are pointers indicate they are optional. Passing nil
// for optional parameters will use the default value.
func NewImportMultiCmd(requests []ImportMultiRequest, options *ImportMultiOptions) *ImportMultiCmd {
	return &ImportMultiCmd{
		Requests: requests,
		Options:  options,
	}
}

// PsbtInput represents an input to include in the PSBT created by the
// WalletCreateFundedPsbtCmd command.
type PsbtInput struct {
	Txid     string `json:"txid"`
	Vout     uint32 `json:"vout"`
	Sequence uint32 `json:"sequence"`
}

// PsbtOutput represents an output to include in the PSBT created by the
// WalletCreateFundedPsbtCmd command.
type PsbtOutput map[string]interface{}

// NewPsbtOutput returns a new instance of a PSBT output to use with the
// WalletCreateFundedPsbtCmd command.
func NewPsbtOutput(address string, amount btcutil.Amount) PsbtOutput {
	return PsbtOutput{address: amount.ToBTC()}
}

// NewPsbtDataOutput returns a new instance of a PSBT data output to use with
// the WalletCreateFundedPsbtCmd command.
func NewPsbtDataOutput(data []byte) PsbtOutput {
	return PsbtOutput{"data": hex.EncodeToString(data)}
}

// WalletCreateFundedPsbtOpts represents the optional options struct provided
// with a WalletCreateFundedPsbtCmd command.
type WalletCreateFundedPsbtOpts struct {
	ChangeAddress          *string     `json:"changeAddress,omitempty"`
	ChangePosition         *int64      `json:"changePosition,omitempty"`
	ChangeType             *ChangeType `json:"change_type,omitempty"`
	IncludeWatching        *bool       `json:"includeWatching,omitempty"`
	LockUnspents           *bool       `json:"lockUnspents,omitempty"`
	FeeRate                *int64      `json:"feeRate,omitempty"`
	SubtractFeeFromOutputs *[]int64    `json:"subtractFeeFromOutputs,omitempty"`
	Replaceable            *bool       `json:"replaceable,omitempty"`
	ConfTarget             *int64      `json:"conf_target,omitempty"`
	EstimateMode           *string     `json:"estimate_mode,omitempty"`
}

// WalletCreateFundedPsbtCmd defines the walletcreatefundedpsbt JSON-RPC command.
type WalletCreateFundedPsbtCmd struct {
	Inputs      []PsbtInput
	Outputs     []PsbtOutput
	Locktime    *uint32
	Options     *WalletCreateFundedPsbtOpts
	Bip32Derivs *bool
}

// NewWalletCreateFundedPsbtCmd returns a new instance which can be used to issue a
// walletcreatefundedpsbt JSON-RPC command.
func NewWalletCreateFundedPsbtCmd(
	inputs []PsbtInput, outputs []PsbtOutput, locktime *uint32,
	options *WalletCreateFundedPsbtOpts, bip32Derivs *bool,
) *WalletCreateFundedPsbtCmd {
	return &WalletCreateFundedPsbtCmd{
		Inputs:      inputs,
		Outputs:     outputs,
		Locktime:    locktime,
		Options:     options,
		Bip32Derivs: bip32Derivs,
	}
}

// WalletProcessPsbtCmd defines the walletprocesspsbt JSON-RPC command.
type WalletProcessPsbtCmd struct {
	Psbt        string
	Sign        *bool   `jsonrpcdefault:"true"`
	SighashType *string `jsonrpcdefault:"\"ALL\""`
	Bip32Derivs *bool
}

// NewWalletProcessPsbtCmd returns a new instance which can be used to issue a
// walletprocesspsbt JSON-RPC command.
func NewWalletProcessPsbtCmd(psbt string, sign *bool, sighashType *string, bip32Derivs *bool) *WalletProcessPsbtCmd {
	return &WalletProcessPsbtCmd{
		Psbt:        psbt,
		Sign:        sign,
		SighashType: sighashType,
		Bip32Derivs: bip32Derivs,
	}
}

func init() {
	// The commands in this file are only usable with a wallet server.
	flags := UFWalletOnly

	MustRegisterCmd("addmultisigaddress", (*AddMultisigAddressCmd)(nil), flags)
	MustRegisterCmd("addwitnessaddress", (*AddWitnessAddressCmd)(nil), flags)
	MustRegisterCmd("backupwallet", (*BackupWalletCmd)(nil), flags)
	MustRegisterCmd("createmultisig", (*CreateMultisigCmd)(nil), flags)
	MustRegisterCmd("createwallet", (*CreateWalletCmd)(nil), flags)
	MustRegisterCmd("dumpprivkey", (*DumpPrivKeyCmd)(nil), flags)
	MustRegisterCmd("encryptwallet", (*EncryptWalletCmd)(nil), flags)
	MustRegisterCmd("estimatesmartfee", (*EstimateSmartFeeCmd)(nil), flags)
	MustRegisterCmd("estimatefee", (*EstimateFeeCmd)(nil), flags)
	MustRegisterCmd("estimatepriority", (*EstimatePriorityCmd)(nil), flags)
	MustRegisterCmd("getaccount", (*GetAccountCmd)(nil), flags)
	MustRegisterCmd("getaccountaddress", (*GetAccountAddressCmd)(nil), flags)
	MustRegisterCmd("getaddressesbyaccount", (*GetAddressesByAccountCmd)(nil), flags)
	MustRegisterCmd("getaddressinfo", (*GetAddressInfoCmd)(nil), flags)
	MustRegisterCmd("getbalance", (*GetBalanceCmd)(nil), flags)
	MustRegisterCmd("getbalances", (*GetBalancesCmd)(nil), flags)
	MustRegisterCmd("getnewaddress", (*GetNewAddressCmd)(nil), flags)
	MustRegisterCmd("getrawchangeaddress", (*GetRawChangeAddressCmd)(nil), flags)
	MustRegisterCmd("getreceivedbyaccount", (*GetReceivedByAccountCmd)(nil), flags)
	MustRegisterCmd("getreceivedbyaddress", (*GetReceivedByAddressCmd)(nil), flags)
	MustRegisterCmd("gettransaction", (*GetTransactionCmd)(nil), flags)
	MustRegisterCmd("getwalletinfo", (*GetWalletInfoCmd)(nil), flags)
	MustRegisterCmd("importmulti", (*ImportMultiCmd)(nil), flags)
	MustRegisterCmd("importprivkey", (*ImportPrivKeyCmd)(nil), flags)
	MustRegisterCmd("keypoolrefill", (*KeyPoolRefillCmd)(nil), flags)
	MustRegisterCmd("listaccounts", (*ListAccountsCmd)(nil), flags)
	MustRegisterCmd("listaddressgroupings", (*ListAddressGroupingsCmd)(nil), flags)
	MustRegisterCmd("listlockunspent", (*ListLockUnspentCmd)(nil), flags)
	MustRegisterCmd("listreceivedbyaccount", (*ListReceivedByAccountCmd)(nil), flags)
	MustRegisterCmd("listreceivedbyaddress", (*ListReceivedByAddressCmd)(nil), flags)
	MustRegisterCmd("listsinceblock", (*ListSinceBlockCmd)(nil), flags)
	MustRegisterCmd("listtransactions", (*ListTransactionsCmd)(nil), flags)
	MustRegisterCmd("listunspent", (*ListUnspentCmd)(nil), flags)
	MustRegisterCmd("loadwallet", (*LoadWalletCmd)(nil), flags)
	MustRegisterCmd("lockunspent", (*LockUnspentCmd)(nil), flags)
	MustRegisterCmd("move", (*MoveCmd)(nil), flags)
	MustRegisterCmd("sendfrom", (*SendFromCmd)(nil), flags)
	MustRegisterCmd("sendmany", (*SendManyCmd)(nil), flags)
	MustRegisterCmd("sendtoaddress", (*SendToAddressCmd)(nil), flags)
	MustRegisterCmd("setaccount", (*SetAccountCmd)(nil), flags)
	MustRegisterCmd("settxfee", (*SetTxFeeCmd)(nil), flags)
	MustRegisterCmd("signmessage", (*SignMessageCmd)(nil), flags)
	MustRegisterCmd("signrawtransaction", (*SignRawTransactionCmd)(nil), flags)
	MustRegisterCmd("signrawtransactionwithwallet", (*SignRawTransactionWithWalletCmd)(nil), flags)
	MustRegisterCmd("unloadwallet", (*UnloadWalletCmd)(nil), flags)
	MustRegisterCmd("walletlock", (*WalletLockCmd)(nil), flags)
	MustRegisterCmd("walletpassphrase", (*WalletPassphraseCmd)(nil), flags)
	MustRegisterCmd("walletpassphrasechange", (*WalletPassphraseChangeCmd)(nil), flags)
	MustRegisterCmd("walletcreatefundedpsbt", (*WalletCreateFundedPsbtCmd)(nil), flags)
	MustRegisterCmd("walletprocesspsbt", (*WalletProcessPsbtCmd)(nil), flags)
}
