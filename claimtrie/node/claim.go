package node

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/wire"
)

type Status int

const (
	Accepted Status = iota
	Activated
	Deactivated
)

// Claim defines a structure of stake, which could be a Claim or Support.
type Claim struct {
	OutPoint wire.OutPoint
	ClaimID  change.ClaimID
	Amount   int64
	// CreatedAt  int32 // the very first block, unused at present
	AcceptedAt int32 // the latest update height
	ActiveAt   int32 // AcceptedAt + actual delay
	VisibleAt  int32

	Status   Status `msgpack:",omitempty"`
	Sequence int32  `msgpack:",omitempty"`
}

func (c *Claim) setOutPoint(op wire.OutPoint) *Claim {
	c.OutPoint = op
	return c
}

func (c *Claim) SetAmt(amt int64) *Claim {
	c.Amount = amt
	return c
}

func (c *Claim) setAccepted(height int32) *Claim {
	c.AcceptedAt = height
	return c
}

func (c *Claim) setActiveAt(height int32) *Claim {
	c.ActiveAt = height
	return c
}

func (c *Claim) setStatus(status Status) *Claim {
	c.Status = status
	return c
}

func OutPointLess(a, b wire.OutPoint) bool {

	switch cmp := bytes.Compare(a.Hash[:], b.Hash[:]); {
	case cmp < 0:
		return true
	case cmp > 0:
		return false
	default:
		return a.Index < b.Index
	}
}

func NewOutPointFromString(str string) *wire.OutPoint {

	f := strings.Split(str, ":")
	if len(f) != 2 {
		return nil
	}
	hash, _ := chainhash.NewHashFromStr(f[0])
	idx, _ := strconv.Atoi(f[1])

	return wire.NewOutPoint(hash, uint32(idx))
}
