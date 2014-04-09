// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcws

import (
	"encoding/json"
	"errors"
	"github.com/conformal/btcjson"
)

var (
	// ErrNotANtfn describes an error where a JSON-RPC Request
	// object cannot be successfully parsed as a notification
	// due to having an ID.
	ErrNotANtfn = errors.New("notifications may not have IDs")
)

const (
	// AccountBalanceNtfnMethod is the method of the btcwallet
	// accountbalance notification.
	AccountBalanceNtfnMethod = "accountbalance"

	// AllTxNtfnMethod is the method of the btcd alltx
	// notification
	AllTxNtfnMethod = "alltx"

	// AllVerboseTxNtfnMethod is the method of the btcd
	// allverbosetx notifications.
	AllVerboseTxNtfnMethod = "allverbosetx"

	// BlockConnectedNtfnMethod is the method of the btcd
	// blockconnected notification.
	BlockConnectedNtfnMethod = "blockconnected"

	// BlockDisconnectedNtfnMethod is the method of the btcd
	// blockdisconnected notification.
	BlockDisconnectedNtfnMethod = "blockdisconnected"

	// BtcdConnectedNtfnMethod is the method of the btcwallet
	// btcdconnected notification.
	BtcdConnectedNtfnMethod = "btcdconnected"

	// RecvTxNtfnMethod is the method of the btcd recvtx notification.
	RecvTxNtfnMethod = "recvtx"

	// TxNtfnMethod is the method of the btcwallet newtx
	// notification.
	TxNtfnMethod = "newtx"

	// RedeemingTxNtfnMethod is the method of the btcd redeemingtx
	// notification.
	RedeemingTxNtfnMethod = "redeemingtx"

	// RescanProgressNtfnMethod is the method of the btcd rescanprogress
	// notification.
	RescanProgressNtfnMethod = "rescanprogress"

	// WalletLockStateNtfnMethod is the method of the btcwallet
	// walletlockstate notification.
	WalletLockStateNtfnMethod = "walletlockstate"
)

