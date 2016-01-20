// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC websocket notifications that are
// supported by a chain server.

package dcrjson

const (
	// TicketPurchasedNtfnMethod is the method of the dcrwallet
	// ticketpurchased notification.
	TicketPurchasedNtfnMethod = "ticketpurchased"

	// VoteCreatedNtfnMethod is the method of the dcrwallet
	// votecreated notification.
	VoteCreatedNtfnMethod = "votecreated"

	// RevocationCreatedNtfnMethod is the method of the dcrwallet
	// revocationcreated notification.
	RevocationCreatedNtfnMethod = "revocationcreated"

	// WinningTicketsNtfnMethod is the method of the daemon
	// winningtickets notification.
	WinningTicketsNtfnMethod = "winningtickets"

	// SpentAndMissedTicketsNtfnMethod is the method of the daemon
	// spentandmissedtickets notification.
	SpentAndMissedTicketsNtfnMethod = "spentandmissedtickets"

	// NewTicketsNtfnMethod is the method of the daemon
	// newtickets notification.
	NewTicketsNtfnMethod = "newtickets"

	// StakeDifficultyNtfnMethod is the method of the daemon
	// stakedifficulty notification.
	StakeDifficultyNtfnMethod = "stakedifficulty"
)

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

func init() {
	// The commands in this file are only usable by websockets and are
	// notifications.
	flags := UFWalletOnly | UFWebsocketOnly | UFNotification

	MustRegisterCmd(TicketPurchasedNtfnMethod, (*TicketPurchasedNtfn)(nil), flags)
	MustRegisterCmd(VoteCreatedNtfnMethod, (*VoteCreatedNtfn)(nil), flags)
	MustRegisterCmd(RevocationCreatedNtfnMethod, (*RevocationCreatedNtfn)(nil), flags)
	MustRegisterCmd(WinningTicketsNtfnMethod, (*WinningTicketsNtfn)(nil), flags)
	MustRegisterCmd(SpentAndMissedTicketsNtfnMethod, (*SpentAndMissedTicketsNtfn)(nil), flags)
	MustRegisterCmd(NewTicketsNtfnMethod, (*NewTicketsNtfn)(nil), flags)
	MustRegisterCmd(StakeDifficultyNtfnMethod, (*StakeDifficultyNtfn)(nil), flags)
}
