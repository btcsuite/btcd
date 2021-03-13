// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package btcjson

// QuorumCmdSubCmd defines the sub command used in the quorum JSON-RPC command.
type QuorumCmdSubCmd string

// Quorum commands https://dashcore.readme.io/docs/core-api-ref-remote-procedure-calls-evo#quorum
const (
	// QuorumSign indicates the specified host should be added as a persistent
	// peer.
	QuorumSign QuorumCmdSubCmd = "sign"

	// QuorumInfo indicates the specified peer should be removed.
	QuorumInfo QuorumCmdSubCmd = "info"

	// QuorumList lists all quorums
	QuorumList QuorumCmdSubCmd = "list"

	QuorumSelectQuorum QuorumCmdSubCmd = "selectquorum"
	QuorumDKGStatus    QuorumCmdSubCmd = "dkgstatus"
	QuorumMemberOf     QuorumCmdSubCmd = "memberof"
)

// DetailLevel is the level of detail used in dkgstatus
type DetailLevel int

// Detail Levels for dkgstatsu
const (
	DetailLevelCounts             DetailLevel = 0
	DetailLevelIndexes            DetailLevel = 1
	DetailLevelMembersProTxHashes DetailLevel = 2
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

	LLMQType  *LLMQType `json:",omitempty"`
	RequestID *string   `json:",omitempty"`

	SignMessageHash *string `json:",omitempty"`
	SignQuorumHash  *string `json:",omitempty"`
	SignSubmit      *bool   `json:",omitempty"`

	InfoQuorumHash     *string `json:",omitempty"`
	InfoIncludeSkShare *bool   `json:",omitempty"`

	DKGStatusDetailLevel *DetailLevel `json:",omitempty"`

	ProTxHash        *string `json:",omitempty"`
	ScanQuorumsCount *int    `json:",omitempty"`
}

// NewQuorumSignCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewQuorumSignCmd(quorumType LLMQType, requestID, messageHash, quorumHash string, submit bool) *QuorumCmd {
	cmd := &QuorumCmd{
		SubCmd:          QuorumSign,
		LLMQType:        &quorumType,
		RequestID:       &requestID,
		SignMessageHash: &messageHash,
	}
	if quorumHash != "" {
		cmd.SignQuorumHash = &quorumHash
	}
	if !submit {
		cmd.SignSubmit = &submit
	}
	return cmd

}

// NewQuorumInfoCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewQuorumInfoCmd(quorumType LLMQType, quorumHash string, includeSkShare bool) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:             QuorumInfo,
		LLMQType:           &quorumType,
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

// NewQuorumSelectQuorumCmd returns the selected quorum
func NewQuorumSelectQuorumCmd(quorumType LLMQType, requestID string) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:    QuorumSelectQuorum,
		LLMQType:  &quorumType,
		RequestID: &requestID,
	}
}

// NewQuorumDKGStatusCmd returns the result from quorum dkgstatus
func NewQuorumDKGStatusCmd(detailLevel DetailLevel) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:               QuorumDKGStatus,
		DKGStatusDetailLevel: &detailLevel,
	}
}

// NewQuorumMemberOfCmd returns the result from quorum memberof
func NewQuorumMemberOfCmd(proTxHash string, scanQuorumsCount int) *QuorumCmd {
	cmd := &QuorumCmd{
		SubCmd:    QuorumMemberOf,
		ProTxHash: &proTxHash,
	}
	if scanQuorumsCount != 0 {
		cmd.ScanQuorumsCount = &scanQuorumsCount
	}
	return cmd
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("quorum", (*QuorumCmd)(nil), flags)
}
