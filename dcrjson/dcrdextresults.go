// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

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

// TicketsForAddressResult models the data returned from the ticketforaddress
// command.
type TicketsForAddressResult struct {
	Tickets []string `json:"tickets"`
}
