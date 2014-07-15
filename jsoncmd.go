// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrTooManyOptArgs describes an error where too many optional
// arguments were passed when creating a new Cmd.
var ErrTooManyOptArgs = errors.New("too many optional arguments")

// ErrWrongNumberOfParams describes an error where an incorrect number of
// parameters are found in an unmarshalled command.
var ErrWrongNumberOfParams = errors.New("incorrect number of parameters")

// Cmd is an interface for all Bitcoin JSON API commands to marshal
// and unmarshal as a JSON object.
type Cmd interface {
	json.Marshaler
	json.Unmarshaler
	Id() interface{}
	Method() string
}

// RawCmd is a type for unmarshaling raw commands into before the
// custom command type is set.  Other packages may register their
// own RawCmd to Cmd converters by calling RegisterCustomCmd.
type RawCmd struct {
	Jsonrpc string            `json:"jsonrpc"`
	Id      interface{}       `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

// NewRawCmd returns a new raw command given the provided id, method, and
// parameters.  The parameters are marshalled into a json.RawMessage for the
// Params field of the returned raw command.
func NewRawCmd(id interface{}, method string, params []interface{}) (*RawCmd, error) {
	rawParams := make([]json.RawMessage, 0, len(params))
	for _, param := range params {
		marshalledParam, err := json.Marshal(param)
		if err != nil {
			return nil, err
		}
		rawMessage := json.RawMessage(marshalledParam)
		rawParams = append(rawParams, rawMessage)
	}

	return &RawCmd{
		Jsonrpc: "1.0",
		Id:      id,
		Method:  method,
		Params:  rawParams,
	}, nil
}

// RawCmdParser is a function to create a custom Cmd from a RawCmd.
type RawCmdParser func(*RawCmd) (Cmd, error)

// ReplyParser is a function a custom Cmd can use to unmarshal the results of a
// reply into a concrete struct.
type ReplyParser func(json.RawMessage) (interface{}, error)

type cmd struct {
	parser      RawCmdParser
	replyParser ReplyParser
	helpString  string
}

var customCmds = make(map[string]cmd)

// RegisterCustomCmd registers a custom RawCmd parsing func, reply parsing func,
// and help text for a non-standard Bitcoin command.
func RegisterCustomCmd(method string, parser RawCmdParser, replyParser ReplyParser, helpString string) {
	customCmds[method] = cmd{
		parser:      parser,
		replyParser: replyParser,
		helpString:  helpString,
	}
}

// ParseMarshaledCmd parses a raw command and unmarshals as a Cmd.
// Code that reads and handles commands should switch on the type and
// type assert as the particular commands supported by the program.
//
// In all cases where b is a valid JSON-RPC message, and unmarshalling
// succeeds, a non-nil Cmd will always be returned.  This even
// includes error cases where parsing the message into a concrete Cmd
// type fails.  This behavior allows RPC server code to reply back with
// a detailed error using the Id and Method functions of the Cmd
// interface.
func ParseMarshaledCmd(b []byte) (Cmd, error) {
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}

	// Return a custom command type for recognized
	var cmd Cmd
	switch r.Method {
	case "addmultisigaddress":
		cmd = new(AddMultisigAddressCmd)

	case "addnode":
		cmd = new(AddNodeCmd)

	case "backupwallet":
		cmd = new(BackupWalletCmd)

	case "createmultisig":
		cmd = new(CreateMultisigCmd)

	case "createrawtransaction":
		cmd = new(CreateRawTransactionCmd)

	case "debuglevel":
		cmd = new(DebugLevelCmd)

	case "decoderawtransaction":
		cmd = new(DecodeRawTransactionCmd)

	case "decodescript":
		cmd = new(DecodeScriptCmd)

	case "dumpprivkey":
		cmd = new(DumpPrivKeyCmd)

	case "dumpwallet":
		cmd = new(DumpWalletCmd)

	case "encryptwallet":
		cmd = new(EncryptWalletCmd)

	case "estimatefee":
		cmd = new(EstimateFeeCmd)

	case "estimatepriority":
		cmd = new(EstimatePriorityCmd)

	case "getaccount":
		cmd = new(GetAccountCmd)

	case "getaccountaddress":
		cmd = new(GetAccountAddressCmd)

	case "getaddednodeinfo":
		cmd = new(GetAddedNodeInfoCmd)

	case "getaddressesbyaccount":
		cmd = new(GetAddressesByAccountCmd)

	case "getbalance":
		cmd = new(GetBalanceCmd)

	case "getbestblockhash":
		cmd = new(GetBestBlockHashCmd)

	case "getblock":
		cmd = new(GetBlockCmd)

	case "getblockchaininfo":
		cmd = new(GetBlockChainInfoCmd)

	case "getblockcount":
		cmd = new(GetBlockCountCmd)

	case "getblockhash":
		cmd = new(GetBlockHashCmd)

	case "getblocktemplate":
		cmd = new(GetBlockTemplateCmd)

	case "getconnectioncount":
		cmd = new(GetConnectionCountCmd)

	case "getdifficulty":
		cmd = new(GetDifficultyCmd)

	case "getgenerate":
		cmd = new(GetGenerateCmd)

	case "gethashespersec":
		cmd = new(GetHashesPerSecCmd)

	case "getinfo":
		cmd = new(GetInfoCmd)

	case "getmininginfo":
		cmd = new(GetMiningInfoCmd)

	case "getnettotals":
		cmd = new(GetNetTotalsCmd)

	case "getnetworkhashps":
		cmd = new(GetNetworkHashPSCmd)

	case "getnetworkinfo":
		cmd = new(GetNetworkInfoCmd)

	case "getnewaddress":
		cmd = new(GetNewAddressCmd)

	case "getpeerinfo":
		cmd = new(GetPeerInfoCmd)

	case "getrawchangeaddress":
		cmd = new(GetRawChangeAddressCmd)

	case "getrawmempool":
		cmd = new(GetRawMempoolCmd)

	case "getrawtransaction":
		cmd = new(GetRawTransactionCmd)

	case "getreceivedbyaccount":
		cmd = new(GetReceivedByAccountCmd)

	case "getreceivedbyaddress":
		cmd = new(GetReceivedByAddressCmd)

	case "gettransaction":
		cmd = new(GetTransactionCmd)

	case "gettxout":
		cmd = new(GetTxOutCmd)

	case "gettxoutsetinfo":
		cmd = new(GetTxOutSetInfoCmd)

	case "getwork":
		cmd = new(GetWorkCmd)

	case "help":
		cmd = new(HelpCmd)

	case "importprivkey":
		cmd = new(ImportPrivKeyCmd)

	case "importwallet":
		cmd = new(ImportWalletCmd)

	case "keypoolrefill":
		cmd = new(KeyPoolRefillCmd)

	case "listaccounts":
		cmd = new(ListAccountsCmd)

	case "listaddressgroupings":
		cmd = new(ListAddressGroupingsCmd)

	case "listlockunspent":
		cmd = new(ListLockUnspentCmd)

	case "listreceivedbyaccount":
		cmd = new(ListReceivedByAccountCmd)

	case "listreceivedbyaddress":
		cmd = new(ListReceivedByAddressCmd)

	case "listsinceblock":
		cmd = new(ListSinceBlockCmd)

	case "listtransactions":
		cmd = new(ListTransactionsCmd)

	case "listunspent":
		cmd = new(ListUnspentCmd)

	case "lockunspent":
		cmd = new(LockUnspentCmd)

	case "move":
		cmd = new(MoveCmd)

	case "ping":
		cmd = new(PingCmd)

	case "sendfrom":
		cmd = new(SendFromCmd)

	case "sendmany":
		cmd = new(SendManyCmd)

	case "sendrawtransaction":
		cmd = new(SendRawTransactionCmd)

	case "sendtoaddress":
		cmd = new(SendToAddressCmd)

	case "setaccount":
		cmd = new(SetAccountCmd)

	case "setgenerate":
		cmd = new(SetGenerateCmd)

	case "settxfee":
		cmd = new(SetTxFeeCmd)

	case "signmessage":
		cmd = new(SignMessageCmd)

	case "signrawtransaction":
		cmd = new(SignRawTransactionCmd)

	case "stop":
		cmd = new(StopCmd)

	case "submitblock":
		cmd = new(SubmitBlockCmd)

	case "validateaddress":
		cmd = new(ValidateAddressCmd)

	case "verifychain":
		cmd = new(VerifyChainCmd)

	case "verifymessage":
		cmd = new(VerifyMessageCmd)

	case "walletlock":
		cmd = new(WalletLockCmd)

	case "walletpassphrase":
		cmd = new(WalletPassphraseCmd)

	case "walletpassphrasechange":
		cmd = new(WalletPassphraseChangeCmd)

	default:
		// None of the standard Bitcoin RPC methods matched.  Try
		// registered custom commands.
		if c, ok := customCmds[r.Method]; ok {
			cmd, err := c.parser(&r)
			if err != nil {
				cmd = newUnparsableCmd(r.Id, r.Method)
				return cmd, err
			}
			return cmd, nil
		}
	}

	if cmd == nil {
		cmd = newUnparsableCmd(r.Id, r.Method)
		return cmd, ErrMethodNotFound
	}

	// If we get here we have a cmd that can unmarshal itself.
	if err := cmd.UnmarshalJSON(b); err != nil {
		cmd = newUnparsableCmd(r.Id, r.Method)
		return cmd, err
	}
	return cmd, nil
}

// unparsableCmd is a type representing a valid unmarshalled JSON-RPC
// request, but is used for cases where parsing the RPC request into a
// concrete Cmd type failed, either due to an unknown method, or trying
// to parse incorrect arguments for a known method.
type unparsableCmd struct {
	id     interface{}
	method string
}

// Enforce that unparsableCmd satisifies the Cmd interface.
var _ Cmd = &unparsableCmd{}

func newUnparsableCmd(id interface{}, method string) *unparsableCmd {
	return &unparsableCmd{
		id:     id,
		method: method,
	}
}

// Id satisifies the Cmd interface by returning the id of the command.
func (cmd *unparsableCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *unparsableCmd) Method() string {
	return cmd.method
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *unparsableCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.method, nil)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *unparsableCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	cmd.id = r.Id
	cmd.method = r.Method
	return nil
}

// AddMultisigAddressCmd is a type handling custom marshaling and
// unmarshaling of addmultisigaddress JSON RPC commands.
type AddMultisigAddressCmd struct {
	id        interface{}
	NRequired int
	Keys      []string
	Account   string
}

// Enforce that AddMultisigAddress satisifies the Cmd interface.
var _ Cmd = &AddMultisigAddressCmd{}

// NewAddMultisigAddressCmd creates a new AddMultisigAddressCmd,
// parsing the optional arguments optArgs which may be either empty or an
// optional account name.
func NewAddMultisigAddressCmd(id interface{}, nRequired int, keys []string,
	optArgs ...string) (*AddMultisigAddressCmd, error) {
	// Optional parameters set to their defaults.
	var account string

	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		account = optArgs[0]
	}

	return &AddMultisigAddressCmd{
		id:        id,
		NRequired: nRequired,
		Keys:      keys,
		Account:   account,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *AddMultisigAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *AddMultisigAddressCmd) Method() string {
	return "addmultisigaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *AddMultisigAddressCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 2, 3)
	params[0] = cmd.NRequired
	params[1] = cmd.Keys
	if cmd.Account != "" {
		params = append(params, cmd.Account)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *AddMultisigAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) < 2 || len(r.Params) > 3 {
		return ErrWrongNumberOfParams
	}

	var nRequired int
	if err := json.Unmarshal(r.Params[0], &nRequired); err != nil {
		return fmt.Errorf("first parameter 'nrequired' must be an integer: %v", err)
	}

	var keys []string
	if err := json.Unmarshal(r.Params[1], &keys); err != nil {
		return fmt.Errorf("second parameter 'keys' must be an array of strings: %v", err)
	}

	var account string
	if len(r.Params) > 2 {
		if err := json.Unmarshal(r.Params[2], &account); err != nil {
			return fmt.Errorf("third optional parameter 'account' must be a string: %v", err)
		}
	}
	newCmd, err := NewAddMultisigAddressCmd(r.Id, nRequired, keys, account)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// AddNodeCmd is a type handling custom marshaling and
// unmarshaling of addmultisigaddress JSON RPC commands.
type AddNodeCmd struct {
	id     interface{}
	Addr   string
	SubCmd string // One of  "add","remove","onetry". TODO(oga) enum?
}

// Enforce that AddNodeCmd satisifies the Cmd interface.
var _ Cmd = &AddNodeCmd{}

// NewAddNodeCmd creates a new AddNodeCmd for the given address and subcommand.
func NewAddNodeCmd(id interface{}, addr string, subcmd string) (
	*AddNodeCmd, error) {

	switch subcmd {
	case "add":
		// fine
	case "remove":
		// fine
	case "onetry":
		// fine
	default:
		return nil, errors.New("invalid subcommand for addnode")
	}

	return &AddNodeCmd{
		id:     id,
		Addr:   addr,
		SubCmd: subcmd,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *AddNodeCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *AddNodeCmd) Method() string {
	return "addnode"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *AddNodeCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Addr,
		cmd.SubCmd,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *AddNodeCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 2 {
		return ErrWrongNumberOfParams
	}

	var addr string
	if err := json.Unmarshal(r.Params[0], &addr); err != nil {
		return fmt.Errorf("first parameter 'addr' must be a string: %v", err)
	}

	var subcmd string
	if err := json.Unmarshal(r.Params[1], &subcmd); err != nil {
		return fmt.Errorf("second parameter 'subcmd' must be a string: %v", err)
	}

	newCmd, err := NewAddNodeCmd(r.Id, addr, subcmd)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// BackupWalletCmd is a type handling custom marshaling and
// unmarshaling of backupwallet JSON RPC commands.
type BackupWalletCmd struct {
	id          interface{}
	Destination string
}

// Enforce that BackupWalletCmd satisifies the Cmd interface.
var _ Cmd = &BackupWalletCmd{}

// NewBackupWalletCmd creates a new BackupWalletCmd.
func NewBackupWalletCmd(id interface{}, path string) (*BackupWalletCmd, error) {

	return &BackupWalletCmd{
		id:          id,
		Destination: path,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *BackupWalletCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *BackupWalletCmd) Method() string {
	return "backupwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *BackupWalletCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Destination,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *BackupWalletCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var destination string
	if err := json.Unmarshal(r.Params[0], &destination); err != nil {
		return fmt.Errorf("first parameter 'destination' must be a string: %v", err)
	}

	newCmd, err := NewBackupWalletCmd(r.Id, destination)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// CreateMultisigCmd is a type handling custom marshaling and
// unmarshaling of createmultisig JSON RPC commands.
type CreateMultisigCmd struct {
	id        interface{}
	NRequired int
	Keys      []string
}

// Enforce that AddMultisigAddress satisifies the Cmd interface.
var _ Cmd = &CreateMultisigCmd{}

// NewCreateMultisigCmd creates a new CreateMultisigCmd,
// parsing the optional arguments optArgs.
func NewCreateMultisigCmd(id interface{}, nRequired int, keys []string) (*CreateMultisigCmd, error) {
	return &CreateMultisigCmd{
		id:        id,
		NRequired: nRequired,
		Keys:      keys,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *CreateMultisigCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *CreateMultisigCmd) Method() string {
	return "createmultisig"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *CreateMultisigCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.NRequired,
		cmd.Keys,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *CreateMultisigCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 2 {
		return ErrWrongNumberOfParams
	}

	var nRequired int
	if err := json.Unmarshal(r.Params[0], &nRequired); err != nil {
		return fmt.Errorf("first parameter 'nrequired' must be an integer: %v", err)
	}

	var keys []string
	if err := json.Unmarshal(r.Params[1], &keys); err != nil {
		return fmt.Errorf("second parameter 'keys' must be an array of strings: %v", err)
	}

	newCmd, err := NewCreateMultisigCmd(r.Id, nRequired, keys)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// TransactionInput represents the inputs to a transaction. Specifically a
// transactionsha and output number pair.
type TransactionInput struct {
	Txid string `json:"txid"`
	Vout uint32 `json:"vout"`
}

// CreateRawTransactionCmd is a type handling custom marshaling and
// unmarshaling of createrawtransaction JSON RPC commands.
type CreateRawTransactionCmd struct {
	id      interface{}
	Inputs  []TransactionInput
	Amounts map[string]int64
}

// Enforce that CreateRawTransactionCmd satisifies the Cmd interface.
var _ Cmd = &CreateRawTransactionCmd{}

// NewCreateRawTransactionCmd creates a new CreateRawTransactionCmd.
func NewCreateRawTransactionCmd(id interface{}, inputs []TransactionInput, amounts map[string]int64) (*CreateRawTransactionCmd, error) {
	return &CreateRawTransactionCmd{
		id:      id,
		Inputs:  inputs,
		Amounts: amounts,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *CreateRawTransactionCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *CreateRawTransactionCmd) Method() string {
	return "createrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *CreateRawTransactionCmd) MarshalJSON() ([]byte, error) {
	floatAmounts := make(map[string]float64)
	for k, v := range cmd.Amounts {
		floatAmounts[k] = float64(v) / 1e8
	}

	params := []interface{}{
		cmd.Inputs,
		floatAmounts,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *CreateRawTransactionCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 2 {
		return ErrWrongNumberOfParams
	}

	var inputs []TransactionInput
	if err := json.Unmarshal(r.Params[0], &inputs); err != nil {
		return fmt.Errorf("first parameter 'inputs' must be a JSON array "+
			"of transaction input JSON objects: %v", err)
	}

	var amounts map[string]float64
	if err := json.Unmarshal(r.Params[1], &amounts); err != nil {
		return fmt.Errorf("second parameter 'amounts' must be a JSON object: %v", err)
	}

	intAmounts := make(map[string]int64, len(amounts))
	for k, v := range amounts {
		amount, err := JSONToAmount(v)
		if err != nil {
			return err
		}
		intAmounts[k] = amount
	}

	newCmd, err := NewCreateRawTransactionCmd(r.Id, inputs, intAmounts)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// DebugLevelCmd is a type handling custom marshaling and unmarshaling of
// debuglevel JSON RPC commands.  This command is not a standard Bitcoin
// command.  It is an extension for btcd.
type DebugLevelCmd struct {
	id        interface{}
	LevelSpec string
}

// Enforce that DebugLevelCmd satisifies the Cmd interface.
var _ Cmd = &DebugLevelCmd{}

// NewDebugLevelCmd creates a new DebugLevelCmd.  This command is not a standard
// Bitcoin command.  It is an extension for btcd.
func NewDebugLevelCmd(id interface{}, levelSpec string) (*DebugLevelCmd, error) {
	return &DebugLevelCmd{
		id:        id,
		LevelSpec: levelSpec,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *DebugLevelCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DebugLevelCmd) Method() string {
	return "debuglevel"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DebugLevelCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.LevelSpec,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *DebugLevelCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var levelSpec string
	if err := json.Unmarshal(r.Params[0], &levelSpec); err != nil {
		return fmt.Errorf("first parameter 'levelspec' must be a string: %v", err)
	}

	newCmd, err := NewDebugLevelCmd(r.Id, levelSpec)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// DecodeRawTransactionCmd is a type handling custom marshaling and
// unmarshaling of decoderawtransaction JSON RPC commands.
type DecodeRawTransactionCmd struct {
	id    interface{}
	HexTx string
}

// Enforce that DecodeRawTransactionCmd satisifies the Cmd interface.
var _ Cmd = &DecodeRawTransactionCmd{}

// NewDecodeRawTransactionCmd creates a new DecodeRawTransactionCmd.
func NewDecodeRawTransactionCmd(id interface{}, hextx string) (*DecodeRawTransactionCmd, error) {
	return &DecodeRawTransactionCmd{
		id:    id,
		HexTx: hextx,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *DecodeRawTransactionCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DecodeRawTransactionCmd) Method() string {
	return "decoderawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DecodeRawTransactionCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.HexTx,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *DecodeRawTransactionCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var hextx string
	if err := json.Unmarshal(r.Params[0], &hextx); err != nil {
		return fmt.Errorf("first parameter 'hextx' must be a string: %v", err)
	}

	newCmd, err := NewDecodeRawTransactionCmd(r.Id, hextx)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// DecodeScriptCmd is a type handling custom marshaling and
// unmarshaling of decodescript JSON RPC commands.
type DecodeScriptCmd struct {
	id        interface{}
	HexScript string
}

// Enforce that DecodeScriptCmd satisifies the Cmd interface.
var _ Cmd = &DecodeScriptCmd{}

// NewDecodeScriptCmd creates a new DecodeScriptCmd.
func NewDecodeScriptCmd(id interface{}, hexscript string) (*DecodeScriptCmd, error) {
	return &DecodeScriptCmd{
		id:        id,
		HexScript: hexscript,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *DecodeScriptCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DecodeScriptCmd) Method() string {
	return "decodescript"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DecodeScriptCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.HexScript,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *DecodeScriptCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var hexscript string
	if err := json.Unmarshal(r.Params[0], &hexscript); err != nil {
		return fmt.Errorf("first parameter 'hexscript' must be a string: %v", err)
	}

	newCmd, err := NewDecodeScriptCmd(r.Id, hexscript)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// DumpPrivKeyCmd is a type handling custom marshaling and
// unmarshaling of dumpprivkey JSON RPC commands.
type DumpPrivKeyCmd struct {
	id      interface{}
	Address string
}

// Enforce that DumpPrivKeyCmd satisifies the Cmd interface.
var _ Cmd = &DumpPrivKeyCmd{}

// NewDumpPrivKeyCmd creates a new DumpPrivkeyCmd.
func NewDumpPrivKeyCmd(id interface{}, address string) (*DumpPrivKeyCmd, error) {
	return &DumpPrivKeyCmd{
		id:      id,
		Address: address,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *DumpPrivKeyCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DumpPrivKeyCmd) Method() string {
	return "dumpprivkey"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DumpPrivKeyCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Address,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *DumpPrivKeyCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	newCmd, err := NewDumpPrivKeyCmd(r.Id, address)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// DumpWalletCmd is a type handling custom marshaling and
// unmarshaling of dumpwallet JSON RPC commands.
type DumpWalletCmd struct {
	id       interface{}
	Filename string
}

// Enforce that DumpWalletCmd satisifies the Cmd interface.
var _ Cmd = &DumpWalletCmd{}

// NewDumpWalletCmd creates a new DumpPrivkeyCmd.
func NewDumpWalletCmd(id interface{}, filename string) (*DumpWalletCmd, error) {
	return &DumpWalletCmd{
		id:       id,
		Filename: filename,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *DumpWalletCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DumpWalletCmd) Method() string {
	return "dumpwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DumpWalletCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Filename,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *DumpWalletCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var filename string
	if err := json.Unmarshal(r.Params[0], &filename); err != nil {
		return fmt.Errorf("first parameter 'filename' must be a string: %v", err)
	}

	newCmd, err := NewDumpWalletCmd(r.Id, filename)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// EncryptWalletCmd is a type handling custom marshaling and
// unmarshaling of encryptwallet JSON RPC commands.
type EncryptWalletCmd struct {
	id         interface{}
	Passphrase string
}

// Enforce that EncryptWalletCmd satisifies the Cmd interface.
var _ Cmd = &EncryptWalletCmd{}

// NewEncryptWalletCmd creates a new EncryptWalletCmd.
func NewEncryptWalletCmd(id interface{}, passphrase string) (*EncryptWalletCmd, error) {
	return &EncryptWalletCmd{
		id:         id,
		Passphrase: passphrase,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *EncryptWalletCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *EncryptWalletCmd) Method() string {
	return "encryptwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *EncryptWalletCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Passphrase,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *EncryptWalletCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var passphrase string
	if err := json.Unmarshal(r.Params[0], &passphrase); err != nil {
		return fmt.Errorf("first parameter 'passphrase' must be a string: %v", err)
	}

	newCmd, err := NewEncryptWalletCmd(r.Id, passphrase)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// EstimateFeeCmd is a type handling custom marshaling and
// unmarshaling of estimatefee JSON RPC commands.
type EstimateFeeCmd struct {
	id        interface{}
	NumBlocks int64
}

// Enforce that EstimateFeeCmd satisifies the Cmd interface.
var _ Cmd = &EstimateFeeCmd{}

// NewEstimateFeeCmd creates a new EstimateFeeCmd.
func NewEstimateFeeCmd(id interface{}, numblocks int64) (*EstimateFeeCmd, error) {
	return &EstimateFeeCmd{
		id:        id,
		NumBlocks: numblocks,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *EstimateFeeCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *EstimateFeeCmd) Method() string {
	return "estimatefee"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *EstimateFeeCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.NumBlocks,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *EstimateFeeCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var numblocks int64
	if err := json.Unmarshal(r.Params[0], &numblocks); err != nil {
		return fmt.Errorf("first parameter 'numblocks' must be an integer: %v", err)
	}

	newCmd, err := NewEstimateFeeCmd(r.Id, numblocks)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// EstimatePriorityCmd is a type handling custom marshaling and
// unmarshaling of estimatepriority JSON RPC commands.
type EstimatePriorityCmd struct {
	id        interface{}
	NumBlocks int64
}

// Enforce that EstimatePriorityCmd satisifies the Cmd interface.
var _ Cmd = &EstimatePriorityCmd{}

// NewEstimatePriorityCmd creates a new EstimatePriorityCmd.
func NewEstimatePriorityCmd(id interface{}, numblocks int64) (*EstimatePriorityCmd, error) {
	return &EstimatePriorityCmd{
		id:        id,
		NumBlocks: numblocks,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *EstimatePriorityCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *EstimatePriorityCmd) Method() string {
	return "estimatepriority"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *EstimatePriorityCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.NumBlocks,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *EstimatePriorityCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var numblocks int64
	if err := json.Unmarshal(r.Params[0], &numblocks); err != nil {
		return fmt.Errorf("first parameter 'numblocks' must be an integer: %v", err)
	}

	newCmd, err := NewEstimatePriorityCmd(r.Id, numblocks)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetAccountCmd is a type handling custom marshaling and
// unmarshaling of getaccount JSON RPC commands.
type GetAccountCmd struct {
	id      interface{}
	Address string
}

// Enforce that GetAccountCmd satisifies the Cmd interface.
var _ Cmd = &GetAccountCmd{}

// NewGetAccountCmd creates a new GetAccountCmd.
func NewGetAccountCmd(id interface{}, address string) (*GetAccountCmd, error) {
	return &GetAccountCmd{
		id:      id,
		Address: address,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetAccountCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAccountCmd) Method() string {
	return "getaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAccountCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Address,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetAccountCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	newCmd, err := NewGetAccountCmd(r.Id, address)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetAccountAddressCmd is a type handling custom marshaling and
// unmarshaling of getaccountaddress JSON RPC commands.
type GetAccountAddressCmd struct {
	id      interface{}
	Account string
}

// Enforce that GetAccountAddressCmd satisifies the Cmd interface.
var _ Cmd = &GetAccountAddressCmd{}

// NewGetAccountAddressCmd creates a new GetAccountAddressCmd.
func NewGetAccountAddressCmd(id interface{}, account string) (*GetAccountAddressCmd, error) {
	return &GetAccountAddressCmd{
		id:      id,
		Account: account,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetAccountAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAccountAddressCmd) Method() string {
	return "getaccountaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAccountAddressCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Account,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetAccountAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}
	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return fmt.Errorf("first parameter 'account' must be a string: %v", err)
	}

	newCmd, err := NewGetAccountAddressCmd(r.Id, account)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetAddedNodeInfoCmd is a type handling custom marshaling and
// unmarshaling of getaddednodeinfo JSON RPC commands.
type GetAddedNodeInfoCmd struct {
	id   interface{}
	Dns  bool
	Node string
}

// Enforce that GetAddedNodeInfoCmd satisifies the Cmd interface.
var _ Cmd = &GetAddedNodeInfoCmd{}

// NewGetAddedNodeInfoCmd creates a new GetAddedNodeInfoCmd. Optionally the
// node to be queried may be provided. More than one optonal argument is an
// error.
func NewGetAddedNodeInfoCmd(id interface{}, dns bool, optArgs ...string) (*GetAddedNodeInfoCmd, error) {
	var node string

	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		node = optArgs[0]
	}
	return &GetAddedNodeInfoCmd{
		id:   id,
		Dns:  dns,
		Node: node,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetAddedNodeInfoCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAddedNodeInfoCmd) Method() string {
	return "getaddednodeinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAddedNodeInfoCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Dns,
	}

	if cmd.Node != "" {
		params = append(params, cmd.Node)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetAddedNodeInfoCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) < 1 || len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	var dns bool
	if err := json.Unmarshal(r.Params[0], &dns); err != nil {
		return fmt.Errorf("first parameter 'dns' must be a bool: %v", err)
	}

	var node string
	if len(r.Params) > 1 {
		if err := json.Unmarshal(r.Params[1], &node); err != nil {
			return fmt.Errorf("second optional parameter 'node' must be a string: %v", err)
		}
	}

	newCmd, err := NewGetAddedNodeInfoCmd(r.Id, dns, node)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetAddressesByAccountCmd is a type handling custom marshaling and
// unmarshaling of getaddressesbyaccount JSON RPC commands.
type GetAddressesByAccountCmd struct {
	id      interface{}
	Account string
}

// Enforce that GetAddressesByAccountCmd satisifies the Cmd interface.
var _ Cmd = &GetAddressesByAccountCmd{}

// NewGetAddressesByAccountCmd creates a new GetAddressesByAccountCmd.
func NewGetAddressesByAccountCmd(id interface{}, account string) (*GetAddressesByAccountCmd, error) {
	return &GetAddressesByAccountCmd{
		id:      id,
		Account: account,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetAddressesByAccountCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAddressesByAccountCmd) Method() string {
	return "getaddressesbyaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAddressesByAccountCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Account,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetAddressesByAccountCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return fmt.Errorf("first parameter 'account' must be a string: %v", err)
	}

	newCmd, err := NewGetAddressesByAccountCmd(r.Id, account)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetBalanceCmd is a type handling custom marshaling and
// unmarshaling of getbalance JSON RPC commands.
type GetBalanceCmd struct {
	id      interface{}
	Account string
	MinConf int
}

// Enforce that GetBalanceCmd satisifies the Cmd interface.
var _ Cmd = &GetBalanceCmd{}

// NewGetBalanceCmd creates a new GetBalanceCmd. Optionally a string for account
// and an int for minconf may be provided as arguments.
func NewGetBalanceCmd(id interface{}, optArgs ...interface{}) (*GetBalanceCmd, error) {
	var account string
	var minconf = 1

	if len(optArgs) > 2 {
		return nil, ErrWrongNumberOfParams
	}
	if len(optArgs) > 0 {
		a, ok := optArgs[0].(string)
		if !ok {
			return nil, errors.New("first optional argument account is not a string")
		}
		account = a
	}

	if len(optArgs) > 1 {
		m, ok := optArgs[1].(int)
		if !ok {
			return nil, errors.New("second optional argument minconf is not a int")
		}
		minconf = m
	}

	return &GetBalanceCmd{
		id:      id,
		Account: account,
		MinConf: minconf,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetBalanceCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBalanceCmd) Method() string {
	return "getbalance"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBalanceCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 2)
	if cmd.Account != "" || cmd.MinConf != 1 {
		params = append(params, cmd.Account)
	}
	if cmd.MinConf != 1 {
		params = append(params, cmd.MinConf)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBalanceCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 0 {
		var account string
		if err := json.Unmarshal(r.Params[0], &account); err != nil {
			return fmt.Errorf("first optional parameter 'account' must be a string: %v", err)
		}
		optArgs = append(optArgs, account)
	}

	if len(r.Params) > 1 {
		var minconf int
		if err := json.Unmarshal(r.Params[1], &minconf); err != nil {
			return fmt.Errorf("second optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}

	newCmd, err := NewGetBalanceCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetBestBlockHashCmd is a type handling custom marshaling and
// unmarshaling of getbestblockhash JSON RPC commands.
type GetBestBlockHashCmd struct {
	id interface{}
}

// Enforce that GetBestBlockHashCmd satisifies the Cmd interface.
var _ Cmd = &GetBestBlockHashCmd{}

// NewGetBestBlockHashCmd creates a new GetBestBlockHashCmd.
func NewGetBestBlockHashCmd(id interface{}) (*GetBestBlockHashCmd, error) {
	return &GetBestBlockHashCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetBestBlockHashCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBestBlockHashCmd) Method() string {
	return "getbestblockhash"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBestBlockHashCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBestBlockHashCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetBestBlockHashCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetBlockCmd is a type handling custom marshaling and
// unmarshaling of getblock JSON RPC commands.
type GetBlockCmd struct {
	id        interface{}
	Hash      string
	Verbose   bool
	VerboseTx bool
}

// Enforce that GetBlockCmd satisifies the Cmd interface.
var _ Cmd = &GetBlockCmd{}

// NewGetBlockCmd creates a new GetBlockCmd.
func NewGetBlockCmd(id interface{}, hash string, optArgs ...bool) (*GetBlockCmd, error) {
	// default verbose is set to true to match old behavior
	verbose, verboseTx := true, false

	optArgsLen := len(optArgs)
	if optArgsLen > 0 {
		if optArgsLen > 2 {
			return nil, ErrTooManyOptArgs
		}
		verbose = optArgs[0]
		if optArgsLen > 1 {
			verboseTx = optArgs[1]

			if !verbose && verboseTx {
				return nil, ErrInvalidParams
			}
		}
	}

	return &GetBlockCmd{
		id:        id,
		Hash:      hash,
		Verbose:   verbose,
		VerboseTx: verboseTx,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetBlockCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockCmd) Method() string {
	return "getblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 3)
	params[0] = cmd.Hash
	if !cmd.Verbose {
		// set optional verbose argument to false
		params = append(params, false)
	} else if cmd.VerboseTx {
		// set optional verbose argument to true
		params = append(params, true)
		// set optional verboseTx argument to true
		params = append(params, true)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBlockCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 3 || len(r.Params) < 1 {
		return ErrWrongNumberOfParams
	}

	var hash string
	if err := json.Unmarshal(r.Params[0], &hash); err != nil {
		return fmt.Errorf("first parameter 'hash' must be a string: %v", err)
	}

	optArgs := make([]bool, 0, 2)
	if len(r.Params) > 1 {
		var verbose bool
		if err := json.Unmarshal(r.Params[1], &verbose); err != nil {
			return fmt.Errorf("second optional parameter 'verbose' must be a bool: %v", err)
		}
		optArgs = append(optArgs, verbose)
	}
	if len(r.Params) > 2 {
		var verboseTx bool
		if err := json.Unmarshal(r.Params[2], &verboseTx); err != nil {
			return fmt.Errorf("third optional parameter 'verboseTx' must be a bool: %v", err)
		}
		optArgs = append(optArgs, verboseTx)
	}

	newCmd, err := NewGetBlockCmd(r.Id, hash, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetBlockChainInfoCmd is a type handling custom marshaling and
// unmarshaling of getblockchaininfo JSON RPC commands.
type GetBlockChainInfoCmd struct {
	id interface{}
}

// Enforce that GetBlockChainInfoCmd satisifies the Cmd interface.
var _ Cmd = &GetBlockChainInfoCmd{}

// NewGetBlockChainInfoCmd creates a new GetBlockChainInfoCmd.
func NewGetBlockChainInfoCmd(id interface{}) (*GetBlockChainInfoCmd, error) {
	return &GetBlockChainInfoCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetBlockChainInfoCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockChainInfoCmd) Method() string {
	return "getblockchaininfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockChainInfoCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBlockChainInfoCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetBlockChainInfoCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetBlockCountCmd is a type handling custom marshaling and
// unmarshaling of getblockcount JSON RPC commands.
type GetBlockCountCmd struct {
	id interface{}
}

// Enforce that GetBlockCountCmd satisifies the Cmd interface.
var _ Cmd = &GetBlockCountCmd{}

// NewGetBlockCountCmd creates a new GetBlockCountCmd.
func NewGetBlockCountCmd(id interface{}) (*GetBlockCountCmd, error) {
	return &GetBlockCountCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetBlockCountCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockCountCmd) Method() string {
	return "getblockcount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockCountCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBlockCountCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetBlockCountCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetBlockHashCmd is a type handling custom marshaling and
// unmarshaling of getblockhash JSON RPC commands.
type GetBlockHashCmd struct {
	id    interface{}
	Index int64
}

// Enforce that GetBlockHashCmd satisifies the Cmd interface.
var _ Cmd = &GetBlockHashCmd{}

// NewGetBlockHashCmd creates a new GetBlockHashCmd.
func NewGetBlockHashCmd(id interface{}, index int64) (*GetBlockHashCmd, error) {
	return &GetBlockHashCmd{
		id:    id,
		Index: index,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetBlockHashCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockHashCmd) Method() string {
	return "getblockhash"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockHashCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Index,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBlockHashCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var index int64
	if err := json.Unmarshal(r.Params[0], &index); err != nil {
		return fmt.Errorf("first parameter 'index' must be an integer: %v", err)
	}

	newCmd, err := NewGetBlockHashCmd(r.Id, index)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// TemplateRequest is a request object as defined in BIP22
// (https://en.bitcoin.it/wiki/BIP_0022), it is optionally provided as an
// pointer argument to GetBlockTemplateCmd.
type TemplateRequest struct {
	Mode         string   `json:"mode,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`

	// Optional long polling.
	LongPollID string `json:"longpollid,omitempty"`

	// Optional template tweaking.  SigOpLimit and SizeLimit can be int64
	// or bool.
	SigOpLimit interface{} `json:"sigoplimit,omitempty"`
	SizeLimit  interface{} `json:"sizelimit,omitempty"`
	MaxVersion uint32      `json:"maxversion,omitempty"`

	// Basic pool extension from BIP 0023.
	Target string `json:"target,omitempty"`

	// Block proposal from BIP 0023.  Data is only provided when Mode is
	// "proposal".
	Data   string `json:"data,omitempty"`
	WorkID string `json:"workid,omitempty"`
}

