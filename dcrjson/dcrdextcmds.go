// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server with btcd extensions.

package dcrjson

// ExistsAddressCmd defines the existsaddress JSON-RPC command.
type ExistsAddressCmd struct {
	Address string
}

// NewExistsAddressCmd returns a new instance which can be used to issue a
// existsaddress JSON-RPC command.
//
// The parameters which are pointers indicate they are optional.  Passing nil
// for optional parameters will use the default value.
func NewExistsAddressCmd(address string) *ExistsAddressCmd {
	return &ExistsAddressCmd{
		Address: address,
	}
}

// GetStakeDifficultyCmd is a type handling custom marshaling and
// unmarshaling of getstakedifficulty JSON RPC commands.
type GetStakeDifficultyCmd struct{}

// NewGetStakeDifficultyCmd returns a new instance which can be used to
// issue a JSON-RPC getstakedifficulty command.
func NewGetStakeDifficultyCmd() *GetStakeDifficultyCmd {
	return &GetStakeDifficultyCmd{}
}

// MissedTicketsCmd is a type handling custom marshaling and
// unmarshaling of missedtickets JSON RPC commands.
type MissedTicketsCmd struct{}

// NewMissedTicketsCmd returns a new instance which can be used to issue a JSON-RPC
// missedtickets command.
func NewMissedTicketsCmd() *MissedTicketsCmd {
	return &MissedTicketsCmd{}
}

// RebroadcastMissedCmd is a type handling custom marshaling and
// unmarshaling of rebroadcastwinners JSON RPC commands.
type RebroadcastMissedCmd struct{}

// NewRebroadcastMissedCmd returns a new instance which can be used to
// issue a JSON-RPC rebroadcastmissed command.
func NewRebroadcastMissedCmd() *RebroadcastMissedCmd {
	return &RebroadcastMissedCmd{}
}

// RebroadcastWinnersCmd is a type handling custom marshaling and
// unmarshaling of rebroadcastwinners JSON RPC commands.
type RebroadcastWinnersCmd struct{}

// NewRebroadcastWinnersCmd returns a new instance which can be used to
// issue a JSON-RPC rebroadcastwinners command.
func NewRebroadcastWinnersCmd() *RebroadcastWinnersCmd {
	return &RebroadcastWinnersCmd{}
}

// TicketsForAddressCmd defines the ticketsforbucket JSON-RPC command.
type TicketsForAddressCmd struct {
	Address string
}

// NewTicketsForAddressCmd returns a new instance which can be used to issue a
// JSON-RPC tickets for bucket command.
func NewTicketsForAddressCmd(addr string) *TicketsForAddressCmd {
	return &TicketsForAddressCmd{addr}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("existsaddress", (*ExistsAddressCmd)(nil), flags)
	MustRegisterCmd("getstakedifficulty", (*GetStakeDifficultyCmd)(nil), flags)
	MustRegisterCmd("missedtickets", (*MissedTicketsCmd)(nil), flags)
	MustRegisterCmd("rebroadcastmissed", (*RebroadcastMissedCmd)(nil), flags)
	MustRegisterCmd("rebroadcastwinners", (*RebroadcastWinnersCmd)(nil), flags)
	MustRegisterCmd("ticketsforaddress", (*TicketsForAddressCmd)(nil), flags)
}