// Register notifications with btcjson.
func init() {
	btcjson.RegisterCustomCmd(AccountBalanceNtfnMethod,
		parseAccountBalanceNtfn, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(BlockConnectedNtfnMethod,
		parseBlockConnectedNtfn, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(BlockDisconnectedNtfnMethod,
		parseBlockDisconnectedNtfn, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(BtcdConnectedNtfnMethod,
		parseBtcdConnectedNtfn, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(RecvTxNtfnMethod,
		parseRecvTxNtfn, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(RescanProgressNtfnMethod,
		parseRescanProgressNtfn, `TODO(jrick) fillmein`)

	btcjson.RegisterCustomCmd(RedeemingTxNtfnMethod, parseRedeemingTxNtfn,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(TxNtfnMethod, parseTxNtfn,
		`TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(WalletLockStateNtfnMethod,
		parseWalletLockStateNtfn, `TODO(jrick) fillmein`)
	btcjson.RegisterCustomCmd(AllTxNtfnMethod,
		parseAllTxNtfn, `TODO(flam) fillmein`)
	btcjson.RegisterCustomCmd(AllVerboseTxNtfnMethod,
		parseAllVerboseTxNtfn, `TODO(flam) fillmein`)
}

// BlockDetails describes details of a tx in a block.
type BlockDetails struct {
	Height int32  `json:"height"`
	Hash   string `json:"hash"`
	Index  int    `json:"index"`
	Time   int64  `json:"time"`
}

// AccountBalanceNtfn is a type handling custom marshaling and
// unmarshaling of accountbalance JSON websocket notifications.
type AccountBalanceNtfn struct {
	Account   string
	Balance   float64
	Confirmed bool // Whether Balance is confirmed or unconfirmed.
}

// Enforce that AccountBalanceNtfn satisifes the btcjson.Cmd interface.
var _ btcjson.Cmd = &AccountBalanceNtfn{}

// NewAccountBalanceNtfn creates a new AccountBalanceNtfn.
func NewAccountBalanceNtfn(account string, balance float64,
	confirmed bool) *AccountBalanceNtfn {

	return &AccountBalanceNtfn{
		Account:   account,
		Balance:   balance,
		Confirmed: confirmed,
	}
}

// parseAccountBalanceNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseAccountBalanceNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 3 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return nil, errors.New("first parameter 'account' must be a string: " + err.Error())
	}

	var balance float64
	if err := json.Unmarshal(r.Params[1], &balance); err != nil {
		return nil, errors.New("second parameter 'balance' must be a number: " + err.Error())
	}

	var confirmed bool
	if err := json.Unmarshal(r.Params[2], &confirmed); err != nil {
		return nil, errors.New("third parameter 'confirmed' must be a bool: " + err.Error())
	}

	return NewAccountBalanceNtfn(account, balance, confirmed), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *AccountBalanceNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *AccountBalanceNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *AccountBalanceNtfn) Method() string {
	return AccountBalanceNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *AccountBalanceNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.Account,
		n.Balance,
		n.Confirmed,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *AccountBalanceNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseAccountBalanceNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*AccountBalanceNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// BlockConnectedNtfn is a type handling custom marshaling and
// unmarshaling of blockconnected JSON websocket notifications.
type BlockConnectedNtfn struct {
	Hash   string
	Height int32
}

// Enforce that BlockConnectedNtfn satisfies the btcjson.Cmd interface.
var _ btcjson.Cmd = &BlockConnectedNtfn{}

// NewBlockConnectedNtfn creates a new BlockConnectedNtfn.
func NewBlockConnectedNtfn(hash string, height int32) *BlockConnectedNtfn {
	return &BlockConnectedNtfn{
		Hash:   hash,
		Height: height,
	}
}

// parseBlockConnectedNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseBlockConnectedNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var hash string
	if err := json.Unmarshal(r.Params[0], &hash); err != nil {
		return nil, errors.New("first parameter 'hash' must be a string: " + err.Error())
	}

	var height int32
	if err := json.Unmarshal(r.Params[1], &height); err != nil {
		return nil, errors.New("second parameter 'height' must be a 32-bit integer: " + err.Error())
	}

	return NewBlockConnectedNtfn(hash, height), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *BlockConnectedNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *BlockConnectedNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *BlockConnectedNtfn) Method() string {
	return BlockConnectedNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *BlockConnectedNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.Hash,
		n.Height,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *BlockConnectedNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseBlockConnectedNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*BlockConnectedNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// BlockDisconnectedNtfn is a type handling custom marshaling and
// unmarshaling of blockdisconnected JSON websocket notifications.
type BlockDisconnectedNtfn struct {
	Hash   string
	Height int32
}

// Enforce that BlockDisconnectedNtfn satisfies the btcjson.Cmd interface.
var _ btcjson.Cmd = &BlockDisconnectedNtfn{}

// NewBlockDisconnectedNtfn creates a new BlockDisconnectedNtfn.
func NewBlockDisconnectedNtfn(hash string, height int32) *BlockDisconnectedNtfn {
	return &BlockDisconnectedNtfn{
		Hash:   hash,
		Height: height,
	}
}

// parseBlockDisconnectedNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseBlockDisconnectedNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var hash string
	if err := json.Unmarshal(r.Params[0], &hash); err != nil {
		return nil, errors.New("first parameter 'hash' must be a string: " + err.Error())
	}

	var height int32
	if err := json.Unmarshal(r.Params[1], &height); err != nil {
		return nil, errors.New("second parameter 'height' must be a 32-bit integer: " + err.Error())
	}

	return NewBlockDisconnectedNtfn(hash, height), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *BlockDisconnectedNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *BlockDisconnectedNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *BlockDisconnectedNtfn) Method() string {
	return BlockDisconnectedNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *BlockDisconnectedNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.Hash,
		n.Height,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *BlockDisconnectedNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseBlockDisconnectedNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*BlockDisconnectedNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// BtcdConnectedNtfn is a type handling custom marshaling and
// unmarshaling of btcdconnected JSON websocket notifications.
type BtcdConnectedNtfn struct {
	Connected bool
}

// Enforce that BtcdConnectedNtfn satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &BtcdConnectedNtfn{}

// NewBtcdConnectedNtfn creates a new BtcdConnectedNtfn.
func NewBtcdConnectedNtfn(connected bool) *BtcdConnectedNtfn {
	return &BtcdConnectedNtfn{connected}
}

// parseBtcdConnectedNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseBtcdConnectedNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var connected bool
	if err := json.Unmarshal(r.Params[0], &connected); err != nil {
		return nil, errors.New("first parameter 'connected' must be a bool: " + err.Error())
	}

	return NewBtcdConnectedNtfn(connected), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *BtcdConnectedNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *BtcdConnectedNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *BtcdConnectedNtfn) Method() string {
	return BtcdConnectedNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *BtcdConnectedNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.Connected,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *BtcdConnectedNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseBtcdConnectedNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*BtcdConnectedNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// RecvTxNtfn is a type handling custom marshaling and unmarshaling
// of recvtx JSON websocket notifications.
type RecvTxNtfn struct {
	HexTx string
	Block *BlockDetails
}

// Enforce that RecvTxNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &RecvTxNtfn{}

// NewRecvTxNtfn creates a new RecvTxNtfn.
func NewRecvTxNtfn(hextx string, block *BlockDetails) *RecvTxNtfn {
	return &RecvTxNtfn{
		HexTx: hextx,
		Block: block,
	}
}

// parsRecvTxNtfn parses a RawCmd into a concrete type satisifying the
// btcjson.Cmd interface.  This is used when registering the notification with
// the btcjson parser.
func parseRecvTxNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	// Must have one or two parameters.
	if len(r.Params) == 0 || len(r.Params) > 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var hextx string
	if err := json.Unmarshal(r.Params[0], &hextx); err != nil {
		return nil, errors.New("first parameter 'hextx' must be a " +
			"string: " + err.Error())
	}

	var blockDetails *BlockDetails
	if len(r.Params) > 1 {
		if err := json.Unmarshal(r.Params[1], &blockDetails); err != nil {
			return nil, errors.New("second optional parameter " +
				"'details' must be a JSON oject of block " +
				"details: " + err.Error())
		}
	}

	return NewRecvTxNtfn(hextx, blockDetails), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *RecvTxNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *RecvTxNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *RecvTxNtfn) Method() string {
	return RecvTxNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *RecvTxNtfn) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = n.HexTx
	if n.Block != nil {
		params = append(params, n.Block)
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *RecvTxNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseRecvTxNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*RecvTxNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// RedeemingTxNtfn is a type handling custom marshaling and unmarshaling
// of redeemingtx JSON websocket notifications.
type RedeemingTxNtfn struct {
	HexTx string
	Block *BlockDetails
}

// Enforce that RedeemingTxNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &RedeemingTxNtfn{}

// NewRedeemingTxNtfn creates a new RedeemingTxNtfn.
func NewRedeemingTxNtfn(hextx string, block *BlockDetails) *RedeemingTxNtfn {
	return &RedeemingTxNtfn{
		HexTx: hextx,
		Block: block,
	}
}

// parseRedeemingTxNtfn parses a RawCmd into a concrete type satisifying the
// btcjson.Cmd interface.  This is used when registering the notification with
// the btcjson parser.
func parseRedeemingTxNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	// Must have one or two parameters.
	if len(r.Params) == 0 || len(r.Params) > 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var hextx string
	if err := json.Unmarshal(r.Params[0], &hextx); err != nil {
		return nil, errors.New("first parameter 'hextx' must be a " +
			"string: " + err.Error())
	}

	var blockDetails *BlockDetails
	if len(r.Params) > 1 {
		if err := json.Unmarshal(r.Params[1], &blockDetails); err != nil {
			return nil, errors.New("second optional parameter " +
				"'details' must be a JSON oject of block " +
				"details: " + err.Error())
		}
	}

	return NewRedeemingTxNtfn(hextx, blockDetails), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *RedeemingTxNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *RedeemingTxNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *RedeemingTxNtfn) Method() string {
	return RedeemingTxNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *RedeemingTxNtfn) MarshalJSON() ([]byte, error) {
	params := make([]interface{}, 1, 2)
	params[0] = n.HexTx
	if n.Block != nil {
		params = append(params, n.Block)
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *RedeemingTxNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseRedeemingTxNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*RedeemingTxNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// RescanProgressNtfn is type handling custom marshaling and
// unmarshaling of rescanprogress JSON websocket notifications.
type RescanProgressNtfn struct {
	LastProcessed int32
}

// Enforce that RescanProgressNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &RescanProgressNtfn{}

// NewRescanProgressNtfn creates a new RescanProgressNtfn.
func NewRescanProgressNtfn(last int32) *RescanProgressNtfn {
	return &RescanProgressNtfn{last}
}

// parseRescanProgressNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseRescanProgressNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var last int32
	if err := json.Unmarshal(r.Params[0], &last); err != nil {
		return nil, errors.New("first parameter 'last' must be a " +
			"32-bit integer: " + err.Error())
	}

	return NewRescanProgressNtfn(last), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *RescanProgressNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification ID is not modified.
func (n *RescanProgressNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *RescanProgressNtfn) Method() string {
	return RescanProgressNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *RescanProgressNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.LastProcessed,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *RescanProgressNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseRescanProgressNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*RescanProgressNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// TxNtfn is a type handling custom marshaling and
// unmarshaling of newtx JSON websocket notifications.
type TxNtfn struct {
	Account string
	Details *btcjson.ListTransactionsResult
}

// Enforce that TxNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &TxNtfn{}

// NewTxNtfn creates a new TxNtfn.
func NewTxNtfn(account string, details *btcjson.ListTransactionsResult) *TxNtfn {
	return &TxNtfn{
		Account: account,
		Details: details,
	}
}

// parseTxNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseTxNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return nil, errors.New("first parameter 'account' must be a " +
			"string: " + err.Error())
	}

	var details btcjson.ListTransactionsResult
	if err := json.Unmarshal(r.Params[1], &details); err != nil {
		return nil, errors.New("second parameter 'details' must be a " +
			"JSON object of transaction details: " + err.Error())
	}

	return NewTxNtfn(account, &details), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *TxNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *TxNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *TxNtfn) Method() string {
	return TxNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *TxNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.Account,
		n.Details,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *TxNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseTxNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*TxNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// WalletLockStateNtfn is a type handling custom marshaling and
// unmarshaling of walletlockstate JSON websocket notifications.
type WalletLockStateNtfn struct {
	Account string
	Locked  bool
}

// Enforce that WalletLockStateNtfnMethod satisifies the btcjson.Cmd
// interface.
var _ btcjson.Cmd = &WalletLockStateNtfn{}

// NewWalletLockStateNtfn creates a new WalletLockStateNtfn.
func NewWalletLockStateNtfn(account string,
	locked bool) *WalletLockStateNtfn {

	return &WalletLockStateNtfn{
		Account: account,
		Locked:  locked,
	}
}

// parseWalletLockStateNtfn parses a RawCmd into a concrete type
// satisifying the btcjson.Cmd interface.  This is used when registering
// the notification with the btcjson parser.
func parseWalletLockStateNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var account string
	if err := json.Unmarshal(r.Params[0], &account); err != nil {
		return nil, errors.New("first parameter 'account' must be a " +
			"string: " + err.Error())
	}

	var locked bool
	if err := json.Unmarshal(r.Params[1], &locked); err != nil {
		return nil, errors.New("second parameter 'locked' must be a " +
			"bool: " + err.Error())
	}

	return NewWalletLockStateNtfn(account, locked), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *WalletLockStateNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *WalletLockStateNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *WalletLockStateNtfn) Method() string {
	return WalletLockStateNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *WalletLockStateNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.Account,
		n.Locked,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *WalletLockStateNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseWalletLockStateNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*WalletLockStateNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// AllTxNtfn is a type handling custom marshaling and
// unmarshaling of txmined JSON websocket notifications.
type AllTxNtfn struct {
	TxID   string `json:"txid"`
	Amount int64  `json:"amount"`
}

// Enforce that AllTxNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &AllTxNtfn{}

// NewAllTxNtfn creates a new AllTxNtfn.
func NewAllTxNtfn(txid string, amount int64) *AllTxNtfn {
	return &AllTxNtfn{
		TxID:   txid,
		Amount: amount,
	}
}

// parseAllTxNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseAllTxNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 2 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var txid string
	if err := json.Unmarshal(r.Params[0], &txid); err != nil {
		return nil, errors.New("first parameter 'txid' must be a " +
			"string: " + err.Error())
	}

	var amount int64
	if err := json.Unmarshal(r.Params[1], &amount); err != nil {
		return nil, errors.New("second parameter 'amount' must be an " +
			"integer: " + err.Error())
	}

	return NewAllTxNtfn(txid, amount), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *AllTxNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *AllTxNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *AllTxNtfn) Method() string {
	return AllTxNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *AllTxNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.TxID,
		n.Amount,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *AllTxNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseAllTxNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*AllTxNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}

// AllVerboseTxNtfn is a type handling custom marshaling and
// unmarshaling of txmined JSON websocket notifications.
type AllVerboseTxNtfn struct {
	RawTx *btcjson.TxRawResult `json:"rawtx"`
}

// Enforce that AllTxNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &AllVerboseTxNtfn{}

// NewAllVerboseTxNtfn creates a new AllVerboseTxNtfn.
func NewAllVerboseTxNtfn(rawTx *btcjson.TxRawResult) *AllVerboseTxNtfn {
	return &AllVerboseTxNtfn{
		RawTx: rawTx,
	}
}

// parseAllVerboseTxNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseAllVerboseTxNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 1 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	var rawTx *btcjson.TxRawResult
	if err := json.Unmarshal(r.Params[0], &rawTx); err != nil {
		return nil, err
	}

	return NewAllVerboseTxNtfn(rawTx), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *AllVerboseTxNtfn) Id() interface{} {
	return nil
}

// SetId is implemented to satisify the btcjson.Cmd interface.  The
// notification id is not modified.
func (n *AllVerboseTxNtfn) SetId(id interface{}) {}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *AllVerboseTxNtfn) Method() string {
	return AllVerboseTxNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *AllVerboseTxNtfn) MarshalJSON() ([]byte, error) {
	params := []interface{}{
		n.RawTx,
	}

	// No ID for notifications.
	raw, err := btcjson.NewRawCmd(nil, n.Method(), params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(raw)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *AllVerboseTxNtfn) UnmarshalJSON(b []byte) error {
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseAllVerboseTxNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*AllVerboseTxNtfn)
	if !ok {
		return btcjson.ErrInternal
	}
	*n = *concreteNtfn
	return nil
}
