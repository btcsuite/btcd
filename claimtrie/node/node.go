package node

import (
	"fmt"
	"math"
	"sort"

	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/param"
)

type Node struct {
	BestClaim   *Claim    // The claim that has most effective amount at the current height.
	TakenOverAt int32     // The height at when the current BestClaim took over.
	Claims      ClaimList // List of all Claims.
	Supports    ClaimList // List of all Supports, including orphaned ones.
	SupportSums map[string]int64
}

// New returns a new node.
func New() *Node {
	return &Node{SupportSums: map[string]int64{}}
}

func (n *Node) HasActiveBestClaim() bool {
	return n.BestClaim != nil && n.BestClaim.Status == Activated
}

func (n *Node) ApplyChange(chg change.Change, delay int32) error {

	visibleAt := chg.VisibleHeight
	if visibleAt <= 0 {
		visibleAt = chg.Height
	}

	switch chg.Type {
	case change.AddClaim:
		c := &Claim{
			OutPoint: chg.OutPoint,
			Amount:   chg.Amount,
			ClaimID:  chg.ClaimID,
			// CreatedAt:  chg.Height,
			AcceptedAt: chg.Height,
			ActiveAt:   chg.Height + delay,
			VisibleAt:  visibleAt,
			Sequence:   int32(len(n.Claims)),
		}
		// old := n.Claims.find(byOut(chg.OutPoint)) // TODO: remove this after proving ResetHeight works
		// if old != nil {
		// return errors.Errorf("CONFLICT WITH EXISTING TXO! Name: %s, Height: %d", chg.Name, chg.Height)
		// }
		n.Claims = append(n.Claims, c)

	case change.SpendClaim:
		c := n.Claims.find(byOut(chg.OutPoint))
		if c != nil {
			c.setStatus(Deactivated)
		} else {
			LogOnce(fmt.Sprintf("Spending claim but missing existing claim with TXO %s, "+
				"Name: %s, ID: %s", chg.OutPoint, chg.Name, chg.ClaimID))
		}
		// apparently it's legit to be absent in the map:
		// 'two' at 481100, 36a719a156a1df178531f3c712b8b37f8e7cc3b36eea532df961229d936272a1:0

	case change.UpdateClaim:
		// Find and remove the claim, which has just been spent.
		c := n.Claims.find(byID(chg.ClaimID))
		if c != nil && c.Status == Deactivated {

			// Keep its ID, which was generated from the spent claim.
			// And update the rest of properties.
			c.setOutPoint(chg.OutPoint).SetAmt(chg.Amount)
			c.setStatus(Accepted) // it was Deactivated in the spend (but we only activate at the end of the block)
			// that's because the old code would put all insertions into the "queue" that was processed at block's end

			// This forces us to be newer, which may in an unintentional takeover if there's an older one.
			// TODO: reconsider these updates in future hard forks.
			c.setAccepted(chg.Height)
			c.setActiveAt(chg.Height + delay)

		} else {
			LogOnce(fmt.Sprintf("Updating claim but missing existing claim with ID %s", chg.ClaimID))
		}
	case change.AddSupport:
		n.Supports = append(n.Supports, &Claim{
			OutPoint:   chg.OutPoint,
			Amount:     chg.Amount,
			ClaimID:    chg.ClaimID,
			AcceptedAt: chg.Height,
			ActiveAt:   chg.Height + delay,
			VisibleAt:  visibleAt,
		})

	case change.SpendSupport:
		s := n.Supports.find(byOut(chg.OutPoint))
		if s != nil {
			if s.Status == Activated {
				n.SupportSums[s.ClaimID.Key()] -= s.Amount
			}
			// TODO: we could do without this Deactivated flag if we set expiration instead
			// That would eliminate the above Sum update.
			// We would also need to track the update situation, though, but that could be done locally.
			s.setStatus(Deactivated)
		} else {
			LogOnce(fmt.Sprintf("Spending support but missing existing claim with TXO %s, "+
				"Name: %s, ID: %s", chg.OutPoint, chg.Name, chg.ClaimID))
		}
	}
	return nil
}

