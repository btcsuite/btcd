package node

import (
	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/claimtrie/param"
)

type HashV2Manager struct {
	Manager
}

func (nm *HashV2Manager) computeClaimHashes(name []byte) (*chainhash.Hash, int32) {

	n, err := nm.NodeAt(nm.Height(), name)
	if err != nil || n == nil {
		return nil, 0
	}

	n.SortClaimsByBid()
	claimHashes := make([]*chainhash.Hash, 0, len(n.Claims))
	for _, c := range n.Claims {
		if c.Status == Activated { // TODO: unit test this line
			claimHashes = append(claimHashes, calculateNodeHash(c.OutPoint, n.TakenOverAt))
		}
	}
	if len(claimHashes) > 0 {
		return ComputeMerkleRoot(claimHashes), n.NextUpdate()
	}
	return nil, n.NextUpdate()
}

func (nm *HashV2Manager) Hash(name []byte) (*chainhash.Hash, int32) {

	if nm.Height() >= param.ActiveParams.AllClaimsInMerkleForkHeight {
		return nm.computeClaimHashes(name)
	}

	return nm.Manager.Hash(name)
}
