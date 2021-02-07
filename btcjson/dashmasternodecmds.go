// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2021-2021 Dash Core Group
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package btcjson

// MasternodeCmdSubCmd defines the sub command used in the quorum JSON-RPC command.
type MasternodeCmdSubCmd string

const (
	// MasternodeStatus indicates the specified host should be added as a persistent
	// peer.
	MasternodeStatus MasternodeCmdSubCmd = "status"
)

// MasternodeStatusCmd defines the quorum JSON-RPC command.
type MasternodeStatusCmd struct {
}

// NewMasternodeStatusCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewMasternodeStatusCmd() *MasternodeStatusCmd {
	return &MasternodeStatusCmd{}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("masternode status", (*MasternodeStatusCmd)(nil), flags)
}