// AdjustTo activates claims and computes takeovers until it reaches the specified height.
func (n *Node) AdjustTo(height, maxHeight int32, name []byte) {
	changed := n.handleExpiredAndActivated(height) > 0
	n.updateTakeoverHeight(height, name, changed)
	if maxHeight > height {
		for h := n.NextUpdate(); h <= maxHeight; h = n.NextUpdate() {
			changed = n.handleExpiredAndActivated(h) > 0
			n.updateTakeoverHeight(h, name, changed)
			height = h
		}
	}
}

func (n *Node) updateTakeoverHeight(height int32, name []byte, refindBest bool) {

	candidate := n.BestClaim
	if refindBest {
		candidate = n.findBestClaim() // so expensive...
	}

	hasCandidate := candidate != nil
	hasCurrentWinner := n.HasActiveBestClaim()

	takeoverHappening := !hasCandidate || !hasCurrentWinner || candidate.ClaimID != n.BestClaim.ClaimID

	if takeoverHappening {
		if n.activateAllClaims(height) > 0 {
			candidate = n.findBestClaim()
		}
	}

	if !takeoverHappening && height < param.ActiveParams.MaxRemovalWorkaroundHeight {
		// This is a super ugly hack to work around bug in old code.
		// The bug: un/support a name then update it. This will cause its takeover height to be reset to current.
		// This is because the old code would add to the cache without setting block originals when dealing in supports.
		_, takeoverHappening = param.TakeoverWorkarounds[fmt.Sprintf("%d_%s", height, name)] // TODO: ditch the fmt call
	}

	if takeoverHappening {
		n.TakenOverAt = height
		n.BestClaim = candidate
	}
}

func (n *Node) handleExpiredAndActivated(height int32) int {

	ot := param.ActiveParams.OriginalClaimExpirationTime
	et := param.ActiveParams.ExtendedClaimExpirationTime
	fk := param.ActiveParams.ExtendedClaimExpirationForkHeight
	expiresAt := func(c *Claim) int32 {
		if c.AcceptedAt+ot > fk {
			return c.AcceptedAt + et
		}
		return c.AcceptedAt + ot
	}

	changes := 0
	update := func(items ClaimList, sums map[string]int64) ClaimList {
		for i := 0; i < len(items); i++ {
			c := items[i]
			if c.Status == Accepted && c.ActiveAt <= height && c.VisibleAt <= height {
				c.setStatus(Activated)
				changes++
				if sums != nil {
					sums[c.ClaimID.Key()] += c.Amount
				}
			}
			if c.Status == Deactivated || expiresAt(c) <= height {
				if i < len(items)-1 {
					items[i] = items[len(items)-1]
					i--
				}
				items = items[:len(items)-1]
				changes++
				if sums != nil && c.Status != Deactivated {
					sums[c.ClaimID.Key()] -= c.Amount
				}
			}
		}
		return items
	}
	n.Claims = update(n.Claims, nil)
	n.Supports = update(n.Supports, n.SupportSums)
	return changes
}

// NextUpdate returns the nearest height in the future that the node should
// be refreshed due to changes of claims or supports.
func (n Node) NextUpdate() int32 {

	ot := param.ActiveParams.OriginalClaimExpirationTime
	et := param.ActiveParams.ExtendedClaimExpirationTime
	fk := param.ActiveParams.ExtendedClaimExpirationForkHeight
	expiresAt := func(c *Claim) int32 {
		if c.AcceptedAt+ot > fk {
			return c.AcceptedAt + et
		}
		return c.AcceptedAt + ot
	}

	next := int32(math.MaxInt32)

	for _, c := range n.Claims {
		ea := expiresAt(c)
		if ea < next {
			next = ea
		}
		// if we're not active, we need to go to activeAt unless we're still invisible there
		if c.Status == Accepted {
			min := c.ActiveAt
			if c.VisibleAt > min {
				min = c.VisibleAt
			}
			if min < next {
				next = min
			}
		}
	}

	for _, s := range n.Supports {
		es := expiresAt(s)
		if es < next {
			next = es
		}
		if s.Status == Accepted {
			min := s.ActiveAt
			if s.VisibleAt > min {
				min = s.VisibleAt
			}
			if min < next {
				next = min
			}
		}
	}

	return next
}

