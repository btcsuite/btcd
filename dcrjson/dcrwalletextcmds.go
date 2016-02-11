// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are
// supported by a wallet server with btcwallet extensions.

package dcrjson

// SStxInput represents the inputs to an SStx transaction. Specifically a
// transactionsha and output number pair, along with the output amounts.
type SStxInput struct {
	Txid string `json:"txid"`
	Vout uint32 `json:"vout"`
	Tree int8   `json:"tree"`
	Amt  int64  `json:"amt"`
}

// SStxOutput represents the output to an SStx transaction. Specifically a
// a commitment address and amount, and a change address and amount.
type SStxCommitOut struct {
	Addr       string `json:"addr"`
	CommitAmt  int64  `json:"commitamt"`
	ChangeAddr string `json:"changeaddr"`
	ChangeAmt  int64  `json:"changeamt"`
}

// CreateRawSStxCmd is a type handling custom marshaling and
// unmarshaling of createrawsstx JSON RPC commands.
type CreateRawSStxCmd struct {
	Inputs []SStxInput
	Amount map[string]int64
	COuts  []SStxCommitOut
}

// NewCreateRawSStxCmd creates a new CreateRawSStxCmd.
func NewCreateRawSStxCmd(inputs []SStxInput, amount map[string]int64,
	couts []SStxCommitOut) *CreateRawSStxCmd {
	return &CreateRawSStxCmd{
		Inputs: inputs,
		Amount: amount,
		COuts:  couts,
	}
}

// CreateRawSSGenTxCmd is a type handling custom marshaling and
// unmarshaling of createrawssgentxcmd JSON RPC commands.
type CreateRawSSGenTxCmd struct {
	Inputs   []TransactionInput
	VoteBits uint16
}

// NewCreateRawSSGenTxCmd creates a new CreateRawSSGenTxCmd.
func NewCreateRawSSGenTxCmd(inputs []TransactionInput,
	vb uint16) *CreateRawSSGenTxCmd {
	return &CreateRawSSGenTxCmd{
		Inputs:   inputs,
		VoteBits: vb,
	}
}

// CreateRawSSRtxCmd is a type handling custom marshaling and
// unmarshaling of createrawssrtx JSON RPC commands.
type CreateRawSSRtxCmd struct {
	Inputs []TransactionInput
}

// NewCreateRawSSRtxCmd creates a new CreateRawSSRtxCmd.
func NewCreateRawSSRtxCmd(inputs []TransactionInput) *CreateRawSSRtxCmd {
	return &CreateRawSSRtxCmd{
		Inputs: inputs,
	}
}

// GetMultisigOutInfoCmd is a type handling custom marshaling and
// unmarshaling of getmultisigoutinfo JSON websocket extension
// commands.
type GetMultisigOutInfoCmd struct {
	Hash  string
	Index uint32
}

// NewGetMultisigOutInfoCmd creates a new GetMultisigOutInfoCmd.
func NewGetMultisigOutInfoCmd(hash string, index uint32) *GetMultisigOutInfoCmd {
	return &GetMultisigOutInfoCmd{hash, index}
}

// GetMasterPubkeyCmd is a type handling custom marshaling and unmarshaling of
// getmasterpubkey JSON wallet extension commands.
type GetMasterPubkeyCmd struct {
}

// NewGetSeedCmd creates a new GetSeedCmd.
func NewGetMasterPubkeyCmd() *GetMasterPubkeyCmd {
	return &GetMasterPubkeyCmd{}
}

// GetSeedCmd is a type handling custom marshaling and
// unmarshaling of getseed JSON wallet extension
// commands.
type GetSeedCmd struct {
}

// NewGetSeedCmd creates a new GetSeedCmd.
func NewGetSeedCmd() *GetSeedCmd {
	return &GetSeedCmd{}
}

// GetTicketMaxPriceCmd is a type handling custom marshaling and
// unmarshaling of getticketmaxprice JSON wallet extension
// commands.
type GetTicketMaxPriceCmd struct {
}

