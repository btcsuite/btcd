// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are
// supported by a wallet server with btcwallet extensions.

package dcrjson

// AccountAddressIndexCmd is a type handling custom marshaling and
// unmarshaling of accountaddressindex JSON wallet extension
// commands.
type AccountAddressIndexCmd struct {
	Account string `json:"account"`
	Branch  int    `json:"branch"`
}

// NewAccountAddressIndexCmd creates a new AccountAddressIndexCmd.
func NewAccountAddressIndexCmd(acct string, branch int) *AccountAddressIndexCmd {
	return &AccountAddressIndexCmd{
		Account: acct,
		Branch:  branch,
	}
}

// AccountSyncAddressIndexCmd is a type handling custom marshaling and
// unmarshaling of accountsyncaddressindex JSON wallet extension
// commands.
type AccountSyncAddressIndexCmd struct {
	Account string `json:"account"`
	Branch  int    `json:"branch"`
	Index   int    `json:"index"`
}

// NewAccountSyncAddressIndexCmd creates a new AccountSyncAddressIndexCmd.
func NewAccountSyncAddressIndexCmd(acct string, branch int,
	idx int) *AccountSyncAddressIndexCmd {
	return &AccountSyncAddressIndexCmd{
		Account: acct,
		Branch:  branch,
		Index:   idx,
	}
}

// AddTicketCmd forces a ticket into the wallet stake manager, for example if
// someone made an invalid ticket for a stake pool and the pool manager wanted
// to manually add it.
type AddTicketCmd struct {
	TicketHex string `json:"tickethex"`
}

// NewAddTicketCmd creates a new AddTicketCmd.
func NewAddTicketCmd(ticketHex string) *AddTicketCmd {
	return &AddTicketCmd{TicketHex: ticketHex}
}

// ConsolidateCmd is a type handling custom marshaling and
// unmarshaling of consolidate JSON wallet extension
// commands.
type ConsolidateCmd struct {
	Inputs  int `json:"inputs"`
	Account *string
	Address *string
}

// NewConsolidateCmd creates a new ConsolidateCmd.
func NewConsolidateCmd(inputs int, acct *string, addr *string) *ConsolidateCmd {
	return &ConsolidateCmd{Inputs: inputs, Account: acct, Address: addr}
}

// SStxInput represents the inputs to an SStx transaction. Specifically a
// transactionsha and output number pair, along with the output amounts.
type SStxInput struct {
	Txid string `json:"txid"`
	Vout uint32 `json:"vout"`
	Tree int8   `json:"tree"`
	Amt  int64  `json:"amt"`
}

// SStxCommitOut represents the output to an SStx transaction. Specifically a
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
	Fee    *float64
}

// NewCreateRawSSRtxCmd creates a new CreateRawSSRtxCmd.
func NewCreateRawSSRtxCmd(inputs []TransactionInput, fee *float64) *CreateRawSSRtxCmd {
	return &CreateRawSSRtxCmd{
		Inputs: inputs,
		Fee:    fee,
	}
}

// GenerateVoteCmd is a type handling custom marshaling and
// unmarshaling of generatevote JSON wallet extension commands.
type GenerateVoteCmd struct {
	BlockHash   string
	Height      int64
	TicketHash  string
	VoteBits    uint16
	VoteBitsExt string
}