func (n Node) findBestClaim() *Claim {

	// WARNING: this method is called billions of times.
	// if we just had some easy way to know that our best claim was the first one in the list...
	// or it may be faster to cache effective amount in the db at some point.

	var best *Claim
	var bestAmount int64
	for _, candidate := range n.Claims {

		// not using switch here for performance reasons
		if candidate.Status != Activated {
			continue
		}

		if best == nil {
			best = candidate
			continue
		}

		candidateAmount := candidate.Amount + n.SupportSums[candidate.ClaimID.Key()]
		if bestAmount <= 0 {
			bestAmount = best.Amount + n.SupportSums[best.ClaimID.Key()]
		}

		switch {
		case candidateAmount > bestAmount:
			best = candidate
			bestAmount = candidateAmount
		case candidateAmount < bestAmount:
			continue
		case candidate.AcceptedAt < best.AcceptedAt:
			best = candidate
			bestAmount = candidateAmount
		case candidate.AcceptedAt > best.AcceptedAt:
			continue
		case OutPointLess(candidate.OutPoint, best.OutPoint):
			best = candidate
			bestAmount = candidateAmount
		}
	}

	return best
}

func (n *Node) activateAllClaims(height int32) int {
	count := 0
	for _, c := range n.Claims {
		if c.Status == Accepted && c.ActiveAt > height && c.VisibleAt <= height {
			c.setActiveAt(height) // don't necessarily need to change this number?
			c.setStatus(Activated)
			count++
		}
	}

	for _, s := range n.Supports {
		if s.Status == Accepted && s.ActiveAt > height && s.VisibleAt <= height {
			s.setActiveAt(height) // don't necessarily need to change this number?
			s.setStatus(Activated)
			count++
			n.SupportSums[s.ClaimID.Key()] += s.Amount
		}
	}
	return count
}

func (n *Node) SortClaimsByBid() {

	// purposefully sorting by descent via func parameter order:
	sort.Slice(n.Claims, func(j, i int) bool {
		// SupportSums only include active values; do the same for amount. No active claim will have a zero amount
		iAmount := n.SupportSums[n.Claims[i].ClaimID.Key()]
		if n.Claims[i].Status == Activated {
			iAmount += n.Claims[i].Amount
		}
		jAmount := n.SupportSums[n.Claims[j].ClaimID.Key()]
		if n.Claims[j].Status == Activated {
			jAmount += n.Claims[j].Amount
		}
		switch {
		case iAmount < jAmount:
			return true
		case iAmount > jAmount:
			return false
		case n.Claims[i].AcceptedAt > n.Claims[j].AcceptedAt:
			return true
		case n.Claims[i].AcceptedAt < n.Claims[j].AcceptedAt:
			return false
		}
		return OutPointLess(n.Claims[j].OutPoint, n.Claims[i].OutPoint)
	})
}

func (n *Node) Clone() *Node {
	clone := New()
	if n.SupportSums != nil {
		clone.SupportSums = map[string]int64{}
		for key, value := range n.SupportSums {
			clone.SupportSums[key] = value
		}
	}
	clone.Supports = make(ClaimList, len(n.Supports))
	for i, support := range n.Supports {
		clone.Supports[i] = &Claim{}
		*clone.Supports[i] = *support
	}
	clone.Claims = make(ClaimList, len(n.Claims))
	for i, claim := range n.Claims {
		clone.Claims[i] = &Claim{}
		*clone.Claims[i] = *claim
	}
	clone.TakenOverAt = n.TakenOverAt
	if n.BestClaim != nil {
		clone.BestClaim = clone.Claims.find(byID(n.BestClaim.ClaimID))
	}
	return clone
}
