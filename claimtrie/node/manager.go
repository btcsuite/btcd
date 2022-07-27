package node

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/param"
)

type Manager interface {
	AppendChange(chg change.Change)
	IncrementHeightTo(height int32, temporary bool) ([][]byte, error)
	DecrementHeightTo(affectedNames [][]byte, height int32) ([][]byte, error)
	Height() int32
	Close() error
	NodeAt(height int32, name []byte) (*Node, error)
	IterateNames(predicate func(name []byte) bool)
	Hash(name []byte) (*chainhash.Hash, int32)
	Flush() error
}

type BaseManager struct {
	repo Repo

	height  int32
	changes []change.Change

	tempChanges map[string][]change.Change
}

func NewBaseManager(repo Repo) (*BaseManager, error) {

	nm := &BaseManager{
		repo: repo,
	}

	return nm, nil
}

func (nm *BaseManager) NodeAt(height int32, name []byte) (*Node, error) {

	changes, err := nm.repo.LoadChanges(name)
	if err != nil {
		return nil, errors.Wrap(err, "in load changes")
	}

	if nm.tempChanges != nil { // making an assumption that we only ever have tempChanges for a single block
		changes = append(changes, nm.tempChanges[string(name)]...)
	}

	n, err := nm.newNodeFromChanges(changes, height)
	if err != nil {
		return nil, errors.Wrap(err, "in new node")
	}

	return n, nil
}

// Node returns a node at the current height.
// The returned node may have pending changes.
func (nm *BaseManager) node(name []byte) (*Node, error) {
	return nm.NodeAt(nm.height, name)
}

// newNodeFromChanges returns a new Node constructed from the changes.
// The changes must preserve their order received.
func (nm *BaseManager) newNodeFromChanges(changes []change.Change, height int32) (*Node, error) {

	if len(changes) == 0 {
		return nil, nil
	}

	n := New()
	previous := changes[0].Height
	count := len(changes)

	for i, chg := range changes {
		if chg.Height < previous {
			panic("expected the changes to be in order by height")
		}
		if chg.Height > height {
			count = i
			break
		}

		if previous < chg.Height {
			n.AdjustTo(previous, chg.Height-1, chg.Name) // update bids and activation
			previous = chg.Height
		}

		delay := nm.getDelayForName(n, chg)
		err := n.ApplyChange(chg, delay)
		if err != nil {
			return nil, errors.Wrap(err, "in apply change")
		}
	}

	if count <= 0 {
		return nil, nil
	}
	lastChange := changes[count-1]
	return n.AdjustTo(lastChange.Height, height, lastChange.Name), nil
}

func (nm *BaseManager) AppendChange(chg change.Change) {

	nm.changes = append(nm.changes, chg)

	// worth putting in this kind of thing pre-emptively?
	// log.Debugf("CHG: %d, %s, %v, %s, %d", chg.Height, chg.Name, chg.Type, chg.ClaimID, chg.Amount)
}

func collectChildNames(changes []change.Change) {
	// we need to determine which children (names that start with the same name) go with which change
	// if we have the names in order then we can avoid iterating through all names in the change list
	// and we can possibly reuse the previous list.

	// what would happen in the old code:
	// spending a claim (which happens before every update) could remove a node from the cached trie
	// in which case we would fall back on the data from the previous block (where it obviously wasn't spent).
	// It would only delete the node if it had no children, but have even some rare situations
	// Where all of the children happen to be deleted first. That's what we must detect here.

	// Algorithm:
	// For each non-spend change
	//    Loop through all the spends before you and add them to your child list if they are your child

	type pair struct {
		name  string
		order int
	}

	spends := make([]pair, 0, len(changes))
	for i := range changes {
		t := changes[i].Type
		if t != change.SpendClaim {
			continue
		}
		spends = append(spends, pair{string(changes[i].Name), i})
	}
	sort.Slice(spends, func(i, j int) bool {
		return spends[i].name < spends[j].name
	})

	for i := range changes {
		t := changes[i].Type
		if t == change.SpendClaim || t == change.SpendSupport {
			continue
		}
		a := string(changes[i].Name)
		sc := map[string]bool{}
		idx := sort.Search(len(spends), func(k int) bool {
			return spends[k].name > a
		})
		for idx < len(spends) {
			b := spends[idx].name
			if len(b) <= len(a) || a != b[:len(a)] {
				break // since they're ordered alphabetically, we should be able to break out once we're past matches
			}
			if spends[idx].order < i {
				sc[b] = true
			}
			idx++
		}
		changes[i].SpentChildren = sc
	}
}