// isFloatInt64 returns whether the passed float64 is a whole number that safely
// fits into a 64-bit integer.
func isFloatInt64(a float64) bool {
	return a == float64(int64(a))
}

// GetBlockTemplateCmd is a type handling custom marshaling and
// unmarshaling of getblocktemplate JSON RPC commands.
type GetBlockTemplateCmd struct {
	id      interface{}
	Request *TemplateRequest
}

// Enforce that GetBlockTemplateCmd satisifies the Cmd interface.
var _ Cmd = &GetBlockTemplateCmd{}

// NewGetBlockTemplateCmd creates a new GetBlockTemplateCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewGetBlockTemplateCmd(id interface{}, optArgs ...*TemplateRequest) (*GetBlockTemplateCmd, error) {
	var request *TemplateRequest
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		request = optArgs[0]
		switch val := request.SigOpLimit.(type) {
		case nil:
		case bool:
		case int64:
		case float64:
			if !isFloatInt64(val) {
				return nil, errors.New("the sigoplimit field " +
					"must be unspecified, a boolean, or a " +
					"64-bit integer")
			}
			request.SigOpLimit = int64(val)
		default:
			return nil, errors.New("the sigoplimit field " +
				"must be unspecified, a boolean, or a 64-bit " +
				"integer")
		}
		switch val := request.SizeLimit.(type) {
		case nil:
		case bool:
		case int64:
		case float64:
			if !isFloatInt64(val) {
				return nil, errors.New("the sizelimit field " +
					"must be unspecified, a boolean, or a " +
					"64-bit integer")
			}
			request.SizeLimit = int64(val)
		default:
			return nil, errors.New("the sizelimit field " +
				"must be unspecified, a boolean, or a 64-bit " +
				"integer")
		}
	}
	return &GetBlockTemplateCmd{
		id:      id,
		Request: request,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetBlockTemplateCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockTemplateCmd) Method() string {
	return "getblocktemplate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockTemplateCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Request != nil {
		params = append(params, cmd.Request)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBlockTemplateCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]*TemplateRequest, 0, 1)
	if len(r.Params) > 0 {
		var template TemplateRequest
		if err := json.Unmarshal(r.Params[0], &template); err != nil {
			return fmt.Errorf("first optional parameter 'template' must be a template request JSON object: %v", err)
		}
		optArgs = append(optArgs, &template)
	}

	newCmd, err := NewGetBlockTemplateCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetConnectionCountCmd is a type handling custom marshaling and
