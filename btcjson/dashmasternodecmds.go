// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2021-2021 Dash Core Group
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package btcjson

// MasternodeSubCmd defines the sub command used in the quorum JSON-RPC command.
type MasternodeSubCmd string

// Masternode subcommands
const (
	MasternodeStatus MasternodeSubCmd = "status"
	MasternodeCount  MasternodeSubCmd = "count"
)

// MasternodeCmd defines the quorum JSON-RPC command.
type MasternodeCmd struct {
	SubCmd MasternodeSubCmd `jsonrpcusage:"\"status|count\""`
}

// NewMasternodeCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewMasternodeCmd(sub MasternodeSubCmd) *MasternodeCmd {
	return &MasternodeCmd{
		SubCmd: sub,
	}
}

// MasternodelistCmd defines the quorum JSON-RPC command.
type MasternodelistCmd struct {
	Mode   string
	Filter string
}

// NewMasternodelistCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewMasternodelistCmd(mode, filter string) *MasternodelistCmd {
	return &MasternodelistCmd{
		Mode:   mode,
		Filter: filter,
	}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("masternode", (*MasternodeCmd)(nil), flags)
	MustRegisterCmd("masternodelist", (*MasternodelistCmd)(nil), flags)
}
