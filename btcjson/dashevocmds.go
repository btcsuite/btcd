// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server.

package btcjson

type BLSSubCmd string

const (
	BLSGenerate   BLSSubCmd = "generate"
	BLSFromSecret BLSSubCmd = "fromsecret"
)

type BLSCmd struct {
	SubCmd BLSSubCmd `jsonrpcusage:"\"generate|fromsecret\""`
	Secret *string   `json:",omitempty"`
}

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

	QuorumSelectQuorum  QuorumCmdSubCmd = "selectquorum"
	QuorumDKGStatus     QuorumCmdSubCmd = "dkgstatus"
	QuorumMemberOf      QuorumCmdSubCmd = "memberof"
	QuorumGetRecSig     QuorumCmdSubCmd = "getrecsig"
	QuorumHasRecSig     QuorumCmdSubCmd = "hasrecsig"
	QuorumIsConflicting QuorumCmdSubCmd = "isconflicting"
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
	SubCmd QuorumCmdSubCmd `jsonrpcusage:"\"list|info|dkgstatus|sign|getrecsig|hasrecsig|isconflicting|memberof|selectquorum\""`

	LLMQType    *LLMQType `json:",omitempty"`
	RequestID   *string   `json:",omitempty"`
	MessageHash *string   `json:",omitempty"`
	QuorumHash  *string   `json:",omitempty"`

	Submit               *bool        `json:",omitempty"`
	IncludeSkShare       *bool        `json:",omitempty"`
	DKGStatusDetailLevel *DetailLevel `json:",omitempty"`
	ProTxHash            *string      `json:",omitempty"`
	ScanQuorumsCount     *int         `json:",omitempty"`
}

// NewQuorumSignCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewQuorumSignCmd(quorumType LLMQType, requestID, messageHash, quorumHash string, submit bool) *QuorumCmd {
	cmd := &QuorumCmd{
		SubCmd:      QuorumSign,
		LLMQType:    &quorumType,
		RequestID:   &requestID,
		MessageHash: &messageHash,
	}
	if quorumHash != "" {
		cmd.QuorumHash = &quorumHash
	}
	if !submit {
		cmd.Submit = &submit
	}
	return cmd

}

// NewQuorumInfoCmd returns a new instance which can be used to issue a quorum
// JSON-RPC command.
func NewQuorumInfoCmd(quorumType LLMQType, quorumHash string, includeSkShare bool) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:         QuorumInfo,
		LLMQType:       &quorumType,
		QuorumHash:     &quorumHash,
		IncludeSkShare: &includeSkShare,
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

// NewQuorumGetRecSig returns the result from quorum getrecsig
func NewQuorumGetRecSig(quorumType LLMQType, requestID, messageHash string) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:      QuorumGetRecSig,
		LLMQType:    &quorumType,
		RequestID:   &requestID,
		MessageHash: &messageHash,
	}
}

// NewQuorumHasRecSig returns the result from quorum hasrecsig
func NewQuorumHasRecSig(quorumType LLMQType, requestID, messageHash string) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:      QuorumHasRecSig,
		LLMQType:    &quorumType,
		RequestID:   &requestID,
		MessageHash: &messageHash,
	}
}

// NewQuorumIsConflicting returns the result from quorum isconflicting
func NewQuorumIsConflicting(quorumType LLMQType, requestID, messageHash string) *QuorumCmd {
	return &QuorumCmd{
		SubCmd:      QuorumIsConflicting,
		LLMQType:    &quorumType,
		RequestID:   &requestID,
		MessageHash: &messageHash,
	}
}

func NewBLSGenerate() *BLSCmd {
	return &BLSCmd{SubCmd: BLSGenerate}
}

func NewBLSFromSecret(secret string) *BLSCmd {
	return &BLSCmd{
		SubCmd: BLSFromSecret,
		Secret: &secret,
	}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("quorum", (*QuorumCmd)(nil), flags)
	MustRegisterCmd("bls", (*BLSCmd)(nil), flags)
}
