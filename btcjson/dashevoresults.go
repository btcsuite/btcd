package btcjson


// QuorumSignResult models the data from the quorum sign command.
// returns a hex-encoded string.
type QuorumSignResult struct {
	Hash          string  `json:"hash"`
}