// NewGenerateVoteCmd creates a new GenerateVoteCmd.
func NewGenerateVoteCmd(blockhash string, height int64, tickethash string, votebits uint16, voteBitsExt string) *GenerateVoteCmd {
	return &GenerateVoteCmd{
		BlockHash:   blockhash,
		Height:      height,
		TicketHash:  tickethash,
		VoteBits:    votebits,
		VoteBitsExt: voteBitsExt,
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
	Account *string
}

// NewGetMasterPubkeyCmd creates a new GetMasterPubkeyCmd.
func NewGetMasterPubkeyCmd(acct *string) *GetMasterPubkeyCmd {
	return &GetMasterPubkeyCmd{Account: acct}
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

// GetStakeInfoCmd is a type handling custom marshaling and
// unmarshaling of getstakeinfo JSON wallet extension commands.
type GetStakeInfoCmd struct {
}

// NewGetStakeInfoCmd creates a new GetStakeInfoCmd.
func NewGetStakeInfoCmd() *GetStakeInfoCmd {
	return &GetStakeInfoCmd{}
}

// GetTicketFeeCmd is a type handling custom marshaling and
// unmarshaling of getticketfee JSON wallet extension
// commands.
type GetTicketFeeCmd struct {
}

// NewGetTicketFeeCmd creates a new GetTicketFeeCmd.
func NewGetTicketFeeCmd() *GetTicketFeeCmd {
	return &GetTicketFeeCmd{}
}

// GetTicketsCmd is a type handling custom marshaling and
// unmarshaling of gettickets JSON wallet extension
// commands.
type GetTicketsCmd struct {
	IncludeImmature bool
}

// NewGetTicketsCmd returns a new instance which can be used to issue a
// gettickets JSON-RPC command.
func NewGetTicketsCmd(includeImmature bool) *GetTicketsCmd {
	return &GetTicketsCmd{includeImmature}
}

// GetVoteChoicesCmd returns a new instance which can be used to issue a
// getvotechoices JSON-RPC command.
type GetVoteChoicesCmd struct {
}

// NewGetVoteChoicesCmd returns a new instance which can be used to
// issue a JSON-RPC getvotechoices command.
func NewGetVoteChoicesCmd() *GetVoteChoicesCmd {
	return &GetVoteChoicesCmd{}
}

// GetWalletFeeCmd defines the getwalletfee JSON-RPC command.
type GetWalletFeeCmd struct{}

// NewGetWalletFeeCmd returns a new instance which can be used to issue a
// getwalletfee JSON-RPC command.
//
func NewGetWalletFeeCmd() *GetWalletFeeCmd {
	return &GetWalletFeeCmd{}
}

// ImportScriptCmd is a type for handling custom marshaling and
// unmarshaling of importscript JSON wallet extension commands.
type ImportScriptCmd struct {
	Hex      string
	Rescan   *bool `jsonrpcdefault:"true"`
	ScanFrom *int
}

// NewImportScriptCmd creates a new GetImportScriptCmd.
func NewImportScriptCmd(hex string, rescan *bool, scanFrom *int) *ImportScriptCmd {
	return &ImportScriptCmd{hex, rescan, scanFrom}
}

// ListScriptsCmd is a type for handling custom marshaling and
// unmarshaling of listscripts JSON wallet extension commands.
type ListScriptsCmd struct {
}

// NewListScriptsCmd returns a new instance which can be used to issue a
// listscripts JSON-RPC command.
func NewListScriptsCmd() *ListScriptsCmd {
	return &ListScriptsCmd{}
}

// PurchaseTicketCmd is a type handling custom marshaling and
// unmarshaling of purchaseticket JSON RPC commands.
type PurchaseTicketCmd struct {
	FromAccount        string
	SpendLimit         float64 // In Coins
	MinConf            *int    `jsonrpcdefault:"1"`
	TicketAddress      *string
	NumTickets         *int
	PoolAddress        *string
	PoolFees           *float64
	Expiry             *int
	Comment            *string
	NoSplitTransaction *bool
}

// NewPurchaseTicketCmd creates a new PurchaseTicketCmd.
func NewPurchaseTicketCmd(fromAccount string, spendLimit float64, minConf *int,
	ticketAddress *string, numTickets *int, poolAddress *string, poolFees *float64,
	expiry *int, comment *string, noSplitTransaction *bool) *PurchaseTicketCmd {
	return &PurchaseTicketCmd{
		FromAccount:        fromAccount,
		SpendLimit:         spendLimit,
		MinConf:            minConf,
		TicketAddress:      ticketAddress,
		NumTickets:         numTickets,
		PoolAddress:        poolAddress,
		PoolFees:           poolFees,
		Expiry:             expiry,
		Comment:            comment,
		NoSplitTransaction: noSplitTransaction,
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

// NewRedeemMultiSigOutsCmd creates a new RedeemMultiSigOutsCmd.
func NewRedeemMultiSigOutsCmd(from string, to *string,
	number *int) *RedeemMultiSigOutsCmd {
	return &RedeemMultiSigOutsCmd{
		FromScrAddress: from,
		ToAddress:      to,
		Number:         number,
	}
}

// RescanWalletCmd describes the rescanwallet JSON-RPC request and parameters.
type RescanWalletCmd struct {
	BeginHeight *int `jsonrpcdefault:"0"`
}

// RevokeTicketsCmd describes the revoketickets JSON-RPC request and parameters.
type RevokeTicketsCmd struct {
}

// NewRevokeTicketsCmd creates a new RevokeTicketsCmd.
func NewRevokeTicketsCmd() *RevokeTicketsCmd {
	return &RevokeTicketsCmd{}
}

// SendToMultiSigCmd is a type handling custom marshaling and
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

// SendToSSGenCmd models the data needed for sendtossgen.
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

// SetBalanceToMaintainCmd is a type handling custom marshaling and
// unmarshaling of setbalancetomaintain JSON RPC commands.
type SetBalanceToMaintainCmd struct {
	Balance float64
}

// NewSetBalanceToMaintainCmd creates a new instance of the setticketfee
// command.
func NewSetBalanceToMaintainCmd(balance float64) *SetBalanceToMaintainCmd {
	return &SetBalanceToMaintainCmd{
		Balance: balance,
	}
}

// SetTicketFeeCmd is a type handling custom marshaling and
// unmarshaling of setticketfee JSON RPC commands.
type SetTicketFeeCmd struct {
	Fee float64
}

// NewSetTicketFeeCmd creates a new instance of the setticketfee
// command.
func NewSetTicketFeeCmd(fee float64) *SetTicketFeeCmd {
	return &SetTicketFeeCmd{
		Fee: fee,
	}
}

// SetTicketMaxPriceCmd is a type handling custom marshaling and
// unmarshaling of setticketmaxprice JSON RPC commands.
type SetTicketMaxPriceCmd struct {
	Max float64
}

// NewSetTicketMaxPriceCmd creates a new instance of the setticketmaxprice
// command.
func NewSetTicketMaxPriceCmd(max float64) *SetTicketMaxPriceCmd {
	return &SetTicketMaxPriceCmd{
		Max: max,
	}
}

// SetVoteChoiceCmd defines the parameters to the setvotechoice method.
type SetVoteChoiceCmd struct {
	AgendaID string
	ChoiceID string
}

// NewSetVoteChoiceCmd returns a new instance which can be used to issue a
// setvotechoice JSON-RPC command.
func NewSetVoteChoiceCmd(agendaID, choiceID string) *SetVoteChoiceCmd {
	return &SetVoteChoiceCmd{AgendaID: agendaID, ChoiceID: choiceID}
}

// SignRawTransactionsCmd defines the signrawtransactions JSON-RPC command.
type SignRawTransactionsCmd struct {
	RawTxs []string
	Send   *bool `jsonrpcdefault:"true"`
}

// NewSignRawTransactionsCmd returns a new instance which can be used to issue a
// signrawtransactions JSON-RPC command.
func NewSignRawTransactionsCmd(hexEncodedTxs []string,
	send *bool) *SignRawTransactionsCmd {
	return &SignRawTransactionsCmd{
		RawTxs: hexEncodedTxs,
		Send:   send,
	}
}

// StakePoolUserInfoCmd defines the stakepooluserinfo JSON-RPC command.
type StakePoolUserInfoCmd struct {
	User string
}

// NewStakePoolUserInfoCmd returns a new instance which can be used to issue a
// signrawtransactions JSON-RPC command.
func NewStakePoolUserInfoCmd(user string) *StakePoolUserInfoCmd {
	return &StakePoolUserInfoCmd{
		User: user,
	}
}

// WalletInfoCmd defines the walletinfo JSON-RPC command.
type WalletInfoCmd struct {
}

// NewWalletInfoCmd returns a new instance which can be used to issue a
// walletinfo JSON-RPC command.
func NewWalletInfoCmd() *WalletInfoCmd {
	return &WalletInfoCmd{}
}

func init() {
	// The commands in this file are only usable with a wallet
	// server.
	flags := UFWalletOnly

	MustRegisterCmd("accountaddressindex", (*AccountAddressIndexCmd)(nil), flags)
	MustRegisterCmd("accountsyncaddressindex", (*AccountSyncAddressIndexCmd)(nil), flags)
	MustRegisterCmd("addticket", (*AddTicketCmd)(nil), flags)
	MustRegisterCmd("consolidate", (*ConsolidateCmd)(nil), flags)
	MustRegisterCmd("createrawsstx", (*CreateRawSStxCmd)(nil), flags)
	MustRegisterCmd("createrawssgentx", (*CreateRawSSGenTxCmd)(nil), flags)
	MustRegisterCmd("createrawssrtx", (*CreateRawSSRtxCmd)(nil), flags)
	MustRegisterCmd("generatevote", (*GenerateVoteCmd)(nil), flags)
	MustRegisterCmd("getmultisigoutinfo", (*GetMultisigOutInfoCmd)(nil), flags)
	MustRegisterCmd("getmasterpubkey", (*GetMasterPubkeyCmd)(nil), flags)
	MustRegisterCmd("getseed", (*GetSeedCmd)(nil), flags)
	MustRegisterCmd("getstakeinfo", (*GetStakeInfoCmd)(nil), flags)
	MustRegisterCmd("getticketfee", (*GetTicketFeeCmd)(nil), flags)
	MustRegisterCmd("gettickets", (*GetTicketsCmd)(nil), flags)
	MustRegisterCmd("getvotechoices", (*GetVoteChoicesCmd)(nil), flags)
	MustRegisterCmd("importscript", (*ImportScriptCmd)(nil), flags)
	MustRegisterCmd("listscripts", (*ListScriptsCmd)(nil), flags)
	MustRegisterCmd("purchaseticket", (*PurchaseTicketCmd)(nil), flags)
	MustRegisterCmd("redeemmultisigout", (*RedeemMultiSigOutCmd)(nil), flags)
	MustRegisterCmd("redeemmultisigouts", (*RedeemMultiSigOutsCmd)(nil), flags)
	MustRegisterCmd("rescanwallet", (*RescanWalletCmd)(nil), flags)
	MustRegisterCmd("revoketickets", (*RevokeTicketsCmd)(nil), flags)
	MustRegisterCmd("sendtomultisig", (*SendToMultiSigCmd)(nil), flags)
	MustRegisterCmd("sendtosstx", (*SendToSStxCmd)(nil), flags)
	MustRegisterCmd("sendtossgen", (*SendToSSGenCmd)(nil), flags)
	MustRegisterCmd("sendtossrtx", (*SendToSSRtxCmd)(nil), flags)
	MustRegisterCmd("setbalancetomaintain", (*SetBalanceToMaintainCmd)(nil), flags)
	MustRegisterCmd("setticketfee", (*SetTicketFeeCmd)(nil), flags)
	MustRegisterCmd("setticketmaxprice", (*SetTicketMaxPriceCmd)(nil), flags)
	MustRegisterCmd("setvotechoice", (*SetVoteChoiceCmd)(nil), flags)
	MustRegisterCmd("signrawtransactions", (*SignRawTransactionsCmd)(nil), flags)
	MustRegisterCmd("stakepooluserinfo", (*StakePoolUserInfoCmd)(nil), flags)
	MustRegisterCmd("walletinfo", (*WalletInfoCmd)(nil), flags)
}
