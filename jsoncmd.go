// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"errors"
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
	SetId(interface{})
	Method() string
}

// RawCmd is a type for unmarshaling raw commands into before the
// custom command type is set.  Other packages may register their
// own RawCmd to Cmd converters by calling RegisterCustomCmd.
type RawCmd struct {
	Jsonrpc string        `json:"jsonrpc"`
	Id      interface{}   `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// RawCmdParser is a function to create a custom Cmd from a RawCmd.
type RawCmdParser func(*RawCmd) (Cmd, error)

type cmd struct {
	parser     RawCmdParser
	helpString string
}

var customCmds = make(map[string]cmd)

// RegisterCustomCmd registers a custom RawCmd parsing func for a
// non-standard Bitcoin command.
func RegisterCustomCmd(method string, parser RawCmdParser, helpString string) {
	customCmds[method] = cmd{parser: parser, helpString: helpString}
}

// CmdGenerator is a function that returns a new concerete Cmd of
// the appropriate type for a non-standard Bitcoin command.
type CmdGenerator func() Cmd

var customCmdGenerators = make(map[string]CmdGenerator)

// RegisterCustomCmdGenerator registers a custom CmdGenerator func for
// a non-standard Bitcoin command.
func RegisterCustomCmdGenerator(method string, generator CmdGenerator) {
	customCmdGenerators[method] = generator
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
			return c.parser(&r)
		}

		if g, ok := customCmdGenerators[r.Method]; ok {
			cmd = g()
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *unparsableCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *unparsableCmd) Method() string {
	return cmd.method
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *unparsableCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  cmd.method,
		Id:      cmd.id,
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *AddMultisigAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *AddMultisigAddressCmd) Method() string {
	return "addmultisigaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *AddMultisigAddressCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "addmultisigaddress",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.NRequired,
			cmd.Keys,
		},
	}

	if cmd.Account != "" {
		raw.Params = append(raw.Params, cmd.Account)
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
	nRequired, ok := r.Params[0].(float64)
	if !ok {
		return errors.New("first parameter nrequired must be a number")
	}
	ikeys, ok := r.Params[1].([]interface{})
	if !ok {
		return errors.New("second parameter keys must be an array")
	}
	keys := make([]string, len(ikeys))
	for i, val := range ikeys {
		keys[i], ok = val.(string)
		if !ok {
			return errors.New("second parameter keys must be an array of strings")
		}
	}
	var account string
	if len(r.Params) > 2 {
		account, ok = r.Params[2].(string)
		if !ok {
			return errors.New("third (optional) parameter account must be a string")
		}
	}
	newCmd, err := NewAddMultisigAddressCmd(r.Id, int(nRequired), keys,
		account)
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
		return nil, errors.New("Invalid subcommand for addnode")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *AddNodeCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *AddNodeCmd) Method() string {
	return "addnode"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *AddNodeCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "addnode",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Addr,
			cmd.SubCmd,
		},
	})
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
	addr, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter addr must be a string")
	}
	subcmd, ok := r.Params[1].(string)
	if !ok {
		return errors.New("second parameter subcmd must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *BackupWalletCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *BackupWalletCmd) Method() string {
	return "backupwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *BackupWalletCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "backupwallet",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Destination,
		},
	})
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
	destination, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter desitnation must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *CreateMultisigCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *CreateMultisigCmd) Method() string {
	return "createmultisig"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *CreateMultisigCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "createmultisig",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.NRequired,
			cmd.Keys,
		},
	})
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
	nRequired, ok := r.Params[0].(float64)
	if !ok {
		return errors.New("first parameter nrequired must be a number")
	}

	ikeys, ok := r.Params[1].([]interface{})
	if !ok {
		return errors.New("second parameter keys must be an array")
	}
	keys := make([]string, len(ikeys))
	for i, val := range ikeys {
		keys[i], ok = val.(string)
		if !ok {
			return errors.New("second parameter keys must be an array of strings")
		}
	}

	newCmd, err := NewCreateMultisigCmd(r.Id, int(nRequired), keys)
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
	Vout int    `json:"vout"`
}

// ConvertCreateRawTxParams validates and converts the passed parameters from
// raw interfaces into concrete structs.  This is a separate function since
// the createrawtransaction command parameters are caller-crafted JSON as
// opposed to machine generated JSON as is the case for most commands.
func ConvertCreateRawTxParams(inputs, amounts interface{}) ([]TransactionInput, map[string]int64, error) {
	iinputs, ok := inputs.([]interface{})
	if !ok {
		return nil, nil, errors.New("first parameter inputs must be an array")
	}
	rinputs := make([]TransactionInput, len(iinputs))
	for i, iv := range iinputs {
		v, ok := iv.(map[string]interface{})
		if !ok {
			return nil, nil, errors.New("first parameter inputs must be an array of objects")
		}

		if len(v) != 2 {
			return nil, nil, errors.New("input with wrong number of members")
		}
		txid, ok := v["txid"]
		if !ok {
			return nil, nil, errors.New("input without txid")
		}
		rinputs[i].Txid, ok = txid.(string)
		if !ok {
			return nil, nil, errors.New("input txid isn't a string")
		}

		vout, ok := v["vout"]
		if !ok {
			return nil, nil, errors.New("input without vout")
		}
		fvout, ok := vout.(float64)
		if !ok {
			return nil, nil, errors.New("input vout not a number")
		}
		rinputs[i].Vout = int(fvout)
	}
	famounts, ok := amounts.(map[string]interface{})
	if !ok {
		return nil, nil, errors.New("second parameter keys must be a map")
	}

	ramounts := make(map[string]int64)
	for k, v := range famounts {
		fv, ok := v.(float64)
		if !ok {
			return nil, nil, errors.New("second parameter keys must be a number map")
		}
		var err error
		ramounts[k], err = JSONToAmount(fv)
		if err != nil {
			return nil, nil, err
		}
	}

	return rinputs, ramounts, nil
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *CreateRawTransactionCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *CreateRawTransactionCmd) Method() string {
	return "createrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *CreateRawTransactionCmd) MarshalJSON() ([]byte, error) {
	floatAmount := make(map[string]float64)

	for k, v := range cmd.Amounts {
		floatAmount[k] = float64(v) / 1e8
	}
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "createrawtransaction",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Inputs,
			floatAmount,
		},
	})
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

	inputs, amounts, err := ConvertCreateRawTxParams(r.Params[0],
		r.Params[1])
	if err != nil {
		return err
	}

	newCmd, err := NewCreateRawTransactionCmd(r.Id, inputs, amounts)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *DebugLevelCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DebugLevelCmd) Method() string {
	return "debuglevel"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DebugLevelCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  cmd.Method(),
		Id:      cmd.id,
		Params: []interface{}{
			cmd.LevelSpec,
		},
	})
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
	levelSpec, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter levelspec must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *DecodeRawTransactionCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DecodeRawTransactionCmd) Method() string {
	return "decoderawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DecodeRawTransactionCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "decoderawtransaction",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.HexTx,
		},
	})
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
	hextx, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter hextx must be an array of objects")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *DecodeScriptCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DecodeScriptCmd) Method() string {
	return "decodescript"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DecodeScriptCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "decodescript",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.HexScript,
		},
	})
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
	hexscript, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter hexscript must be an array of objects")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *DumpPrivKeyCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DumpPrivKeyCmd) Method() string {
	return "dumpprivkey"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DumpPrivKeyCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "dumpprivkey",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
		},
	})
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
	address, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter address must be an array of objects")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *DumpWalletCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *DumpWalletCmd) Method() string {
	return "dumpwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *DumpWalletCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "dumpwallet",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Filename,
		},
	})
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
	filename, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter filename must be an array of objects")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *EncryptWalletCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *EncryptWalletCmd) Method() string {
	return "encryptwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *EncryptWalletCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "encryptwallet",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Passphrase,
		},
	})
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
	passphrase, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter passphrase must be a string")
	}

	newCmd, err := NewEncryptWalletCmd(r.Id, passphrase)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetAccountCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAccountCmd) Method() string {
	return "getaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAccountCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getaccount",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
		},
	})
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
	address, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter address must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetAccountAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAccountAddressCmd) Method() string {
	return "getaccountaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAccountAddressCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getaccountaddress",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Account,
		},
	})
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
	account, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter account must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetAddedNodeInfoCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAddedNodeInfoCmd) Method() string {
	return "getaddednodeinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAddedNodeInfoCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getaddednodeinfo",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Dns,
		},
	}

	if cmd.Node != "" {
		raw.Params = append(raw.Params, cmd.Node)
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
	dns, ok := r.Params[0].(bool)
	if !ok {
		return errors.New("first parameter dns must be a bool")
	}

	var node string
	if len(r.Params) == 2 {
		node, ok = r.Params[1].(string)
		if !ok {
			return errors.New("second parameter node must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetAddressesByAccountCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetAddressesByAccountCmd) Method() string {
	return "getaddressesbyaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAddressesByAccountCmd) MarshalJSON() ([]byte, error) {
	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getaddressesbyaccount",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Account,
		},
	})
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
	account, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter account must be a string")
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
	var minconf int = 1

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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetBalanceCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBalanceCmd) Method() string {
	return "getbalance"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBalanceCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getbalance",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Account != "" || cmd.MinConf != 1 {
		raw.Params = append(raw.Params, cmd.Account)
	}
	if cmd.MinConf != 1 {
		raw.Params = append(raw.Params, cmd.MinConf)
	}

	// Fill and marshal a RawCmd.
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
		account, ok := r.Params[0].(string)
		if !ok {
			return errors.New("first parameter account must be a string")
		}
		optArgs = append(optArgs, account)
	}

	if len(r.Params) > 1 {
		minconf, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter minconf must be a number")
		}
		optArgs = append(optArgs, int(minconf))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetBestBlockHashCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBestBlockHashCmd) Method() string {
	return "getbestblockhash"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBestBlockHashCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getbestblockhash",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetBlockCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockCmd) Method() string {
	return "getblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getblock",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Hash,
		},
	}

	if !cmd.Verbose {
		// set optional verbose argument to false
		raw.Params = append(raw.Params, false)
	} else {
		if cmd.VerboseTx {
			// set optional verbose argument to true
			raw.Params = append(raw.Params, true)
			// set optional verboseTx argument to true
			raw.Params = append(raw.Params, true)
		}
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

	hash, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter hash must be a string")
	}

	optArgs := make([]bool, 0, 1)
	if len(r.Params) > 1 {
		verbose, ok := r.Params[1].(bool)
		if !ok {
			return errors.New("second optional parameter verbose must be a bool")
		}

		optArgs = append(optArgs, verbose)
	}
	if len(r.Params) == 3 {
		verboseTx, ok := r.Params[2].(bool)
		if !ok {
			return errors.New("third optional parameter verboseTx must be a bool")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetBlockCountCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockCountCmd) Method() string {
	return "getblockcount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockCountCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getblockcount",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetBlockHashCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockHashCmd) Method() string {
	return "getblockhash"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockHashCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getblockhash",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Index,
		},
	})
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
	hash, ok := r.Params[0].(float64)
	if !ok {
		return errors.New("first parameter hash must be a number")
	}

	newCmd, err := NewGetBlockHashCmd(r.Id, int64(hash))
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
	Capabilities []string `json:"capabilities"`
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetBlockTemplateCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetBlockTemplateCmd) Method() string {
	return "getblocktemplate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBlockTemplateCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getblocktemplate",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Request != nil {
		raw.Params = append(raw.Params, cmd.Request)
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
		rmap, ok := r.Params[0].(map[string]interface{})
		if !ok {
			return errors.New("first optional parameter template request must be an object")
		}

		trequest := new(TemplateRequest)
		// optional mode string.
		mode, ok := rmap["mode"]
		if ok {
			smode, ok := mode.(string)
			if !ok {
				return errors.New("TemplateRequest mode must be a string")
			}
			trequest.Mode = smode
		}

		capabilities, ok := rmap["capabilities"]
		if ok {
			icap, ok := capabilities.([]interface{})
			if !ok {
				return errors.New("TemplateRequest mode must be an array")
			}

			cap := make([]string, len(icap))
			for i, val := range icap {
				cap[i], ok = val.(string)
				if !ok {
					return errors.New("TemplateRequest mode must be an aray of strings")
				}
			}

			trequest.Capabilities = cap
		}

		optArgs = append(optArgs, trequest)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetConnectionCountCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetConnectionCountCmd) Method() string {
	return "getconnectioncount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetConnectionCountCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getconnectioncount",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetDifficultyCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetDifficultyCmd) Method() string {
	return "getdifficulty"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetDifficultyCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getdifficulty",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetGenerateCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetGenerateCmd) Method() string {
	return "getgenerate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetGenerateCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getgenerate",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetHashesPerSecCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetHashesPerSecCmd) Method() string {
	return "gethashespersec"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetHashesPerSecCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "gethashespersec",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetInfoCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetInfoCmd) Method() string {
	return "getinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetInfoCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getinfo",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetMiningInfoCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetMiningInfoCmd) Method() string {
	return "getmininginfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetMiningInfoCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getmininginfo",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetNetTotalsCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetNetTotalsCmd) Method() string {
	return "getnettotals"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetNetTotalsCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getnettotals",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetNetworkHashPSCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetNetworkHashPSCmd) Method() string {
	return "getnetworkhashps"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetNetworkHashPSCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getnetworkhashps",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Blocks != 120 || cmd.Height != -1 {
		raw.Params = append(raw.Params, cmd.Blocks)
	}

	if cmd.Height != -1 {
		raw.Params = append(raw.Params, cmd.Height)
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
		blocks, ok := r.Params[0].(float64)
		if !ok {
			return errors.New("first optional parameter blocks must be a number")
		}

		optArgs = append(optArgs, int(blocks))
	}

	if len(r.Params) > 1 {
		height, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter height must be a number")
		}

		optArgs = append(optArgs, int(height))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetNewAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetNewAddressCmd) Method() string {
	return "getnewaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetNewAddressCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getnewaddress",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Account != "" {
		raw.Params = append(raw.Params, cmd.Account)
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
		addr, ok := r.Params[0].(string)
		if !ok {
			return errors.New("first optional parameter address must be a string")
		}

		optArgs = append(optArgs, addr)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetPeerInfoCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetPeerInfoCmd) Method() string {
	return "getpeerinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetPeerInfoCmd) MarshalJSON() ([]byte, error) {

	// Fill and marshal a RawCmd.
	return json.Marshal(RawCmd{
		Jsonrpc: "1.0",
		Method:  "getpeerinfo",
		Id:      cmd.id,
		Params:  []interface{}{},
	})
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetRawChangeAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetRawChangeAddressCmd) Method() string {
	return "getrawchangeaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetRawChangeAddressCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getrawchangeaddress",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Account != "" {
		raw.Params = append(raw.Params, cmd.Account)
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
		account, ok := r.Params[0].(string)
		if !ok {
			return errors.New("first optional parameter account must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetRawMempoolCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetRawMempoolCmd) Method() string {
	return "getrawmempool"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetRawMempoolCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getrawmempool",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Verbose {
		raw.Params = append(raw.Params, cmd.Verbose)
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
	if len(r.Params) == 1 {
		verbose, ok := r.Params[0].(bool)
		if !ok {
			return errors.New("first optional parameter verbose must be a bool")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetRawTransactionCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetRawTransactionCmd) Method() string {
	return "getrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetRawTransactionCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getrawtransaction",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Txid,
		},
	}

	if cmd.Verbose != 0 {
		raw.Params = append(raw.Params, cmd.Verbose)
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

	txid, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter txid must be a string")
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) == 2 {
		verbose, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter verbose must be a number")
		}

		optArgs = append(optArgs, int(verbose))
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
	var minconf int = 1
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetReceivedByAccountCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetReceivedByAccountCmd) Method() string {
	return "getreceivedbyaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetReceivedByAccountCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getreceivedbyaccount",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Account,
		},
	}

	if cmd.MinConf != 1 {
		raw.Params = append(raw.Params, cmd.MinConf)
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

	account, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter account must be a string")
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) > 1 {
		minconf, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter verbose must be a number")
		}

		optArgs = append(optArgs, int(minconf))
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
	var minconf int = 1
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetReceivedByAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetReceivedByAddressCmd) Method() string {
	return "getreceivedbyaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetReceivedByAddressCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getreceivedbyaddress",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
		},
	}

	if cmd.MinConf != 1 {
		raw.Params = append(raw.Params, cmd.MinConf)
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

	address, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter address must be a string")
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) > 1 {
		minconf, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter verbose must be a number")
		}

		optArgs = append(optArgs, int(minconf))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetTransactionCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetTransactionCmd) Method() string {
	return "gettransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetTransactionCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "gettransaction",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Txid,
		},
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

	txid, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter txid must be a string")
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
	var mempool bool
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetTxOutCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetTxOutCmd) Method() string {
	return "gettxout"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetTxOutCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "gettxout",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Txid,
			cmd.Output,
		},
	}

	if cmd.IncludeMempool != false {
		raw.Params = append(raw.Params, cmd.IncludeMempool)
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

	txid, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter txid must be a string")
	}

	output, ok := r.Params[1].(float64)
	if !ok {
		return errors.New("second parameter output must be a number")
	}
	optArgs := make([]bool, 0, 1)
	if len(r.Params) == 3 {
		mempool, ok := r.Params[2].(bool)
		if !ok {
			return errors.New("third optional parameter includemempool must be a bool")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetTxOutSetInfoCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetTxOutSetInfoCmd) Method() string {
	return "gettxoutsetinfo"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetTxOutSetInfoCmd) MarshalJSON() ([]byte, error) {

	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "gettxoutsetinfo",
		Id:      cmd.id,
		Params:  []interface{}{},
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

// WorkRequest is a request object as defined in
// https://en.bitcoin.it/wiki/Getwork, it is provided as a
// pointer argument to GetWorkCmd.
type WorkRequest struct {
	Data      string `json:"data"`
	Target    string `json:"target"`
	Algorithm string `json:"algorithm,omitempty"`
}

// GetWorkCmd is a type handling custom marshaling and
// unmarshaling of getwork JSON RPC commands.
type GetWorkCmd struct {
	id      interface{}
	Request *WorkRequest
}

// Enforce that GetWorkCmd satisifies the Cmd interface.
var _ Cmd = &GetWorkCmd{}

// NewGetWorkCmd creates a new GetWorkCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewGetWorkCmd(id interface{}, optArgs ...*WorkRequest) (*GetWorkCmd, error) {
	var request *WorkRequest
	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, ErrTooManyOptArgs
		}
		request = optArgs[0]
	}
	return &GetWorkCmd{
		id:      id,
		Request: request,
	}, nil
}

// Id satisfies the Cmd interface by returning the id of the command.
func (cmd *GetWorkCmd) Id() interface{} {
	return cmd.id
}

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *GetWorkCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *GetWorkCmd) Method() string {
	return "getwork"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetWorkCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "getwork",
		Id:      cmd.id,
		Params:  []interface{}{},
	}
	if cmd.Request != nil {
		raw.Params = append(raw.Params, cmd.Request)
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
	wrequest := new(WorkRequest)
	if len(r.Params) == 1 {
		rmap, ok := r.Params[0].(map[string]interface{})
		if !ok {
			return errors.New("first optional parameter template request must be an object")
		}

		// data is required
		data, ok := rmap["data"]
		if !ok {
			return errors.New("WorkRequest data must be present")
		}
		sdata, ok := data.(string)
		if !ok {
			return errors.New("WorkRequest data must be a string")
		}
		wrequest.Data = sdata

		// target is required
		target, ok := rmap["target"]
		if !ok {
			return errors.New("WorkRequest target must be present")
		}
		starget, ok := target.(string)
		if !ok {
			return errors.New("WorkRequest target must be a string")
		}
		wrequest.Target = starget

		// algorithm is optional
		algo, ok := rmap["algorithm"]
		if ok {
			salgo, ok := algo.(string)
			if !ok {
				return errors.New("WorkRequest algorithm must be a string")
			}
			wrequest.Algorithm = salgo
		}
	}
	newCmd, err := NewGetWorkCmd(r.Id, wrequest)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *HelpCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *HelpCmd) Method() string {
	return "help"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *HelpCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "help",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Command != "" {
		raw.Params = append(raw.Params, cmd.Command)
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
	if len(r.Params) == 1 {
		command, ok := r.Params[0].(string)
		if !ok {
			return errors.New("first optional parameter command must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ImportPrivKeyCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ImportPrivKeyCmd) Method() string {
	return "importprivkey"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ImportPrivKeyCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "importprivkey",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.PrivKey,
		},
	}

	if cmd.Label != "" || !cmd.Rescan {
		raw.Params = append(raw.Params, cmd.Label)
	}

	if !cmd.Rescan {
		raw.Params = append(raw.Params, cmd.Rescan)
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

	privkey, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter privkey must be a string")
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 1 {
		label, ok := r.Params[1].(string)
		if !ok {
			return errors.New("second optional parameter label must be a string")
		}

		optArgs = append(optArgs, label)
	}

	if len(r.Params) > 2 {
		rescan, ok := r.Params[2].(bool)
		if !ok {
			return errors.New("third optional parameter rescan must be a bool")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ImportWalletCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ImportWalletCmd) Method() string {
	return "importwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ImportWalletCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "importwallet",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Filename,
		},
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

	filename, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter filename must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *KeyPoolRefillCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *KeyPoolRefillCmd) Method() string {
	return "keypoolrefill"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *KeyPoolRefillCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "keypoolrefill",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.NewSize != 0 {
		raw.Params = append(raw.Params, cmd.NewSize)
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
		newsize, ok := r.Params[0].(float64)
		if !ok {
			return errors.New("first optional parameter newsize must be a number")
		}
		optArgs = append(optArgs, uint(newsize))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListAccountsCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListAccountsCmd) Method() string {
	return "listaccounts"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAccountsCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listaccounts",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.MinConf != 1 {
		raw.Params = append(raw.Params, cmd.MinConf)
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
		minconf, ok := r.Params[0].(float64)
		if !ok {
			return errors.New("first parameter minconf must be a number")
		}
		optArgs = append(optArgs, int(minconf))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListAddressGroupingsCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListAddressGroupingsCmd) Method() string {
	return "listaddressgroupings"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAddressGroupingsCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listaddressgroupings",
		Id:      cmd.id,
		Params:  []interface{}{},
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListLockUnspentCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListLockUnspentCmd) Method() string {
	return "listlockunspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListLockUnspentCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listlockunspent",
		Id:      cmd.id,
		Params:  []interface{}{},
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListReceivedByAccountCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListReceivedByAccountCmd) Method() string {
	return "listreceivedbyaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListReceivedByAccountCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listreceivedbyaccount",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.MinConf != 1 || cmd.IncludeEmpty != false {
		raw.Params = append(raw.Params, cmd.MinConf)
	}

	if cmd.IncludeEmpty != false {
		raw.Params = append(raw.Params, cmd.IncludeEmpty)
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
		minconf, ok := r.Params[0].(float64)
		if !ok {
			return errors.New("first optional parameter minconf must be a number")
		}
		optArgs = append(optArgs, int(minconf))
	}
	if len(r.Params) > 1 {
		includeempty, ok := r.Params[1].(bool)
		if !ok {
			return errors.New("second optional parameter includeempt must be a bool")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListReceivedByAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListReceivedByAddressCmd) Method() string {
	return "listreceivedbyaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListReceivedByAddressCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listreceivedbyaddress",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.MinConf != 1 || cmd.IncludeEmpty != false {
		raw.Params = append(raw.Params, cmd.MinConf)
	}

	if cmd.IncludeEmpty != false {
		raw.Params = append(raw.Params, cmd.IncludeEmpty)
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
		minconf, ok := r.Params[0].(float64)
		if !ok {
			return errors.New("first optional parameter minconf must be a number")
		}
		optArgs = append(optArgs, int(minconf))
	}
	if len(r.Params) > 1 {
		includeempty, ok := r.Params[1].(bool)
		if !ok {
			return errors.New("second optional parameter includeempt must be a bool")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListSinceBlockCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListSinceBlockCmd) Method() string {
	return "listsinceblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListSinceBlockCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listsinceblock",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.BlockHash != "" || cmd.TargetConfirmations != 1 {
		raw.Params = append(raw.Params, cmd.BlockHash)
	}

	if cmd.TargetConfirmations != 1 {
		raw.Params = append(raw.Params, cmd.TargetConfirmations)
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
		blockhash, ok := r.Params[0].(string)
		if !ok {
			return errors.New("first optional parameter blockhash must be a string")
		}
		optArgs = append(optArgs, blockhash)
	}
	if len(r.Params) > 1 {
		targetconfirmations, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter targetconfirmations must be a number")
		}
		optArgs = append(optArgs, int(targetconfirmations))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListTransactionsCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListTransactionsCmd) Method() string {
	return "listtransactions"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListTransactionsCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listtransactions",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.Account != "" || cmd.Count != 10 || cmd.From != 0 {
		raw.Params = append(raw.Params, cmd.Account)
	}

	if cmd.Count != 10 || cmd.From != 0 {
		raw.Params = append(raw.Params, cmd.Count)
	}
	if cmd.From != 0 {
		raw.Params = append(raw.Params, cmd.From)
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
		account, ok := r.Params[0].(string)
		if !ok {
			return errors.New("first optional parameter account must be a string")
		}
		optArgs = append(optArgs, account)
	}
	if len(r.Params) > 1 {
		count, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter count must be a number")
		}
		optArgs = append(optArgs, int(count))
	}
	if len(r.Params) > 2 {
		from, ok := r.Params[2].(float64)
		if !ok {
			return errors.New("third optional parameter from must be a number")
		}
		optArgs = append(optArgs, int(from))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ListUnspentCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ListUnspentCmd) Method() string {
	return "listunspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListUnspentCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "listunspent",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	if cmd.MinConf != 1 || cmd.MaxConf != 99999 || len(cmd.Addresses) != 0 {
		raw.Params = append(raw.Params, cmd.MinConf)
	}

	if cmd.MaxConf != 99999 || len(cmd.Addresses) != 0 {
		raw.Params = append(raw.Params, cmd.MaxConf)
	}

	if len(cmd.Addresses) != 0 {
		raw.Params = append(raw.Params, cmd.Addresses)
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
		minconf, ok := r.Params[0].(float64)
		if !ok {
			return errors.New("first optional parameter minconf must be a number")
		}
		optArgs = append(optArgs, int(minconf))
	}
	if len(r.Params) > 1 {
		maxconf, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter maxconf must be a number")
		}
		optArgs = append(optArgs, int(maxconf))
	}
	if len(r.Params) > 2 {
		iaddr, ok := r.Params[2].([]interface{})
		if !ok {
			return errors.New("third  optional parameter addresses must be an array")
		}

		addr := make([]string, len(iaddr))
		for i, val := range iaddr {
			addr[i], ok = val.(string)
			if !ok {
				return errors.New("optional parameter addreses must be an array of strings")
			}
		}

		optArgs = append(optArgs, addr)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *LockUnspentCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *LockUnspentCmd) Method() string {
	return "lockunspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *LockUnspentCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "lockunspent",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Unlock,
		},
	}

	if len(cmd.Transactions) > 0 {
		raw.Params = append(raw.Params, cmd.Transactions)
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

	unlock, ok := r.Params[0].(bool)
	if !ok {
		return errors.New("first parameter unlock must be a bool")
	}

	optArgs := make([][]TransactionInput, 0, 1)
	if len(r.Params) > 1 {
		iinputs, ok := r.Params[1].([]map[string]interface{})
		if !ok {
			return errors.New("second optional parameter transactions must be an array of objects")
		}
		inputs := make([]TransactionInput, len(iinputs))
		for i, v := range iinputs {
			if len(v) != 2 {
				return errors.New("input with wrong number of members")
			}
			txid, ok := v["txid"]
			if !ok {
				return errors.New("input without txid")
			}
			inputs[i].Txid, ok = txid.(string)
			if !ok {
				return errors.New("input txid isn't a string")
			}

			vout, ok := v["vout"]
			if !ok {
				return errors.New("input without vout")
			}
			fvout, ok := vout.(float64)
			if !ok {
				return errors.New("input vout not a number")
			}
			inputs[i].Vout = int(fvout)
		}
		optArgs = append(optArgs, inputs)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *MoveCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *MoveCmd) Method() string {
	return "move"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *MoveCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "move",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.FromAccount,
			cmd.ToAccount,
			float64(cmd.Amount) / 1e8, //convert to BTC
		},
	}

	if cmd.MinConf != 1 || cmd.Comment != "" {
		raw.Params = append(raw.Params, cmd.MinConf)
	}

	if cmd.Comment != "" {
		raw.Params = append(raw.Params, cmd.Comment)
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

	fromaccount, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter fromaccount must be a string")
	}

	toaccount, ok := r.Params[1].(string)
	if !ok {
		return errors.New("second parameter toaccount must be a string")
	}

	amount, ok := r.Params[2].(float64)
	if !ok {
		return errors.New("third parameter toaccount must be a number")
	}

	samount, err := JSONToAmount(amount)
	if err != nil {
		return err
	}
	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 3 {
		minconf, ok := r.Params[3].(float64)
		if !ok {
			return errors.New("fourth optional parameter minconf must be a number")
		}
		optArgs = append(optArgs, int(minconf))
	}
	if len(r.Params) > 4 {
		comment, ok := r.Params[4].(string)
		if !ok {
			return errors.New("fifth optional parameter comment must be a string")
		}
		optArgs = append(optArgs, comment)
	}

	newCmd, err := NewMoveCmd(r.Id, fromaccount, toaccount, samount,
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *PingCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *PingCmd) Method() string {
	return "ping"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *PingCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "ping",
		Id:      cmd.id,
		Params:  []interface{}{},
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SendFromCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendFromCmd) Method() string {
	return "sendfrom"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendFromCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "sendfrom",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.FromAccount,
			cmd.ToAddress,
			float64(cmd.Amount) / 1e8, //convert to BTC
		},
	}

	if cmd.MinConf != 1 || cmd.Comment != "" || cmd.CommentTo != "" {
		raw.Params = append(raw.Params, cmd.MinConf)
	}

	if cmd.Comment != "" || cmd.CommentTo != "" {
		raw.Params = append(raw.Params, cmd.Comment)
	}

	if cmd.CommentTo != "" {
		raw.Params = append(raw.Params, cmd.CommentTo)
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

	fromaccount, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter fromaccount must be a string")
	}

	toaccount, ok := r.Params[1].(string)
	if !ok {
		return errors.New("second parameter toaccount must be a string")
	}

	amount, ok := r.Params[2].(float64)
	if !ok {
		return errors.New("third parameter toaccount must be a number")
	}

	samount, err := JSONToAmount(amount)
	if err != nil {
		return err
	}

	optArgs := make([]interface{}, 0, 3)
	if len(r.Params) > 3 {
		minconf, ok := r.Params[3].(float64)
		if !ok {
			return errors.New("fourth optional parameter minconf must be a number")
		}

		optArgs = append(optArgs, int(minconf))
	}
	if len(r.Params) > 4 {
		comment, ok := r.Params[4].(string)
		if !ok {
			return errors.New("fifth optional parameter comment must be a string")
		}
		optArgs = append(optArgs, comment)
	}
	if len(r.Params) > 5 {
		commentto, ok := r.Params[5].(string)
		if !ok {
			return errors.New("sixth optional parameter commentto must be a string")
		}
		optArgs = append(optArgs, commentto)
	}

	newCmd, err := NewSendFromCmd(r.Id, fromaccount, toaccount, samount,
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SendManyCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendManyCmd) Method() string {
	return "sendmany"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendManyCmd) MarshalJSON() ([]byte, error) {
	floatAmount := make(map[string]float64)

	for k, v := range cmd.Amounts {
		floatAmount[k] = float64(v) / 1e8
	}
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "sendmany",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.FromAccount,
			floatAmount,
		},
	}

	if cmd.MinConf != 1 || cmd.Comment != "" {
		raw.Params = append(raw.Params, cmd.MinConf)
	}

	if cmd.Comment != "" {
		raw.Params = append(raw.Params, cmd.Comment)
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

	fromaccount, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter fromaccount must be a string")
	}

	iamounts, ok := r.Params[1].(map[string]interface{})
	if !ok {
		return errors.New("second parameter toaccount must be a JSON object")
	}

	amounts := make(map[string]int64)
	for k, v := range iamounts {
		famount, ok := v.(float64)
		if !ok {
			return errors.New("second parameter toaccount must be a string to number map")
		}
		var err error
		amounts[k], err = JSONToAmount(famount)
		if err != nil {
			return err
		}
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 2 {
		minconf, ok := r.Params[2].(float64)
		if !ok {
			return errors.New("third optional parameter minconf must be a number")
		}

		optArgs = append(optArgs, int(minconf))
	}
	if len(r.Params) > 3 {
		comment, ok := r.Params[3].(string)
		if !ok {
			return errors.New("fourth optional parameter comment must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SendRawTransactionCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendRawTransactionCmd) Method() string {
	return "sendrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendRawTransactionCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "sendrawtransaction",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.HexTx,
		},
	}

	if cmd.AllowHighFees {
		raw.Params = append(raw.Params, cmd.AllowHighFees)
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

	hextx, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter hextx must be a string")
	}

	optArgs := make([]bool, 0, 1)
	if len(r.Params) > 1 {
		allowHighFees, ok := r.Params[1].(bool)
		if !ok {
			return errors.New("second optional parameter allowhighfees must be a bool")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SendToAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SendToAddressCmd) Method() string {
	return "sendtoaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SendToAddressCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "sendtoaddress",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
			float64(cmd.Amount) / 1e8, //convert to BTC
		},
	}

	if cmd.Comment != "" || cmd.CommentTo != "" {
		raw.Params = append(raw.Params, cmd.Comment)
	}

	if cmd.CommentTo != "" {
		raw.Params = append(raw.Params, cmd.CommentTo)
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

	toaccount, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter toaccount must be a string")
	}

	amount, ok := r.Params[1].(float64)
	if !ok {
		return errors.New("second parameter amount must be a number")
	}

	samount, err := JSONToAmount(amount)
	if err != nil {
		return err
	}
	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 2 {
		comment, ok := r.Params[2].(string)
		if !ok {
			return errors.New("third optional parameter comment must be a string")
		}
		optArgs = append(optArgs, comment)
	}
	if len(r.Params) > 3 {
		commentto, ok := r.Params[3].(string)
		if !ok {
			return errors.New("sixth optional parameter commentto must be a string")
		}
		optArgs = append(optArgs, commentto)
	}

	newCmd, err := NewSendToAddressCmd(r.Id, toaccount, samount,
		optArgs...)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SetAccountCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SetAccountCmd) Method() string {
	return "setaccount"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SetAccountCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "setaccount",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
			cmd.Account,
		},
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

	address, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter address must be a string")
	}

	account, ok := r.Params[1].(string)
	if !ok {
		return errors.New("second parameter account must be a string")
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

	genproclimit := 0
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SetGenerateCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SetGenerateCmd) Method() string {
	return "setgenerate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SetGenerateCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "setgenerate",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Generate,
		},
	}

	if cmd.GenProcLimit != 0 {
		raw.Params = append(raw.Params, cmd.GenProcLimit)
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

	generate, ok := r.Params[0].(bool)
	if !ok {
		return errors.New("first parameter generate must be a bool")
	}

	optArgs := make([]int, 0, 1)
	if len(r.Params) > 1 {
		genproclimit, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second parameter genproclimit must be a number")
		}
		optArgs = append(optArgs, int(genproclimit))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SetTxFeeCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SetTxFeeCmd) Method() string {
	return "settxfee"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SetTxFeeCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "settxfee",
		Id:      cmd.id,
		Params: []interface{}{
			float64(cmd.Amount) / 1e8, //convert to BTC
		},
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

	amount, ok := r.Params[0].(float64)
	if !ok {
		return errors.New("first parameter amount must be a number")
	}

	samount, err := JSONToAmount(amount)
	if err != nil {
		return err
	}

	newCmd, err := NewSetTxFeeCmd(r.Id, samount)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SignMessageCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SignMessageCmd) Method() string {
	return "signmessage"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SignMessageCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "signmessage",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
			cmd.Message,
		},
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

	address, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter address must be a string")
	}

	message, ok := r.Params[1].(string)
	if !ok {
		return errors.New("second parameter message must be a string")
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
	Vout         int    `json:"vout"`
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SignRawTransactionCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SignRawTransactionCmd) Method() string {
	return "signrawtransaction"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SignRawTransactionCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "signrawtransaction",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.RawTx,
		},
	}

	if len(cmd.Inputs) > 0 || len(cmd.PrivKeys) > 0 || cmd.Flags != "" {
		raw.Params = append(raw.Params, cmd.Inputs)
	}
	if len(cmd.PrivKeys) > 0 || cmd.Flags != "" {
		raw.Params = append(raw.Params, cmd.PrivKeys)
	}
	if cmd.Flags != "" {
		raw.Params = append(raw.Params, cmd.Flags)
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

	rawtx, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter rawtx must be a string")
	}

	optArgs := make([]interface{}, 0, 3)
	if len(r.Params) > 1 {
		ip, ok := r.Params[1].([]interface{})
		if !ok {
			return errors.New("second optional parameter inputs must be an array")
		}
		inputs := make([]RawTxInput, len(ip))
		for i, val := range ip {
			mip, ok := val.(map[string]interface{})
			if !ok {
				return errors.New("second optional parameter inputs must be an array of objects")
			}
			txid, ok := mip["txid"]
			if !ok {
				return errors.New("txid missing in input object")
			}

			inputs[i].Txid, ok = txid.(string)
			if !ok {
				return errors.New("txid not a string in input object")
			}

			vout, ok := mip["vout"]
			if !ok {
				return errors.New("vout missing in input object")
			}
			fvout, ok := vout.(float64)
			if !ok {
				return errors.New("vout not a number in input object")
			}
			inputs[i].Vout = int(fvout)

			scriptpubkey, ok := mip["scriptpubkey"]
			if !ok {
				return errors.New("scriptpubkey missing in input object")
			}

			inputs[i].ScriptPubKey, ok = scriptpubkey.(string)
			if !ok {
				return errors.New("scriptpubkey not a string in input object")
			}

			redeemScript, ok := mip["redeemScript"]
			if !ok {
				return errors.New("redeemScript missing in input object")
			}

			inputs[i].RedeemScript, ok = redeemScript.(string)
			if !ok {
				return errors.New("redeemScript not a string in input object")
			}

		}
		optArgs = append(optArgs, inputs)
	}

	if len(r.Params) > 2 {
		privkeys, ok := r.Params[2].([]string)
		if !ok {
			return errors.New("third optional parameter privkeys is not an array of strings")
		}

		optArgs = append(optArgs, privkeys)
	}
	if len(r.Params) > 3 {
		flags, ok := r.Params[3].([]string)
		if !ok {
			return errors.New("fourth optional parameter flags is not a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *StopCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *StopCmd) Method() string {
	return "stop"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *StopCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "stop",
		Id:      cmd.id,
		Params:  []interface{}{},
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
	WorkId string `json:"workid,omitempty"`
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *SubmitBlockCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *SubmitBlockCmd) Method() string {
	return "submitblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *SubmitBlockCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "submitblock",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.HexBlock,
		},
	}

	if cmd.Options != nil {
		raw.Params = append(raw.Params, cmd.Options)
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

	hexblock, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter hexblock must be a string")
	}

	optArgs := make([]*SubmitBlockOptions, 0, 1)
	if len(r.Params) == 2 {
		obj, ok := r.Params[1].(map[string]interface{})
		if !ok {
			return errors.New("second optioanl parameter options must be an object")
		}
		options := new(SubmitBlockOptions)

		// workid is optional
		iworkid, ok := obj["workid"]
		if ok {
			workid, ok := iworkid.(string)
			if !ok {
				return errors.New("object member workid must be a string")
			}
			options.WorkId = workid
		}
		optArgs = append(optArgs, options)
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *ValidateAddressCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *ValidateAddressCmd) Method() string {
	return "validateaddress"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ValidateAddressCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "validateaddress",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
		},
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

	address, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter address must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *VerifyChainCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *VerifyChainCmd) Method() string {
	return "verifychain"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *VerifyChainCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "verifychain",
		Id:      cmd.id,
		Params:  []interface{}{},
	}

	// XXX(oga) magic numbers
	if cmd.CheckLevel != 3 || cmd.CheckDepth != 288 {
		raw.Params = append(raw.Params, cmd.CheckLevel)
	}

	if cmd.CheckDepth != 288 {
		raw.Params = append(raw.Params, cmd.CheckDepth)
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
		checklevel, ok := r.Params[0].(float64)
		if !ok {
			return errors.New("first optional parameter checklevel must be a number")
		}

		optArgs = append(optArgs, int32(checklevel))
	}

	if len(r.Params) > 1 {
		checkdepth, ok := r.Params[1].(float64)
		if !ok {
			return errors.New("second optional parameter checkdepth must be a number")
		}

		optArgs = append(optArgs, int32(checkdepth))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *VerifyMessageCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *VerifyMessageCmd) Method() string {
	return "verifymessage"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *VerifyMessageCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "verifymessage",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
			cmd.Signature,
			cmd.Message,
		},
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

	address, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter address must be a string")
	}

	signature, ok := r.Params[1].(string)
	if !ok {
		return errors.New("second parameter signature must be a string")
	}

	message, ok := r.Params[2].(string)
	if !ok {
		return errors.New("third parameter message must be a string")
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *WalletLockCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *WalletLockCmd) Method() string {
	return "walletlock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletLockCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "walletlock",
		Id:      cmd.id,
		Params:  []interface{}{},
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *WalletPassphraseCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *WalletPassphraseCmd) Method() string {
	return "walletpassphrase"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletPassphraseCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "walletpassphrase",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Passphrase,
			cmd.Timeout,
		},
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

	passphrase, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter passphrase must be a string")
	}

	timeout, ok := r.Params[1].(float64)
	if !ok {
		return errors.New("second parameter timeout must be a number")
	}

	newCmd, err := NewWalletPassphraseCmd(r.Id, passphrase, int64(timeout))
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

// SetId allows one to modify the Id of a Cmd to help in relaying them.
func (cmd *WalletPassphraseChangeCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the json method.
func (cmd *WalletPassphraseChangeCmd) Method() string {
	return "walletpassphrasechange"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletPassphraseChangeCmd) MarshalJSON() ([]byte, error) {
	raw := RawCmd{
		Jsonrpc: "1.0",
		Method:  "walletpassphrasechange",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.OldPassphrase,
			cmd.NewPassphrase,
		},
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

	oldpassphrase, ok := r.Params[0].(string)
	if !ok {
		return errors.New("first parameter oldpassphrase must be a string")
	}

	newpassphrase, ok := r.Params[1].(string)
	if !ok {
		return errors.New("second parameter newpassphrase must be a string")
	}

	newCmd, err := NewWalletPassphraseChangeCmd(r.Id, oldpassphrase, newpassphrase)
	if err != nil {
		return err
	}

	*cmd = *newCmd
	return nil
}
