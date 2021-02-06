package btcjson

// QuorumSignResult models the data from the quorum sign command.
// returns a hex-encoded string.
type QuorumSignResult struct {
	LLMQType          int  `json:"llmqType"`
	QuorumHash          string  `json:"quorumHash"`
	QuorumMember          int  `json:"quorumMember"`
	ID          string  `json:"id"`
	MsgHash          string  `json:"msgHash"`
	SignHash          string  `json:"signHash"`
	Signature string `json:"signature"`
}

type QuorumMember struct {
	ProTxHash string `json:"proTxHash"`
	PubKeyOperator string `json:"pubKeyOperator"`
	Valid bool `json:"valid"`
	PubKeyShare string `json:"pubKeyShare"`
}

// QuorumInfoResult models the data from the quorum info command.
// returns a hex-encoded string.
type QuorumInfoResult struct {
	Height          uint32  `json:"height"`
	Type string `json:"type"`
	QuorumHash string `json:"quorumHash"`
	MinedBlock string `json:"minedBlock"`
	Members []QuorumMember `json:"members"`
	QuorumPublicKey string `json:"quorumPublicKey"`
	SecretKeyShare string `json:"secretKeyShare"`
}