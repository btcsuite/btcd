// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcws

import (
	"encoding/json"
	"errors"
	"github.com/conformal/btcdb"
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
	btcjson.RegisterCustomCmd("authenticate", parseAuthenticateCmd,
		authenticateHelp)
	btcjson.RegisterCustomCmd("createencryptedwallet",
		parseCreateEncryptedWalletCmd, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("exportwatchingwallet",
		parseExportWatchingWalletCmd, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("getaddressbalance",
		parseGetAddressBalanceCmd, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("getbestblock", parseGetBestBlockCmd,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("getcurrentnet", parseGetCurrentNetCmd,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("getunconfirmedbalance",
		parseGetUnconfirmedBalanceCmd, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("listaddresstransactions",
		parseListAddressTransactionsCmd, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("listalltransactions",
		parseListAllTransactionsCmd, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("notifyblocks", parseNotifyBlocksCmd,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("notifynewtxs", parseNotifyNewTXsCmd,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("notifyspent", parseNotifySpentCmd,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("recoveraddresses", parseRecoverAddressesCmd,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("rescan", parseRescanCmd,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd("walletislocked", parseWalletIsLockedCmd,
		`TODO(jrick) fillmein`)
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

	username, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter username must be a string")
	}

	passphrase, ok := r.Params[1].(string)
	if !ok {
		return nil, errors.New("second parameter passphrase must be a string")
	}

	return NewAuthenticateCmd(r.Id, username, passphrase), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *AuthenticateCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *AuthenticateCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *AuthenticateCmd) Method() string {
	return "authenticate"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *AuthenticateCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  cmd.Method(),
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Username,
			cmd.Passphrase,
		},
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

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *GetCurrentNetCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *GetCurrentNetCmd) Method() string {
	return "getcurrentnet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetCurrentNetCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "getcurrentnet",
		Id:      cmd.id,
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
	return NewExportWatchingWalletCmd(r.Id, r.Params...)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *ExportWatchingWalletCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *ExportWatchingWalletCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisifies the Cmd interface by returning the RPC method.
func (cmd *ExportWatchingWalletCmd) Method() string {
	return "exportwatchingwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ExportWatchingWalletCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "exportwatchingwallet",
		Id:      cmd.id,
	}

	if cmd.Account != "" || cmd.Download {
		raw.Params = append(raw.Params, cmd.Account)
	}
	if cmd.Download {
		raw.Params = append(raw.Params, cmd.Download)
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

	if len(r.Params) == 0 {
		// No optional args.
		return NewGetUnconfirmedBalanceCmd(r.Id)
	}

	// One optional parameter for account.
	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter account must be a string")
	}
	return NewGetUnconfirmedBalanceCmd(r.Id, account)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *GetUnconfirmedBalanceCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *GetUnconfirmedBalanceCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisifies the Cmd interface by returning the RPC method.
func (cmd *GetUnconfirmedBalanceCmd) Method() string {
	return "getunconfirmedbalance"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetUnconfirmedBalanceCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "getunconfirmedbalance",
		Id:      cmd.id,
	}

	if cmd.Account != "" {
		raw.Params = append(raw.Params, cmd.Account)
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

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *GetBestBlockCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *GetBestBlockCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *GetBestBlockCmd) Method() string {
	return "getbestblock"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetBestBlockCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "getbestblock",
		Id:      cmd.id,
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

	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter account must be a string")
	}
	n, ok := r.Params[1].(float64)
	if !ok {
		return nil, errors.New("second parameter n must be a number")
	}
	return NewRecoverAddressesCmd(r.Id, account, int(n)), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *RecoverAddressesCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *RecoverAddressesCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *RecoverAddressesCmd) Method() string {
	return "recoveraddresses"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *RecoverAddressesCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "recoveraddresses",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Account,
			cmd.N,
		},
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

// RescanCmd is a type handling custom marshaling and
// unmarshaling of rescan JSON websocket extension
// commands.
type RescanCmd struct {
	id         interface{}
	BeginBlock int32
	Addresses  map[string]struct{}
	EndBlock   int64 // TODO: switch this and btcdb.AllShas to int32
}

// Enforce that RescanCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &RescanCmd{}

// NewRescanCmd creates a new RescanCmd, parsing the optional
// arguments optArgs which may either be empty or a single upper
// block height.
func NewRescanCmd(id interface{}, begin int32, addresses map[string]struct{},
	optArgs ...int64) (*RescanCmd, error) {

	// Optional parameters set to their defaults.
	end := btcdb.AllShas

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
		EndBlock:   end,
	}, nil
}

// parseRescanCmd parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the custom
// command with the btcjson parser.
func parseRescanCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) < 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	begin, ok := r.Params[0].(float64)
	if !ok {
		return nil, errors.New("first parameter must be a number")
	}
	iaddrs, ok := r.Params[1].(map[string]interface{})
	if !ok {
		return nil, errors.New("second parameter must be a JSON object")
	}
	addresses := make(map[string]struct{}, len(iaddrs))
	for addr := range iaddrs {
		addresses[addr] = struct{}{}
	}
	params := make([]int64, len(r.Params[2:]))
	for i, val := range r.Params[2:] {
		fval, ok := val.(float64)
		if !ok {
			return nil, errors.New("optional parameters must " +
				"be be numbers")
		}
		params[i] = int64(fval)
	}

	return NewRescanCmd(r.Id, int32(begin), addresses, params...)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *RescanCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *RescanCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *RescanCmd) Method() string {
	return "rescan"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *RescanCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "rescan",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.BeginBlock,
			cmd.Addresses,
		},
	}

	if cmd.EndBlock != btcdb.AllShas {
		raw.Params = append(raw.Params, cmd.EndBlock)
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

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *NotifyBlocksCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *NotifyBlocksCmd) Method() string {
	return "notifyblocks"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *NotifyBlocksCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "notifyblocks",
		Id:      cmd.id,
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

// NotifyNewTXsCmd is a type handling custom marshaling and
// unmarshaling of notifynewtxs JSON websocket extension
// commands.
type NotifyNewTXsCmd struct {
	id        interface{}
	Addresses []string
}

// Enforce that NotifyNewTXsCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &NotifyNewTXsCmd{}

// NewNotifyNewTXsCmd creates a new NotifyNewTXsCmd.
func NewNotifyNewTXsCmd(id interface{}, addresses []string) *NotifyNewTXsCmd {
	return &NotifyNewTXsCmd{
		id:        id,
		Addresses: addresses,
	}
}

// parseNotifyNewTXsCmd parses a NotifyNewTXsCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the custom command with the btcjson parser.
func parseNotifyNewTXsCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	iaddrs, ok := r.Params[0].([]interface{})
	if !ok {
		return nil, errors.New("first parameter must be a JSON array")
	}
	addresses := make([]string, len(iaddrs))
	for i := range iaddrs {
		addr, ok := iaddrs[i].(string)
		if !ok {
			return nil, errors.New("first parameter must be an " +
				"array of strings")
		}
		addresses[i] = addr
	}

	return NewNotifyNewTXsCmd(r.Id, addresses), nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *NotifyNewTXsCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *NotifyNewTXsCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *NotifyNewTXsCmd) Method() string {
	return "notifynewtxs"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *NotifyNewTXsCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "notifynewtxs",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Addresses,
		},
	}

	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *NotifyNewTXsCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseNotifyNewTXsCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*NotifyNewTXsCmd)
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
	id interface{}
	*btcwire.OutPoint
}

// Enforce that NotifySpentCmd satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &NotifySpentCmd{}

// NewNotifySpentCmd creates a new NotifySpentCmd.
func NewNotifySpentCmd(id interface{}, op *btcwire.OutPoint) *NotifySpentCmd {
	return &NotifySpentCmd{
		id:       id,
		OutPoint: op,
	}
}

// parseNotifySpentCmd parses a NotifySpentCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the custom command with the btcjson parser.
func parseNotifySpentCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	hashStr, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter must be a string")
	}
	hash, err := btcwire.NewShaHashFromStr(hashStr)
	if err != nil {
		return nil, errors.New("first parameter is not a valid " +
			"hash string")
	}
	idx, ok := r.Params[1].(float64)
	if !ok {
		return nil, errors.New("second parameter is not a number")
	}
	if idx < 0 {
		return nil, errors.New("second parameter cannot be negative")
	}

	cmd := NewNotifySpentCmd(r.Id, btcwire.NewOutPoint(hash, uint32(idx)))
	return cmd, nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *NotifySpentCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *NotifySpentCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *NotifySpentCmd) Method() string {
	return "notifyspent"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *NotifySpentCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "notifyspent",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.OutPoint.Hash.String(),
			cmd.OutPoint.Index,
		},
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
	id          interface{}
	Account     string
	Description string
	Passphrase  string
}

// Enforce that CreateEncryptedWalletCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &CreateEncryptedWalletCmd{}

// NewCreateEncryptedWalletCmd creates a new CreateEncryptedWalletCmd.
func NewCreateEncryptedWalletCmd(id interface{},
	account, description, passphrase string) *CreateEncryptedWalletCmd {

	return &CreateEncryptedWalletCmd{
		id:          id,
		Account:     account,
		Description: description,
		Passphrase:  passphrase,
	}
}

// parseCreateEncryptedWalletCmd parses a CreateEncryptedWalletCmd
// into a concrete type satisifying the btcjson.Cmd interface.
// This is used when registering the custom command with the btcjson
// parser.
func parseCreateEncryptedWalletCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if len(r.Params) != 3 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter must be a string")
	}
	description, ok := r.Params[1].(string)
	if !ok {
		return nil, errors.New("second parameter is not a string")
	}
	passphrase, ok := r.Params[2].(string)
	if !ok {
		return nil, errors.New("third parameter is not a string")
	}

	cmd := NewCreateEncryptedWalletCmd(r.Id, account, description,
		passphrase)
	return cmd, nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *CreateEncryptedWalletCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *CreateEncryptedWalletCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *CreateEncryptedWalletCmd) Method() string {
	return "createencryptedwallet"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *CreateEncryptedWalletCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "createencryptedwallet",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Account,
			cmd.Description,
			cmd.Passphrase,
		},
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

	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("account must be a string")
	}
	return NewWalletIsLockedCmd(r.Id, account)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *WalletIsLockedCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *WalletIsLockedCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *WalletIsLockedCmd) Method() string {
	return "walletislocked"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *WalletIsLockedCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "walletislocked",
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

	iaddrs, ok := r.Params[0].([]interface{})
	if !ok {
		return nil, errors.New("first parameter must be a JSON array")
	}
	addresses := make([]string, len(iaddrs))
	for i := range iaddrs {
		addr, ok := iaddrs[i].(string)
		if !ok {
			return nil, errors.New("first parameter must be an " +
				"array of strings")
		}
		addresses[i] = addr
	}

	if len(r.Params) == 1 {
		// No optional parameters.
		return NewListAddressTransactionsCmd(r.Id, addresses)
	}

	account, ok := r.Params[1].(string)
	if !ok {
		return nil, errors.New("second parameter must be a string")
	}
	return NewListAddressTransactionsCmd(r.Id, addresses, account)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *ListAddressTransactionsCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *ListAddressTransactionsCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *ListAddressTransactionsCmd) Method() string {
	return "listaddresstransactions"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAddressTransactionsCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  cmd.Method(),
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Addresses,
		},
	}

	if cmd.Account != "" {
		raw.Params = append(raw.Params, cmd.Account)
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

	if len(r.Params) == 0 {
		return NewListAllTransactionsCmd(r.Id)
	}

	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("account must be a string")
	}
	return NewListAllTransactionsCmd(r.Id, account)
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *ListAllTransactionsCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *ListAllTransactionsCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *ListAllTransactionsCmd) Method() string {
	return "listalltransactions"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *ListAllTransactionsCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "listalltransactions",
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

// GetAddressBalanceCmd is a type handling custom marshaling
// and unmarshaling of getaddressbalance JSON websocket extension
// commands.
type GetAddressBalanceCmd struct {
	id      interface{}
	Address string
	Minconf int
}

// Enforce that GetAddressBalanceCmd satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &GetAddressBalanceCmd{}

// parseGetAddressBalanceCmd parses a GetAddressBalanceCmd into a concrete
// type satisifying the btcjson.Cmd interface.  This is used when
// registering the custom command with the btcjson parser.
func parseGetAddressBalanceCmd(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	// Length of param slice must be minimum 1 (one required parameter)
	// and maximum 2 (1 optional parameter).
	if len(r.Params) < 1 || len(r.Params) > 2 {
		return nil, btcjson.ErrInvalidParams
	}

	address, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("address must be a string")
	}

	if len(r.Params) == 1 {
		// No optional params.
		return NewGetAddressBalanceCmd(r.Id, address)
	}

	// 1 optional param for minconf.
	fminConf, ok := r.Params[1].(float64)
	if !ok {
		return nil, errors.New("first optional parameter minconf must be a number")
	}
	return NewGetAddressBalanceCmd(r.Id, address, int(fminConf))
}

