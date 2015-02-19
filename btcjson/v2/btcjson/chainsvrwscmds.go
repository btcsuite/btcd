// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server, but are only available via websockets.

package btcjson

import (
	"github.com/btcsuite/btcd/wire"
)

// AuthenticateCmd defines the authenticate JSON-RPC command.
type AuthenticateCmd struct {
	Username   string
	Passphrase string
}

// NewAuthenticateCmd returns a new instance which can be used to issue an
// authenticate JSON-RPC command.
func NewAuthenticateCmd(username, passphrase string) *AuthenticateCmd {
	return &AuthenticateCmd{
		Username:   username,
		Passphrase: passphrase,
	}
}

// NotifyBlocksCmd defines the notifyblocks JSON-RPC command.
type NotifyBlocksCmd struct{}

// NewNotifyBlocksCmd returns a new instance which can be used to issue a
// notifyblocks JSON-RPC command.
func NewNotifyBlocksCmd() *NotifyBlocksCmd {
	return &NotifyBlocksCmd{}
}

// NotifyNewTransactionsCmd defines the notifynewtransactions JSON-RPC command.
type NotifyNewTransactionsCmd struct {
	Verbose *bool `jsonrpcdefault:"false"`
}

// NewNotifyNewTransactionsCmd returns a new instance which can be used to issue
// a notifynewtransactions JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewNotifyNewTransactionsCmd(verbose *bool) *NotifyNewTransactionsCmd {
	return &NotifyNewTransactionsCmd{
		Verbose: verbose,
	}
}

// NotifyReceivedCmd defines the notifyreceived JSON-RPC command.
type NotifyReceivedCmd struct {
	Addresses []string
}

// NewNotifyReceivedCmd returns a new instance which can be used to issue a
// notifyreceived JSON-RPC command.
func NewNotifyReceivedCmd(addresses []string) *NotifyReceivedCmd {
	return &NotifyReceivedCmd{
		Addresses: addresses,
	}
}

// OutPoint describes a transaction outpoint that will be marshalled to and
// from JSON.
type OutPoint struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"index"`
}

// NewOutPointFromWire creates a new OutPoint from the OutPoint structure
// of the btcwire package.
func NewOutPointFromWire(op *wire.OutPoint) *OutPoint {
	return &OutPoint{
		Hash:  op.Hash.String(),
		Index: op.Index,
	}
}

// NotifySpentCmd defines the notifyspent JSON-RPC command.
type NotifySpentCmd struct {
	OutPoints []OutPoint
}

// NewNotifySpentCmd returns a new instance which can be used to issue a
// notifyspent JSON-RPC command.
func NewNotifySpentCmd(outPoints []OutPoint) *NotifySpentCmd {
	return &NotifySpentCmd{
		OutPoints: outPoints,
	}
}

// RescanCmd defines the rescan JSON-RPC command.
type RescanCmd struct {
	BeginBlock string
	Addresses  []string
	OutPoints  []OutPoint
	EndBlock   *string
}

// NewRescanCmd returns a new instance which can be used to issue a rescan
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewRescanCmd(beginBlock string, addresses []string, outPoints []OutPoint, endBlock *string) *RescanCmd {
	return &RescanCmd{
		BeginBlock: beginBlock,
		Addresses:  addresses,
		OutPoints:  outPoints,
		EndBlock:   endBlock,
	}
}

func init() {
	// The commands in this file are only usable by websockets.
	flags := UFWebsocketOnly

	MustRegisterCmd("authenticate", (*AuthenticateCmd)(nil), flags)
	MustRegisterCmd("notifyblocks", (*NotifyBlocksCmd)(nil), flags)
	MustRegisterCmd("notifynewtransactions", (*NotifyNewTransactionsCmd)(nil), flags)
	MustRegisterCmd("notifyreceived", (*NotifyReceivedCmd)(nil), flags)
	MustRegisterCmd("notifyspent", (*NotifySpentCmd)(nil), flags)
	MustRegisterCmd("rescan", (*RescanCmd)(nil), flags)
}
