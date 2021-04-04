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

type ProTxSubCmd string

const (
	ProTxRegister        ProTxSubCmd = "register"
	ProTxRegisterFund    ProTxSubCmd = "register_fund"
	ProTxRegisterPrepare ProTxSubCmd = "register_prepare"
	ProTxRegisterSubmit  ProTxSubCmd = "register_submit"
	ProTxList            ProTxSubCmd = "list"
	ProTxInfo            ProTxSubCmd = "info"
	ProTxUpdateService   ProTxSubCmd = "update_service"
	ProTxUpdateRegistrar ProTxSubCmd = "update_registrar"
	ProTxRevoke          ProTxSubCmd = "revoke"
	ProTxDiff            ProTxSubCmd = "diff"
)

type ProTxCmd struct {
	SubCmd ProTxSubCmd `jsonrpcusage:"\"register|register_fund|register_prepare|register_submit|list|info|update_service|update_registrar|revoke|diff\""`

	ProTxHash *string `json:",omitempty"`

	Type     *ProTxListType `json:",omitempty"`
	Detailed *bool          `json:",omitempty"`
	Height   *int           `json:",omitempty"`

	BaseBlock *int `json:",omitempty"`
	Block     *int `json:",omitempty"`

	CollateralHash        *string  `json:",omitempty"`
	CollateralIndex       *int     `json:",omitempty"`
	CollateralAddress     *string  `json:",omitempty"`
	IPAndPort             *string  `json:",omitempty"`
	OwnerAddress          *string  `json:",omitempty"`
	OperatorPubKey        *string  `json:",omitempty"`
	OperatorPrivateKey    *string  `json:",omitempty"`
	OperatorPayoutAddress *string  `json:",omitempty"`
	VotingAddress         *string  `json:",omitempty"`
	OperatorReward        *float64 `json:",omitempty"`
	PayoutAddress         *string  `json:",omitempty"`
	FundAddress           *string  `json:",omitempty"`
	Reason                *int     `json:",omitempty"`
	FeeSourceAddress      *string  `json:",omitempty"`
	Submit                *bool    `json:",omitempty"`

	Tx  *string `json:",omitempty"`
	Sig *string `json:",omitempty"`
}

type ProTxListType string

const (
	ProTxListTypeRegistered ProTxListType = "registered"
	ProTxListTypeValid      ProTxListType = "valid"
	ProTxListTypeWallet     ProTxListType = "wallet"
)

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
	if quorumHash == "" {
		return cmd
	}
	cmd.QuorumHash = &quorumHash
	cmd.Submit = &submit
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

// NewProTxRegisterCmd returns a new instance which can be used to issue a protx register
// JSON-RPC command.
func NewProTxRegisterCmd(collateralHash string, collateralIndex int, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, feeSourceAddress string, submit bool) *ProTxCmd {
	r := &ProTxCmd{
		SubCmd:          ProTxRegister,
		CollateralHash:  &collateralHash,
		CollateralIndex: &collateralIndex,
		IPAndPort:       &ipAndPort,
		OwnerAddress:    &ownerAddress,
		OperatorPubKey:  &operatorPubKey,
		VotingAddress:   &votingAddress,
		OperatorReward:  &operatorReward,
		PayoutAddress:   &payoutAddress,
	}
	if feeSourceAddress == "" {
		return r
	}
	r.FeeSourceAddress = &feeSourceAddress
	r.Submit = &submit
	return r
}

// NewProTxRegisterFundCmd returns a new instance which can be used to issue a protx register_fund
// JSON-RPC command.
func NewProTxRegisterFundCmd(collateralAddress, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, fundAddress string, submit bool) *ProTxCmd {
	r := &ProTxCmd{
		SubCmd:            ProTxRegisterFund,
		CollateralAddress: &collateralAddress,
		IPAndPort:         &ipAndPort,
		OwnerAddress:      &ownerAddress,
		OperatorPubKey:    &operatorPubKey,
		VotingAddress:     &votingAddress,
		OperatorReward:    &operatorReward,
		PayoutAddress:     &payoutAddress,
	}
	if fundAddress == "" {
		return r
	}
	r.FundAddress = &fundAddress
	r.Submit = &submit
	return r
}

