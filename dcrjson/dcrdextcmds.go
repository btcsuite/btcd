// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a chain server with btcd extensions.

package dcrjson

// EstimateStakeDiffCmd defines the eststakedifficulty JSON-RPC command.
type EstimateStakeDiffCmd struct {
	Tickets *uint32
}

// NewEstimateStakeDiffCmd defines the eststakedifficulty JSON-RPC command.
func NewEstimateStakeDiffCmd(tickets *uint32) *EstimateStakeDiffCmd {
	return &EstimateStakeDiffCmd{
		Tickets: tickets,
	}
}

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

// TicketFeeInfoCmd defines the ticketsfeeinfo JSON-RPC command.
type TicketFeeInfoCmd struct {
	Blocks  *uint32
	Windows *uint32
}

// NewTicketFeeInfoCmd returns a new instance which can be used to issue a
// JSON-RPC ticket fee info command.
func NewTicketFeeInfoCmd(blocks *uint32, windows *uint32) *TicketFeeInfoCmd {
	return &TicketFeeInfoCmd{
		Blocks:  blocks,
		Windows: windows,
	}
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

// TicketVWAPCmd defines the ticketvwap JSON-RPC command.
type TicketVWAPCmd struct {
	Start *uint32
	End   *uint32
}

// NewTicketVWAPCmd returns a new instance which can be used to issue a
// JSON-RPC ticket volume weight average price command.
func NewTicketVWAPCmd(start *uint32, end *uint32) *TicketVWAPCmd {
	return &TicketVWAPCmd{
		Start: start,
		End:   end,
	}
}

// TxFeeInfoCmd defines the ticketsfeeinfo JSON-RPC command.
type TxFeeInfoCmd struct {
	Blocks     *uint32
	RangeStart *uint32
	RangeEnd   *uint32
}

// NewTxFeeInfoCmd returns a new instance which can be used to issue a
// JSON-RPC ticket fee info command.
func NewTxFeeInfoCmd(blocks *uint32, start *uint32, end *uint32) *TxFeeInfoCmd {
	return &TxFeeInfoCmd{
		Blocks:     blocks,
		RangeStart: start,
		RangeEnd:   end,
	}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("estimatestakediff", (*EstimateStakeDiffCmd)(nil), flags)
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
	MustRegisterCmd("ticketfeeinfo", (*TicketFeeInfoCmd)(nil), flags)
	MustRegisterCmd("ticketsforaddress", (*TicketsForAddressCmd)(nil), flags)
	MustRegisterCmd("ticketvwap", (*TicketVWAPCmd)(nil), flags)
	MustRegisterCmd("txfeeinfo", (*TxFeeInfoCmd)(nil), flags)
}
