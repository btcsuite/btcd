package btcjson

import (
	"encoding/json"
	"strconv"
)

// QuorumSignResult models the data from the quorum sign command.
// returns a hex-encoded string.
type QuorumSignResult struct {
	LLMQType     int    `json:"llmqType,omitempty"`
	QuorumHash   string `json:"quorumHash,omitempty"`
	QuorumMember int    `json:"quorumMember,omitempty"`
	ID           string `json:"id,omitempty"`
	MsgHash      string `json:"msgHash,omitempty"`
	SignHash     string `json:"signHash,omitempty"`
	Signature    string `json:"signature,omitempty"`
}

type QuorumSignResultWithBool struct {
	QuorumSignResult
	// Result is the output if submit was true
	Result bool `json:"result,omitempty"`
}

// QuorumVerifyResult models the data from the quorum verify command.
// returns a boolean.
type QuorumVerifyResult struct {
	// Result is the output if submit was true
	Result bool `json:"result,omitempty"`
}

// UnmarshalJSON is a custom unmarshal because the result can be just a boolean
func (qsr *QuorumSignResultWithBool) UnmarshalJSON(data []byte) error {
	if bl, err := strconv.ParseBool(string(data)); err == nil {
		qsr.Result = bl
		return nil
	}

	type avoidInititeLoop QuorumSignResultWithBool
	var ail avoidInititeLoop
	err := json.Unmarshal(data, &ail)
	if err != nil {
		return err
	}
	// Cast the new type instance to the original type and assign.
	*qsr = QuorumSignResultWithBool(ail)
	return nil
}

type QuorumMember struct {
	ProTxHash      string `json:"proTxHash"`
	PubKeyOperator string `json:"pubKeyOperator"`
	Valid          bool   `json:"valid"`
	PubKeyShare    string `json:"pubKeyShare"`
}

// QuorumInfoResult models the data from the quorum info command.
// returns a hex-encoded string.
type QuorumInfoResult struct {
	Height          uint32         `json:"height"`
	Type            string         `json:"type"`
	QuorumHash      string         `json:"quorumHash"`
	MinedBlock      string         `json:"minedBlock"`
	Members         []QuorumMember `json:"members"`
	QuorumPublicKey string         `json:"quorumPublicKey"`
	SecretKeyShare  string         `json:"secretKeyShare"`
}

// QuorumListResult models the data from the quorum list command.
type QuorumListResult struct {
	Llmq50_60  []string `json:"llmq_50_60"`
	Llmq400_60 []string `json:"llmq_400_60"`
	Llmq400_85 []string `json:"llmq_400_85"`
	Llmq100_67 []string `json:"llmq_100_67"`
}

// QuorumSelectQuorumResult models the data from the quorum selectquorum command.
type QuorumSelectQuorumResult struct {
	QuorumHash      string   `json:"quorumHash"`
	RecoveryMembers []string `json:"recoveryMembers"`
}

// QuorumDKGStatusResultShared models shared data between different levels of dkgstatus
type QuorumDKGStatusResultShared struct {
	ProTxHash          string `json:"proTxHash"`
	Time               int    `json:"time"`
	TimeStr            string `json:"timeStr"`
	MinableCommitments struct {
		Llmq50_60  MinableCommitment `json:"llmq_50_60"`
		Llmq400_60 MinableCommitment `json:"llmq_400_60"`
		Llmq400_85 MinableCommitment `json:"llmq_400_85"`
		Llmq100_67 MinableCommitment `json:"llmq_100_67"`
	} `json:"minableCommitments"`
	QuorumConnections struct {
		Llmq50_60  []QuorumConnection `json:"llmq_50_60"`
		Llmq400_60 []QuorumConnection `json:"llmq_400_60"`
		Llmq400_85 []QuorumConnection `json:"llmq_400_85"`
		Llmq100_67 []QuorumConnection `json:"llmq_100_67"`
	} `json:"quorumConnections"`
}

// QuorumDKGStatusCountsResult models the data from quorum dkgstatus command.
type QuorumDKGStatusCountsResult struct {
	QuorumDKGStatusResultShared
	Session struct {
		Llmq50_60  DKGSessionCounts `json:"llmq_50_60"`
		Llmq400_60 DKGSessionCounts `json:"llmq_400_60"`
		Llmq400_85 DKGSessionCounts `json:"llmq_400_85"`
		Llmq100_67 DKGSessionCounts `json:"llmq_100_67"`
	} `json:"session"`
}