// NewGetTicketMaxPriceCmd creates a new GetTicketMaxPriceCmd.
func NewGetTicketMaxPriceCmd() *GetTicketMaxPriceCmd {
	return &GetTicketMaxPriceCmd{}
}

// GetTicketsCmd is a type handling custom marshaling and
// unmarshaling of gettickets JSON wallet extension
// commands.
type GetTicketsCmd struct {
	IncludeImmature bool
}

// GetWalletFeeCmd defines the getwalletfee JSON-RPC command.
type GetWalletFeeCmd struct{}

// NewGetWalletFeeCmd returns a new instance which can be used to issue a
// getwalletfee JSON-RPC command.
//
func NewGetWalletFeeCmd() *GetWalletFeeCmd {
	return &GetWalletFeeCmd{}
}

// NewGetTicketsCmd creates a new GetTicketsCmd.
func NewGetTicketsCmd(includeImmature bool) *GetTicketsCmd {
	return &GetTicketsCmd{includeImmature}
}

// GetMultisigOutInfoCmd is a type handling custom marshaling and
// unmarshaling of getmultisigoutinfo JSON websocket extension
// commands.
type ImportScriptCmd struct {
	Hex string
}

// NewGetMultisigOutInfoCmd creates a new GetMultisigOutInfoCmd.
func NewImportScriptCmd(hex string) *ImportScriptCmd {
	return &ImportScriptCmd{hex}
}

// NotifyWinningTicketsCmd is a type handling custom marshaling and
// unmarshaling of notifywinningtickets JSON websocket extension
// commands.
type NotifyWinningTicketsCmd struct {
}

// NewNotifyWinningTicketsCmd creates a new NotifyWinningTicketsCmd.
func NewNotifyWinningTicketsCmd() *NotifyWinningTicketsCmd {
	return &NotifyWinningTicketsCmd{}
}

// NotifySpentAndMissedTicketsCmd is a type handling custom marshaling and
// unmarshaling of notifyspentandmissedtickets JSON websocket extension
// commands.
type NotifySpentAndMissedTicketsCmd struct {
}

// NewNotifySpentAndMissedTicketsCmd creates a new NotifySpentAndMissedTicketsCmd.
func NewNotifySpentAndMissedTicketsCmd() *NotifySpentAndMissedTicketsCmd {
	return &NotifySpentAndMissedTicketsCmd{}
}

// NotifyNewTicketsCmd is a type handling custom marshaling and
// unmarshaling of notifynewtickets JSON websocket extension
// commands.
type NotifyNewTicketsCmd struct {
}

// NewNotifyNewTicketsCmd creates a new NotifyNewTicketsCmd.
func NewNotifyNewTicketsCmd() *NotifyNewTicketsCmd {
	return &NotifyNewTicketsCmd{}
}

// NotifyStakeDifficultyCmd is a type handling custom marshaling and
// unmarshaling of notifystakedifficulty JSON websocket extension
// commands.
type NotifyStakeDifficultyCmd struct {
}

// NewNotifyStakeDifficultyCmd creates a new NotifyStakeDifficultyCmd.
func NewNotifyStakeDifficultyCmd() *NotifyStakeDifficultyCmd {
	return &NotifyStakeDifficultyCmd{}
}

// PurchaseTicketCmd is a type handling custom marshaling and
// unmarshaling of purchaseticket JSON RPC commands.
type PurchaseTicketCmd struct {
	FromAccount   string
	SpendLimit    float64 // In Coins
	MinConf       *int    `jsonrpcdefault:"1"`
	TicketAddress *string
	Comment       *string
}

// NewPurchaseTicketCmd creates a new PurchaseTicketCmd.
func NewPurchaseTicketCmd(fromAccount string, spendLimit float64, minConf *int,
	ticketAddress *string, comment *string) *PurchaseTicketCmd {
	return &PurchaseTicketCmd{
		FromAccount:   fromAccount,
		SpendLimit:    spendLimit,
		MinConf:       minConf,
		TicketAddress: ticketAddress,
		Comment:       comment,
	}
}

