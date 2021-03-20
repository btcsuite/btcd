// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2021-2021 Dash Core Group
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package btcjson

import "strconv"

// MasternodeSubCmd defines the sub command used in the quorum JSON-RPC command.
type MasternodeSubCmd string

// Masternode subcommands
const (
	MasternodeCount   MasternodeSubCmd = "count"
	MasternodeCurrent MasternodeSubCmd = "current"
	MasternodeOutputs MasternodeSubCmd = "outputs"
	MasternodeStatus  MasternodeSubCmd = "status"
	// MasternodeList    MasternodeSubCmd = "list" // same as masternodelist
	MasternodeWinner  MasternodeSubCmd = "winner"
	MasternodeWinners MasternodeSubCmd = "winners"
)

// MasternodeCmd defines the quorum JSON-RPC command.
type MasternodeCmd struct {
	SubCmd MasternodeSubCmd `jsonrpcusage:"\"count|current|outputs|status|winner|winners\""`
	Count  *string          `json:",omitempty"`
	Filter *string          `json:",omitempty"`
}

// NewMasternodeCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewMasternodeCmd(sub MasternodeSubCmd) *MasternodeCmd {
	return &MasternodeCmd{
		SubCmd: sub,
	}
}

// NewMasternodeCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewMasternodeWinnersCmd(count int, filter string) *MasternodeCmd {
	r := &MasternodeCmd{
		SubCmd: MasternodeWinners,
	}
	if count != 0 {
		c := strconv.Itoa(count)
		r.Count = &c
	}
	if filter != "" {
		r.Filter = &filter
	}
	return r
}

// MasternodelistCmd defines the quorum JSON-RPC command.
type MasternodelistCmd struct {
	Mode   string
	Filter string
}

// NewMasternodeListCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewMasternodeListCmd(mode, filter string) *MasternodelistCmd {
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