// QuorumDKGStatusIndexesResult models the data from quorum dkgstatus command.
type QuorumDKGStatusIndexesResult struct {
	QuorumDKGStatusResultShared
	Session struct {
		Llmq50_60  DKGSessionIndexes `json:"llmq_50_60"`
		Llmq400_60 DKGSessionIndexes `json:"llmq_400_60"`
		Llmq400_85 DKGSessionIndexes `json:"llmq_400_85"`
		Llmq100_67 DKGSessionIndexes `json:"llmq_100_67"`
	} `json:"session"`
}

// QuorumDKGStatusMembersProTxHashesResult models the data from quorum dkgstatus command.
type QuorumDKGStatusMembersProTxHashesResult struct {
	QuorumDKGStatusResultShared
	Session struct {
		Llmq50_60  DKGSessionMembersProTxHashes `json:"llmq_50_60"`
		Llmq400_60 DKGSessionMembersProTxHashes `json:"llmq_400_60"`
		Llmq400_85 DKGSessionMembersProTxHashes `json:"llmq_400_85"`
		Llmq100_67 DKGSessionMembersProTxHashes `json:"llmq_100_67"`
	} `json:"session"`
}

// DKGSessionMemeber is a memeber in in dkgstatus
type DKGSessionMemeber struct {
	MemberIndex int    `json:"memberIndex"`
	ProTxHash   string `json:"proTxHash"`
}

// MinableCommitment are the minableCommitments from dkgstatus
type MinableCommitment struct {
	Version           int    `json:"version"`
	LLMQType          int    `json:"llmqType"`
	QuorumHash        string `json:"quorumHash"`
	SignersCount      int    `json:"signersCount"`
	ValidMembersCount int    `json:"validMembersCount"`
	QuorumPublicKey   string `json:"quorumPublicKey"`
}

// QuorumConnection are the quorumConnections from dkgstatus
type QuorumConnection struct {
	ProTxHash string `json:"proTxHash"`
	Connected bool   `json:"connected"`
	Address   string `json:"address,omitempty"`
	Outbound  bool   `json:"outbound"`
}

// DKGSessionShared are the parts that are shared between the default and

type DKGSessionShared struct {
	LLMQType                int    `json:"llmqType"`
	QuorumHash              string `json:"quorumHash"`
	QuorumHeight            int    `json:"quorumHeight"`
	Phase                   int    `json:"phase"`
	SentContributions       bool   `json:"sentContributions"`
	SentComplaint           bool   `json:"sentComplaint"`
	SentJustification       bool   `json:"sentJustification"`
	SentPrematureCommitment bool   `json:"sentPrematureCommitment"`
	Aborted                 bool   `json:"aborted"`
}

// DKGSessionCounts is the session section of a dkgstatus with counts
type DKGSessionCounts struct {
	DKGSessionShared
	BadMembers                   int `json:"badMembers"`
	WeComplain                   int `json:"weComplain"`
	ReceivedContributions        int `json:"receivedContributions"`
	ReceivedComplaints           int `json:"receivedComplaints"`
	ReceivedJustifications       int `json:"receivedJustifications"`
	ReceivedPrematureCommitments int `json:"receivedPrematureCommitments"`
}

// DKGSessionIndexes is the session section of a dkgstatus with indexes
type DKGSessionIndexes struct {
	DKGSessionShared
	BadMembers                   []int `json:"badMembers"`
	WeComplain                   []int `json:"weComplain"`
	ReceivedContributions        []int `json:"receivedContributions"`
	ReceivedComplaints           []int `json:"receivedComplaints"`
	ReceivedJustifications       []int `json:"receivedJustifications"`
	ReceivedPrematureCommitments []int `json:"receivedPrematureCommitments"`
}

// DKGSessionMembersProTxHashes is the session section of a dkgstatus with member info
type DKGSessionMembersProTxHashes struct {
	DKGSessionShared
	BadMembers                   []DKGSessionMemeber `json:"badMembers"`
	WeComplain                   []DKGSessionMemeber `json:"weComplain"`
	ReceivedContributions        []DKGSessionMemeber `json:"receivedContributions"`
	ReceivedComplaints           []DKGSessionMemeber `json:"receivedComplaints"`
	ReceivedJustifications       []DKGSessionMemeber `json:"receivedJustifications"`
	ReceivedPrematureCommitments []DKGSessionMemeber `json:"receivedPrematureCommitments"`
	AllMembers                   []string            `json:"allMembers"`
}

// QuorumMemberOfResult data return by quorum memberof
type QuorumMemberOfResult struct {
	Height          int    `json:"height"`
	Type            string `json:"type"`
	QuorumHash      string `json:"quorumHash"`
	MinedBlock      string `json:"minedBlock"`
	QuorumPublicKey string `json:"quorumPublicKey"`
	IsValidMember   bool   `json:"isValidMember"`
	MemberIndex     int    `json:"memberIndex"`
}

