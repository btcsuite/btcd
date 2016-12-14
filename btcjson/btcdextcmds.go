// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server with btcd extensions.

package btcjson

// NodeSubCmd defines the type used in the addnode JSON-RPC command for the
// sub command field.
type NodeSubCmd string

const (
	// NConnect indicates the specified host that should be connected to.
	NConnect NodeSubCmd = "connect"

	// NRemove indicates the specified peer that should be removed as a
	// persistent peer.
	NRemove NodeSubCmd = "remove"

	// NDisconnect indicates the specified peer should be disonnected.
	NDisconnect NodeSubCmd = "disconnect"
)

// NodeCmd defines the dropnode JSON-RPC command.
type NodeCmd struct {
	SubCmd        NodeSubCmd `jsonrpcusage:"\"connect|remove|disconnect\""`
	Target        string
	ConnectSubCmd *string `jsonrpcusage:"\"perm|temp\""`
}

// NewNodeCmd returns a new instance which can be used to issue a `node`
// JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewNodeCmd(subCmd NodeSubCmd, target string, connectSubCmd *string) *NodeCmd {
	return &NodeCmd{
		SubCmd:        subCmd,
		Target:        target,
		ConnectSubCmd: connectSubCmd,
	}
}

// DebugLevelCmd defines the debuglevel JSON-RPC command.  This command is not a
// standard Bitcoin command.  It is an extension for btcd.
type DebugLevelCmd struct {
	LevelSpec string
}

// NewDebugLevelCmd returns a new DebugLevelCmd which can be used to issue a
// debuglevel JSON-RPC command.  This command is not a standard Bitcoin command.
// It is an extension for btcd.
func NewDebugLevelCmd(levelSpec string) *DebugLevelCmd {
	return &DebugLevelCmd{
		LevelSpec: levelSpec,
	}
}

// GenerateCmd defines the generate JSON-RPC command.
type GenerateCmd struct {
	NumBlocks uint32
}

// NewGenerateCmd returns a new instance which can be used to issue a generate
// JSON-RPC command.
func NewGenerateCmd(numBlocks uint32) *GenerateCmd {
	return &GenerateCmd{
		NumBlocks: numBlocks,
	}
}

// GetBestBlockCmd defines the getbestblock JSON-RPC command.
type GetBestBlockCmd struct{}

// NewGetBestBlockCmd returns a new instance which can be used to issue a
// getbestblock JSON-RPC command.
func NewGetBestBlockCmd() *GetBestBlockCmd {
	return &GetBestBlockCmd{}
}

// GetCurrentNetCmd defines the getcurrentnet JSON-RPC command.
type GetCurrentNetCmd struct{}

// NewGetCurrentNetCmd returns a new instance which can be used to issue a
// getcurrentnet JSON-RPC command.
func NewGetCurrentNetCmd() *GetCurrentNetCmd {
	return &GetCurrentNetCmd{}
}

// AddMiningAddrCmd defines the addminingaddr JSON-RPC command.
type AddMiningAddrCmd struct {
	Address string `json:"address"`
}

// NewAddMiningAddrCmd returns a new instance which can be used to issue a
// addminingaddr JSON-RPC command.
func NewAddMiningAddrCmd(addr string) *AddMiningAddrCmd {
	return &AddMiningAddrCmd{
		Address: addr,
	}
}

// DelMiningAddrCmd defines the delminingaddr JSON-RPC command.
type DelMiningAddrCmd struct {
	Address string `json:"address"`
}

// NewDelMiningAddrCmd returns a new instance which can be used to issue a
// delminingaddr JSON-RPC command.
func NewDelMiningAddrCmd(addr string) *DelMiningAddrCmd {
	return &DelMiningAddrCmd{
		Address: addr,
	}
}

// ListMiningAddrsCmd defines the listminingaddrs JSON-RPC command.
type ListMiningAddrsCmd struct{}

// NewListMiningAddrsCmd returns a new instance which can be used to issue a
// listminingaddrs JSON-RPC command.
func NewListMiningAddrsCmd() *ListMiningAddrsCmd {
	return &ListMiningAddrsCmd{}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("debuglevel", (*DebugLevelCmd)(nil), flags)
	MustRegisterCmd("node", (*NodeCmd)(nil), flags)
	MustRegisterCmd("generate", (*GenerateCmd)(nil), flags)
	MustRegisterCmd("getbestblock", (*GetBestBlockCmd)(nil), flags)
	MustRegisterCmd("getcurrentnet", (*GetCurrentNetCmd)(nil), flags)
	MustRegisterCmd("addminingaddr", (*AddMiningAddrCmd)(nil), flags)
	MustRegisterCmd("delminingaddr", (*DelMiningAddrCmd)(nil), flags)
	MustRegisterCmd("listminingaddrs", (*ListMiningAddrsCmd)(nil), flags)
}
