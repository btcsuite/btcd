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

	// Result is the output if submit was true
	Result bool `json:"result,omitempty"`
}

// UnmarshalJSON is a custom unmarshal because the result can be just a boolean
func (qsr *QuorumSignResult) UnmarshalJSON(data []byte) error {
	if bl, err := strconv.ParseBool(string(data)); err == nil {
		qsr.Result = bl
		return nil
	}
	if err := json.Unmarshal(data, qsr); err != nil {
		return err
	}
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
