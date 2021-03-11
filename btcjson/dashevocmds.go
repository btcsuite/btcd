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

	// QuorumList lists all quorums
	QuorumList QuorumCmdSubCmd = "list"
)

// LLMQType is the type of quorum
type LLMQType int

// Enum of LLMQTypes
// https://github.com/dashpay/dips/blob/master/dip-0006.md#current-llmq-types
const (
	LLMQType_50_60  LLMQType = 1   //every 24 blocks
	LLMQType_400_60 LLMQType = 2   //288 blocks
	LLMQType_400_85 LLMQType = 3   //576 blocks
	LLMQType_100_67 LLMQType = 4   //every 24 blocks
	LLMQType_5_60   LLMQType = 100 //24 blocks
)

// QuorumCmd defines the quorum JSON-RPC command.
type QuorumCmd struct {
	SubCmd QuorumCmdSubCmd `jsonrpcusage:"\"info|list|sign\""`

	SignLLMQType    *LLMQType `json:",omitempty"`
	SignRequestID   *string   `json:",omitempty"`
	SignMessageHash *string   `json:",omitempty"`
	SignQuorumHash  *string   `json:",omitempty"`
	SignSubmit      *bool     `json:",omitempty"`

	InfoLLMQType       *LLMQType `json:",omitempty"`
	InfoQuorumHash     *string   `json:",omitempty"`
	InfoIncludeSkShare *bool     `json:",omitempty"`
}

// NewQuorumSignCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewQuorumSignCmd(quorumType LLMQType, requestID, messageHash, quorumHash string, submit bool) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:          QuorumSign,
		SignLLMQType:    &quorumType,
		SignRequestID:   &requestID,
		SignMessageHash: &messageHash,
		SignQuorumHash:  &quorumHash,
		SignSubmit:      &submit,
	}
}

// NewQuorumInfoCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewQuorumInfoCmd(quorumType LLMQType, quorumHash string, includeSkShare bool) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:             QuorumInfo,
		InfoLLMQType:       &quorumType,
		InfoQuorumHash:     &quorumHash,
		InfoIncludeSkShare: &includeSkShare,
	}
}

// NewQuorumListCmd returns a list of quorums
// JSON-RPC command.
func NewQuorumListCmd() *QuorumCmd {
	return &QuorumCmd{
		SubCmd: QuorumList,
	}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("quorum", (*QuorumCmd)(nil), flags)
}
