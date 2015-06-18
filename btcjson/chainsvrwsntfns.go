// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC websocket notifications that are
// supported by a chain server.

package btcjson

const (
	// BlockConnectedNtfnMethod is the method used for notifications from
	// the chain server that a block has been connected.
	BlockConnectedNtfnMethod = "blockconnected"

	// BlockDisconnectedNtfnMethod is the method used for notifications from
	// the chain server that a block has been disconnected.
	BlockDisconnectedNtfnMethod = "blockdisconnected"

	// RecvTxNtfnMethod is the method used for notifications from the chain
	// server that a transaction which pays to a registered address has been
	// processed.
	RecvTxNtfnMethod = "recvtx"

	// RedeemingTxNtfnMethod is the method used for notifications from the
	// chain server that a transaction which spends a registered outpoint
	// has been processed.
	RedeemingTxNtfnMethod = "redeemingtx"

	// RescanFinishedNtfnMethod is the method used for notifications from
	// the chain server that a rescan operation has finished.
	RescanFinishedNtfnMethod = "rescanfinished"

	// RescanProgressNtfnMethod is the method used for notifications from
	// the chain server that a rescan operation this is underway has made
	// progress.
	RescanProgressNtfnMethod = "rescanprogress"

	// TxAcceptedNtfnMethod is the method used for notifications from the
	// chain server that a transaction has been accepted into the mempool.
	TxAcceptedNtfnMethod = "txaccepted"

	// TxAcceptedVerboseNtfnMethod is the method used for notifications from
	// the chain server that a transaction has been accepted into the
	// mempool.  This differs from TxAcceptedNtfnMethod in that it provides
	// more details in the notification.
	TxAcceptedVerboseNtfnMethod = "txacceptedverbose"
)

// BlockConnectedNtfn defines the blockconnected JSON-RPC notification.
type BlockConnectedNtfn struct {
	Hash   string
	Height int32
	Time   int64
}

// NewBlockConnectedNtfn returns a new instance which can be used to issue a
// blockconnected JSON-RPC notification.
func NewBlockConnectedNtfn(hash string, height int32, time int64) *BlockConnectedNtfn {
	return &BlockConnectedNtfn{
		Hash:   hash,
		Height: height,
		Time:   time,
	}
}

// BlockDisconnectedNtfn defines the blockdisconnected JSON-RPC notification.
type BlockDisconnectedNtfn struct {
	Hash   string
	Height int32
	Time   int64
}

// NewBlockDisconnectedNtfn returns a new instance which can be used to issue a
// blockdisconnected JSON-RPC notification.
func NewBlockDisconnectedNtfn(hash string, height int32, time int64) *BlockDisconnectedNtfn {
	return &BlockDisconnectedNtfn{
		Hash:   hash,
		Height: height,
		Time:   time,
	}
}

// BlockDetails describes details of a tx in a block.
type BlockDetails struct {
	Height int32  `json:"height"`
	Hash   string `json:"hash"`
	Index  int    `json:"index"`
	Time   int64  `json:"time"`
}

// RecvTxNtfn defines the recvtx JSON-RPC notification.
type RecvTxNtfn struct {
	HexTx string
	Block *BlockDetails
}

// NewRecvTxNtfn returns a new instance which can be used to issue a recvtx
// JSON-RPC notification.
func NewRecvTxNtfn(hexTx string, block *BlockDetails) *RecvTxNtfn {
	return &RecvTxNtfn{
		HexTx: hexTx,
		Block: block,
	}
}

// RedeemingTxNtfn defines the redeemingtx JSON-RPC notification.
type RedeemingTxNtfn struct {
	HexTx string
	Block *BlockDetails
}

// NewRedeemingTxNtfn returns a new instance which can be used to issue a
// redeemingtx JSON-RPC notification.
func NewRedeemingTxNtfn(hexTx string, block *BlockDetails) *RedeemingTxNtfn {
	return &RedeemingTxNtfn{
		HexTx: hexTx,
		Block: block,
	}
}

// RescanFinishedNtfn defines the rescanfinished JSON-RPC notification.
type RescanFinishedNtfn struct {
	Hash   string
	Height int32
	Time   int64
}

// NewRescanFinishedNtfn returns a new instance which can be used to issue a
// rescanfinished JSON-RPC notification.
func NewRescanFinishedNtfn(hash string, height int32, time int64) *RescanFinishedNtfn {
	return &RescanFinishedNtfn{
		Hash:   hash,
		Height: height,
		Time:   time,
	}
}

// RescanProgressNtfn defines the rescanprogress JSON-RPC notification.
type RescanProgressNtfn struct {
	Hash   string
	Height int32
	Time   int64
}

// NewRescanProgressNtfn returns a new instance which can be used to issue a
// rescanprogress JSON-RPC notification.
func NewRescanProgressNtfn(hash string, height int32, time int64) *RescanProgressNtfn {
	return &RescanProgressNtfn{
		Hash:   hash,
		Height: height,
		Time:   time,
	}
}

// TxAcceptedNtfn defines the txaccepted JSON-RPC notification.
type TxAcceptedNtfn struct {
	TxID   string
	Amount float64
}

// NewTxAcceptedNtfn returns a new instance which can be used to issue a
// txaccepted JSON-RPC notification.
func NewTxAcceptedNtfn(txHash string, amount float64) *TxAcceptedNtfn {
	return &TxAcceptedNtfn{
		TxID:   txHash,
		Amount: amount,
	}
}

// TxAcceptedVerboseNtfn defines the txacceptedverbose JSON-RPC notification.
type TxAcceptedVerboseNtfn struct {
	RawTx TxRawResult
}

// NewTxAcceptedVerboseNtfn returns a new instance which can be used to issue a
// txacceptedverbose JSON-RPC notification.
func NewTxAcceptedVerboseNtfn(rawTx TxRawResult) *TxAcceptedVerboseNtfn {
	return &TxAcceptedVerboseNtfn{
		RawTx: rawTx,
	}
}

func init() {
	// The commands in this file are only usable by websockets and are
	// notifications.
	flags := UFWebsocketOnly | UFNotification

	MustRegisterCmd(BlockConnectedNtfnMethod, (*BlockConnectedNtfn)(nil), flags)
	MustRegisterCmd(BlockDisconnectedNtfnMethod, (*BlockDisconnectedNtfn)(nil), flags)
	MustRegisterCmd(RecvTxNtfnMethod, (*RecvTxNtfn)(nil), flags)
	MustRegisterCmd(RedeemingTxNtfnMethod, (*RedeemingTxNtfn)(nil), flags)
	MustRegisterCmd(RescanFinishedNtfnMethod, (*RescanFinishedNtfn)(nil), flags)
	MustRegisterCmd(RescanProgressNtfnMethod, (*RescanProgressNtfn)(nil), flags)
	MustRegisterCmd(TxAcceptedNtfnMethod, (*TxAcceptedNtfn)(nil), flags)
	MustRegisterCmd(TxAcceptedVerboseNtfnMethod, (*TxAcceptedVerboseNtfn)(nil), flags)
}
