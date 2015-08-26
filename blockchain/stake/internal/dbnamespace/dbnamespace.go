// Package dbnamespace contains constants that define the database namespaces
// for the purpose of the blockchain, so that external callers may easily access
// this data.
package dbnamespace

import (
	"encoding/binary"
)

var (
	// ByteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	ByteOrder = binary.LittleEndian

	// StakeDbInfoBucketName is the name of the database bucket used to
	// house a single k->v that stores global versioning and date information for
	// the stake database.
	StakeDbInfoBucketName = []byte("stakedbinfo")

	// StakeChainStateKeyName is the name of the db key used to store the best
	// chain state from the perspective of the stake database.
	StakeChainStateKeyName = []byte("stakechainstate")

	// LiveTicketsBucketName is the name of the db bucket used to house the
	// list of live tickets keyed to their entry height.
	LiveTicketsBucketName = []byte("livetickets")

	// MissedTicketsBucketName is the name of the db bucket used to house the
	// list of missed tickets keyed to their entry height.
	MissedTicketsBucketName = []byte("missedtickets")

	// RevokedTicketsBucketName is the name of the db bucket used to house the
	// list of revoked tickets keyed to their entry height.
	RevokedTicketsBucketName = []byte("revokedtickets")

	// StakeBlockUndoDataBucketName is the name of the db bucket used to house the
	// information used to roll back the three main databases when regressing
	// backwards through the blockchain and restoring the stake information
	// to that of an earlier height. It is keyed to a mainchain height.
	StakeBlockUndoDataBucketName = []byte("stakeblockundo")

	// TicketsInBlockBucketName is the name of the db bucket used to house the
	// list of tickets in a block added to the mainchain, so that it can be
	// looked up later to insert new tickets into the live ticket database.
	TicketsInBlockBucketName = []byte("ticketsinblock")
)
