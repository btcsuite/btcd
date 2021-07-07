package node

import (
	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/wire"
)

type ClaimList []*Claim

type comparator func(c *Claim) bool

func byID(id change.ClaimID) comparator {
	return func(c *Claim) bool {
		return c.ClaimID == id
	}
}

func byOut(out wire.OutPoint) comparator {
	return func(c *Claim) bool {
		return c.OutPoint == out // assuming value comparison
	}
}

func (l ClaimList) find(cmp comparator) *Claim {

	for i := range l {
		if cmp(l[i]) {
			return l[i]
		}
	}

	return nil
}
