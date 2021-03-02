package btcjson

// MasternodeStatusResult models the data from the quorum sign command.
// returns a hex-encoded string.
type MasternodeStatusResult struct {
	Outpoint        string   `json:"outpoint"`
	Service         string   `json:"service"`
	ProTxHash       string   `json:"proTxHash"`
	CollateralHash  string   `json:"collateralHash"`
	CollateralIndex int      `json:"collateralIndex"`
	DMNState        DMNState `json:"dmnState"`
	State           string   `json:"state"`
	Status          string   `json:"status"`
}

// MasternodeCountResult models the data from the masternode count command.
// https://dashcore.readme.io/docs/core-api-ref-remote-procedure-calls-dash#masternode-count
type MasternodeCountResult struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

// DMNState is used in masternode status
type DMNState struct {
	Service           string `json:"service"`
	RegisteredHeight  int    `json:"registeredHeight"`
	LastPaidHeight    int    `json:"lastPaidHeight"`
	PoSePenalty       int    `json:"PoSePenalty"`
	PoSeRevivedHeight int    `json:"PoSeRevivedHeight"`
	PoSeBanHeight     int    `json:"PoSeBanHeight"`
	RevocationReason  int    `json:"revocationReason"`
	OwnerAddress      string `json:"ownerAddress"`
	VotingAddress     string `json:"votingAddress"`
	PayoutAddress     string `json:"payoutAddress"`
	PubKeyOperator    string `json:"pubKeyOperator"`
}