// unmarshaling of getconnectioncount JSON RPC commands.
type GetConnectionCountCmd struct {
	id interface{}
}

// Enforce that GetConnectionCountCmd satisifies the Cmd interface.
var _ Cmd = &GetConnectionCountCmd{}

// NewGetConnectionCountCmd creates a new GetConnectionCountCmd.
func NewGetConnectionCountCmd(id interface{}) (*GetConnectionCountCmd, error) {
	return &GetConnectionCountCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetConnectionCountCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetConnectionCountCmd) Method() string {
	return "getconnectioncount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetConnectionCountCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetConnectionCountCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetConnectionCountCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetDifficultyCmd is a type handling custom marshaling and
// unmarshaling of getdifficulty JSON RPC commands.
type GetDifficultyCmd struct {
	id interface{}
}

// Enforce that GetDifficultyCmd satisifies the Cmd interface.
var _ Cmd = &GetDifficultyCmd{}

// NewGetDifficultyCmd creates a new GetDifficultyCmd.
func NewGetDifficultyCmd(id interface{}) (*GetDifficultyCmd, error) {
	return &GetDifficultyCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetDifficultyCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetDifficultyCmd) Method() string {
	return "getdifficulty"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetDifficultyCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetDifficultyCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetDifficultyCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetGenerateCmd is a type handling custom marshaling and
// unmarshaling of getgenerate JSON RPC commands.
type GetGenerateCmd struct {
	id interface{}
}

// Enforce that GetGenerateCmd satisifies the Cmd interface.
var _ Cmd = &GetGenerateCmd{}

// NewGetGenerateCmd creates a new GetGenerateCmd.
func NewGetGenerateCmd(id interface{}) (*GetGenerateCmd, error) {
	return &GetGenerateCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetGenerateCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetGenerateCmd) Method() string {
	return "getgenerate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetGenerateCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetGenerateCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetGenerateCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetHashesPerSecCmd is a type handling custom marshaling and
// unmarshaling of gethashespersec JSON RPC commands.
type GetHashesPerSecCmd struct {
	id interface{}
}

// Enforce that GetHashesPerSecCmd satisifies the Cmd interface.
var _ Cmd = &GetHashesPerSecCmd{}

// NewGetHashesPerSecCmd creates a new GetHashesPerSecCmd.
func NewGetHashesPerSecCmd(id interface{}) (*GetHashesPerSecCmd, error) {
	return &GetHashesPerSecCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetHashesPerSecCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetHashesPerSecCmd) Method() string {
	return "gethashespersec"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetHashesPerSecCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetHashesPerSecCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetHashesPerSecCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetInfoCmd is a type handling custom marshaling and
// unmarshaling of getinfo JSON RPC commands.
type GetInfoCmd struct {
	id interface{}
}

// Enforce that GetInfoCmd satisifies the Cmd interface.
var _ Cmd = &GetInfoCmd{}

// NewGetInfoCmd creates a new GetInfoCmd.
func NewGetInfoCmd(id interface{}) (*GetInfoCmd, error) {
	return &GetInfoCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetInfoCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetInfoCmd) Method() string {
	return "getinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetInfoCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetInfoCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetInfoCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetMiningInfoCmd is a type handling custom marshaling and
// unmarshaling of getmininginfo JSON RPC commands.
type GetMiningInfoCmd struct {
	id interface{}
}

// Enforce that GetMiningInfoCmd satisifies the Cmd interface.
var _ Cmd = &GetMiningInfoCmd{}

// NewGetMiningInfoCmd creates a new GetMiningInfoCmd.
func NewGetMiningInfoCmd(id interface{}) (*GetMiningInfoCmd, error) {
	return &GetMiningInfoCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetMiningInfoCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetMiningInfoCmd) Method() string {
	return "getmininginfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetMiningInfoCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetMiningInfoCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetMiningInfoCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetNetworkInfoCmd is a type handling custom marshaling and
// unmarshaling of getnetworkinfo JSON RPC commands.
type GetNetworkInfoCmd struct {
	id interface{}
}

// Enforce that GetNetworkInfoCmd satisifies the Cmd interface.
var _ Cmd = &GetNetworkInfoCmd{}

// NewGetNetworkInfoCmd creates a new GetNetworkInfoCmd.
func NewGetNetworkInfoCmd(id interface{}) (*GetNetworkInfoCmd, error) {
	return &GetNetworkInfoCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetNetworkInfoCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetNetworkInfoCmd) Method() string {
	return "getnetworkinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetNetworkInfoCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetNetworkInfoCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetNetworkInfoCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetNetTotalsCmd is a type handling custom marshaling and
// unmarshaling of getnettotals JSON RPC commands.
type GetNetTotalsCmd struct {
	id interface{}
}

// Enforce that GetNetTotalsCmd satisifies the Cmd interface.
var _ Cmd = &GetNetTotalsCmd{}

// NewGetNetTotalsCmd creates a new GetNetTotalsCmd.
func NewGetNetTotalsCmd(id interface{}) (*GetNetTotalsCmd, error) {
	return &GetNetTotalsCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetNetTotalsCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetNetTotalsCmd) Method() string {
	return "getnettotals"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetNetTotalsCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetNetTotalsCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetNetTotalsCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetNetworkHashPSCmd is a type handling custom marshaling and
// unmarshaling of getnetworkhashps JSON RPC commands.
type GetNetworkHashPSCmd struct {
	id     interface{}
	Blocks int
	Height int
}

// Enforce that GetNetworkHashPSCmd satisifies the Cmd interface.
var _ Cmd = &GetNetworkHashPSCmd{}

// NewGetNetworkHashPSCmd creates a new GetNetworkHashPSCmd.
func NewGetNetworkHashPSCmd(id interface{}, optArgs ...int) (*GetNetworkHashPSCmd, error) {
	blocks := 120
	height := -1

	if len(optArgs) > 0 {
		if len(optArgs) > 2 {
			return nil, ErrTooManyOptArgs
		}

		blocks = optArgs[0]

		if len(optArgs) > 1 {
			height = optArgs[1]
		}
	}
	return &GetNetworkHashPSCmd{
		id:     id,
		Blocks: blocks,
		Height: height,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetNetworkHashPSCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetNetworkHashPSCmd) Method() string {
	return "getnetworkhashps"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetNetworkHashPSCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 2)
	if cmd.Blocks != 120 || cmd.Height != -1 {
		params = append(params, cmd.Blocks)
	}
	if cmd.Height != -1 {
		params = append(params, cmd.Height)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetNetworkHashPSCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]int, 0, 2)
	if len(r.Params) > 0 {
		var blocks int
		if err := json.Unmarshal(r.Params[0], &blocks); err != nil {
			return fmt.Errorf("first optional parameter 'blocks' must be an integer: %v", err)
		}
		optArgs = append(optArgs, blocks)
	}

	if len(r.Params) > 1 {
		var height int
		if err := json.Unmarshal(r.Params[1], &height); err != nil {
			return fmt.Errorf("second optional parameter 'height' must be an integer: %v", err)
		}
		optArgs = append(optArgs, height)
	}
	newCmd, err := NewGetNetworkHashPSCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetNewAddressCmd is a type handling custom marshaling and
// unmarshaling of getnewaddress JSON RPC commands.
type GetNewAddressCmd struct {
	id      interface{}
	Account string
}

// Enforce that GetNewAddressCmd satisifies the Cmd interface.
var _ Cmd = &GetNewAddressCmd{}

// NewGetNewAddressCmd creates a new GetNewAddressCmd.
func NewGetNewAddressCmd(id interface{}, optArgs ...string) (*GetNewAddressCmd, error) {
	var account string
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		account = optArgs[0]

	}
	return &GetNewAddressCmd{
		id:      id,
		Account: account,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetNewAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetNewAddressCmd) Method() string {
	return "getnewaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetNewAddressCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Account != "" {
		params = append(params, cmd.Account)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetNewAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]string, 0, 1)
	if len(r.Params) > 0 {
		var account string
		if err := json.Unmarshal(r.Params[0], &account); err != nil {
			return fmt.Errorf("first optional parameter 'account' must be a string: %v", err)
		}
		optArgs = append(optArgs, account)
	}

	newCmd, err := NewGetNewAddressCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetPeerInfoCmd is a type handling custom marshaling and
// unmarshaling of getpeerinfo JSON RPC commands.
type GetPeerInfoCmd struct {
	id interface{}
}

// Enforce that GetPeerInfoCmd satisifies the Cmd interface.
var _ Cmd = &GetPeerInfoCmd{}

// NewGetPeerInfoCmd creates a new GetPeerInfoCmd.
func NewGetPeerInfoCmd(id interface{}) (*GetPeerInfoCmd, error) {
	return &GetPeerInfoCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetPeerInfoCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetPeerInfoCmd) Method() string {
	return "getpeerinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetPeerInfoCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetPeerInfoCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetPeerInfoCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetRawChangeAddressCmd is a type handling custom marshaling and
// unmarshaling of getrawchangeaddress JSON RPC commands.
type GetRawChangeAddressCmd struct {
	id      interface{}
	Account string
}

// Enforce that GetRawChangeAddressCmd satisifies the Cmd interface.
var _ Cmd = &GetRawChangeAddressCmd{}

// NewGetRawChangeAddressCmd creates a new GetRawChangeAddressCmd.
func NewGetRawChangeAddressCmd(id interface{}, optArgs ...string) (*GetRawChangeAddressCmd, error) {
	var account string
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		account = optArgs[0]

	}
	return &GetRawChangeAddressCmd{
		id:      id,
		Account: account,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetRawChangeAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetRawChangeAddressCmd) Method() string {
	return "getrawchangeaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetRawChangeAddressCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Account != "" {
		params = append(params, cmd.Account)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetRawChangeAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]string, 0, 1)
	if len(r.Params) > 0 {
		var account string
		if err := json.Unmarshal(r.Params[0], &account); err != nil {
			return fmt.Errorf("first optional parameter 'account' must be a string: %v", err)
		}
		optArgs = append(optArgs, account)
	}

	newCmd, err := NewGetRawChangeAddressCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetRawMempoolCmd is a type handling custom marshaling and
// unmarshaling of getrawmempool JSON RPC commands.
type GetRawMempoolCmd struct {
	id      interface{}
	Verbose bool
}

// Enforce that GetRawMempoolCmd satisifies the Cmd interface.
var _ Cmd = &GetRawMempoolCmd{}

// NewGetRawMempoolCmd creates a new GetRawMempoolCmd.
func NewGetRawMempoolCmd(id interface{}, optArgs ...bool) (*GetRawMempoolCmd, error) {
	verbose := false
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		verbose = optArgs[0]
	}
	return &GetRawMempoolCmd{
		id:      id,
		Verbose: verbose,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetRawMempoolCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetRawMempoolCmd) Method() string {
	return "getrawmempool"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetRawMempoolCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Verbose {
		params = append(params, cmd.Verbose)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetRawMempoolCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]bool, 0, 1)
	if len(r.Params) > 0 {
		var verbose bool
		if err := json.Unmarshal(r.Params[0], &verbose); err != nil {
			return fmt.Errorf("first optional parameter 'verbose' must be a bool: %v", err)
		}
		optArgs = append(optArgs, verbose)
	}

	newCmd, err := NewGetRawMempoolCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetRawTransactionCmd is a type handling custom marshaling and
// unmarshaling of getrawtransaction JSON RPC commands.
type GetRawTransactionCmd struct {
	id      interface{}
	Txid    string
	Verbose int
}

// Enforce that GetRawTransactionCmd satisifies the Cmd interface.
var _ Cmd = &GetRawTransactionCmd{}

// NewGetRawTransactionCmd creates a new GetRawTransactionCmd.
func NewGetRawTransactionCmd(id interface{}, txid string, optArgs ...int) (*GetRawTransactionCmd, error) {
	var verbose int
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		verbose = optArgs[0]

	}
	return &GetRawTransactionCmd{
		id:      id,
		Txid:    txid,
		Verbose: verbose,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetRawTransactionCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetRawTransactionCmd) Method() string {
	return "getrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetRawTransactionCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.Txid
	if cmd.Verbose != 0 {
		params = append(params, cmd.Verbose)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetRawTransactionCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 || len(r.Params) < 1 {
		return ErrWrongNumberOfParams
	}

	var txid string
	if err := json.Unmarshal(r.Params[0], &txid); err != nil {
		return fmt.Errorf("first parameter 'txid' must be a string: %v", err)
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) > 1 {
		var verbose int
		if err := json.Unmarshal(r.Params[1], &verbose); err != nil {
			return fmt.Errorf("second optional parameter 'verbose' must be an integer: %v", err)
		}
		optArgs = append(optArgs, verbose)
	}

	newCmd, err := NewGetRawTransactionCmd(r.Id, txid, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetReceivedByAccountCmd is a type handling custom marshaling and
// unmarshaling of getreceivedbyaccount JSON RPC commands.
type GetReceivedByAccountCmd struct {
	id      interface{}
	Account string
	MinConf int
}

// Enforce that GetReceivedByAccountCmd satisifies the Cmd interface.
var _ Cmd = &GetReceivedByAccountCmd{}

// NewGetReceivedByAccountCmd creates a new GetReceivedByAccountCmd.
func NewGetReceivedByAccountCmd(id interface{}, account string, optArgs ...int) (*GetReceivedByAccountCmd, error) {
	if len(optArgs) > 1 {
		return nil, ErrTooManyOptArgs
	}
	var minconf = 1
	if len(optArgs) > 0 {
		minconf = optArgs[0]
	}
	return &GetReceivedByAccountCmd{
		id:      id,
		Account: account,
		MinConf: minconf,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetReceivedByAccountCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetReceivedByAccountCmd) Method() string {
	return "getreceivedbyaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetReceivedByAccountCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.Account
	if cmd.MinConf != 1 {
		params = append(params, cmd.MinConf)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetReceivedByAccountCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return fmt.Errorf("first parameter 'account' must be a string: %v", err)
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) > 1 {
		var minconf int
		if err := json.Unmarshal(r.Params[1], &minconf); err != nil {
			return fmt.Errorf("second optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}

	newCmd, err := NewGetReceivedByAccountCmd(r.Id, account, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetReceivedByAddressCmd is a type handling custom marshaling and
// unmarshaling of getreceivedbyaddress JSON RPC commands.
type GetReceivedByAddressCmd struct {
	id      interface{}
	Address string
	MinConf int
}

// Enforce that GetReceivedByAddressCmd satisifies the Cmd interface.
var _ Cmd = &GetReceivedByAddressCmd{}

// NewGetReceivedByAddressCmd creates a new GetReceivedByAddressCmd.
func NewGetReceivedByAddressCmd(id interface{}, address string, optArgs ...int) (*GetReceivedByAddressCmd, error) {
	if len(optArgs) > 1 {
		return nil, ErrTooManyOptArgs
	}
	var minconf = 1
	if len(optArgs) > 0 {
		minconf = optArgs[0]
	}
	return &GetReceivedByAddressCmd{
		id:      id,
		Address: address,
		MinConf: minconf,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetReceivedByAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetReceivedByAddressCmd) Method() string {
	return "getreceivedbyaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetReceivedByAddressCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.Address
	if cmd.MinConf != 1 {
		params = append(params, cmd.MinConf)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetReceivedByAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) > 1 {
		var minconf int
		if err := json.Unmarshal(r.Params[1], &minconf); err != nil {
			return fmt.Errorf("second optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}

	newCmd, err := NewGetReceivedByAddressCmd(r.Id, address, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetTransactionCmd is a type handling custom marshaling and
// unmarshaling of gettransaction JSON RPC commands.
type GetTransactionCmd struct {
	id   interface{}
	Txid string
}

// Enforce that GetTransactionCmd satisifies the Cmd interface.
var _ Cmd = &GetTransactionCmd{}

// NewGetTransactionCmd creates a new GetTransactionCmd.
func NewGetTransactionCmd(id interface{}, txid string) (*GetTransactionCmd, error) {
	return &GetTransactionCmd{
		id:   id,
		Txid: txid,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetTransactionCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetTransactionCmd) Method() string {
	return "gettransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetTransactionCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Txid,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetTransactionCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var txid string
	if err := json.Unmarshal(r.Params[0], &txid); err != nil {
		return fmt.Errorf("first parameter 'txid' must be a string: %v", err)
	}

	newCmd, err := NewGetTransactionCmd(r.Id, txid)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetTxOutCmd is a type handling custom marshaling and
// unmarshaling of gettxout JSON RPC commands.
type GetTxOutCmd struct {
	id             interface{}
	Txid           string
	Output         int
	IncludeMempool bool
}

// Enforce that GetTxOutCmd satisifies the Cmd interface.
var _ Cmd = &GetTxOutCmd{}

// NewGetTxOutCmd creates a new GetTxOutCmd.
func NewGetTxOutCmd(id interface{}, txid string, output int, optArgs ...bool) (*GetTxOutCmd, error) {
	mempool := true
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		mempool = optArgs[0]
	}
	return &GetTxOutCmd{
		id:             id,
		Txid:           txid,
		Output:         output,
		IncludeMempool: mempool,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetTxOutCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetTxOutCmd) Method() string {
	return "gettxout"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetTxOutCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 2, 3)
	params[0] = cmd.Txid
	params[1] = cmd.Output
	if !cmd.IncludeMempool {
		params = append(params, cmd.IncludeMempool)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetTxOutCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 3 || len(r.Params) < 2 {
		return ErrWrongNumberOfParams
	}

	var txid string
	if err := json.Unmarshal(r.Params[0], &txid); err != nil {
		return fmt.Errorf("first parameter 'txid' must be a string: %v", err)
	}

	var output int
	if err := json.Unmarshal(r.Params[1], &output); err != nil {
		return fmt.Errorf("second parameter 'output' must be an integer: %v", err)
	}

	optArgs := make([]bool, 0, 1)
	if len(r.Params) > 2 {
		var mempool bool
		if err := json.Unmarshal(r.Params[2], &mempool); err != nil {
			return fmt.Errorf("third optional parameter 'includemempool' must be a bool: %v", err)
		}
		optArgs = append(optArgs, mempool)
	}

	newCmd, err := NewGetTxOutCmd(r.Id, txid, int(output), optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetTxOutSetInfoCmd is a type handling custom marshaling and
// unmarshaling of gettxoutsetinfo JSON RPC commands.
type GetTxOutSetInfoCmd struct {
	id interface{}
}

// Enforce that GetTxOutSetInfoCmd satisifies the Cmd interface.
var _ Cmd = &GetTxOutSetInfoCmd{}

// NewGetTxOutSetInfoCmd creates a new GetTxOutSetInfoCmd.
func NewGetTxOutSetInfoCmd(id interface{}) (*GetTxOutSetInfoCmd, error) {
	return &GetTxOutSetInfoCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetTxOutSetInfoCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetTxOutSetInfoCmd) Method() string {
	return "gettxoutsetinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetTxOutSetInfoCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetTxOutSetInfoCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewGetTxOutSetInfoCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// GetWorkCmd is a type handling custom marshaling and
// unmarshaling of getwork JSON RPC commands.
type GetWorkCmd struct {
	id   interface{}
	Data string `json:"data,omitempty"`
}

// Enforce that GetWorkCmd satisifies the Cmd interface.
var _ Cmd = &GetWorkCmd{}

// NewGetWorkCmd creates a new GetWorkCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewGetWorkCmd(id interface{}, optArgs ...string) (*GetWorkCmd, error) {
	var data string
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		data = optArgs[0]
	}
	return &GetWorkCmd{
		id:   id,
		Data: data,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetWorkCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetWorkCmd) Method() string {
	return "getwork"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetWorkCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Data != "" {
		params = append(params, cmd.Data)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetWorkCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	var data string
	if len(r.Params) > 0 {
		if err := json.Unmarshal(r.Params[0], &data); err != nil {
			return fmt.Errorf("first optional parameter 'data' must be a string: %v", err)
		}
	}

	newCmd, err := NewGetWorkCmd(r.Id, data)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// HelpCmd is a type handling custom marshaling and
// unmarshaling of help JSON RPC commands.
type HelpCmd struct {
	id      interface{}
	Command string
}

// Enforce that HelpCmd satisifies the Cmd interface.
var _ Cmd = &HelpCmd{}

// NewHelpCmd creates a new HelpCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewHelpCmd(id interface{}, optArgs ...string) (*HelpCmd, error) {

	var command string
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		command = optArgs[0]
	}
	return &HelpCmd{
		id:      id,
		Command: command,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *HelpCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *HelpCmd) Method() string {
	return "help"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *HelpCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Command != "" {
		params = append(params, cmd.Command)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *HelpCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]string, 0, 1)
	if len(r.Params) > 0 {
		var command string
		if err := json.Unmarshal(r.Params[0], &command); err != nil {
			return fmt.Errorf("first optional parameter 'command' must be a string: %v", err)
		}
		optArgs = append(optArgs, command)
	}

	newCmd, err := NewHelpCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ImportPrivKeyCmd is a type handling custom marshaling and
// unmarshaling of importprivkey JSON RPC commands.
type ImportPrivKeyCmd struct {
	id      interface{}
	PrivKey string
	Label   string
	Rescan  bool
}

// Enforce that ImportPrivKeyCmd satisifies the Cmd interface.
var _ Cmd = &ImportPrivKeyCmd{}

// NewImportPrivKeyCmd creates a new ImportPrivKeyCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewImportPrivKeyCmd(id interface{}, privkey string, optArgs ...interface{}) (*ImportPrivKeyCmd, error) {
	var label string
	rescan := true
	var ok bool

	if len(optArgs) > 2 {
		return nil, ErrTooManyOptArgs
	}
	if len(optArgs) > 0 {
		label, ok = optArgs[0].(string)
		if !ok {
			return nil, errors.New("first optional argument label is not a string")
		}
	}
	if len(optArgs) > 1 {
		rescan, ok = optArgs[1].(bool)
		if !ok {
			return nil, errors.New("first optional argument rescan is not a bool")
		}
	}
	return &ImportPrivKeyCmd{
		id:      id,
		PrivKey: privkey,
		Label:   label,
		Rescan:  rescan,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ImportPrivKeyCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ImportPrivKeyCmd) Method() string {
	return "importprivkey"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ImportPrivKeyCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 3)
	params[0] = cmd.PrivKey
	if cmd.Label != "" || !cmd.Rescan {
		params = append(params, cmd.Label)
	}
	if !cmd.Rescan {
		params = append(params, cmd.Rescan)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ImportPrivKeyCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) == 0 || len(r.Params) > 3 {
		return ErrWrongNumberOfParams
	}

	var privkey string
	if err := json.Unmarshal(r.Params[0], &privkey); err != nil {
		return fmt.Errorf("first parameter 'privkey' must be a string: %v", err)
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 1 {
		var label string
		if err := json.Unmarshal(r.Params[1], &label); err != nil {
			return fmt.Errorf("second optional parameter 'label' must be a string: %v", err)
		}
		optArgs = append(optArgs, label)
	}

	if len(r.Params) > 2 {
		var rescan bool
		if err := json.Unmarshal(r.Params[2], &rescan); err != nil {
			return fmt.Errorf("third optional parameter 'rescan' must be a bool: %v", err)
		}
		optArgs = append(optArgs, rescan)
	}

	newCmd, err := NewImportPrivKeyCmd(r.Id, privkey, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ImportWalletCmd is a type handling custom marshaling and
// unmarshaling of importwallet JSON RPC commands.
type ImportWalletCmd struct {
	id       interface{}
	Filename string
}

// Enforce that ImportWalletCmd satisifies the Cmd interface.
var _ Cmd = &ImportWalletCmd{}

// NewImportWalletCmd creates a new ImportWalletCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewImportWalletCmd(id interface{}, filename string) (*ImportWalletCmd, error) {
	return &ImportWalletCmd{
		id:       id,
		Filename: filename,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ImportWalletCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ImportWalletCmd) Method() string {
	return "importwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ImportWalletCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Filename,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ImportWalletCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var filename string
	if err := json.Unmarshal(r.Params[0], &filename); err != nil {
		return fmt.Errorf("first parameter 'filename' must be a string: %v", err)
	}

	newCmd, err := NewImportWalletCmd(r.Id, filename)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// KeyPoolRefillCmd is a type handling custom marshaling and
// unmarshaling of keypoolrefill JSON RPC commands.
type KeyPoolRefillCmd struct {
	id      interface{}
	NewSize uint
}

// Enforce that KeyPoolRefillCmd satisifies the Cmd interface.
var _ Cmd = &KeyPoolRefillCmd{}

// NewKeyPoolRefillCmd creates a new KeyPoolRefillCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewKeyPoolRefillCmd(id interface{}, optArgs ...uint) (*KeyPoolRefillCmd, error) {
	newSize := uint(0)

	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		newSize = optArgs[0]
	}

	return &KeyPoolRefillCmd{
		id:      id,
		NewSize: newSize,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *KeyPoolRefillCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *KeyPoolRefillCmd) Method() string {
	return "keypoolrefill"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *KeyPoolRefillCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.NewSize != 0 {
		params = append(params, cmd.NewSize)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *KeyPoolRefillCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]uint, 0, 1)
	if len(r.Params) > 0 {
		var newsize uint
		if err := json.Unmarshal(r.Params[0], &newsize); err != nil {
			return fmt.Errorf("first optional parameter 'newsize' must be an unsigned integer: %v", err)
		}
		optArgs = append(optArgs, newsize)
	}

	newCmd, err := NewKeyPoolRefillCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListAccountsCmd is a type handling custom marshaling and
// unmarshaling of listaccounts JSON RPC commands.
type ListAccountsCmd struct {
	id      interface{}
	MinConf int
}

// Enforce that ListAccountsCmd satisifies the Cmd interface.
var _ Cmd = &ListAccountsCmd{}

// NewListAccountsCmd creates a new ListAccountsCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListAccountsCmd(id interface{}, optArgs ...int) (*ListAccountsCmd, error) {
	var minconf = 1
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		minconf = optArgs[0]
	}
	return &ListAccountsCmd{
		id:      id,
		MinConf: minconf,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListAccountsCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListAccountsCmd) Method() string {
	return "listaccounts"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAccountsCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.MinConf != 1 {
		params = append(params, cmd.MinConf)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListAccountsCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 1 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) == 1 {
		var minconf int
		if err := json.Unmarshal(r.Params[0], &minconf); err != nil {
			return fmt.Errorf("first optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}

	newCmd, err := NewListAccountsCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListAddressGroupingsCmd is a type handling custom marshaling and
// unmarshaling of listaddressgroupings JSON RPC commands.
type ListAddressGroupingsCmd struct {
	id interface{}
}

// Enforce that ListAddressGroupingsCmd satisifies the Cmd interface.
var _ Cmd = &ListAddressGroupingsCmd{}

// NewListAddressGroupingsCmd creates a new ListAddressGroupingsCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListAddressGroupingsCmd(id interface{}) (*ListAddressGroupingsCmd, error) {
	return &ListAddressGroupingsCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListAddressGroupingsCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListAddressGroupingsCmd) Method() string {
	return "listaddressgroupings"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAddressGroupingsCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListAddressGroupingsCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewListAddressGroupingsCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListLockUnspentCmd is a type handling custom marshaling and
// unmarshaling of listlockunspent JSON RPC commands.
type ListLockUnspentCmd struct {
	id interface{}
}

// Enforce that ListLockUnspentCmd satisifies the Cmd interface.
var _ Cmd = &ListLockUnspentCmd{}

// NewListLockUnspentCmd creates a new ListLockUnspentCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListLockUnspentCmd(id interface{}) (*ListLockUnspentCmd, error) {
	return &ListLockUnspentCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListLockUnspentCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListLockUnspentCmd) Method() string {
	return "listlockunspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListLockUnspentCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListLockUnspentCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewListLockUnspentCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListReceivedByAccountCmd is a type handling custom marshaling and
// unmarshaling of listreceivedbyaccount JSON RPC commands.
type ListReceivedByAccountCmd struct {
	id           interface{}
	MinConf      int
	IncludeEmpty bool
}

// Enforce that ListReceivedByAccountCmd satisifies the Cmd interface.
var _ Cmd = &ListReceivedByAccountCmd{}

// NewListReceivedByAccountCmd creates a new ListReceivedByAccountCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListReceivedByAccountCmd(id interface{}, optArgs ...interface{}) (*ListReceivedByAccountCmd, error) {
	minconf := 1
	includeempty := false

	if len(optArgs) > 2 {
		return nil, ErrWrongNumberOfParams
	}
	if len(optArgs) > 0 {
		m, ok := optArgs[0].(int)
		if !ok {
			return nil, errors.New("first optional argument minconf is not an int")
		}
		minconf = m
	}
	if len(optArgs) > 1 {
		ie, ok := optArgs[1].(bool)
		if !ok {
			return nil, errors.New("second optional argument includeempty is not a bool")
		}

		includeempty = ie
	}
	return &ListReceivedByAccountCmd{
		id:           id,
		MinConf:      minconf,
		IncludeEmpty: includeempty,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListReceivedByAccountCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListReceivedByAccountCmd) Method() string {
	return "listreceivedbyaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListReceivedByAccountCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 2)
	if cmd.MinConf != 1 || cmd.IncludeEmpty != false {
		params = append(params, cmd.MinConf)
	}
	if cmd.IncludeEmpty != false {
		params = append(params, cmd.IncludeEmpty)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListReceivedByAccountCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 0 {
		var minconf int
		if err := json.Unmarshal(r.Params[0], &minconf); err != nil {
			return fmt.Errorf("first optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}
	if len(r.Params) > 1 {
		var includeempty bool
		if err := json.Unmarshal(r.Params[1], &includeempty); err != nil {
			return fmt.Errorf("second optional parameter 'includeempty' must be a bool: %v", err)
		}
		optArgs = append(optArgs, includeempty)
	}

	newCmd, err := NewListReceivedByAccountCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListReceivedByAddressCmd is a type handling custom marshaling and
// unmarshaling of listreceivedbyaddress JSON RPC commands.
type ListReceivedByAddressCmd struct {
	id           interface{}
	MinConf      int
	IncludeEmpty bool
}

// Enforce that ListReceivedByAddressCmd satisifies the Cmd interface.
var _ Cmd = &ListReceivedByAddressCmd{}

// NewListReceivedByAddressCmd creates a new ListReceivedByAddressCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListReceivedByAddressCmd(id interface{}, optArgs ...interface{}) (*ListReceivedByAddressCmd, error) {
	minconf := 1
	includeempty := false

	if len(optArgs) > 2 {
		return nil, ErrWrongNumberOfParams
	}
	if len(optArgs) > 0 {
		m, ok := optArgs[0].(int)
		if !ok {
			return nil, errors.New("first optional argument minconf is not an int")
		}
		minconf = m
	}
	if len(optArgs) > 1 {
		ie, ok := optArgs[1].(bool)
		if !ok {
			return nil, errors.New("second optional argument includeempty is not a bool")
		}

		includeempty = ie
	}
	return &ListReceivedByAddressCmd{
		id:           id,
		MinConf:      minconf,
		IncludeEmpty: includeempty,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListReceivedByAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListReceivedByAddressCmd) Method() string {
	return "listreceivedbyaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListReceivedByAddressCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 2)
	if cmd.MinConf != 1 || cmd.IncludeEmpty != false {
		params = append(params, cmd.MinConf)
	}
	if cmd.IncludeEmpty != false {
		params = append(params, cmd.IncludeEmpty)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListReceivedByAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 0 {
		var minconf int
		if err := json.Unmarshal(r.Params[0], &minconf); err != nil {
			return fmt.Errorf("first optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}
	if len(r.Params) > 1 {
		var includeempty bool
		if err := json.Unmarshal(r.Params[1], &includeempty); err != nil {
			return fmt.Errorf("second optional parameter 'includeempty' must be a bool: %v", err)
		}
		optArgs = append(optArgs, includeempty)
	}

	newCmd, err := NewListReceivedByAddressCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListSinceBlockCmd is a type handling custom marshaling and
// unmarshaling of listsinceblock JSON RPC commands.
type ListSinceBlockCmd struct {
	id                  interface{}
	BlockHash           string
	TargetConfirmations int
}

// Enforce that ListSinceBlockCmd satisifies the Cmd interface.
var _ Cmd = &ListSinceBlockCmd{}

// NewListSinceBlockCmd creates a new ListSinceBlockCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListSinceBlockCmd(id interface{}, optArgs ...interface{}) (*ListSinceBlockCmd, error) {
	blockhash := ""
	targetconfirmations := 1

	if len(optArgs) > 2 {
		return nil, ErrWrongNumberOfParams
	}
	if len(optArgs) > 0 {
		bh, ok := optArgs[0].(string)
		if !ok {
			return nil, errors.New("first optional argument blockhash is not a string")
		}
		blockhash = bh
	}
	if len(optArgs) > 1 {
		tc, ok := optArgs[1].(int)
		if !ok {
			return nil, errors.New("second optional argument targetconfirmations is not an int")
		}

		targetconfirmations = tc
	}
	return &ListSinceBlockCmd{
		id:                  id,
		BlockHash:           blockhash,
		TargetConfirmations: targetconfirmations,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListSinceBlockCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListSinceBlockCmd) Method() string {
	return "listsinceblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListSinceBlockCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 2)
	if cmd.BlockHash != "" || cmd.TargetConfirmations != 1 {
		params = append(params, cmd.BlockHash)
	}
	if cmd.TargetConfirmations != 1 {
		params = append(params, cmd.TargetConfirmations)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListSinceBlockCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 0 {
		var blockhash string
		if err := json.Unmarshal(r.Params[0], &blockhash); err != nil {
			return fmt.Errorf("first optional parameter 'blockhash' must be a string: %v", err)
		}
		optArgs = append(optArgs, blockhash)
	}
	if len(r.Params) > 1 {
		var targetconfirmations int
		if err := json.Unmarshal(r.Params[1], &targetconfirmations); err != nil {
			return fmt.Errorf("second optional parameter 'targetconfirmations' must be an integer: %v", err)
		}
		optArgs = append(optArgs, targetconfirmations)
	}

	newCmd, err := NewListSinceBlockCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListTransactionsCmd is a type handling custom marshaling and
// unmarshaling of listtransactions JSON RPC commands.
type ListTransactionsCmd struct {
	id      interface{}
	Account string
	Count   int
	From    int
}

// Enforce that ListTransactionsCmd satisifies the Cmd interface.
var _ Cmd = &ListTransactionsCmd{}

// NewListTransactionsCmd creates a new ListTransactionsCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListTransactionsCmd(id interface{}, optArgs ...interface{}) (*ListTransactionsCmd, error) {
	account := ""
	count := 10
	from := 0

	if len(optArgs) > 3 {
		return nil, ErrWrongNumberOfParams
	}
	if len(optArgs) > 0 {
		ac, ok := optArgs[0].(string)
		if !ok {
			return nil, errors.New("first optional argument account is not a string")
		}
		account = ac
	}
	if len(optArgs) > 1 {
		cnt, ok := optArgs[1].(int)
		if !ok {
			return nil, errors.New("second optional argument count is not an int")
		}

		count = cnt
	}
	if len(optArgs) > 2 {
		frm, ok := optArgs[2].(int)
		if !ok {
			return nil, errors.New("third optional argument from is not an int")
		}

		from = frm
	}
	return &ListTransactionsCmd{
		id:      id,
		Account: account,
		Count:   count,
		From:    from,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListTransactionsCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListTransactionsCmd) Method() string {
	return "listtransactions"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListTransactionsCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 3)
	if cmd.Account != "" || cmd.Count != 10 || cmd.From != 0 {
		params = append(params, cmd.Account)
	}
	if cmd.Count != 10 || cmd.From != 0 {
		params = append(params, cmd.Count)
	}
	if cmd.From != 0 {
		params = append(params, cmd.From)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListTransactionsCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 3 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]interface{}, 0, 3)
	if len(r.Params) > 0 {
		var account string
		if err := json.Unmarshal(r.Params[0], &account); err != nil {
			return fmt.Errorf("first optional parameter 'account' must be a string: %v", err)
		}
		optArgs = append(optArgs, account)
	}
	if len(r.Params) > 1 {
		var count int
		if err := json.Unmarshal(r.Params[1], &count); err != nil {
			return fmt.Errorf("second optional parameter 'count' must be an integer: %v", err)
		}
		optArgs = append(optArgs, count)
	}
	if len(r.Params) > 2 {
		var from int
		if err := json.Unmarshal(r.Params[2], &from); err != nil {
			return fmt.Errorf("third optional parameter 'from' must be an integer: %v", err)
		}
		optArgs = append(optArgs, from)
	}

	newCmd, err := NewListTransactionsCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ListUnspentCmd is a type handling custom marshaling and
// unmarshaling of listunspent JSON RPC commands.
type ListUnspentCmd struct {
	id        interface{}
	MinConf   int
	MaxConf   int
	Addresses []string
}

// Enforce that ListUnspentCmd satisifies the Cmd interface.
var _ Cmd = &ListUnspentCmd{}

// NewListUnspentCmd creates a new ListUnspentCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewListUnspentCmd(id interface{}, optArgs ...interface{}) (*ListUnspentCmd, error) {
	minconf := 1
	maxconf := 999999
	var addresses []string

	if len(optArgs) > 3 {
		return nil, ErrWrongNumberOfParams
	}
	if len(optArgs) > 0 {
		m, ok := optArgs[0].(int)
		if !ok {
			return nil, errors.New("first optional argument minconf is not an int")
		}
		minconf = m
	}
	if len(optArgs) > 1 {
		m, ok := optArgs[1].(int)
		if !ok {
			return nil, errors.New("second optional argument maxconf is not an int")
		}
		maxconf = m
	}
	if len(optArgs) > 2 {
		a, ok := optArgs[2].([]string)
		if !ok {
			return nil, errors.New("third optional argument addresses is not an array of strings")
		}
		addresses = a
	}
	return &ListUnspentCmd{
		id:        id,
		MinConf:   minconf,
		MaxConf:   maxconf,
		Addresses: addresses,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ListUnspentCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListUnspentCmd) Method() string {
	return "listunspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListUnspentCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 3)
	if cmd.MinConf != 1 || cmd.MaxConf != 99999 || len(cmd.Addresses) != 0 {
		params = append(params, cmd.MinConf)
	}
	if cmd.MaxConf != 99999 || len(cmd.Addresses) != 0 {
		params = append(params, cmd.MaxConf)
	}
	if len(cmd.Addresses) != 0 {
		params = append(params, cmd.Addresses)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListUnspentCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 3 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]interface{}, 0, 3)
	if len(r.Params) > 0 {
		var minconf int
		if err := json.Unmarshal(r.Params[0], &minconf); err != nil {
			return fmt.Errorf("first optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}
	if len(r.Params) > 1 {
		var maxconf int
		if err := json.Unmarshal(r.Params[1], &maxconf); err != nil {
			return fmt.Errorf("second optional parameter 'maxconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, maxconf)
	}
	if len(r.Params) > 2 {
		var addrs []string
		if err := json.Unmarshal(r.Params[2], &addrs); err != nil {
			return fmt.Errorf("third optional parameter 'addresses' must be an array of strings: %v", err)
		}
		optArgs = append(optArgs, addrs)
	}

	newCmd, err := NewListUnspentCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// LockUnspentCmd is a type handling custom marshaling and
// unmarshaling of lockunspent JSON RPC commands.
type LockUnspentCmd struct {
	id           interface{}
	Unlock       bool
	Transactions []TransactionInput
}

// Enforce that LockUnspentCmd satisifies the Cmd interface.
var _ Cmd = &LockUnspentCmd{}

// NewLockUnspentCmd creates a new LockUnspentCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewLockUnspentCmd(id interface{}, unlock bool, optArgs ...[]TransactionInput) (*LockUnspentCmd, error) {
	var transactions []TransactionInput

	if len(optArgs) > 1 {
		return nil, ErrWrongNumberOfParams
	}

	if len(optArgs) > 0 {
		transactions = optArgs[0]
	}

	return &LockUnspentCmd{
		id:           id,
		Unlock:       unlock,
		Transactions: transactions,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *LockUnspentCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *LockUnspentCmd) Method() string {
	return "lockunspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *LockUnspentCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.Unlock
	if len(cmd.Transactions) > 0 {
		params = append(params, cmd.Transactions)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *LockUnspentCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 || len(r.Params) < 1 {
		return ErrWrongNumberOfParams
	}

	var unlock bool
	if err := json.Unmarshal(r.Params[0], &unlock); err != nil {
		return fmt.Errorf("first parameter 'unlock' must be a bool: %v", err)
	}

	optArgs := make([][]TransactionInput, 0, 1)
	if len(r.Params) > 1 {
		var transactions []TransactionInput
		if err := json.Unmarshal(r.Params[1], &transactions); err != nil {
			return fmt.Errorf("second optional parameter 'transactions' "+
				"must be a JSON array of transaction input JSON objects: %v", err)
		}
		optArgs = append(optArgs, transactions)
	}

	newCmd, err := NewLockUnspentCmd(r.Id, unlock, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// MoveCmd is a type handling custom marshaling and
// unmarshaling of move JSON RPC commands.
type MoveCmd struct {
	id          interface{}
	FromAccount string
	ToAccount   string
	Amount      int64
	MinConf     int
	Comment     string
}

// Enforce that MoveCmd satisifies the Cmd interface.
var _ Cmd = &MoveCmd{}

// NewMoveCmd creates a new MoveCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewMoveCmd(id interface{}, fromaccount string, toaccount string, amount int64, optArgs ...interface{}) (*MoveCmd, error) {
	minconf := 1
	comment := ""

	if len(optArgs) > 2 {
		return nil, ErrWrongNumberOfParams
	}

	if len(optArgs) > 0 {
		m, ok := optArgs[0].(int)
		if !ok {
			return nil, errors.New("first optional parameter minconf is not a int64")
		}
		minconf = m
	}
	if len(optArgs) > 1 {
		c, ok := optArgs[1].(string)
		if !ok {
			return nil, errors.New("second optional parameter comment is not a string")
		}
		comment = c
	}

	return &MoveCmd{
		id:          id,
		FromAccount: fromaccount,
		ToAccount:   toaccount,
		Amount:      amount,
		MinConf:     minconf,
		Comment:     comment,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *MoveCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *MoveCmd) Method() string {
	return "move"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *MoveCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 3, 5)
	params[0] = cmd.FromAccount
	params[1] = cmd.ToAccount
	params[2] = float64(cmd.Amount) / 1e8 //convert to BTC
	if cmd.MinConf != 1 || cmd.Comment != "" {
		params = append(params, cmd.MinConf)
	}
	if cmd.Comment != "" {
		params = append(params, cmd.Comment)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *MoveCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 5 || len(r.Params) < 3 {
		return ErrWrongNumberOfParams
	}

	var fromaccount string
	if err := json.Unmarshal(r.Params[0], &fromaccount); err != nil {
		return fmt.Errorf("first parameter 'fromaccount' must be a string: %v", err)
	}

	var toaccount string
	if err := json.Unmarshal(r.Params[1], &toaccount); err != nil {
		return fmt.Errorf("second parameter 'toaccount' must be a string: %v", err)
	}

	var famount float64
	if err := json.Unmarshal(r.Params[2], &famount); err != nil {
		return fmt.Errorf("third parameter 'amount' must be a number: %v", err)
	}
	amount, err := JSONToAmount(famount)
	if err != nil {
		return err
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 3 {
		var minconf int
		if err := json.Unmarshal(r.Params[3], &minconf); err != nil {
			return fmt.Errorf("fourth optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}
	if len(r.Params) > 4 {
		var comment string
		if err := json.Unmarshal(r.Params[4], &comment); err != nil {
			return fmt.Errorf("fifth optional parameter 'comment' must be a string: %v", err)
		}
		optArgs = append(optArgs, comment)
	}

	newCmd, err := NewMoveCmd(r.Id, fromaccount, toaccount, amount,
		optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// PingCmd is a type handling custom marshaling and
// unmarshaling of ping JSON RPC commands.
type PingCmd struct {
	id interface{}
}

// Enforce that PingCmd satisifies the Cmd interface.
var _ Cmd = &PingCmd{}

// NewPingCmd creates a new PingCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewPingCmd(id interface{}) (*PingCmd, error) {
	return &PingCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *PingCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *PingCmd) Method() string {
	return "ping"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *PingCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *PingCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewPingCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SendFromCmd is a type handling custom marshaling and
// unmarshaling of sendfrom JSON RPC commands.
type SendFromCmd struct {
	id          interface{}
	FromAccount string
	ToAddress   string
	Amount      int64
	MinConf     int
	Comment     string
	CommentTo   string
}

// Enforce that SendFromCmd satisifies the Cmd interface.
var _ Cmd = &SendFromCmd{}

// NewSendFromCmd creates a new SendFromCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSendFromCmd(id interface{}, fromaccount string, toaddress string, amount int64, optArgs ...interface{}) (*SendFromCmd, error) {
	minconf := 1
	comment := ""
	commentto := ""

	if len(optArgs) > 3 {
		return nil, ErrWrongNumberOfParams
	}

	if len(optArgs) > 0 {
		m, ok := optArgs[0].(int)
		if !ok {
			return nil, errors.New("first optional parameter minconf is not a int64")
		}
		minconf = m
	}
	if len(optArgs) > 1 {
		c, ok := optArgs[1].(string)
		if !ok {
			return nil, errors.New("second optional parameter comment is not a string")
		}
		comment = c
	}

	if len(optArgs) > 2 {
		cto, ok := optArgs[2].(string)
		if !ok {
			return nil, errors.New("third optional parameter commentto is not a string")
		}
		commentto = cto
	}

	return &SendFromCmd{
		id:          id,
		FromAccount: fromaccount,
		ToAddress:   toaddress,
		Amount:      amount,
		MinConf:     minconf,
		Comment:     comment,
		CommentTo:   commentto,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SendFromCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendFromCmd) Method() string {
	return "sendfrom"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendFromCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 3, 6)
	params[0] = cmd.FromAccount
	params[1] = cmd.ToAddress
	params[2] = float64(cmd.Amount) / 1e8 //convert to BTC
	if cmd.MinConf != 1 || cmd.Comment != "" || cmd.CommentTo != "" {
		params = append(params, cmd.MinConf)
	}
	if cmd.Comment != "" || cmd.CommentTo != "" {
		params = append(params, cmd.Comment)
	}
	if cmd.CommentTo != "" {
		params = append(params, cmd.CommentTo)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SendFromCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 6 || len(r.Params) < 3 {
		return ErrWrongNumberOfParams
	}

	var fromaccount string
	if err := json.Unmarshal(r.Params[0], &fromaccount); err != nil {
		return fmt.Errorf("first parameter 'fromaccount' must be a string: %v", err)
	}

	var toaddress string
	if err := json.Unmarshal(r.Params[1], &toaddress); err != nil {
		return fmt.Errorf("second parameter 'toaddress' must be a string: %v", err)
	}

	var famount float64
	if err := json.Unmarshal(r.Params[2], &famount); err != nil {
		return fmt.Errorf("third parameter 'amount' must be a number: %v", err)
	}
	amount, err := JSONToAmount(famount)
	if err != nil {
		return err
	}

	optArgs := make([]interface{}, 0, 3)
	if len(r.Params) > 3 {
		var minconf int
		if err := json.Unmarshal(r.Params[3], &minconf); err != nil {
			return fmt.Errorf("fourth optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}
	if len(r.Params) > 4 {
		var comment string
		if err := json.Unmarshal(r.Params[4], &comment); err != nil {
			return fmt.Errorf("fifth optional parameter 'comment' must be a string: %v", err)
		}
		optArgs = append(optArgs, comment)
	}
	if len(r.Params) > 5 {
		var commentto string
		if err := json.Unmarshal(r.Params[5], &commentto); err != nil {
			return fmt.Errorf("sixth optional parameter 'commentto' must be a string: %v", err)
		}
		optArgs = append(optArgs, commentto)
	}

	newCmd, err := NewSendFromCmd(r.Id, fromaccount, toaddress, amount,
		optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SendManyCmd is a type handling custom marshaling and
// unmarshaling of sendmany JSON RPC commands.
type SendManyCmd struct {
	id          interface{}
	FromAccount string
	Amounts     map[string]int64
	MinConf     int
	Comment     string
}

// Enforce that SendManyCmd satisifies the Cmd interface.
var _ Cmd = &SendManyCmd{}

// NewSendManyCmd creates a new SendManyCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSendManyCmd(id interface{}, fromaccount string, amounts map[string]int64, optArgs ...interface{}) (*SendManyCmd, error) {
	minconf := 1
	comment := ""

	if len(optArgs) > 2 {
		return nil, ErrWrongNumberOfParams
	}

	if len(optArgs) > 0 {
		m, ok := optArgs[0].(int)
		if !ok {
			return nil, errors.New("first optional parameter minconf is not a int64")
		}
		minconf = m
	}
	if len(optArgs) > 1 {
		c, ok := optArgs[1].(string)
		if !ok {
			return nil, errors.New("second optional parameter comment is not a string")
		}
		comment = c
	}

	return &SendManyCmd{
		id:          id,
		FromAccount: fromaccount,
		Amounts:     amounts,
		MinConf:     minconf,
		Comment:     comment,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SendManyCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendManyCmd) Method() string {
	return "sendmany"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendManyCmd) MarshalJSON() ([]byte, error) {
	floatAmounts := make(map[string]float64, len(cmd.Amounts))
	for k, v := range cmd.Amounts {
		floatAmounts[k] = float64(v) / 1e8
	}

	params := make([]interface{}, 2, 4)
	params[0] = cmd.FromAccount
	params[1] = floatAmounts
	if cmd.MinConf != 1 || cmd.Comment != "" {
		params = append(params, cmd.MinConf)
	}
	if cmd.Comment != "" {
		params = append(params, cmd.Comment)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SendManyCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 4 || len(r.Params) < 2 {
		return ErrWrongNumberOfParams
	}

	var fromaccount string
	if err := json.Unmarshal(r.Params[0], &fromaccount); err != nil {
		return fmt.Errorf("first parameter 'fromaccount' must be a string: %v", err)
	}

	var famounts map[string]float64
	if err := json.Unmarshal(r.Params[1], &famounts); err != nil {
		return fmt.Errorf("second parameter 'amounts' must be a JSON object of address to amount mappings: %v", err)
	}
	amounts := make(map[string]int64, len(famounts))
	for k, v := range famounts {
		amount, err := JSONToAmount(v)
		if err != nil {
			return err
		}
		amounts[k] = amount
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 2 {
		var minconf int
		if err := json.Unmarshal(r.Params[2], &minconf); err != nil {
			return fmt.Errorf("third optional parameter 'minconf' must be an integer: %v", err)
		}
		optArgs = append(optArgs, minconf)
	}
	if len(r.Params) > 3 {
		var comment string
		if err := json.Unmarshal(r.Params[3], &comment); err != nil {
			return fmt.Errorf("fourth optional parameter 'comment' must be a string: %v", err)
		}
		optArgs = append(optArgs, comment)
	}

	newCmd, err := NewSendManyCmd(r.Id, fromaccount, amounts, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SendRawTransactionCmd is a type handling custom marshaling and
// unmarshaling of sendrawtransaction JSON RPC commands.
type SendRawTransactionCmd struct {
	id            interface{}
	HexTx         string
	AllowHighFees bool
}

// Enforce that SendRawTransactionCmd satisifies the Cmd interface.
var _ Cmd = &SendRawTransactionCmd{}

// NewSendRawTransactionCmd creates a new SendRawTransactionCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSendRawTransactionCmd(id interface{}, hextx string, optArgs ...bool) (*SendRawTransactionCmd, error) {
	allowHighFees := false
	if len(optArgs) > 1 {
		return nil, ErrTooManyOptArgs
	}
	if len(optArgs) == 1 {
		allowHighFees = optArgs[0]
	}

	return &SendRawTransactionCmd{
		id:            id,
		HexTx:         hextx,
		AllowHighFees: allowHighFees,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SendRawTransactionCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendRawTransactionCmd) Method() string {
	return "sendrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendRawTransactionCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.HexTx
	if cmd.AllowHighFees {
		params = append(params, cmd.AllowHighFees)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SendRawTransactionCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 || len(r.Params) < 1 {
		return ErrWrongNumberOfParams
	}

	var hextx string
	if err := json.Unmarshal(r.Params[0], &hextx); err != nil {
		return fmt.Errorf("first parameter 'hextx' must be a string: %v", err)
	}

	optArgs := make([]bool, 0, 1)
	if len(r.Params) > 1 {
		var allowHighFees bool
		if err := json.Unmarshal(r.Params[1], &allowHighFees); err != nil {
			return fmt.Errorf("second optional parameter 'allowhighfees' must be a bool: %v", err)
		}
		optArgs = append(optArgs, allowHighFees)
	}

	newCmd, err := NewSendRawTransactionCmd(r.Id, hextx, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SendToAddressCmd is a type handling custom marshaling and
// unmarshaling of sendtoaddress JSON RPC commands.
type SendToAddressCmd struct {
	id        interface{}
	Address   string
	Amount    int64
	Comment   string
	CommentTo string
}

// Enforce that SendToAddressCmd satisifies the Cmd interface.
var _ Cmd = &SendToAddressCmd{}

// NewSendToAddressCmd creates a new SendToAddressCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSendToAddressCmd(id interface{}, address string, amount int64, optArgs ...interface{}) (*SendToAddressCmd, error) {
	comment := ""
	commentto := ""

	if len(optArgs) > 2 {
		return nil, ErrWrongNumberOfParams
	}

	if len(optArgs) > 0 {
		c, ok := optArgs[0].(string)
		if !ok {
			return nil, errors.New("first optional parameter comment is not a string")
		}
		comment = c
	}
	if len(optArgs) > 1 {
		cto, ok := optArgs[1].(string)
		if !ok {
			return nil, errors.New("second optional parameter commentto is not a string")
		}
		commentto = cto
	}

	return &SendToAddressCmd{
		id:        id,
		Address:   address,
		Amount:    amount,
		Comment:   comment,
		CommentTo: commentto,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SendToAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendToAddressCmd) Method() string {
	return "sendtoaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendToAddressCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 2, 4)
	params[0] = cmd.Address
	params[1] = float64(cmd.Amount) / 1e8 //convert to BTC
	if cmd.Comment != "" || cmd.CommentTo != "" {
		params = append(params, cmd.Comment)
	}
	if cmd.CommentTo != "" {
		params = append(params, cmd.CommentTo)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SendToAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 4 || len(r.Params) < 2 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	var famount float64
	if err := json.Unmarshal(r.Params[1], &famount); err != nil {
		return fmt.Errorf("second parameter 'amount' must be a number: %v", err)
	}
	amount, err := JSONToAmount(famount)
	if err != nil {
		return err
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 2 {
		var comment string
		if err := json.Unmarshal(r.Params[2], &comment); err != nil {
			return fmt.Errorf("third optional parameter 'comment' must be a string: %v", err)
		}
		optArgs = append(optArgs, comment)
	}
	if len(r.Params) > 3 {
		var commentto string
		if err := json.Unmarshal(r.Params[3], &commentto); err != nil {
			return fmt.Errorf("fourth optional parameter 'commentto' must be a string: %v", err)
		}
		optArgs = append(optArgs, commentto)
	}

	newCmd, err := NewSendToAddressCmd(r.Id, address, amount, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SetAccountCmd is a type handling custom marshaling and
// unmarshaling of setaccount JSON RPC commands.
type SetAccountCmd struct {
	id      interface{}
	Address string
	Account string
}

// Enforce that SetAccountCmd satisifies the Cmd interface.
var _ Cmd = &SetAccountCmd{}

// NewSetAccountCmd creates a new SetAccountCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSetAccountCmd(id interface{}, address string, account string) (*SetAccountCmd, error) {

	return &SetAccountCmd{
		id:      id,
		Address: address,
		Account: account,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SetAccountCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SetAccountCmd) Method() string {
	return "setaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SetAccountCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Address,
		cmd.Account,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SetAccountCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 2 || len(r.Params) < 2 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	var account string
	if err := json.Unmarshal(r.Params[1], &account); err != nil {
		return fmt.Errorf("second parameter 'account' must be a string: %v", err)
	}

	newCmd, err := NewSetAccountCmd(r.Id, address, account)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SetGenerateCmd is a type handling custom marshaling and
// unmarshaling of setgenerate JSON RPC commands.
type SetGenerateCmd struct {
	id           interface{}
	Generate     bool
	GenProcLimit int
}

// Enforce that SetGenerateCmd satisifies the Cmd interface.
var _ Cmd = &SetGenerateCmd{}

// NewSetGenerateCmd creates a new SetGenerateCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSetGenerateCmd(id interface{}, generate bool, optArgs ...int) (*SetGenerateCmd, error) {
	genproclimit := -1
	if len(optArgs) > 1 {
		return nil, ErrTooManyOptArgs
	}
	if len(optArgs) == 1 {
		genproclimit = optArgs[0]
	}

	return &SetGenerateCmd{
		id:           id,
		Generate:     generate,
		GenProcLimit: genproclimit,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SetGenerateCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SetGenerateCmd) Method() string {
	return "setgenerate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SetGenerateCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.Generate
	if cmd.GenProcLimit != -1 {
		params = append(params, cmd.GenProcLimit)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SetGenerateCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 || len(r.Params) < 1 {
		return ErrWrongNumberOfParams
	}

	var generate bool
	if err := json.Unmarshal(r.Params[0], &generate); err != nil {
		return fmt.Errorf("first parameter 'generate' must be a bool: %v", err)
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) > 1 {
		var genproclimit int
		if err := json.Unmarshal(r.Params[1], &genproclimit); err != nil {
			return fmt.Errorf("second optional parameter 'genproclimit' must be an integer: %v", err)
		}
		optArgs = append(optArgs, genproclimit)
	}

	newCmd, err := NewSetGenerateCmd(r.Id, generate, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SetTxFeeCmd is a type handling custom marshaling and
// unmarshaling of settxfee JSON RPC commands.
type SetTxFeeCmd struct {
	id     interface{}
	Amount int64
}

// Enforce that SetTxFeeCmd satisifies the Cmd interface.
var _ Cmd = &SetTxFeeCmd{}

// NewSetTxFeeCmd creates a new SetTxFeeCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSetTxFeeCmd(id interface{}, amount int64) (*SetTxFeeCmd, error) {
	return &SetTxFeeCmd{
		id:     id,
		Amount: amount,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SetTxFeeCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SetTxFeeCmd) Method() string {
	return "settxfee"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SetTxFeeCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		float64(cmd.Amount) / 1e8, //convert to BTC
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SetTxFeeCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var famount float64
	if err := json.Unmarshal(r.Params[0], &famount); err != nil {
		return fmt.Errorf("first parameter 'amount' must be a number: %v", err)
	}
	amount, err := JSONToAmount(famount)
	if err != nil {
		return err
	}

	newCmd, err := NewSetTxFeeCmd(r.Id, amount)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SignMessageCmd is a type handling custom marshaling and
// unmarshaling of signmessage JSON RPC commands.
type SignMessageCmd struct {
	id      interface{}
	Address string
	Message string
}

// Enforce that SignMessageCmd satisifies the Cmd interface.
var _ Cmd = &SignMessageCmd{}

// NewSignMessageCmd creates a new SignMessageCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSignMessageCmd(id interface{}, address string, message string) (*SignMessageCmd, error) {
	return &SignMessageCmd{
		id:      id,
		Address: address,
		Message: message,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SignMessageCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SignMessageCmd) Method() string {
	return "signmessage"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SignMessageCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Address,
		cmd.Message,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SignMessageCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 2 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	var message string
	if err := json.Unmarshal(r.Params[1], &message); err != nil {
		return fmt.Errorf("second parameter 'message' must be a string: %v", err)
	}

	newCmd, err := NewSignMessageCmd(r.Id, address, message)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// RawTxInput models the data needed for a raw tx input.
type RawTxInput struct {
	Txid         string `json:"txid"`
	Vout         uint32 `json:"vout"`
	ScriptPubKey string `json:"scriptPubKey"`
	RedeemScript string `json:"redeemScript"`
}

// SignRawTransactionCmd is a type handling custom marshaling and
// unmarshaling of signrawtransaction JSON RPC commands.
type SignRawTransactionCmd struct {
	id       interface{}
	RawTx    string
	Inputs   []RawTxInput
	PrivKeys []string
	Flags    string
}

// Enforce that SignRawTransactionCmd satisifies the Cmd interface.
var _ Cmd = &SignRawTransactionCmd{}

// NewSignRawTransactionCmd creates a new SignRawTransactionCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSignRawTransactionCmd(id interface{}, rawTx string, optArgs ...interface{}) (*SignRawTransactionCmd, error) {
	var inputs []RawTxInput
	var privkeys []string
	var flags string
	if len(optArgs) > 3 {
		return nil, ErrTooManyOptArgs
	}
	if len(optArgs) > 0 {
		ip, ok := optArgs[0].([]RawTxInput)
		if !ok {
			return nil, errors.New("first optional parameter inputs should be an array of RawTxInput")
		}

		inputs = ip
	}
	if len(optArgs) > 1 {
		pk, ok := optArgs[1].([]string)
		if !ok {
			return nil, errors.New("second optional parameter inputs should be an array of string")
		}

		privkeys = pk
	}
	if len(optArgs) > 2 {
		fl, ok := optArgs[2].(string)
		if !ok {
			return nil, errors.New("third optional parameter flags should be a string")
		}

		flags = fl
	}
	return &SignRawTransactionCmd{
		id:       id,
		RawTx:    rawTx,
		Inputs:   inputs,
		PrivKeys: privkeys,
		Flags:    flags,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SignRawTransactionCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SignRawTransactionCmd) Method() string {
	return "signrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SignRawTransactionCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 4)
	params[0] = cmd.RawTx
	if len(cmd.Inputs) > 0 || len(cmd.PrivKeys) > 0 || cmd.Flags != "" {
		params = append(params, cmd.Inputs)
	}
	if len(cmd.PrivKeys) > 0 || cmd.Flags != "" {
		params = append(params, cmd.PrivKeys)
	}
	if cmd.Flags != "" {
		params = append(params, cmd.Flags)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SignRawTransactionCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 4 || len(r.Params) < 1 {
		return ErrWrongNumberOfParams
	}

	var rawtx string
	if err := json.Unmarshal(r.Params[0], &rawtx); err != nil {
		return fmt.Errorf("first parameter 'rawtx' must be a string: %v", err)
	}

	optArgs := make([]interface{}, 0, 3)
	if len(r.Params) > 1 {
		var inputs []RawTxInput
		if err := json.Unmarshal(r.Params[1], &inputs); err != nil {
			return fmt.Errorf("second optional parameter 'inputs' "+
				"must be a JSON array of raw transaction input JSON objects: %v", err)
		}
		optArgs = append(optArgs, inputs)
	}

	if len(r.Params) > 2 {
		var privkeys []string
		if err := json.Unmarshal(r.Params[2], &privkeys); err != nil {
			return fmt.Errorf("third optional parameter 'privkeys' must be an array of strings: %v", err)
		}
		optArgs = append(optArgs, privkeys)
	}
	if len(r.Params) > 3 {
		var flags string
		if err := json.Unmarshal(r.Params[3], &flags); err != nil {
			return fmt.Errorf("fourth optional parameter 'flags' must be a string: %v", err)
		}
		optArgs = append(optArgs, flags)
	}

	newCmd, err := NewSignRawTransactionCmd(r.Id, rawtx, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// StopCmd is a type handling custom marshaling and
// unmarshaling of stop JSON RPC commands.
type StopCmd struct {
	id interface{}
}

// Enforce that StopCmd satisifies the Cmd interface.
var _ Cmd = &StopCmd{}

// NewStopCmd creates a new StopCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewStopCmd(id interface{}) (*StopCmd, error) {

	return &StopCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *StopCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *StopCmd) Method() string {
	return "stop"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *StopCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *StopCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewStopCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// SubmitBlockOptions represents the optional options struct provided with
// a SubmitBlockCmd command.
type SubmitBlockOptions struct {
	// must be provided if server provided a workid with template.
	WorkID string `json:"workid,omitempty"`
}

// SubmitBlockCmd is a type handling custom marshaling and
// unmarshaling of submitblock JSON RPC commands.
type SubmitBlockCmd struct {
	id       interface{}
	HexBlock string
	Options  *SubmitBlockOptions
}

// Enforce that SubmitBlockCmd satisifies the Cmd interface.
var _ Cmd = &SubmitBlockCmd{}

// NewSubmitBlockCmd creates a new SubmitBlockCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSubmitBlockCmd(id interface{}, hexblock string, optArgs ...*SubmitBlockOptions) (*SubmitBlockCmd, error) {
	var options *SubmitBlockOptions
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		options = optArgs[0]
	}

	return &SubmitBlockCmd{
		id:       id,
		HexBlock: hexblock,
		Options:  options,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *SubmitBlockCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SubmitBlockCmd) Method() string {
	return "submitblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SubmitBlockCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.HexBlock
	if cmd.Options != nil {
		params = append(params, cmd.Options)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *SubmitBlockCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 || len(r.Params) < 1 {
		return ErrWrongNumberOfParams
	}

	var hexblock string
	if err := json.Unmarshal(r.Params[0], &hexblock); err != nil {
		return fmt.Errorf("first parameter 'hexblock' must be a string: %v", err)
	}

	optArgs := make([]*SubmitBlockOptions, 0, 1)
	if len(r.Params) == 2 {
		var options SubmitBlockOptions
		if err := json.Unmarshal(r.Params[1], &options); err != nil {
			return fmt.Errorf("second optional parameter 'options' must "+
				"be a JSON object of submit block options: %v", err)
		}
		optArgs = append(optArgs, &options)
	}

	newCmd, err := NewSubmitBlockCmd(r.Id, hexblock, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// ValidateAddressCmd is a type handling custom marshaling and
// unmarshaling of validateaddress JSON RPC commands.
type ValidateAddressCmd struct {
	id      interface{}
	Address string
}

// Enforce that ValidateAddressCmd satisifies the Cmd interface.
var _ Cmd = &ValidateAddressCmd{}

// NewValidateAddressCmd creates a new ValidateAddressCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewValidateAddressCmd(id interface{}, address string) (*ValidateAddressCmd, error) {

	return &ValidateAddressCmd{
		id:      id,
		Address: address,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *ValidateAddressCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ValidateAddressCmd) Method() string {
	return "validateaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ValidateAddressCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Address,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ValidateAddressCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 1 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	newCmd, err := NewValidateAddressCmd(r.Id, address)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// VerifyChainCmd is a type handling custom marshaling and
// unmarshaling of verifychain JSON RPC commands.
type VerifyChainCmd struct {
	id         interface{}
	CheckLevel int32
	CheckDepth int32
}

// Enforce that VerifyChainCmd satisifies the Cmd interface.
var _ Cmd = &VerifyChainCmd{}

// NewVerifyChainCmd creates a new VerifyChainCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewVerifyChainCmd(id interface{}, optArgs ...int32) (*VerifyChainCmd, error) {
	// bitcoind default, but they do vary it based on cli args.
	var checklevel int32 = 3
	var checkdepth int32 = 288

	if len(optArgs) > 0 {
		if len(optArgs) > 2 {
			return nil, ErrTooManyOptArgs
		}
		checklevel = optArgs[0]

		if len(optArgs) > 1 {
			checkdepth = optArgs[1]
		}
	}

	return &VerifyChainCmd{
		id:         id,
		CheckLevel: checklevel,
		CheckDepth: checkdepth,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *VerifyChainCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *VerifyChainCmd) Method() string {
	return "verifychain"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *VerifyChainCmd) MarshalJSON() ([]byte, error) {
	// XXX(oga) magic numbers
	params := make([]interface{}, 0, 2)
	if cmd.CheckLevel != 3 || cmd.CheckDepth != 288 {
		params = append(params, cmd.CheckLevel)
	}
	if cmd.CheckDepth != 288 {
		params = append(params, cmd.CheckDepth)
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *VerifyChainCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) > 2 {
		return ErrWrongNumberOfParams
	}

	optArgs := make([]int32, 0, 2)
	if len(r.Params) > 0 {
		var checklevel int32
		if err := json.Unmarshal(r.Params[0], &checklevel); err != nil {
			return fmt.Errorf("first optional parameter 'checklevel' must be a 32-bit integer: %v", err)
		}
		optArgs = append(optArgs, checklevel)
	}

	if len(r.Params) > 1 {
		var checkdepth int32
		if err := json.Unmarshal(r.Params[1], &checkdepth); err != nil {
			return fmt.Errorf("second optional parameter 'checkdepth' must be a 32-bit integer: %v", err)
		}
		optArgs = append(optArgs, checkdepth)
	}

	newCmd, err := NewVerifyChainCmd(r.Id, optArgs...)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// VerifyMessageCmd is a type handling custom marshaling and
// unmarshaling of verifymessage JSON RPC commands.
type VerifyMessageCmd struct {
	id        interface{}
	Address   string
	Signature string
	Message   string
}

// Enforce that VerifyMessageCmd satisifies the Cmd interface.
var _ Cmd = &VerifyMessageCmd{}

// NewVerifyMessageCmd creates a new VerifyMessageCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewVerifyMessageCmd(id interface{}, address string, signature string,
	message string) (*VerifyMessageCmd, error) {

	return &VerifyMessageCmd{
		id:        id,
		Address:   address,
		Signature: signature,
		Message:   message,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *VerifyMessageCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *VerifyMessageCmd) Method() string {
	return "verifymessage"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *VerifyMessageCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Address,
		cmd.Signature,
		cmd.Message,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *VerifyMessageCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 3 {
		return ErrWrongNumberOfParams
	}

	var address string
	if err := json.Unmarshal(r.Params[0], &address); err != nil {
		return fmt.Errorf("first parameter 'address' must be a string: %v", err)
	}

	var signature string
	if err := json.Unmarshal(r.Params[1], &signature); err != nil {
		return fmt.Errorf("second parameter 'signature' must be a string: %v", err)
	}

	var message string
	if err := json.Unmarshal(r.Params[2], &message); err != nil {
		return fmt.Errorf("third parameter 'message' must be a string: %v", err)
	}

	newCmd, err := NewVerifyMessageCmd(r.Id, address, signature, message)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// WalletLockCmd is a type handling custom marshaling and
// unmarshaling of walletlock JSON RPC commands.
type WalletLockCmd struct {
	id interface{}
}

// Enforce that WalletLockCmd satisifies the Cmd interface.
var _ Cmd = &WalletLockCmd{}

// NewWalletLockCmd creates a new WalletLockCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewWalletLockCmd(id interface{}) (*WalletLockCmd, error) {

	return &WalletLockCmd{
		id: id,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *WalletLockCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *WalletLockCmd) Method() string {
	return "walletlock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletLockCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *WalletLockCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 0 {
		return ErrWrongNumberOfParams
	}

	newCmd, err := NewWalletLockCmd(r.Id)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// WalletPassphraseCmd is a type handling custom marshaling and
// unmarshaling of walletpassphrase JSON RPC commands.
type WalletPassphraseCmd struct {
	id         interface{}
	Passphrase string
	Timeout    int64
}

// Enforce that WalletPassphraseCmd satisifies the Cmd interface.
var _ Cmd = &WalletPassphraseCmd{}

// NewWalletPassphraseCmd creates a new WalletPassphraseCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewWalletPassphraseCmd(id interface{}, passphrase string, timeout int64) (*WalletPassphraseCmd, error) {

	return &WalletPassphraseCmd{
		id:         id,
		Passphrase: passphrase,
		Timeout:    timeout,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *WalletPassphraseCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *WalletPassphraseCmd) Method() string {
	return "walletpassphrase"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletPassphraseCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Passphrase,
		cmd.Timeout,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *WalletPassphraseCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 2 {
		return ErrWrongNumberOfParams
	}

	var passphrase string
	if err := json.Unmarshal(r.Params[0], &passphrase); err != nil {
		return fmt.Errorf("first parameter 'passphrase' must be a string: %v", err)
	}

	var timeout int64
	if err := json.Unmarshal(r.Params[1], &timeout); err != nil {
		return fmt.Errorf("second parameter 'timeout' must be an integer: %v", err)
	}

	newCmd, err := NewWalletPassphraseCmd(r.Id, passphrase, timeout)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}

// WalletPassphraseChangeCmd is a type handling custom marshaling and
// unmarshaling of walletpassphrasechange JSON RPC commands.
type WalletPassphraseChangeCmd struct {
	id            interface{}
	OldPassphrase string
	NewPassphrase string
}

// Enforce that WalletPassphraseChangeCmd satisifies the Cmd interface.
var _ Cmd = &WalletPassphraseChangeCmd{}

// NewWalletPassphraseChangeCmd creates a new WalletPassphraseChangeCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewWalletPassphraseChangeCmd(id interface{}, oldpassphrase, newpassphrase string) (*WalletPassphraseChangeCmd, error) {

	return &WalletPassphraseChangeCmd{
		id:            id,
		OldPassphrase: oldpassphrase,
		NewPassphrase: newpassphrase,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *WalletPassphraseChangeCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *WalletPassphraseChangeCmd) Method() string {
	return "walletpassphrasechange"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletPassphraseChangeCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.OldPassphrase,
		cmd.NewPassphrase,
	}

	// Fill and marshal a RawCmd.
	raw, err := NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *WalletPassphraseChangeCmd) UnmarshalJSON(b []byte) error {
	// Unmashal into a RawCmd
	var r RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	if len(r.Params) != 2 {
		return ErrWrongNumberOfParams
	}

	var oldpassphrase string
	if err := json.Unmarshal(r.Params[0], &oldpassphrase); err != nil {
		return fmt.Errorf("first parameter 'oldpassphrase' must be a string: %v", err)
	}

	var newpassphrase string
	if err := json.Unmarshal(r.Params[1], &newpassphrase); err != nil {
		return fmt.Errorf("second parameter 'newpassphrase' must be a string: %v", err)
	}

	newCmd, err := NewWalletPassphraseChangeCmd(r.Id, oldpassphrase, newpassphrase)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}
