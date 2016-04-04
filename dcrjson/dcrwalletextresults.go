// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

// AccountFetchAddressesResult models the data returned from the
// accountfetchaddresses command.
type AccountFetchAddressesResult struct {
	Addresses []string `json:"addresses"`
}

// GetMultisigOutInfoResult models the data returned from the getmultisigoutinfo
// command.
type GetMultisigOutInfoResult struct {
	Address      string   `json:"address"`
	RedeemScript string   `json:"redeemscript"`
	M            uint8    `json:"m"`
	N            uint8    `json:"n"`
	Pubkeys      []string `json:"pubkeys"`
	TxHash       string   `json:"txhash"`
	BlockHeight  uint32   `json:"blockheight"`
	BlockHash    string   `json:"blockhash"`
	Spent        bool     `json:"spent"`
	SpentBy      string   `json:"spentby"`
	SpentByIndex uint32   `json:"spentbyindex"`
	Amount       float64  `json:"amount"`
}

// GetStakeInfoResult models the data returned from the getstakeinfo
// command.
type GetStakeInfoResult struct {
	PoolSize         uint32  `json:"poolsize"`
	Difficulty       float64 `json:"difficulty"`
	AllMempoolTix    uint32  `json:"allmempooltix"`
	OwnMempoolTix    uint32  `json:"ownmempooltix"`
	Immature         uint32  `json:"immature"`
	Live             uint32  `json:"live"`
	ProportionLive   float64 `json:"proportionlive"`
	Voted            uint32  `json:"voted"`
	TotalSubsidy     float64 `json:"totalsubsidy"`
	Missed           uint32  `json:"missed"`
	ProportionMissed float64 `json:"proportionmissed"`
	Revoked          uint32  `json:"revoked"`
}

// GetTicketsResult models the data returned from the gettickets
// command.
type GetTicketsResult struct {
	Hashes []string `json:"hashes"`
}

// VoteBitsData models the data stored for any given voteBits for a ticket.
type VoteBitsData struct {
	VoteBits    uint16 `json:"votebits"`
	VoteBitsExt string `json:"votebitsext"`
}

// GetTicketVoteBitsResult models the data returned from the getticketvotebits
// command.
type GetTicketVoteBitsResult struct {
	VoteBitsData
}

// GetTicketsVoteBitsResult models the data returned from the getticketsvotebits
// command.
type GetTicketsVoteBitsResult struct {
	VoteBitsList []VoteBitsData `json:"votebitslist"`
}

// ScriptInfo is the structure representing a redeem script, its hash,
// and its address.
type ScriptInfo struct {
	Hash160      string `json:"hash160"`
	Address      string `json:"address"`
	RedeemScript string `json:"redeemscript"`
}

// ListScriptsResult models the data returned from the listscripts
// command.
type ListScriptsResult struct {
	Scripts []ScriptInfo `json:"scripts"`
}

// RedeemMultiSigOutResult models the data returned from the redeemmultisigout
// command.
type RedeemMultiSigOutResult struct {
	Hex      string                    `json:"hex"`
	Complete bool                      `json:"complete"`
	Errors   []SignRawTransactionError `json:"errors,omitempty"`
}

// RedeemMultiSigOutsResult models the data returned from the redeemmultisigouts
// command.
type RedeemMultiSigOutsResult struct {
	Results []RedeemMultiSigOutResult `json:"results"`
}

// SendToMultiSigResult models the data returned from the sendtomultisig
// command.
type SendToMultiSigResult struct {
	TxHash       string `json:"txhash"`
	Address      string `json:"address"`
	RedeemScript string `json:"redeemscript"`
}

// SignedTransaction is a signed transaction resulting from a signrawtransactions
// command.
type SignedTransaction struct {
	SigningResult SignRawTransactionResult `json:"signingresult"`
	Sent          bool                     `json:"sent"`
	TxHash        *string                  `json:"txhash,omitempty"`
}

// SignRawTransactionsResult models the data returned from the signrawtransactions
// command.
type SignRawTransactionsResult struct {
	Results []SignedTransaction `json:"results"`
}

// WalletInfoResult models the data returned from the walletinfo
// command.
type WalletInfoResult struct {
	DaemonConnected   bool    `json:"daemonconnected"`
	Unlocked          bool    `json:"unlocked"`
	TxFee             float64 `json:"txfee"`
	TicketFee         float64 `json:"ticketfee"`
	TicketMaxPrice    float64 `json:"ticketmaxprice"`
	BalanceToMaintain float64 `json:"balancetomaintain"`
	StakeMining       bool    `json:"stakemining"`
}