type BLSResult struct {
	Secret string `json:"secret"`
	Public string `json:"public"`
}

// ProTxInfoResult models the data from the protx info command.
type ProTxInfoResult struct {
	ProTxHash         string        `json:"proTxHash"`
	CollateralHash    string        `json:"collateralHash"`
	CollateralIndex   int           `json:"collateralIndex"`
	CollateralAddress string        `json:"collateralAddress"`
	OperatorReward    float64       `json:"operatorReward"`
	State             ProTxState    `json:"state"`
	Confirmations     int           `json:"confirmations"`
	Wallet            ProTxWallet   `json:"wallet"`
	MetaInfo          ProTxMetaInfo `json:"metaInfo"`
}

type ProTxState struct {
	Service               string `json:"service"`
	RegisteredHeight      int    `json:"registeredHeight"`
	LastPaidHeight        int    `json:"lastPaidHeight"`
	PoSePenalty           int    `json:"PoSePenalty"`
	PoSeRevivedHeight     int    `json:"PoSeRevivedHeight"`
	PoSeBanHeight         int    `json:"PoSeBanHeight"`
	RevocationReason      int    `json:"revocationReason"`
	OwnerAddress          string `json:"ownerAddress"`
	VotingAddress         string `json:"votingAddress"`
	PayoutAddress         string `json:"payoutAddress"`
	PubKeyOperator        string `json:"pubKeyOperator"`
	OperatorPayoutAddress string `json:"operatorPayoutAddress"`
}

type ProTxWallet struct {
	HasOwnerKey              bool `json:"hasOwnerKey"`
	HasOperatorKey           bool `json:"hasOperatorKey"`
	HasVotingKey             bool `json:"hasVotingKey"`
	OwnsCollateral           bool `json:"ownsCollateral"`
	OwnsPayeeScript          bool `json:"ownsPayeeScript"`
	OwnsOperatorRewardScript bool `json:"ownsOperatorRewardScript"`
}

type ProTxMetaInfo struct {
	LastDSQ                    int `json:"lastDSQ"`
	MixingTxCount              int `json:"mixingTxCount"`
	LastOutboundAttempt        int `json:"lastOutboundAttempt"`
	LastOutboundAttemptElapsed int `json:"lastOutboundAttemptElapsed"`
	LastOutboundSuccess        int `json:"lastOutboundSuccess"`
	LastOutboundSuccessElapsed int `json:"lastOutboundSuccessElapsed"`
}

type ProTxDiffResult struct {
	BaseBlockHash    string                   `json:"baseBlockHash"`
	BlockHash        string                   `json:"blockHash"`
	CbTxMerkleTree   string                   `json:"cbTxMerkleTree"`
	CbTx             string                   `json:"cbTx"`
	DeletedMNs       []string                 `json:"deletedMNs"`
	MnList           []ProTxDiffMN            `json:"mnList"`
	DeletedQuorums   []ProTxDiffDeletedQuorum `json:"deletedQuorums"`
	NewQuorums       []ProTxDiffNewQuorum     `json:"newQuorums"`
	MerkleRootMNList string                   `json:"merkleRootMNList"`
}

type ProTxDiffMN struct {
	ProRegTxHash   string `json:"proRegTxHash"`
	ConfirmedHash  string `json:"confirmedHash"`
	Service        string `json:"service"`
	PubKeyOperator string `json:"pubKeyOperator"`
	VotingAddress  string `json:"votingAddress"`
	IsValid        bool   `json:"isValid"`
}

type ProTxDiffDeletedQuorum struct {
	LlmqType   int    `json:"llmqType"`
	QuorumHash string `json:"quorumHash"`
}
type ProTxDiffNewQuorum struct {
	Version           int    `json:"version"`
	LlmqType          int    `json:"llmqType"`
	QuorumHash        string `json:"quorumHash"`
	SignersCount      int    `json:"signersCount"`
	Signers           string `json:"signers"`
	ValidMembersCount int    `json:"validMembersCount"`
	ValidMembers      string `json:"validMembers"`
	QuorumPublicKey   string `json:"quorumPublicKey"`
	QuorumVvecHash    string `json:"quorumVvecHash"`
	QuorumSig         string `json:"quorumSig"`
	MembersSig        string `json:"membersSig"`
}

type ProTxRegisterPrepareResult struct {
	Tx                string `json:"tx"`
	CollateralAddress string `json:"collateralAddress"`
	SignMessage       string `json:"signMessage"`
}