// RedeemMultiSigOutCmd is a type handling custom marshaling and
// unmarshaling of redeemmultisigout JSON RPC commands.
type RedeemMultiSigOutCmd struct {
	Hash    string
	Index   uint32
	Tree    int8
	Address *string
}

// NewRedeemMultiSigOutCmd creates a new RedeemMultiSigOutCmd.
func NewRedeemMultiSigOutCmd(hash string, index uint32, tree int8,
	address *string) *RedeemMultiSigOutCmd {
	return &RedeemMultiSigOutCmd{
		Hash:    hash,
		Index:   index,
		Tree:    tree,
		Address: address,
	}
}

// RedeemMultiSigOutsCmd is a type handling custom marshaling and
// unmarshaling of redeemmultisigout JSON RPC commands.
type RedeemMultiSigOutsCmd struct {
	FromScrAddress string
	ToAddress      *string
	Number         *int
}

// NewRedeemMultiSigOutCmd creates a new RedeemMultiSigOutCmd.
func NewRedeemMultiSigOutsCmd(from string, to *string,
	number *int) *RedeemMultiSigOutsCmd {
	return &RedeemMultiSigOutsCmd{
		FromScrAddress: from,
		ToAddress:      to,
		Number:         number,
	}
}

// SendToMultisigCmd is a type handling custom marshaling and
// unmarshaling of sendtomultisig JSON RPC commands.
type SendToMultiSigCmd struct {
	FromAccount string
	Amount      float64
	Pubkeys     []string
	NRequired   *int `jsonrpcdefault:"1"`
	MinConf     *int `jsonrpcdefault:"1"`
	Comment     *string
}

// NewSendToMultiSigCmd creates a new SendToMultiSigCmd.
func NewSendToMultiSigCmd(fromaccount string, amount float64, pubkeys []string,
	nrequired *int, minConf *int, comment *string) *SendToMultiSigCmd {
	return &SendToMultiSigCmd{
		FromAccount: fromaccount,
		Amount:      amount,
		Pubkeys:     pubkeys,
		NRequired:   nrequired,
		MinConf:     minConf,
		Comment:     comment,
	}
}

// SendToSStxCmd is a type handling custom marshaling and
// unmarshaling of sendtosstx JSON RPC commands.
type SendToSStxCmd struct {
	FromAccount string
	Amounts     map[string]int64
	Inputs      []SStxInput
	COuts       []SStxCommitOut
	MinConf     *int `jsonrpcdefault:"1"`
	Comment     *string
}

// NewSendToSStxCmd creates a new SendToSStxCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSendToSStxCmd(fromaccount string, amounts map[string]int64,
	inputs []SStxInput, couts []SStxCommitOut, minConf *int,
	comment *string) *SendToSStxCmd {
	return &SendToSStxCmd{
		FromAccount: fromaccount,
		Amounts:     amounts,
		Inputs:      inputs,
		COuts:       couts,
		MinConf:     minConf,
		Comment:     comment,
	}
}

type SendToSSGenCmd struct {
	FromAccount string
	TicketHash  string
	BlockHash   string
	Height      int64
	VoteBits    uint16
	Comment     *string
}

// NewSendToSSGenCmd creates a new SendToSSGenCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSendToSSGenCmd(fromaccount string, tickethash string, blockhash string,
	height int64, votebits uint16, comment *string) *SendToSSGenCmd {
	return &SendToSSGenCmd{
		FromAccount: fromaccount,
		TicketHash:  tickethash,
		BlockHash:   blockhash,
		Height:      height,
		VoteBits:    votebits,
		Comment:     comment,
	}
}

// SendToSSRtxCmd is a type handling custom marshaling and
// unmarshaling of sendtossrtx JSON RPC commands.
type SendToSSRtxCmd struct {
	FromAccount string
	TicketHash  string
	Comment     *string
}

