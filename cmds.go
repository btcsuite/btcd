// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcws

import (
	"encoding/json"
	"errors"

	"github.com/conformal/btcjson"
	"github.com/conformal/btcwire"
)

// Help texts
const (
	authenticateHelp = `authenticate "username" "passphrase"
Authenticate the websocket with the RPC server.  This is only required if the
credentials were not already supplied via HTTP auth headers.  It must be the
first command sent or you will be disconnected.`
)

func init() {
	btcjson.RegisterCustomCmd("authenticate", parseAuthenticateCmd, nil,
		authenticateHelp)
	btcjson.RegisterCustomCmd("createencryptedwallet",
		parseCreateEncryptedWalletCmd, nil, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("exportwatchingwallet",
		parseExportWatchingWalletCmd, nil, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("getbestblock", parseGetBestBlockCmd,
		parseGetBestBlockCmdReply, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("getcurrentnet", parseGetCurrentNetCmd, nil,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("getunconfirmedbalance",
		parseGetUnconfirmedBalanceCmd, nil, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("listaddresstransactions",
		parseListAddressTransactionsCmd,
		parseListAddressTransactionsCmdReply, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("listalltransactions",
		parseListAllTransactionsCmd, nil, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("notifyblocks", parseNotifyBlocksCmd, nil,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("notifyreceived", parseNotifyReceivedCmd, nil,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("notifynewtransactions",
		parseNotifyNewTransactionsCmd, nil, `TODO(flam) fillmein`)
	btcjson.RegisterCustomCmd("notifyspent", parseNotifySpentCmd,
		nil, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("recoveraddresses", parseRecoverAddressesCmd,
		nil, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("rescan", parseRescanCmd,
		nil, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("walletislocked", parseWalletIsLockedCmd,
		nil, `TODO(jrick) fillmein`)
}

// AuthenticateCmd is a type handling custom marshaling and
// unmarshaling of authenticate JSON websocket extension
// commands.
type AuthenticateCmd struct {
	id         interface{}
	Username   string
	Passphrase string
}

// Enforce that AuthenticateCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &AuthenticateCmd{}

// NewAuthenticateCmd creates a new GetCurrentNetCmd.
func NewAuthenticateCmd(id interface{}, username, passphrase string) *AuthenticateCmd {
	return &AuthenticateCmd{
		id:         id,
		Username:   username,
		Passphrase: passphrase,
	}
}

// parseAuthenticateCmd parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the custom
// command with the btcjson parser.
func parseAuthenticateCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var username string
	if err := json.Unmarshal(r.Params[0], &username); err != nil {
		return nil, errors.New("first parameter 'username' must be " +
			"a string: " + err.Error())
	}

	var passphrase string
	if err := json.Unmarshal(r.Params[1], &passphrase); err != nil {
		return nil, errors.New("second parameter 'passphrase' must " +
			"be a string: " + err.Error())
	}

	return NewAuthenticateCmd(r.Id, username, passphrase), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *AuthenticateCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *AuthenticateCmd) Method() string {
	return "authenticate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *AuthenticateCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Username,
		cmd.Passphrase,
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *AuthenticateCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseAuthenticateCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*AuthenticateCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// GetCurrentNetCmd is a type handling custom marshaling and
// unmarshaling of getcurrentnet JSON websocket extension
// commands.
type GetCurrentNetCmd struct {
	id interface{}
}

// Enforce that GetCurrentNetCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &GetCurrentNetCmd{}

// NewGetCurrentNetCmd creates a new GetCurrentNetCmd.
func NewGetCurrentNetCmd(id interface{}) *GetCurrentNetCmd {
	return &GetCurrentNetCmd{id: id}
}

// parseGetCurrentNetCmd parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the custom
// command with the btcjson parser.
func parseGetCurrentNetCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 0 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	return NewGetCurrentNetCmd(r.Id), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *GetCurrentNetCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *GetCurrentNetCmd) Method() string {
	return "getcurrentnet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetCurrentNetCmd) MarshalJSON() ([]byte, error) {
	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetCurrentNetCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseGetCurrentNetCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*GetCurrentNetCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// ExportWatchingWalletCmd is a type handling custom marshaling and
// unmarshaling of exportwatchingwallet JSON websocket extension
// commands.
type ExportWatchingWalletCmd struct {
	id       interface{}
	Account  string
	Download bool
}

// Enforce that ExportWatchingWalletCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &ExportWatchingWalletCmd{}

// NewExportWatchingWalletCmd creates a new ExportWatchingWalletCmd.
func NewExportWatchingWalletCmd(id interface{}, optArgs ...interface{}) (*ExportWatchingWalletCmd, error) {
	if len(optArgs) > 2 {
		return nil, btcjson.ErrTooManyOptArgs
	}

	// Optional parameters set to their defaults.
	account := ""
	dl := false

	if len(optArgs) > 0 {
		a, ok := optArgs[0].(string)
		if !ok {
			return nil, errors.New("first optarg account must be a string")
		}
		account = a
	}
	if len(optArgs) > 1 {
		b, ok := optArgs[1].(bool)
		if !ok {
			return nil, errors.New("second optarg zip must be a boolean")
		}
		dl = b
	}

	return &ExportWatchingWalletCmd{
		id:       id,
		Account:  account,
		Download: dl,
	}, nil
}

// parseExportWatchingWalletCmd parses a RawCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the custom command with the btcjson parser.
func parseExportWatchingWalletCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) > 2 {
		return nil, btcjson.ErrTooManyOptArgs
	}

	optArgs := make([]interface{}, 0, 2)
	if len(r.Params) > 0 {
		var account string
		if err := json.Unmarshal(r.Params[0], &account); err != nil {
			return nil, errors.New("first optional parameter " +
				" 'account' must be a string: " + err.Error())
		}
		optArgs = append(optArgs, account)
	}

	if len(r.Params) > 1 {
		var download bool
		if err := json.Unmarshal(r.Params[1], &download); err != nil {
			return nil, errors.New("second optional parameter " +
				" 'download' must be a bool: " + err.Error())
		}
		optArgs = append(optArgs, download)
	}

	return NewExportWatchingWalletCmd(r.Id, optArgs...)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *ExportWatchingWalletCmd) Id() interface{} {
	return cmd.id
}

// Method satisifies the Cmd interface by returning the RPC method.
func (cmd *ExportWatchingWalletCmd) Method() string {
	return "exportwatchingwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ExportWatchingWalletCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 2)
	if cmd.Account != "" || cmd.Download {
		params = append(params, cmd.Account)
	}
	if cmd.Download {
		params = append(params, cmd.Download)
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ExportWatchingWalletCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseExportWatchingWalletCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*ExportWatchingWalletCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// GetUnconfirmedBalanceCmd is a type handling custom marshaling and
// unmarshaling of getunconfirmedbalance JSON websocket extension
// commands.
type GetUnconfirmedBalanceCmd struct {
	id      interface{}
	Account string
}

// Enforce that GetUnconfirmedBalanceCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &GetUnconfirmedBalanceCmd{}

// NewGetUnconfirmedBalanceCmd creates a new GetUnconfirmedBalanceCmd.
func NewGetUnconfirmedBalanceCmd(id interface{},
	optArgs ...string) (*GetUnconfirmedBalanceCmd, error) {

	if len(optArgs) > 1 {
		return nil, btcjson.ErrTooManyOptArgs
	}

	// Optional parameters set to their defaults.
	account := ""

	if len(optArgs) == 1 {
		account = optArgs[0]
	}

	return &GetUnconfirmedBalanceCmd{
		id:      id,
		Account: account,
	}, nil
}

// parseGetUnconfirmedBalanceCmd parses a RawCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the custom command with the btcjson parser.
func parseGetUnconfirmedBalanceCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) > 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	optArgs := make([]string, 0, 1)
	if len(r.Params) > 0 {
		var account string
		if err := json.Unmarshal(r.Params[0], &account); err != nil {
			return nil, errors.New("first optional parameter " +
				" 'account' must be a string: " + err.Error())
		}
		optArgs = append(optArgs, account)
	}

	return NewGetUnconfirmedBalanceCmd(r.Id, optArgs...)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *GetUnconfirmedBalanceCmd) Id() interface{} {
	return cmd.id
}

// Method satisifies the Cmd interface by returning the RPC method.
func (cmd *GetUnconfirmedBalanceCmd) Method() string {
	return "getunconfirmedbalance"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetUnconfirmedBalanceCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Account != "" {
		params = append(params, cmd.Account)
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetUnconfirmedBalanceCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseGetUnconfirmedBalanceCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*GetUnconfirmedBalanceCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// GetBestBlockResult holds the result of a getbestblock response.
type GetBestBlockResult struct {
	Hash   string `json:"hash"`
	Height int32  `json:"height"`
}

// GetBestBlockCmd is a type handling custom marshaling and
// unmarshaling of getbestblock JSON websocket extension
// commands.
type GetBestBlockCmd struct {
	id interface{}
}

// Enforce that GetBestBlockCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &GetBestBlockCmd{}

// NewGetBestBlockCmd creates a new GetBestBlock.
func NewGetBestBlockCmd(id interface{}) *GetBestBlockCmd {
	return &GetBestBlockCmd{id: id}
}

// parseGetBestBlockCmd parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the custom
// command with the btcjson parser.
func parseGetBestBlockCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 0 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	return NewGetBestBlockCmd(r.Id), nil
}

// parseGetBestBlockCmdReply parses a the reply to a GetBestBlockCmd into a
// concrete type and returns it packed into an interface.  This is used when
// registering the custom command with btcjson.
func parseGetBestBlockCmdReply(message json.RawMessage) (interface{}, error) {
	var res *GetBestBlockResult
	if err := json.Unmarshal(message, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *GetBestBlockCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *GetBestBlockCmd) Method() string {
	return "getbestblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBestBlockCmd) MarshalJSON() ([]byte, error) {
	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetBestBlockCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseGetBestBlockCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*GetBestBlockCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// RecoverAddressesCmd is a type handling custom marshaling and
// unmarshaling of recoveraddresses JSON websocket extension
// commands.
type RecoverAddressesCmd struct {
	id      interface{}
	Account string
	N       int
}

// Enforce that RecoverAddressesCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &RecoverAddressesCmd{}

// NewRecoverAddressesCmd creates a new RecoverAddressesCmd.
func NewRecoverAddressesCmd(id interface{}, account string, n int) *RecoverAddressesCmd {
	return &RecoverAddressesCmd{
		id:      id,
		Account: account,
		N:       n,
	}
}

// parseRecoverAddressesCmd parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the custom
// command with the btcjson parser.
func parseRecoverAddressesCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return nil, errors.New("first parameter 'account' must be a " +
			"string: " + err.Error())
	}

	var n int
	if err := json.Unmarshal(r.Params[1], &n); err != nil {
		return nil, errors.New("second parameter 'n' must be an " +
			"integer: " + err.Error())
	}

	return NewRecoverAddressesCmd(r.Id, account, n), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *RecoverAddressesCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *RecoverAddressesCmd) Method() string {
	return "recoveraddresses"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *RecoverAddressesCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Account,
		cmd.N,
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *RecoverAddressesCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseRecoverAddressesCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*RecoverAddressesCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// OutPoint describes a transaction outpoint that will be marshalled to and
// from JSON.
type OutPoint struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"index"`
}

// NewOutPointFromWire creates a new OutPoint from the OutPoint structure
// of the btcwire package.
func NewOutPointFromWire(op *btcwire.OutPoint) *OutPoint {
	return &OutPoint{
		Hash:  op.Hash.String(),
		Index: op.Index,
	}
}

// RescanCmd is a type handling custom marshaling and
// unmarshaling of rescan JSON websocket extension
// commands.
type RescanCmd struct {
	id         interface{}
	BeginBlock string
	Addresses  []string
	OutPoints  []OutPoint
	EndBlock   string
}

// Enforce that RescanCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &RescanCmd{}

// NewRescanCmd creates a new RescanCmd, parsing the optional
// arguments optArgs which may either be empty or a single upper
// block hash.
func NewRescanCmd(id interface{}, begin string, addresses []string,
	outpoints []OutPoint, optArgs ...string) (*RescanCmd, error) {

	// Optional parameters set to their defaults.
	var end string

	if len(optArgs) > 0 {
		if len(optArgs) > 1 {
			return nil, btcjson.ErrTooManyOptArgs
		}
		end = optArgs[0]
	}

	return &RescanCmd{
		id:         id,
		BeginBlock: begin,
		Addresses:  addresses,
		OutPoints:  outpoints,
		EndBlock:   end,
	}, nil
}

// parseRescanCmd parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the custom
// command with the btcjson parser.
func parseRescanCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) < 3 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var begin string
	if err := json.Unmarshal(r.Params[0], &begin); err != nil {
		return nil, errors.New("first parameter 'begin' must be a " +
			"string: " + err.Error())
	}

	var addresses []string
	if err := json.Unmarshal(r.Params[1], &addresses); err != nil {
		return nil, errors.New("second parameter 'addresses' must be " +
			"an array of strings: " + err.Error())
	}

	var outpoints []OutPoint
	if err := json.Unmarshal(r.Params[2], &outpoints); err != nil {
		return nil, errors.New("third parameter 'outpoints' must be " +
			"an array of transaction outpoint JSON objects: " +
			err.Error())
	}

	optArgs := make([]string, 0, 1)
	if len(r.Params) > 3 {
		var endblock string
		if err := json.Unmarshal(r.Params[3], &endblock); err != nil {
			return nil, errors.New("fourth optional parameter " +
				"'endblock' must be a string: " + err.Error())
		}
		optArgs = append(optArgs, endblock)
	}

	return NewRescanCmd(r.Id, begin, addresses, outpoints, optArgs...)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *RescanCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *RescanCmd) Method() string {
	return "rescan"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *RescanCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 3, 4)
	params[0] = cmd.BeginBlock
	params[1] = cmd.Addresses
	params[2] = cmd.OutPoints
	if cmd.EndBlock != "" {
		params = append(params, cmd.EndBlock)
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *RescanCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseRescanCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*RescanCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// NotifyBlocksCmd is a type handling custom marshaling and
// unmarshaling of notifyblocks JSON websocket extension
// commands.
type NotifyBlocksCmd struct {
	id interface{}
}

// Enforce that NotifyBlocksCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &NotifyBlocksCmd{}

// NewNotifyBlocksCmd creates a new NotifyBlocksCmd.
func NewNotifyBlocksCmd(id interface{}) *NotifyBlocksCmd {
	return &NotifyBlocksCmd{
		id: id,
	}
}

// parseNotifyBlocksCmd parses a NotifyBlocksCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the custom command with the btcjson parser.
func parseNotifyBlocksCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 0 {
		return nil, btcjson.ErrWrongNumberOfParams
	}
	return NewNotifyBlocksCmd(r.Id), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *NotifyBlocksCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *NotifyBlocksCmd) Method() string {
	return "notifyblocks"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *NotifyBlocksCmd) MarshalJSON() ([]byte, error) {
	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), []interface{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *NotifyBlocksCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseNotifyBlocksCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*NotifyBlocksCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// NotifyReceivedCmd is a type handling custom marshaling and
// unmarshaling of notifyreceived JSON websocket extension
// commands.
type NotifyReceivedCmd struct {
	id        interface{}
	Addresses []string
}

// Enforce that NotifyReceivedCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &NotifyReceivedCmd{}

// NewNotifyReceivedCmd creates a new NotifyReceivedCmd.
func NewNotifyReceivedCmd(id interface{}, addresses []string) *NotifyReceivedCmd {
	return &NotifyReceivedCmd{
		id:        id,
		Addresses: addresses,
	}
}

// parseNotifyReceivedCmd parses a NotifyNewTXsCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the custom command with the btcjson parser.
func parseNotifyReceivedCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var addresses []string
	if err := json.Unmarshal(r.Params[0], &addresses); err != nil {
		return nil, errors.New("first parameter 'addresses' must be " +
			"an array of strings: " + err.Error())
	}

	return NewNotifyReceivedCmd(r.Id, addresses), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *NotifyReceivedCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *NotifyReceivedCmd) Method() string {
	return "notifyreceived"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *NotifyReceivedCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Addresses,
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *NotifyReceivedCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseNotifyReceivedCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*NotifyReceivedCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// NotifyNewTransactionsCmd is a type handling custom marshaling and
// unmarshaling of notifynewtransactions JSON websocket extension
// commands.
type NotifyNewTransactionsCmd struct {
	id      interface{}
	Verbose bool
}

// Enforce that NotifyNewTransactionsCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &NotifyNewTransactionsCmd{}

// NewNotifyNewTransactionsCmd creates a new NotifyNewTransactionsCmd that
// optionally takes a single verbose parameter that defaults to false.
func NewNotifyNewTransactionsCmd(id interface{}, optArgs ...bool) (*NotifyNewTransactionsCmd, error) {
	verbose := false

	optArgsLen := len(optArgs)
	if optArgsLen > 0 {
		if optArgsLen > 1 {
			return nil, btcjson.ErrTooManyOptArgs
		}
		verbose = optArgs[0]
	}

	return &NotifyNewTransactionsCmd{
		id:      id,
		Verbose: verbose,
	}, nil
}

// parseNotifyNewTransactionsCmd parses a NotifyNewTransactionsCmd into a
// concrete type satisifying the btcjson.Cmd interface.  This is used when
// registering the custom command with the btcjson parser.
func parseNotifyNewTransactionsCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) > 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	optArgs := make([]bool, 0, 1)
	if len(r.Params) > 0 {
		var verbose bool
		if err := json.Unmarshal(r.Params[0], &verbose); err != nil {
			return nil, errors.New("first optional parameter " +
				"'verbose' must be a bool: " + err.Error())
		}
		optArgs = append(optArgs, verbose)
	}

	return NewNotifyNewTransactionsCmd(r.Id, optArgs...)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *NotifyNewTransactionsCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *NotifyNewTransactionsCmd) Method() string {
	return "notifynewtransactions"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *NotifyNewTransactionsCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Verbose,
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *NotifyNewTransactionsCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseNotifyNewTransactionsCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*NotifyNewTransactionsCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// NotifySpentCmd is a type handling custom marshaling and
// unmarshaling of notifyspent JSON websocket extension
// commands.
type NotifySpentCmd struct {
	id        interface{}
	OutPoints []OutPoint
}

// Enforce that NotifySpentCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &NotifySpentCmd{}

// NewNotifySpentCmd creates a new NotifySpentCmd.
func NewNotifySpentCmd(id interface{}, outpoints []OutPoint) *NotifySpentCmd {
	return &NotifySpentCmd{
		id:        id,
		OutPoints: outpoints,
	}
}

// parseNotifySpentCmd parses a NotifySpentCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the custom command with the btcjson parser.
func parseNotifySpentCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var outpoints []OutPoint
	if err := json.Unmarshal(r.Params[0], &outpoints); err != nil {
		return nil, errors.New("first parameter 'outpoints' must be a " +
			"an array of transaction outpoint JSON objects: " +
			err.Error())
	}

	return NewNotifySpentCmd(r.Id, outpoints), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *NotifySpentCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *NotifySpentCmd) Method() string {
	return "notifyspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *NotifySpentCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.OutPoints,
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *NotifySpentCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseNotifySpentCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*NotifySpentCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// CreateEncryptedWalletCmd is a type handling custom
// marshaling and unmarshaling of createencryptedwallet
// JSON websocket extension commands.
type CreateEncryptedWalletCmd struct {
	id         interface{}
	Passphrase string
}

// Enforce that CreateEncryptedWalletCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &CreateEncryptedWalletCmd{}

// NewCreateEncryptedWalletCmd creates a new CreateEncryptedWalletCmd.
func NewCreateEncryptedWalletCmd(id interface{}, passphrase string) *CreateEncryptedWalletCmd {
	return &CreateEncryptedWalletCmd{
		id:         id,
		Passphrase: passphrase,
	}
}

// parseCreateEncryptedWalletCmd parses a CreateEncryptedWalletCmd
// into a concrete type satisifying the btcjson.Cmd interface.
// This is used when registering the custom command with the btcjson
// parser.
func parseCreateEncryptedWalletCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var passphrase string
	if err := json.Unmarshal(r.Params[0], &passphrase); err != nil {
		return nil, errors.New("first parameter 'passphrase' must be " +
			"a string: " + err.Error())
	}

	return NewCreateEncryptedWalletCmd(r.Id, passphrase), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *CreateEncryptedWalletCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *CreateEncryptedWalletCmd) Method() string {
	return "createencryptedwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *CreateEncryptedWalletCmd) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		cmd.Passphrase,
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *CreateEncryptedWalletCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseCreateEncryptedWalletCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*CreateEncryptedWalletCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// WalletIsLockedCmd is a type handling custom marshaling and
// unmarshaling of walletislocked JSON websocket extension commands.
type WalletIsLockedCmd struct {
	id      interface{}
	Account string
}

// Enforce that WalletIsLockedCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &WalletIsLockedCmd{}

// NewWalletIsLockedCmd creates a new WalletIsLockedCmd.
func NewWalletIsLockedCmd(id interface{},
	optArgs ...string) (*WalletIsLockedCmd, error) {

	// Optional arguments set to their default values.
	account := ""

	if len(optArgs) > 1 {
		return nil, btcjson.ErrInvalidParams
	}

	if len(optArgs) == 1 {
		account = optArgs[0]
	}

	return &WalletIsLockedCmd{
		id:      id,
		Account: account,
	}, nil
}

// parseWalletIsLockedCmd parses a WalletIsLockedCmd into a concrete
// type satisifying the btcjson.Cmd interface.  This is used when
// registering the custom command with the btcjson parser.
func parseWalletIsLockedCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) > 1 {
		return nil, btcjson.ErrInvalidParams
	}

	if len(r.Params) == 0 {
		return NewWalletIsLockedCmd(r.Id)
	}

	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return nil, errors.New("first parameter 'account' must be a " +
			"string: " + err.Error())
	}

	return NewWalletIsLockedCmd(r.Id, account)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *WalletIsLockedCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *WalletIsLockedCmd) Method() string {
	return "walletislocked"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletIsLockedCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Account != "" {
		params = append(params, cmd.Account)
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *WalletIsLockedCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseWalletIsLockedCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*WalletIsLockedCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// ListAddressTransactionsCmd is a type handling custom marshaling and
// unmarshaling of listaddresstransactions JSON websocket extension commands.
type ListAddressTransactionsCmd struct {
	id        interface{}
	Account   string
	Addresses []string
}

// Enforce that ListAddressTransactionsCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &ListAddressTransactionsCmd{}

// NewListAddressTransactionsCmd creates a new ListAddressTransactionsCmd.
func NewListAddressTransactionsCmd(id interface{}, addresses []string,
	optArgs ...string) (*ListAddressTransactionsCmd, error) {

	if len(optArgs) > 1 {
		return nil, btcjson.ErrTooManyOptArgs
	}

	// Optional arguments set to their default values.
	account := ""

	if len(optArgs) == 1 {
		account = optArgs[0]
	}

	return &ListAddressTransactionsCmd{
		id:        id,
		Account:   account,
		Addresses: addresses,
	}, nil
}

// parseListAddressTransactionsCmd parses a ListAddressTransactionsCmd into
// a concrete type satisifying the btcjson.Cmd interface.  This is used
// when registering the custom command with the btcjson parser.
func parseListAddressTransactionsCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) == 0 || len(r.Params) > 2 {
		return nil, btcjson.ErrInvalidParams
	}

	var addresses []string
	if err := json.Unmarshal(r.Params[0], &addresses); err != nil {
		return nil, errors.New("first parameter 'addresses' must be " +
			"an array of strings: " + err.Error())
	}

	optArgs := make([]string, 0, 1)
	if len(r.Params) > 1 {
		var account string
		if err := json.Unmarshal(r.Params[1], &account); err != nil {
			return nil, errors.New("second optional parameter " +
				"'account' must be a string: " + err.Error())
		}
		optArgs = append(optArgs, account)
	}

	return NewListAddressTransactionsCmd(r.Id, addresses, optArgs...)
}

// parseListAddressTransactionsCmdReply parses a the reply to a
// ListAddressTransactionsCmd into a concrete type and returns it packed into
// an interface.  This is used when registering the custom command with btcjson.
func parseListAddressTransactionsCmdReply(message json.RawMessage) (interface{}, error) {
	var res []btcjson.ListTransactionsResult
	if err := json.Unmarshal(message, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *ListAddressTransactionsCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *ListAddressTransactionsCmd) Method() string {
	return "listaddresstransactions"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAddressTransactionsCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = cmd.Addresses
	if cmd.Account != "" {
		params = append(params, cmd.Account)
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListAddressTransactionsCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseListAddressTransactionsCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*ListAddressTransactionsCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}

// ListAllTransactionsCmd is a type handling custom marshaling and
// unmarshaling of listalltransactions JSON websocket extension commands.
type ListAllTransactionsCmd struct {
	id      interface{}
	Account string
}

// Enforce that ListAllTransactionsCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &ListAllTransactionsCmd{}

// NewListAllTransactionsCmd creates a new ListAllTransactionsCmd.
func NewListAllTransactionsCmd(id interface{},
	optArgs ...string) (*ListAllTransactionsCmd, error) {

	// Optional arguments set to their default values.
	account := ""

	if len(optArgs) > 1 {
		return nil, btcjson.ErrInvalidParams
	}

	if len(optArgs) == 1 {
		account = optArgs[0]
	}

	return &ListAllTransactionsCmd{
		id:      id,
		Account: account,
	}, nil
}

// parseListAllTransactionsCmd parses a ListAllTransactionsCmd into a concrete
// type satisifying the btcjson.Cmd interface.  This is used when
// registering the custom command with the btcjson parser.
func parseListAllTransactionsCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) > 1 {
		return nil, btcjson.ErrInvalidParams
	}

	optArgs := make([]string, 0, 1)
	if len(r.Params) > 0 {
		var account string
		if err := json.Unmarshal(r.Params[0], &account); err != nil {
			return nil, errors.New("first optional parameter " +
				"'account' must be a string: " + err.Error())
		}
		optArgs = append(optArgs, account)
	}

	return NewListAllTransactionsCmd(r.Id, optArgs...)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *ListAllTransactionsCmd) Id() interface{} {
	return cmd.id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *ListAllTransactionsCmd) Method() string {
	return "listalltransactions"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAllTransactionsCmd) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 0, 1)
	if cmd.Account != "" {
		params = append(params, cmd.Account)
	}

	raw, err := btcjson.NewRawCmd(cmd.id, cmd.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *ListAllTransactionsCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseListAllTransactionsCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*ListAllTransactionsCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}
