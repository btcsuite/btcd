// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

// GetStakeDifficultyResult models the data returned from the
// getstakedifficulty command.
type GetStakeDifficultyResult struct {
	CurrentStakeDifficulty float64 `json:"current"`
	NextStakeDifficulty    float64 `json:"next"`
}

// VersionCount models a generic version:count tuple.
type VersionCount struct {
	Version uint32 `json:"version"`
	Count   uint32 `json:"count"`
}

// VersionInterval models a cooked version count for an interval.
type VersionInterval struct {
	StartHeight  int64          `json:"startheight"`
	EndHeight    int64          `json:"endheight"`
	PoSVersions  []VersionCount `json:"posversions"`
	VoteVersions []VersionCount `json:"voteversions"`
}

// GetStakeVersionInfoResult models the resulting data for getstakeversioninfo
// command.
type GetStakeVersionInfoResult struct {
	CurrentHeight int64             `json:"currentheight"`
	Hash          string            `json:"hash"`
	Intervals     []VersionInterval `json:"intervals"`
}

// VersionBits models a generic version:bits tuple.
type VersionBits struct {
	Version uint32 `json:"version"`
	Bits    uint16 `json:"bits"`
}

// StakeVersions models the data for GetStakeVersionsResult.
type StakeVersions struct {
	Hash         string        `json:"hash"`
	Height       int64         `json:"height"`
	BlockVersion int32         `json:"blockversion"`
	StakeVersion uint32        `json:"stakeversion"`
	Votes        []VersionBits `json:"votes"`
}

// GetStakeVersionsResult models the data returned from the getstakeversions
// command.
type GetStakeVersionsResult struct {
	StakeVersions []StakeVersions `json:"stakeversions"`
}

// Choice models an individual choice inside an Agenda.
type Choice struct {
	Id          string  `json:"id"`
	Description string  `json:"description"`
	Bits        uint16  `json:"bits"`
	IsAbstain   bool    `json:"isabstain"`
	IsNo        bool    `json:"isno"`
	Count       uint32  `json:"count"`
	Progress    float64 `json:"progress"`
}

// Agenda models an individual agenda including its choices.
type Agenda struct {
	Id             string   `json:"id"`
	Description    string   `json:"description"`
	Mask           uint16   `json:"mask"`
	StartTime      uint64   `json:"starttime"`
	ExpireTime     uint64   `json:"expiretime"`
	Status         string   `json:"status"`
	QuorumProgress float64  `json:"quorumprogress"`
	Choices        []Choice `json:"choices"`
}

// GetVoteInfoResult models the data returned from the getvoteinfo command.
type GetVoteInfoResult struct {
	CurrentHeight int64    `json:"currentheight"`
	StartHeight   int64    `json:"startheight"`
	EndHeight     int64    `json:"endheight"`
	Hash          string   `json:"hash"`
	VoteVersion   uint32   `json:"voteversion"`
	Quorum        uint32   `json:"quorum"`
	TotalVotes    uint32   `json:"totalvotes"`
	Agendas       []Agenda `json:"agendas,omitempty"`
}

// EstimateStakeDiffResult models the data returned from the estimatestakediff
// command.
type EstimateStakeDiffResult struct {
	Min      float64  `json:"min"`
	Max      float64  `json:"max"`
	Expected float64  `json:"expected"`
	User     *float64 `json:"user,omitempty"`
}

// LiveTicketsResult models the data returned from the livetickets
// command.
type LiveTicketsResult struct {
	Tickets []string `json:"tickets"`
}

// MissedTicketsResult models the data returned from the missedtickets
// command.
type MissedTicketsResult struct {
	Tickets []string `json:"tickets"`
}

// Ticket is the structure representing a ticket.
type Ticket struct {
	Hash  string `json:"hash"`
	Owner string `json:"owner"`
}

// FeeInfoBlock is ticket fee information about a block.
type FeeInfoBlock struct {
	Height uint32  `json:"height"`
	Number uint32  `json:"number"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"stddev"`
}

// FeeInfoMempool is ticket fee information about the mempool.
type FeeInfoMempool struct {
	Number uint32  `json:"number"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"stddev"`
}

// FeeInfoRange is ticket fee information about a range.
type FeeInfoRange struct {
	Number uint32  `json:"number"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"stddev"`
}

// FeeInfoWindow is ticket fee information about an adjustment window.
type FeeInfoWindow struct {
	StartHeight uint32  `json:"startheight"`
	EndHeight   uint32  `json:"endheight"`
	Number      uint32  `json:"number"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Mean        float64 `json:"mean"`
	Median      float64 `json:"median"`
	StdDev      float64 `json:"stddev"`
}

// TicketFeeInfoResult models the data returned from the ticketfeeinfo command.
// command.
type TicketFeeInfoResult struct {
	FeeInfoMempool FeeInfoMempool  `json:"feeinfomempool"`
	FeeInfoBlocks  []FeeInfoBlock  `json:"feeinfoblocks"`
	FeeInfoWindows []FeeInfoWindow `json:"feeinfowindows"`
}

// TicketsForAddressResult models the data returned from the ticketforaddress
// command.
type TicketsForAddressResult struct {
	Tickets []string `json:"tickets"`
}

// TxFeeInfoResult models the data returned from the ticketfeeinfo command.
// command.
type TxFeeInfoResult struct {
	FeeInfoMempool FeeInfoMempool `json:"feeinfomempool"`
	FeeInfoBlocks  []FeeInfoBlock `json:"feeinfoblocks"`
	FeeInfoRange   FeeInfoRange   `json:"feeinforange"`
}

// VersionResult models objects included in the version response.  In the actual
// result, these objects are keyed by the program or API name.
type VersionResult struct {
	VersionString string `json:"versionstring"`
	Major         uint32 `json:"major"`
	Minor         uint32 `json:"minor"`
	Patch         uint32 `json:"patch"`
	Prerelease    string `json:"prerelease"`
	BuildMetadata string `json:"buildmetadata"`
}
