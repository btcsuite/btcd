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

	// BlockConnectedNtfnMethod is the method of the btcd
	// blockconnected notification.
	BlockConnectedNtfnMethod = "blockconnected"

	// BlockDisconnectedNtfnMethod is the method of the btcd
	// blockdisconnected notification.
	BlockDisconnectedNtfnMethod = "blockdisconnected"

	// BtcdConnectedNtfnMethod is the method of the btcwallet
	// btcdconnected notification.
	BtcdConnectedNtfnMethod = "btcdconnected"

	// TxMinedNtfnMethod is the method of the btcd txmined
	// notification.
	TxMinedNtfnMethod = "txmined"

	// TxNtfnMethod is the method of the btcwallet newtx
	// notification.
	TxNtfnMethod = "newtx"

	// WalletLockStateNtfnMethod is the method of the btcwallet
	// walletlockstate notification.
	WalletLockStateNtfnMethod = "walletlockstate"
)

// Register notifications with btcjson.
func init() {
	btcjson.RegisterCustomCmd(AccountBalanceNtfnMethod, parseAccountBalanceNtfn)
	btcjson.RegisterCustomCmd(BlockConnectedNtfnMethod, parseBlockConnectedNtfn)
	btcjson.RegisterCustomCmd(BlockDisconnectedNtfnMethod, parseBlockDisconnectedNtfn)
	btcjson.RegisterCustomCmd(BtcdConnectedNtfnMethod, parseBtcdConnectedNtfn)
	btcjson.RegisterCustomCmd(TxMinedNtfnMethod, parseTxMinedNtfn)
	btcjson.RegisterCustomCmd(TxNtfnMethod, parseTxNtfn)
	btcjson.RegisterCustomCmd(WalletLockStateNtfnMethod, parseWalletLockStateNtfn)
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

	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter account must be a string")
	}
	balance, ok := r.Params[1].(float64)
	if !ok {
		return nil, errors.New("second parameter balance must be a number")
	}
	confirmed, ok := r.Params[2].(bool)
	if !ok {
		return nil, errors.New("third parameter confirmed must be a boolean")
	}

	return NewAccountBalanceNtfn(account, balance, confirmed), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *AccountBalanceNtfn) Id() interface{} {
	return nil
}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *AccountBalanceNtfn) Method() string {
	return AccountBalanceNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *AccountBalanceNtfn) MarshalJSON() ([]byte, error) {
	ntfn := btcjson.Message{
		Jsonrpc: "1.0",
		Method:  n.Method(),
		Params: []interface{}{
			n.Account,
			n.Balance,
			n.Confirmed,
		},
	}
	return json.Marshal(ntfn)
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

	hash, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter hash must be a string")
	}
	fheight, ok := r.Params[1].(float64)
	if !ok {
		return nil, errors.New("second parameter height must be a number")
	}

	return NewBlockConnectedNtfn(hash, int32(fheight)), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *BlockConnectedNtfn) Id() interface{} {
	return nil
}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *BlockConnectedNtfn) Method() string {
	return BlockConnectedNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *BlockConnectedNtfn) MarshalJSON() ([]byte, error) {
	ntfn := btcjson.Message{
		Jsonrpc: "1.0",
		Method:  n.Method(),
		Params: []interface{}{
			n.Hash,
			n.Height,
		},
	}
	return json.Marshal(ntfn)
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

	hash, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter hash must be a string")
	}
	fheight, ok := r.Params[1].(float64)
	if !ok {
		return nil, errors.New("second parameter height must be a number")
	}

	return NewBlockDisconnectedNtfn(hash, int32(fheight)), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *BlockDisconnectedNtfn) Id() interface{} {
	return nil
}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *BlockDisconnectedNtfn) Method() string {
	return BlockDisconnectedNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *BlockDisconnectedNtfn) MarshalJSON() ([]byte, error) {
	ntfn := btcjson.Message{
		Jsonrpc: "1.0",
		Method:  n.Method(),
		Params: []interface{}{
			n.Hash,
			n.Height,
		},
	}
	return json.Marshal(ntfn)
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

	connected, ok := r.Params[0].(bool)
	if !ok {
		return nil, errors.New("first parameter connected is not a boolean")
	}

	return NewBtcdConnectedNtfn(connected), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *BtcdConnectedNtfn) Id() interface{} {
	return nil
}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *BtcdConnectedNtfn) Method() string {
	return BtcdConnectedNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *BtcdConnectedNtfn) MarshalJSON() ([]byte, error) {
	ntfn := btcjson.Message{
		Jsonrpc: "1.0",
		Method:  n.Method(),
		Params: []interface{}{
			n.Connected,
		},
	}
	return json.Marshal(ntfn)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *BtcdConnectedNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseTxMinedNtfn(&r)
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

// TxMinedNtfn is a type handling custom marshaling and
// unmarshaling of txmined JSON websocket notifications.
type TxMinedNtfn struct {
	TxID        string
	BlockHash   string
	BlockHeight int32
	BlockTime   int64
	Index       int
}

// Enforce that TxMinedNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &TxMinedNtfn{}

// NewTxMinedNtfn creates a new TxMinedNtfn.
func NewTxMinedNtfn(txid, blockhash string, blockheight int32,
	blocktime int64, index int) *TxMinedNtfn {

	return &TxMinedNtfn{
		TxID:        txid,
		BlockHash:   blockhash,
		BlockHeight: blockheight,
		BlockTime:   blocktime,
		Index:       index,
	}
}

// parseTxMinedNtfn parses a RawCmd into a concrete type satisifying
// the btcjson.Cmd interface.  This is used when registering the notification
// with the btcjson parser.
func parseTxMinedNtfn(r *btcjson.RawCmd) (btcjson.Cmd, error) {
	if r.Id != nil {
		return nil, ErrNotANtfn
	}

	if len(r.Params) != 5 {
		return nil, btcjson.ErrWrongNumberOfParams
	}

	txid, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter txid must be a string")
	}
	blockhash, ok := r.Params[1].(string)
	if !ok {
		return nil, errors.New("second parameter blockhash must be a string")
	}
	fblockheight, ok := r.Params[2].(float64)
	if !ok {
		return nil, errors.New("third parameter blockheight must be a number")
	}
	fblocktime, ok := r.Params[3].(float64)
	if !ok {
		return nil, errors.New("fourth parameter blocktime must be a number")
	}
	findex, ok := r.Params[4].(float64)
	if !ok {
		return nil, errors.New("fifth parameter index must be a number")
	}

	return NewTxMinedNtfn(txid, blockhash, int32(fblockheight),
		int64(fblocktime), int(findex)), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *TxMinedNtfn) Id() interface{} {
	return nil
}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *TxMinedNtfn) Method() string {
	return TxMinedNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *TxMinedNtfn) MarshalJSON() ([]byte, error) {
	ntfn := btcjson.Message{
		Jsonrpc: "1.0",
		Method:  n.Method(),
		Params: []interface{}{
			n.TxID,
			n.BlockHash,
			n.BlockHeight,
			n.BlockTime,
			n.Index,
		},
	}
	return json.Marshal(ntfn)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the btcjson.Cmd interface.
func (n *TxMinedNtfn) UnmarshalJSON(b []byte) error {
	// Unmarshal into a RawCmd.
	var r btcjson.RawCmd
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}

	newNtfn, err := parseTxMinedNtfn(&r)
	if err != nil {
		return err
	}

	concreteNtfn, ok := newNtfn.(*TxMinedNtfn)
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
	Details map[string]interface{}
}

