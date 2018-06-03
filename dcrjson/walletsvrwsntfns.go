// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC websocket notifications that are
// supported by a wallet server.

package dcrjson

const (
	// AccountBalanceNtfnMethod is the method used for account balance
	// notifications.
	AccountBalanceNtfnMethod = "accountbalance"

	// DcrdConnectedNtfnMethod is the method used for notifications when
	// a wallet server is connected to a chain server.
	DcrdConnectedNtfnMethod = "dcrdconnected"

	// NewTicketsNtfnMethod is the method of the daemon
	// newtickets notification.
	NewTicketsNtfnMethod = "newtickets"

	// NewTxNtfnMethod is the method used to notify that a wallet server has
	// added a new transaction to the transaction store.
	NewTxNtfnMethod = "newtx"

	// RevocationCreatedNtfnMethod is the method of the dcrwallet
	// revocationcreated notification.
	RevocationCreatedNtfnMethod = "revocationcreated"

	// SpentAndMissedTicketsNtfnMethod is the method of the daemon
	// spentandmissedtickets notification.
	SpentAndMissedTicketsNtfnMethod = "spentandmissedtickets"

	// StakeDifficultyNtfnMethod is the method of the daemon
	// stakedifficulty notification.
	StakeDifficultyNtfnMethod = "stakedifficulty"

	// TicketPurchasedNtfnMethod is the method of the dcrwallet
	// ticketpurchased notification.
	TicketPurchasedNtfnMethod = "ticketpurchased"

	// VoteCreatedNtfnMethod is the method of the dcrwallet
	// votecreated notification.
	VoteCreatedNtfnMethod = "votecreated"

	// WinningTicketsNtfnMethod is the method of the daemon
	// winningtickets notification.
	WinningTicketsNtfnMethod = "winningtickets"

	// WalletLockStateNtfnMethod is the method used to notify the lock state
	// of a wallet has changed.
	WalletLockStateNtfnMethod = "walletlockstate"
)

// AccountBalanceNtfn defines the accountbalance JSON-RPC notification.
type AccountBalanceNtfn struct {
	Account   string
	Balance   float64 // In DCR
	Confirmed bool    // Whether Balance is confirmed or unconfirmed.
}

// NewAccountBalanceNtfn returns a new instance which can be used to issue an
// accountbalance JSON-RPC notification.
func NewAccountBalanceNtfn(account string, balance float64, confirmed bool) *AccountBalanceNtfn {
	return &AccountBalanceNtfn{
		Account:   account,
		Balance:   balance,
		Confirmed: confirmed,
	}
}

// DcrdConnectedNtfn defines the dcrddconnected JSON-RPC notification.
type DcrdConnectedNtfn struct {
	Connected bool
}

// NewDcrdConnectedNtfn returns a new instance which can be used to issue a
// dcrddconnected JSON-RPC notification.
func NewDcrdConnectedNtfn(connected bool) *DcrdConnectedNtfn {
	return &DcrdConnectedNtfn{
		Connected: connected,
	}
}

// NewTicketsNtfn is a type handling custom marshaling and
// unmarshaling of newtickets JSON websocket notifications.
type NewTicketsNtfn struct {
	Hash      string
	Height    int32
	StakeDiff int64
	Tickets   []string
}

// NewNewTicketsNtfn creates a new NewTicketsNtfn.
func NewNewTicketsNtfn(hash string, height int32, stakeDiff int64, tickets []string) *NewTicketsNtfn {
	return &NewTicketsNtfn{
		Hash:      hash,
		Height:    height,
		StakeDiff: stakeDiff,
		Tickets:   tickets,
	}
}

// NewTxNtfn defines the newtx JSON-RPC notification.
type NewTxNtfn struct {
	Account string
	Details ListTransactionsResult
}

// NewNewTxNtfn returns a new instance which can be used to issue a newtx
// JSON-RPC notification.
func NewNewTxNtfn(account string, details ListTransactionsResult) *NewTxNtfn {
	return &NewTxNtfn{
		Account: account,
		Details: details,
	}
}

// TicketPurchasedNtfn is a type handling custom marshaling and
// unmarshaling of ticketpurchased JSON websocket notifications.
type TicketPurchasedNtfn struct {
	TxHash string
	Amount int64 // SStx only
}

// NewTicketPurchasedNtfn creates a new TicketPurchasedNtfn.
func NewTicketPurchasedNtfn(txHash string, amount int64) *TicketPurchasedNtfn {
	return &TicketPurchasedNtfn{
		TxHash: txHash,
		Amount: amount,
	}
}

// RevocationCreatedNtfn is a type handling custom marshaling and
// unmarshaling of ticketpurchased JSON websocket notifications.
type RevocationCreatedNtfn struct {
	TxHash string
	SStxIn string
}

