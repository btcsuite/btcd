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
	// ErrNtfnUnexpected describes an error where an unexpected
	// notification is received when unmarshaling into a concrete
	// Notification variable.
	ErrNtfnUnexpected = errors.New("notification unexpected")

	// ErrNtfnNotFound describes an error where a parser does not
	// handle unmarshaling a notification.
	ErrNtfnNotFound = errors.New("notification not found")
)

const (
	// BlockConnectedNtfnId is the id of the btcd blockconnected
	// notification.
	BlockConnectedNtfnId = "btcd:blockconnected"

	// BlockDisconnectedNtfnId is the id of the btcd blockdisconnected
	// notification.
	BlockDisconnectedNtfnId = "btcd:blockdisconnected"

	//TxMinedNtfnId is the id of the btcd txmined notification.
	TxMinedNtfnId = "btcd:txmined"
)

type newNtfnFn func() Notification

func newBlockConnectedNtfn() Notification {
	return &BlockConnectedNtfn{}
}

func newBlockDisconnectedNtfn() Notification {
	return &BlockDisconnectedNtfn{}
}

func newTxMinedNtfn() Notification {
	return &TxMinedNtfn{}
}

var newNtfnFns = map[string]newNtfnFn{
	BlockConnectedNtfnId:    newBlockConnectedNtfn,
	BlockDisconnectedNtfnId: newBlockDisconnectedNtfn,
	TxMinedNtfnId:           newTxMinedNtfn,
}

// ParseMarshaledNtfn attempts to unmarshal a marshaled notification
// into the notification described by id.
func ParseMarshaledNtfn(id string, b []byte) (Notification, error) {
	if newFn, ok := newNtfnFns[id]; ok {
		n := newFn()
		if err := n.UnmarshalJSON(b); err != nil {
			return nil, err
		}
		return n, nil
	}
	return nil, ErrNtfnNotFound
}

// Notification is an interface implemented by all notification types.
type Notification interface {
	json.Marshaler
	json.Unmarshaler
	Id() interface{}
}

// BlockConnectedNtfn is a type handling custom marshaling and
// unmarshaling of blockconnected JSON websocket notifications.
type BlockConnectedNtfn struct {
	Hash   string `json:"hash"`
	Height int32  `json:"height"`
}

type blockConnectedResult BlockConnectedNtfn

// Enforce that BlockConnectedNtfn satisfies the Notification interface.
var _ Notification = &BlockConnectedNtfn{}

// NewBlockConnectedNtfn creates a new BlockConnectedNtfn.
func NewBlockConnectedNtfn(hash string, height int32) *BlockConnectedNtfn {
	return &BlockConnectedNtfn{
		Hash:   hash,
		Height: height,
	}
}

// Id satisifies the Notification interface by returning the id of the
// notification.
func (n *BlockConnectedNtfn) Id() interface{} {
	return BlockConnectedNtfnId
}

// MarshalJSON returns the JSON encoding of n.  Part of the Notification
// interface.
func (n *BlockConnectedNtfn) MarshalJSON() ([]byte, error) {
	id := n.Id()
	reply := btcjson.Reply{
		Result: *n,
		Id:     &id,
	}
	return json.Marshal(reply)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the Notification interface.
func (n *BlockConnectedNtfn) UnmarshalJSON(b []byte) error {
	var ntfn struct {
		Result blockConnectedResult `json:"result"`
		Error  *btcjson.Error       `json:"error"`
		Id     interface{}          `json:"id"`
	}
	if err := json.Unmarshal(b, &ntfn); err != nil {
		return err
	}

	// Notification IDs must match expected.
	if n.Id() != ntfn.Id {
		return ErrNtfnUnexpected
	}

	*n = BlockConnectedNtfn(ntfn.Result)
	return nil
}

// BlockDisconnectedNtfn is a type handling custom marshaling and
// unmarshaling of blockdisconnected JSON websocket notifications.
type BlockDisconnectedNtfn struct {
	Hash   string `json:"hash"`
	Height int32  `json:"height"`
}

type blockDisconnectedResult BlockDisconnectedNtfn

// Enforce that BlockDisconnectedNtfn satisfies the Notification interface.
var _ Notification = &BlockDisconnectedNtfn{}

// NewBlockDisconnectedNtfn creates a new BlockConnectedNtfn.
func NewBlockDisconnectedNtfn(hash string, height int32) *BlockDisconnectedNtfn {
	return &BlockDisconnectedNtfn{
		Hash:   hash,
		Height: height,
	}
}

// Id satisifies the Notification interface by returning the id of the
// notification.
func (n *BlockDisconnectedNtfn) Id() interface{} {
	return BlockDisconnectedNtfnId
}

// MarshalJSON returns the JSON encoding of n.  Part of the Notification
// interface.
func (n *BlockDisconnectedNtfn) MarshalJSON() ([]byte, error) {
	id := n.Id()
	reply := btcjson.Reply{
		Result: *n,
		Id:     &id,
	}
	return json.Marshal(reply)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the Notification interface.
func (n *BlockDisconnectedNtfn) UnmarshalJSON(b []byte) error {
	var ntfn struct {
		Result blockDisconnectedResult `json:"result"`
		Error  *btcjson.Error          `json:"error"`
		Id     interface{}             `json:"id"`
	}
	if err := json.Unmarshal(b, &ntfn); err != nil {
		return err
	}

	// Notification IDs must match expected.
	if n.Id() != ntfn.Id {
		return ErrNtfnUnexpected
	}

	*n = BlockDisconnectedNtfn(ntfn.Result)
	return nil
}

// TxMinedNtfn is a type handling custom marshaling and
// unmarshaling of txmined JSON websocket notifications.
type TxMinedNtfn struct {
	Hash string `json:"hash"`
}

type txMinedResult TxMinedNtfn

// Enforce that TxMinedNtfn satisfies the Notification interface.
var _ Notification = &TxMinedNtfn{}

// NewTxMinedNtfn creates a new TxMinedNtfn.
func NewTxMinedNtfn(hash string) *TxMinedNtfn {
	return &TxMinedNtfn{Hash: hash}
}

// Id satisifies the Notification interface by returning the id of the
// notification.
func (n *TxMinedNtfn) Id() interface{} {
	return TxMinedNtfnId
}

// MarshalJSON returns the JSON encoding of n.  Part of the Notification
// interface.
func (n *TxMinedNtfn) MarshalJSON() ([]byte, error) {
	id := n.Id()
	reply := btcjson.Reply{
		Result: *n,
		Id:     &id,
	}
	return json.Marshal(reply)
}

// UnmarshalJSON unmarshals the JSON encoding of n into n.  Part of
// the Notification interface.
func (n *TxMinedNtfn) UnmarshalJSON(b []byte) error {
	var ntfn struct {
		Result txMinedResult  `json:"result"`
		Error  *btcjson.Error `json:"error"`
		Id     interface{}    `json:"id"`
	}
	if err := json.Unmarshal(b, &ntfn); err != nil {
		return err
	}

	// Notification IDs must match expected.
	if n.Id() != ntfn.Id {
		return ErrNtfnUnexpected
	}

	*n = TxMinedNtfn(ntfn.Result)
	return nil
}