// NewProTxRegisterPrepareCmd returns a new instance which can be used to issue a protx register_prepare
// JSON-RPC command.
func NewProTxRegisterPrepareCmd(collateralHash string, collateralIndex int, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, feeSourceAddress string) *ProTxCmd {
	r := &ProTxCmd{
		SubCmd:          ProTxRegisterPrepare,
		CollateralHash:  &collateralHash,
		CollateralIndex: &collateralIndex,
		IPAndPort:       &ipAndPort,
		OwnerAddress:    &ownerAddress,
		OperatorPubKey:  &operatorPubKey,
		VotingAddress:   &votingAddress,
		OperatorReward:  &operatorReward,
		PayoutAddress:   &payoutAddress,
	}
	if feeSourceAddress == "" {
		return r
	}
	r.FeeSourceAddress = &feeSourceAddress
	return r
}

// NewProTxInfoCmd returns a new instance which can be used to issue a protx info
// JSON-RPC command.
func NewProTxInfoCmd(proTxHash string) *ProTxCmd {
	return &ProTxCmd{
		SubCmd:    ProTxInfo,
		ProTxHash: &proTxHash,
	}
}

// NewProTxListCmd returns a new instance which can be used to issue a protx list
// JSON-RPC command.
func NewProTxListCmd(cmdType ProTxListType, detailed bool, height int) *ProTxCmd {
	r := &ProTxCmd{
		SubCmd: ProTxList,
	}
	if cmdType == "" {
		return r
	}
	r.Type = &cmdType
	r.Detailed = &detailed
	if height == 0 {
		return r
	}
	r.Height = &height
	return r
}

// NewProTxRegisterSubmitCmd returns a new instance which can be used to issue a protx register_submit
// JSON-RPC command.
func NewProTxRegisterSubmitCmd(tx, sig string) *ProTxCmd {
	return &ProTxCmd{
		SubCmd: ProTxRegisterSubmit,
		Tx:     &tx,
		Sig:    &sig,
	}
}

// NewProTxDiffCmd returns a new instance which can be used to issue a protx diff
// JSON-RPC command.
func NewProTxDiffCmd(baseBlock, block int) *ProTxCmd {
	return &ProTxCmd{
		SubCmd:    ProTxDiff,
		BaseBlock: &baseBlock,
		Block:     &block,
	}
}

// NewProTxUpdateServiceCmd returns a new instance which can be used to issue a protx update_service
// JSON-RPC command.
func NewProTxUpdateServiceCmd(proTxHash, ipAndPort, operatorPubKey, operatorPayoutAddress, feeSourceAddress string) *ProTxCmd {
	r := &ProTxCmd{
		SubCmd:         ProTxUpdateService,
		ProTxHash:      &proTxHash,
		IPAndPort:      &ipAndPort,
		OperatorPubKey: &operatorPubKey,
	}
	if operatorPayoutAddress == "" {
		return r
	}
	r.OperatorPayoutAddress = &operatorPayoutAddress
	if feeSourceAddress == "" {
		return r
	}
	r.FeeSourceAddress = &feeSourceAddress
	return r
}

// NewProTxUpdateRegistrarCmd returns a new instance which can be used to issue a protx update_registrar
// JSON-RPC command.
func NewProTxUpdateRegistrarCmd(proTxHash, operatorPubKey, votingAddress, payoutAddress, feeSourceAddress string) *ProTxCmd {
	r := &ProTxCmd{
		SubCmd:         ProTxUpdateRegistrar,
		ProTxHash:      &proTxHash,
		OperatorPubKey: &operatorPubKey,
		VotingAddress:  &votingAddress,
		PayoutAddress:  &payoutAddress,
	}
	if feeSourceAddress == "" {
		return r
	}
	r.FeeSourceAddress = &feeSourceAddress
	return r
}

// NewProTxRevokeCmd returns a new instance which can be used to issue a protx revoke
// JSON-RPC command.
func NewProTxRevokeCmd(proTxHash, operatorPrivateKey string, reason int, feeSourceAddress string) *ProTxCmd {
	r := &ProTxCmd{
		SubCmd:             ProTxRevoke,
		ProTxHash:          &proTxHash,
		OperatorPrivateKey: &operatorPrivateKey,
	}
	if reason == 0 {
		return r
	}
	r.Reason = &reason

	if feeSourceAddress == "" {
		return r
	}
	r.FeeSourceAddress = &feeSourceAddress
	return r
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("quorum", (*QuorumCmd)(nil), flags)
	MustRegisterCmd("bls", (*BLSCmd)(nil), flags)
	MustRegisterCmd("protx", (*ProTxCmd)(nil), flags)
}
