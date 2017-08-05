// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a wallet server with btcwallet extensions.

package dcrjson

// CreateNewAccountCmd defines the createnewaccount JSON-RPC command.
type CreateNewAccountCmd struct {
	Account string
}

// NewCreateNewAccountCmd returns a new instance which can be used to issue a
// createnewaccount JSON-RPC command.
func NewCreateNewAccountCmd(account string) *CreateNewAccountCmd {
	return &CreateNewAccountCmd{
		Account: account,
	}
}

// ImportAddressCmd defines the importaddress JSON-RPC command.
type ImportAddressCmd struct {
	Address string
	Rescan  *bool `jsonrpcdefault:"true"`
}

// NewImportAddressCmd returns a new instance which can be used to issue an
// importaddress JSON-RPC command.
func NewImportAddressCmd(address string, rescan *bool) *ImportAddressCmd {
	return &ImportAddressCmd{
		Address: address,
		Rescan:  rescan,
	}
}

// ImportPubKeyCmd defines the importpubkey JSON-RPC command.
type ImportPubKeyCmd struct {
	PubKey string
	Rescan *bool `jsonrpcdefault:"true"`
}

// NewImportPubKeyCmd returns a new instance which can be used to issue an
// importpubkey JSON-RPC command.
func NewImportPubKeyCmd(pubKey string, rescan *bool) *ImportPubKeyCmd {
	return &ImportPubKeyCmd{
		PubKey: pubKey,
		Rescan: rescan,
	}
}

// RenameAccountCmd defines the renameaccount JSON-RPC command.
type RenameAccountCmd struct {
	OldAccount string
	NewAccount string
}

// NewRenameAccountCmd returns a new instance which can be used to issue a
// renameaccount JSON-RPC command.
func NewRenameAccountCmd(oldAccount, newAccount string) *RenameAccountCmd {
	return &RenameAccountCmd{
		OldAccount: oldAccount,
		NewAccount: newAccount,
	}
}

func init() {
	// The commands in this file are only usable with a wallet server.
	flags := UFWalletOnly

	MustRegisterCmd("createnewaccount", (*CreateNewAccountCmd)(nil), flags)
	MustRegisterCmd("importaddress", (*ImportAddressCmd)(nil), flags)
	MustRegisterCmd("importpubkey", (*ImportPubKeyCmd)(nil), flags)
	MustRegisterCmd("renameaccount", (*RenameAccountCmd)(nil), flags)
}
