package blockchain

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// ChainCtx is an interface that abstracts away blockchain parameters.
type ChainCtx interface {
	// ChainParams returns the chain's configured chaincfg.Params.
	ChainParams() *chaincfg.Params

	// BlocksPerRetarget returns the number of blocks before retargeting
	// occurs.
	BlocksPerRetarget() int32

	// MinRetargetTimespan returns the minimum amount of time to use in the
	// difficulty calculation.
	MinRetargetTimespan() int64

	// MaxRetargetTimespan returns the maximum amount of time to use in the
	// difficulty calculation.
	MaxRetargetTimespan() int64

	// VerifyCheckpoint returns whether the passed height and hash match
	// the checkpoint data. Not all instances of VerifyCheckpoint will use
	// this function for validation.
	VerifyCheckpoint(height int32, hash *chainhash.Hash) bool

	// FindPreviousCheckpoint returns the most recent checkpoint that we
	// have validated. Not all instances of FindPreviousCheckpoint will use
	// this function for validation.
	FindPreviousCheckpoint() (HeaderCtx, error)
}

// HeaderCtx is an interface that describes information about a block. This is
// used so that external libraries can provide their own context (the header's
// parent, bits, etc.) when attempting to contextually validate a header.
type HeaderCtx interface {
	// Height returns the header's height.
	Height() int32

	// Bits returns the header's bits.
	Bits() uint32

	// Timestamp returns the header's timestamp.
	Timestamp() int64

	// Parent returns the header's parent.
	Parent() HeaderCtx

	// RelativeAncestorCtx returns the header's ancestor that is distance
	// blocks before it in the chain.
	RelativeAncestorCtx(distance int32) HeaderCtx
}