// to understand the above function, it may be helpful to refer to the slower implementation:
//func collectChildNamesSlow(changes []change.Change) {
//	for i := range changes {
//		t := changes[i].Type
//		if t == change.SpendClaim || t == change.SpendSupport {
//			continue
//		}
//		a := changes[i].Name
//		sc := map[string]bool{}
//		for j := 0; j < i; j++ {
//			t = changes[j].Type
//			if t != change.SpendClaim {
//				continue
//			}
//			b := changes[j].Name
//			if len(b) >= len(a) && bytes.Equal(a, b[:len(a)]) {
//				sc[string(b)] = true
//			}
//		}
//		changes[i].SpentChildren = sc
//	}
//}

func (nm *BaseManager) IncrementHeightTo(height int32, temporary bool) ([][]byte, error) {

	if height <= nm.height {
		panic("invalid height")
	}

	if height >= param.ActiveParams.MaxRemovalWorkaroundHeight {
		// not technically needed until block 884430, but to be true to the arbitrary rollback length...
		collectChildNames(nm.changes)
	}

	if temporary {
		if nm.tempChanges != nil {
			return nil, errors.Errorf("expected nil temporary changes")
		}
		nm.tempChanges = map[string][]change.Change{}
	}
	names := make([][]byte, 0, len(nm.changes))
	for i := range nm.changes {
		names = append(names, nm.changes[i].Name)
		if temporary {
			name := string(nm.changes[i].Name)
			nm.tempChanges[name] = append(nm.tempChanges[name], nm.changes[i])
		}
	}

	if !temporary {
		if err := nm.repo.AppendChanges(nm.changes); err != nil { // destroys names
			return nil, errors.Wrap(err, "in append changes")
		}
	}

	// Truncate the buffer size to zero.
	if len(nm.changes) > 1000 { // TODO: determine a good number here
		nm.changes = nil // release the RAM
	} else {
		nm.changes = nm.changes[:0]
	}
	nm.height = height

	return names, nil
}

func (nm *BaseManager) DecrementHeightTo(affectedNames [][]byte, height int32) ([][]byte, error) {
	if height >= nm.height {
		return affectedNames, errors.Errorf("invalid height of %d for %d", height, nm.height)
	}

	if nm.tempChanges != nil {
		if height != nm.height-1 {
			return affectedNames, errors.Errorf("invalid temporary rollback at %d to %d", height, nm.height)
		}
		for key := range nm.tempChanges {
			affectedNames = append(affectedNames, []byte(key))
		}
		nm.tempChanges = nil
	} else {
		for _, name := range affectedNames {
			if err := nm.repo.DropChanges(name, height); err != nil {
				return affectedNames, errors.Wrap(err, "in drop changes")
			}
		}
	}
	nm.height = height

	return affectedNames, nil
}

func (nm *BaseManager) getDelayForName(n *Node, chg change.Change) int32 {
	// Note: we don't consider the active status of BestClaim here on purpose.
	// That's because we deactivate and reactivate as part of claim updates.
	// However, the final status will be accounted for when we compute the takeover heights;
	// claims may get activated early at that point.

	hasBest := n.BestClaim != nil
	if hasBest && n.BestClaim.ClaimID == chg.ClaimID {
		return 0
	}
	if chg.ActiveHeight >= chg.Height { // ActiveHeight is usually unset (aka, zero)
		return chg.ActiveHeight - chg.Height
	}
	if !hasBest {
		return 0
	}

	delay := calculateDelay(chg.Height, n.TakenOverAt)
	if delay > 0 && nm.aWorkaroundIsNeeded(n, chg) {
		if chg.Height >= nm.height {
			LogOnce(fmt.Sprintf("Delay workaround applies to %s at %d, ClaimID: %s",
				chg.Name, chg.Height, chg.ClaimID))
		}
		return 0
	}
	return delay
}