// NewGetAddressBalanceCmd creates a new GetAddressBalanceCmd.
func NewGetAddressBalanceCmd(id interface{}, address string,
	optArgs ...int) (*GetAddressBalanceCmd, error) {

	// Optional arguments set to their default values.
	minconf := 1

	if len(optArgs) > 1 {
		return nil, btcjson.ErrInvalidParams
	}

	if len(optArgs) == 1 {
		minconf = optArgs[0]
	}

	return &GetAddressBalanceCmd{
		id:      id,
		Address: address,
		Minconf: minconf,
	}, nil
}

// Id satisifies the Cmd interface by returning the ID of the command.
func (cmd *GetAddressBalanceCmd) Id() interface{} {
	return cmd.id
}

// SetId satisifies the Cmd interface by setting the ID of the command.
func (cmd *GetAddressBalanceCmd) SetId(id interface{}) {
	cmd.id = id
}

// Method satisfies the Cmd interface by returning the RPC method.
func (cmd *GetAddressBalanceCmd) Method() string {
	return "getaddressbalance"
}

// MarshalJSON returns the JSON encoding of cmd.  Part of the Cmd interface.
func (cmd *GetAddressBalanceCmd) MarshalJSON() ([]byte, error) {
	// Fill a RawCmd and marshal.
	raw := btcjson.RawCmd{
		Jsonrpc: "1.0",
		Method:  "getaddressbalance",
		Id:      cmd.id,
		Params: []interface{}{
			cmd.Address,
		},
	}

	if cmd.Minconf != 1 {
		raw.Params = append(raw.Params, cmd.Minconf)
	}

	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of cmd into cmd.  Part of
// the Cmd interface.
func (cmd *GetAddressBalanceCmd) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newCmd, err := parseListAllTransactionsCmd(&r)
	if err != nil {
		return err
	}

	concreteCmd, ok := newCmd.(*GetAddressBalanceCmd)
	if !ok {
		return btcjson.ErrInternal
	}
	*cmd = *concreteCmd
	return nil
}
