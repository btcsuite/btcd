// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package btcjson

// QuorumCmdSubCmd defines the sub command used in the quorum JSON-RPC command.
type QuorumCmdSubCmd string

const (
	// QuorumSign indicates the specified host should be added as a persistent
	// peer.
	QuorumSign QuorumCmdSubCmd = "sign"

	// QuorumInfo indicates the specified peer should be removed.
	QuorumInfo QuorumCmdSubCmd = "info"

)

type LLMQType int

const (
	LLMQType_50_60 LLMQType = 1 //every 24 blocks
	LLMQType_400_60 = 2 //288 blocks
	LLMQType_400_85 = 3 //576 blocks
	LLMQType_100_67 = 4 //every 24 blocks
	LLMQType_5_60 = 100 //24 blocks
)

// QuorumCmd defines the quorum JSON-RPC command.
type QuorumSignCmd struct {
	Type   LLMQType
	RequestId string
	MessageHash string
	QuorumHash string
	Submit bool
}

// NewQuorumCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewQuorumSignCmd(quorumType LLMQType, requestId string, messageHash string, quorumHash string, submit bool) *QuorumSignCmd {
	return &QuorumSignCmd{
		Type:   quorumType,
		RequestId: requestId,
		MessageHash: messageHash,
		QuorumHash: quorumHash,
		Submit: submit,
	}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("quorum sign", (*QuorumSignCmd)(nil), flags)
}