// Enforce that TxNtfn satisifies the btcjson.Cmd interface.
var _ btcjson.Cmd = &TxNtfn{}

// NewTxNtfn creates a new TxNtfn.
func NewTxNtfn(account string, details map[string]interface{}) *TxNtfn {
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

	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter account must be a string")
	}
	details, ok := r.Params[1].(map[string]interface{})
	if !ok {
		return nil, errors.New("second parameter details must be a JSON object")
	}

	return NewTxNtfn(account, details), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *TxNtfn) Id() interface{} {
	return nil
}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *TxNtfn) Method() string {
	return TxNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *TxNtfn) MarshalJSON() ([]byte, error) {
	ntfn := btcjson.Message{
		Jsonrpc: "1.0",
		Method:  n.Method(),
		Params: []interface{}{
			n.Account,
			n.Details,
		},
	}
	return json.Marshal(ntfn)
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

	account, ok := r.Params[0].(string)
	if !ok {
		return nil, errors.New("first parameter account must be a string")
	}
	locked, ok := r.Params[1].(bool)
	if !ok {
		return nil, errors.New("second parameter locked must be a boolean")
	}

	return NewWalletLockStateNtfn(account, locked), nil
}

// Id satisifies the btcjson.Cmd interface by returning nil for a
// notification ID.
func (n *WalletLockStateNtfn) Id() interface{} {
	return nil
}

// Method satisifies the btcjson.Cmd interface by returning the method
// of the notification.
func (n *WalletLockStateNtfn) Method() string {
	return WalletLockStateNtfnMethod
}

// MarshalJSON returns the JSON encoding of n.  Part of the btcjson.Cmd
// interface.
func (n *WalletLockStateNtfn) MarshalJSON() ([]byte, error) {
	ntfn := btcjson.Message{
		Jsonrpc: "1.0",
		Method:  n.Method(),
		Params: []interface{}{
			n.Account,
			n.Locked,
		},
	}
	return json.Marshal(ntfn)
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
