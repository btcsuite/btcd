package node

import (
	"bytes"

	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/normalization"
	"github.com/lbryio/lbcd/claimtrie/param"
)

type NormalizingManager struct { // implements Manager
	Manager
	normalizedAt int32
}

func NewNormalizingManager(baseManager Manager) Manager {
	log.Info(normalization.NormalizeTitle)
	return &NormalizingManager{
		Manager:      baseManager,
		normalizedAt: -1,
	}
}

func (nm *NormalizingManager) AppendChange(chg change.Change) {
	chg.Name = normalization.NormalizeIfNecessary(chg.Name, chg.Height)
	nm.Manager.AppendChange(chg)
}

func (nm *NormalizingManager) IncrementHeightTo(height int32, temporary bool) ([][]byte, error) {
	nm.addNormalizationForkChangesIfNecessary(height)
	return nm.Manager.IncrementHeightTo(height, temporary)
}

func (nm *NormalizingManager) DecrementHeightTo(affectedNames [][]byte, height int32) ([][]byte, error) {
	if nm.normalizedAt > height {
		nm.normalizedAt = -1
		nm.ClearCache()
	}
	return nm.Manager.DecrementHeightTo(affectedNames, height)
}

func (nm *NormalizingManager) addNormalizationForkChangesIfNecessary(height int32) {

	if nm.Manager.Height()+1 != height {
		// initialization phase
		if height >= param.ActiveParams.NormalizedNameForkHeight {
			nm.normalizedAt = param.ActiveParams.NormalizedNameForkHeight // eh, we don't really know that it happened there
		}
	}

	if nm.normalizedAt >= 0 || height != param.ActiveParams.NormalizedNameForkHeight {
		return
	}
	nm.normalizedAt = height
	log.Info("Generating necessary changes for the normalization fork...")

	// the original code had an unfortunate bug where many unnecessary takeovers
	// were triggered at the normalization fork
	predicate := func(name []byte) bool {
		norm := normalization.Normalize(name)
		eq := bytes.Equal(name, norm)
		if eq {
			return true
		}

		clone := make([]byte, len(name))
		copy(clone, name) // iteration name buffer is reused on future loops

		// by loading changes for norm here, you can determine if there will be a conflict

		n, err := nm.Manager.NodeAt(nm.Manager.Height(), clone)
		if err != nil || n == nil {
			return true
		}
		for _, c := range n.Claims {
			nm.Manager.AppendChange(change.Change{
				Type:          change.AddClaim,
				Name:          norm,
				Height:        c.AcceptedAt,
				OutPoint:      c.OutPoint,
				ClaimID:       c.ClaimID,
				Amount:        c.Amount,
				ActiveHeight:  c.ActiveAt, // necessary to match the old hash
				VisibleHeight: height,     // necessary to match the old hash; it would have been much better without
			})
			nm.Manager.AppendChange(change.Change{
				Type:     change.SpendClaim,
				Name:     clone,
				Height:   height,
				OutPoint: c.OutPoint,
			})
		}
		for _, c := range n.Supports {
			nm.Manager.AppendChange(change.Change{
				Type:          change.AddSupport,
				Name:          norm,
				Height:        c.AcceptedAt,
				OutPoint:      c.OutPoint,
				ClaimID:       c.ClaimID,
				Amount:        c.Amount,
				ActiveHeight:  c.ActiveAt,
				VisibleHeight: height,
			})
			nm.Manager.AppendChange(change.Change{
				Type:     change.SpendSupport,
				Name:     clone,
				Height:   height,
				OutPoint: c.OutPoint,
			})
		}

		return true
	}

	nm.Manager.ClearCache()
	nm.Manager.IterateNames(predicate)
}