// NewRevocationCreatedNtfn creates a new RevocationCreatedNtfn.
func NewRevocationCreatedNtfn(txHash string, sstxIn string) *RevocationCreatedNtfn {
	return &RevocationCreatedNtfn{
		TxHash: txHash,
		SStxIn: sstxIn,
	}
}

// SpentAndMissedTicketsNtfn is a type handling custom marshaling and
// unmarshaling of spentandmissedtickets JSON websocket notifications.
type SpentAndMissedTicketsNtfn struct {
	Hash      string
	Height    int32
	StakeDiff int64
	Tickets   map[string]string
}

// NewSpentAndMissedTicketsNtfn creates a new SpentAndMissedTicketsNtfn.
func NewSpentAndMissedTicketsNtfn(hash string, height int32, stakeDiff int64, tickets map[string]string) *SpentAndMissedTicketsNtfn {
	return &SpentAndMissedTicketsNtfn{
		Hash:      hash,
		Height:    height,
		StakeDiff: stakeDiff,
		Tickets:   tickets,
	}
}

// StakeDifficultyNtfn is a type handling custom marshaling and
// unmarshaling of stakedifficulty JSON websocket notifications.
type StakeDifficultyNtfn struct {
	BlockHash   string
	BlockHeight int32
	StakeDiff   int64
}

// NewStakeDifficultyNtfn creates a new StakeDifficultyNtfn.
func NewStakeDifficultyNtfn(hash string, height int32, stakeDiff int64) *StakeDifficultyNtfn {
	return &StakeDifficultyNtfn{
		BlockHash:   hash,
		BlockHeight: height,
		StakeDiff:   stakeDiff,
	}
}

// VoteCreatedNtfn is a type handling custom marshaling and
// unmarshaling of ticketpurchased JSON websocket notifications.
type VoteCreatedNtfn struct {
	TxHash    string
	BlockHash string
	Height    int32
	SStxIn    string
	VoteBits  uint16
}

// NewVoteCreatedNtfn creates a new VoteCreatedNtfn.
func NewVoteCreatedNtfn(txHash string, blockHash string, height int32, sstxIn string, voteBits uint16) *VoteCreatedNtfn {
	return &VoteCreatedNtfn{
		TxHash:    txHash,
		BlockHash: blockHash,
		Height:    height,
		SStxIn:    sstxIn,
		VoteBits:  voteBits,
	}
}

// WalletLockStateNtfn defines the walletlockstate JSON-RPC notification.
type WalletLockStateNtfn struct {
	Locked bool
}

// NewWalletLockStateNtfn returns a new instance which can be used to issue a
// walletlockstate JSON-RPC notification.
func NewWalletLockStateNtfn(locked bool) *WalletLockStateNtfn {
	return &WalletLockStateNtfn{
		Locked: locked,
	}
}

// WinningTicketsNtfn is a type handling custom marshaling and
// unmarshaling of blockconnected JSON websocket notifications.
type WinningTicketsNtfn struct {
	BlockHash   string
	BlockHeight int32
	Tickets     map[string]string
}

// NewWinningTicketsNtfn creates a new WinningTicketsNtfn.
func NewWinningTicketsNtfn(hash string, height int32, tickets map[string]string) *WinningTicketsNtfn {
	return &WinningTicketsNtfn{
		BlockHash:   hash,
		BlockHeight: height,
		Tickets:     tickets,
	}
}

func init() {
	// The commands in this file are only usable with a wallet server via
	// websockets and are notifications.
	flags := UFWalletOnly | UFWebsocketOnly | UFNotification

	MustRegisterCmd(AccountBalanceNtfnMethod, (*AccountBalanceNtfn)(nil), flags)
	MustRegisterCmd(DcrdConnectedNtfnMethod, (*DcrdConnectedNtfn)(nil), flags)
	MustRegisterCmd(NewTicketsNtfnMethod, (*NewTicketsNtfn)(nil), flags)
	MustRegisterCmd(NewTxNtfnMethod, (*NewTxNtfn)(nil), flags)
	MustRegisterCmd(TicketPurchasedNtfnMethod, (*TicketPurchasedNtfn)(nil), flags)
	MustRegisterCmd(RevocationCreatedNtfnMethod, (*RevocationCreatedNtfn)(nil), flags)
	MustRegisterCmd(SpentAndMissedTicketsNtfnMethod, (*SpentAndMissedTicketsNtfn)(nil), flags)
	MustRegisterCmd(StakeDifficultyNtfnMethod, (*StakeDifficultyNtfn)(nil), flags)
	MustRegisterCmd(VoteCreatedNtfnMethod, (*VoteCreatedNtfn)(nil), flags)
	MustRegisterCmd(WalletLockStateNtfnMethod, (*WalletLockStateNtfn)(nil), flags)
	MustRegisterCmd(WinningTicketsNtfnMethod, (*WinningTicketsNtfn)(nil), flags)
}