// NewSendToSSRtxCmd creates a new SendToSSRtxCmd. Optionally a
// pointer to a TemplateRequest may be provided.
func NewSendToSSRtxCmd(fromaccount string, tickethash string,
	comment *string) *SendToSSRtxCmd {
	return &SendToSSRtxCmd{
		FromAccount: fromaccount,
		TicketHash:  tickethash,
		Comment:     comment,
	}
}

// SetTicketMaxPriceCmd is a type handling custom marshaling and
// unmarshaling of setticketmaxprice JSON RPC commands.
type SetTicketMaxPriceCmd struct {
	Max float64
}

func NewSetTicketMaxPriceCmd(max float64) *SetTicketMaxPriceCmd {
	return &SetTicketMaxPriceCmd{
		Max: max,
	}
}

// SignRawTransactionsCmd defines the signrawtransactions JSON-RPC command.
type SignRawTransactionsCmd struct {
	RawTxs []string
	Send   *bool `jsonrpcdefault:"true"`
}

// NewSignRawTransactionCmd returns a new instance which can be used to issue a
// signrawtransactions JSON-RPC command.
func NewSignRawTransactionsCmd(hexEncodedTxs []string,
	send *bool) *SignRawTransactionsCmd {
	return &SignRawTransactionsCmd{
		RawTxs: hexEncodedTxs,
		Send:   send,
	}
}

func init() {
	// The commands in this file are only usable with a wallet
	// server.
	flags := UFWalletOnly

	MustRegisterCmd("createrawsstx", (*CreateRawSStxCmd)(nil), flags)
	MustRegisterCmd("createrawssgentx", (*CreateRawSSGenTxCmd)(nil), flags)
	MustRegisterCmd("createrawssrtx", (*CreateRawSSRtxCmd)(nil), flags)
	MustRegisterCmd("getmultisigoutinfo", (*GetMultisigOutInfoCmd)(nil), flags)
	MustRegisterCmd("getmasterpubkey", (*GetMasterPubkeyCmd)(nil), flags)
	MustRegisterCmd("getseed", (*GetSeedCmd)(nil), flags)
	MustRegisterCmd("getticketmaxprice", (*GetTicketMaxPriceCmd)(nil), flags)
	MustRegisterCmd("gettickets", (*GetTicketsCmd)(nil), flags)
	MustRegisterCmd("importscript", (*ImportScriptCmd)(nil), flags)
	MustRegisterCmd("notifynewtickets", (*NotifyNewTicketsCmd)(nil), flags)
	MustRegisterCmd("notifyspentandmissedtickets",
		(*NotifySpentAndMissedTicketsCmd)(nil), flags)
	MustRegisterCmd("notifystakedifficulty",
		(*NotifyStakeDifficultyCmd)(nil), flags)
	MustRegisterCmd("notifywinningtickets",
		(*NotifyWinningTicketsCmd)(nil), flags)
	MustRegisterCmd("purchaseticket", (*PurchaseTicketCmd)(nil), flags)
	MustRegisterCmd("redeemmultisigout", (*RedeemMultiSigOutCmd)(nil), flags)
	MustRegisterCmd("redeemmultisigouts", (*RedeemMultiSigOutsCmd)(nil), flags)
	MustRegisterCmd("sendtomultisig", (*SendToMultiSigCmd)(nil), flags)
	MustRegisterCmd("sendtosstx", (*SendToSStxCmd)(nil), flags)
	MustRegisterCmd("sendtossgen", (*SendToSSGenCmd)(nil), flags)
	MustRegisterCmd("sendtossrtx", (*SendToSSRtxCmd)(nil), flags)
	MustRegisterCmd("setticketmaxprice", (*SetTicketMaxPriceCmd)(nil), flags)
	MustRegisterCmd("signrawtransactions", (*SignRawTransactionsCmd)(nil), flags)
}