func hasZeroActiveClaims(n *Node) bool {
	// this isn't quite the same as having an active best (since that is only updated after all changes are processed)
	for _, c := range n.Claims {
		if c.Status == Activated {
			return false
		}
	}
	return true
}

// aWorkaroundIsNeeded handles bugs that existed in previous versions
func (nm *BaseManager) aWorkaroundIsNeeded(n *Node, chg change.Change) bool {

	if chg.Type == change.SpendClaim || chg.Type == change.SpendSupport {
		return false
	}

	if chg.Height >= param.ActiveParams.MaxRemovalWorkaroundHeight {
		// TODO: hard fork this out; it's a bug from previous versions:

		// old 17.3 C++ code we're trying to mimic (where empty means no active claims):
		// auto it = nodesToAddOrUpdate.find(name); // nodesToAddOrUpdate is the working changes, base is previous block
		// auto answer = (it || (it = base->find(name))) && !it->empty() ? nNextHeight - it->nHeightOfLastTakeover : 0;

		return hasZeroActiveClaims(n) && nm.hasChildren(chg.Name, chg.Height, chg.SpentChildren, 2)
	} else if len(n.Claims) > 0 {
		// NOTE: old code had a bug in it where nodes with no claims but with children would get left in the cache after removal.
		// This would cause the getNumBlocksOfContinuousOwnership to return zero (causing incorrect takeover height calc).
		w, ok := param.DelayWorkarounds[string(chg.Name)]
		if ok {
			for _, h := range w {
				if chg.Height == h {
					return true
				}
			}
		}
	}
	return false
}

func calculateDelay(curr, tookOver int32) int32 {

	delay := (curr - tookOver) / param.ActiveParams.ActiveDelayFactor
	if delay > param.ActiveParams.MaxActiveDelay {
		return param.ActiveParams.MaxActiveDelay
	}

	return delay
}

func (nm *BaseManager) Height() int32 {
	return nm.height
}

func (nm *BaseManager) Close() error {
	return errors.WithStack(nm.repo.Close())
}

func (nm *BaseManager) hasChildren(name []byte, height int32, spentChildren map[string]bool, required int) bool {
	c := map[byte]bool{}
	if spentChildren == nil {
		spentChildren = map[string]bool{}
	}

	err := nm.repo.IterateChildren(name, func(changes []change.Change) bool {
		// if the key is unseen, generate a node for it to height
		// if that node is active then increase the count
		if len(changes) == 0 {
			return true
		}
		if c[changes[0].Name[len(name)]] { // assuming all names here are longer than starter name
			return true // we already checked a similar name
		}
		if spentChildren[string(changes[0].Name)] {
			return true // children that are spent in the same block cannot count as active children
		}
		n, _ := nm.newNodeFromChanges(changes, height)
		if n != nil && n.HasActiveBestClaim() {
			c[changes[0].Name[len(name)]] = true
			if len(c) >= required {
				return false
			}
		}
		return true
	})
	return err == nil && len(c) >= required
}

func (nm *BaseManager) IterateNames(predicate func(name []byte) bool) {
	nm.repo.IterateAll(predicate)
}

func (nm *BaseManager) Hash(name []byte) (*chainhash.Hash, int32) {

	n, err := nm.node(name)
	if err != nil || n == nil {
		return nil, 0
	}
	if len(n.Claims) > 0 {
		if n.BestClaim != nil && n.BestClaim.Status == Activated {
			h := calculateNodeHash(n.BestClaim.OutPoint, n.TakenOverAt)
			return h, n.NextUpdate()
		}
	}
	return nil, n.NextUpdate()
}

func (nm *BaseManager) Flush() error {
	return nm.repo.Flush()
}
