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
func NewExistsAddressCmd(address string) *ExistsAddressCmd {
	return &ExistsAddressCmd{
		Address: address,
	}
}

// ExistsAddressesCmd defines the existsaddresses JSON-RPC command.
type ExistsAddressesCmd struct {
	Addresses []string
}

// NewExistsAddressesCmd returns a new instance which can be used to issue an
// existsaddresses JSON-RPC command.
func NewExistsAddressesCmd(addresses []string) *ExistsAddressesCmd {
	return &ExistsAddressesCmd{
		Addresses: addresses,
	}
}

// ExistsLiveTicketCmd defines the existsliveticket JSON-RPC command.
type ExistsLiveTicketCmd struct {
	TxHash string
}

// NewExistsLiveTicketCmd returns a new instance which can be used to issue an
// existsliveticket JSON-RPC command.
func NewExistsLiveTicketCmd(txHash string) *ExistsLiveTicketCmd {
	return &ExistsLiveTicketCmd{
		TxHash: txHash,
	}
}

// ExistsLiveTicketsCmd defines the existslivetickets JSON-RPC command.
type ExistsLiveTicketsCmd struct {
	TxHashBlob string
}

// NewExistsLiveTicketsCmd returns a new instance which can be used to issue an
// existslivetickets JSON-RPC command.
func NewExistsLiveTicketsCmd(txHashBlob string) *ExistsLiveTicketsCmd {
	return &ExistsLiveTicketsCmd{
		TxHashBlob: txHashBlob,
	}
}

// ExistsMempoolTxsCmd defines the existsmempooltxs JSON-RPC command.
type ExistsMempoolTxsCmd struct {
	TxHashBlob string
}

// NewExistsMempoolTxsCmd returns a new instance which can be used to issue an
// existslivetickets JSON-RPC command.
func NewExistsMempoolTxsCmd(txHashBlob string) *ExistsMempoolTxsCmd {
	return &ExistsMempoolTxsCmd{
		TxHashBlob: txHashBlob,
	}
}

// GetCoinSupplyCmd defines the getcoinsupply JSON-RPC command.
type GetCoinSupplyCmd struct{}

// NewGetCoinSupplyCmd returns a new instance which can be used to issue a
// getcoinsupply JSON-RPC command.
func NewGetCoinSupplyCmd() *GetCoinSupplyCmd {
	return &GetCoinSupplyCmd{}
}

// GetStakeDifficultyCmd is a type handling custom marshaling and
// unmarshaling of getstakedifficulty JSON RPC commands.
type GetStakeDifficultyCmd struct{}

// NewGetStakeDifficultyCmd returns a new instance which can be used to
// issue a JSON-RPC getstakedifficulty command.
func NewGetStakeDifficultyCmd() *GetStakeDifficultyCmd {
	return &GetStakeDifficultyCmd{}
}

// GetTicketPoolValueCmd defines the getticketpoolvalue JSON-RPC command.
type GetTicketPoolValueCmd struct{}

// NewGetTicketPoolValueCmd returns a new instance which can be used to issue a
// getticketpoolvalue JSON-RPC command.
func NewGetTicketPoolValueCmd() *GetTicketPoolValueCmd {
	return &GetTicketPoolValueCmd{}
}

// LiveTicketsCmd is a type handling custom marshaling and
// unmarshaling of livetickets JSON RPC commands.
type LiveTicketsCmd struct{}

// NewLiveTicketsCmd returns a new instance which can be used to issue a JSON-RPC
// livetickets command.
func NewLiveTicketsCmd() *LiveTicketsCmd {
	return &LiveTicketsCmd{}
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
	MustRegisterCmd("existsaddresses", (*ExistsAddressesCmd)(nil), flags)
	MustRegisterCmd("existsliveticket", (*ExistsLiveTicketCmd)(nil), flags)
	MustRegisterCmd("existslivetickets", (*ExistsLiveTicketsCmd)(nil), flags)
	MustRegisterCmd("existsmempooltxs", (*ExistsMempoolTxsCmd)(nil), flags)
	MustRegisterCmd("getcoinsupply", (*GetCoinSupplyCmd)(nil), flags)
	MustRegisterCmd("getstakedifficulty", (*GetStakeDifficultyCmd)(nil), flags)
	MustRegisterCmd("getticketpoolvalue", (*GetTicketPoolValueCmd)(nil), flags)
	MustRegisterCmd("livetickets", (*LiveTicketsCmd)(nil), flags)
	MustRegisterCmd("missedtickets", (*MissedTicketsCmd)(nil), flags)
	MustRegisterCmd("rebroadcastmissed", (*RebroadcastMissedCmd)(nil), flags)
	MustRegisterCmd("rebroadcastwinners", (*RebroadcastWinnersCmd)(nil), flags)
	MustRegisterCmd("ticketsforaddress", (*TicketsForAddressCmd)(nil), flags)
}
