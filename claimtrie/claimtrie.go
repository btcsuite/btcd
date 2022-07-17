package claimtrie

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/pkg/errors"

	"github.com/lbryio/lbcd/claimtrie/block"
	"github.com/lbryio/lbcd/claimtrie/block/blockrepo"
	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/config"
	"github.com/lbryio/lbcd/claimtrie/merkletrie"
	"github.com/lbryio/lbcd/claimtrie/merkletrie/merkletrierepo"
	"github.com/lbryio/lbcd/claimtrie/node"
	"github.com/lbryio/lbcd/claimtrie/node/noderepo"
	"github.com/lbryio/lbcd/claimtrie/normalization"
	"github.com/lbryio/lbcd/claimtrie/param"
	"github.com/lbryio/lbcd/claimtrie/temporal"
	"github.com/lbryio/lbcd/claimtrie/temporal/temporalrepo"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/wire"
)

// ClaimTrie implements a Merkle Trie supporting linear history of commits.
type ClaimTrie struct {

	// Repository for calculated block hashes.
	blockRepo block.Repo

	// Repository for storing temporal information of nodes at each block height.
	// For example, which nodes (by name) should be refreshed at each block height
	// due to stake expiration or delayed activation.
	temporalRepo temporal.Repo

	// Cache layer of Nodes.
	nodeManager node.Manager

	// Prefix tree (trie) that manages merkle hash of each node.
	merkleTrie merkletrie.MerkleTrie

	// Current block height, which is increased by one when AppendBlock() is called.
	height int32

	// Registrered cleanup functions which are invoked in the Close() in reverse order.
	cleanups []func() error

	// claimLogger communicates progress of claimtrie rebuild.
	claimLogger *claimProgressLogger
}

func New(cfg config.Config) (*ClaimTrie, error) {

	var cleanups []func() error

	// The passed in cfg.DataDir has been prepended with netname.
	dataDir := filepath.Join(cfg.DataDir, "claim_dbs")

	dbPath := filepath.Join(dataDir, cfg.BlockRepoPebble.Path)
	blockRepo, err := blockrepo.NewPebble(dbPath)
	if err != nil {
		return nil, errors.Wrap(err, "creating block repo")
	}
	cleanups = append(cleanups, blockRepo.Close)
	err = blockRepo.Set(0, merkletrie.EmptyTrieHash)
	if err != nil {
		return nil, errors.Wrap(err, "setting block repo genesis")
	}

	dbPath = filepath.Join(dataDir, cfg.TemporalRepoPebble.Path)
	temporalRepo, err := temporalrepo.NewPebble(dbPath)
	if err != nil {
		return nil, errors.Wrap(err, "creating temporal repo")
	}
	cleanups = append(cleanups, temporalRepo.Close)

	// Initialize repository for changes to nodes.
	// The cleanup is delegated to the Node Manager.
	dbPath = filepath.Join(dataDir, cfg.NodeRepoPebble.Path)
	nodeRepo, err := noderepo.NewPebble(dbPath)
	if err != nil {
		return nil, errors.Wrap(err, "creating node repo")
	}

	baseManager, err := node.NewBaseManager(nodeRepo)
	if err != nil {
		return nil, errors.Wrap(err, "creating node base manager")
	}
	normalizingManager := node.NewNormalizingManager(baseManager)
	nodeManager := &node.HashV2Manager{Manager: normalizingManager}
	cleanups = append(cleanups, nodeManager.Close)

	var trie merkletrie.MerkleTrie
	if cfg.RamTrie {
		trie = merkletrie.NewRamTrie()
	} else {

		// Initialize repository for MerkleTrie. The cleanup is delegated to MerkleTrie.
		dbPath = filepath.Join(dataDir, cfg.MerkleTrieRepoPebble.Path)
		trieRepo, err := merkletrierepo.NewPebble(dbPath)
		if err != nil {
			return nil, errors.Wrap(err, "creating trie repo")
		}

		persistentTrie := merkletrie.NewPersistentTrie(trieRepo)
		cleanups = append(cleanups, persistentTrie.Close)
		trie = persistentTrie
	}

	// Restore the last height.
	previousHeight, err := blockRepo.Load()
	if err != nil {
		return nil, errors.Wrap(err, "load block tip")
	}

	ct := &ClaimTrie{
		blockRepo:    blockRepo,
		temporalRepo: temporalRepo,

		nodeManager: nodeManager,
		merkleTrie:  trie,

		height: previousHeight,
	}

	ct.cleanups = cleanups

	if previousHeight > 0 {
		hash, err := blockRepo.Get(previousHeight)
		if err != nil {
			ct.Close() // TODO: the cleanups aren't run when we exit with an err above here (but should be)
			return nil, errors.Wrap(err, "block repo get")
		}
		_, err = nodeManager.IncrementHeightTo(previousHeight, false)
		if err != nil {
			ct.Close()
			return nil, errors.Wrap(err, "increment height to")
		}
		err = trie.SetRoot(hash) // keep this after IncrementHeightTo
		if err == merkletrie.ErrFullRebuildRequired {
			ct.runFullTrieRebuild(nil, cfg.Interrupt)
		}

		if interruptRequested(cfg.Interrupt) || !ct.MerkleHash().IsEqual(hash) {
			ct.Close()
			return nil, errors.Errorf("unable to restore the claim hash to %s at height %d", hash.String(), previousHeight)
		}
	}

	return ct, nil
}

// AddClaim adds a Claim to the ClaimTrie.
func (ct *ClaimTrie) AddClaim(name []byte, op wire.OutPoint, id change.ClaimID, amt int64) error {

	chg := change.Change{
		Type:     change.AddClaim,
		Name:     name,
		OutPoint: op,
		Amount:   amt,
		ClaimID:  id,
	}

	return ct.forwardNodeChange(chg)
}

// UpdateClaim updates a Claim in the ClaimTrie.
func (ct *ClaimTrie) UpdateClaim(name []byte, op wire.OutPoint, amt int64, id change.ClaimID) error {

	chg := change.Change{
		Type:     change.UpdateClaim,
		Name:     name,
		OutPoint: op,
		Amount:   amt,
		ClaimID:  id,
	}

	return ct.forwardNodeChange(chg)
}

// SpendClaim spends a Claim in the ClaimTrie.
func (ct *ClaimTrie) SpendClaim(name []byte, op wire.OutPoint, id change.ClaimID) error {

	chg := change.Change{
		Type:     change.SpendClaim,
		Name:     name,
		OutPoint: op,
		ClaimID:  id,
	}

	return ct.forwardNodeChange(chg)
}

// AddSupport adds a Support to the ClaimTrie.
func (ct *ClaimTrie) AddSupport(name []byte, op wire.OutPoint, amt int64, id change.ClaimID) error {

	chg := change.Change{
		Type:     change.AddSupport,
		Name:     name,
		OutPoint: op,
		Amount:   amt,
		ClaimID:  id,
	}

	return ct.forwardNodeChange(chg)
}

// SpendSupport spends a Support in the ClaimTrie.
func (ct *ClaimTrie) SpendSupport(name []byte, op wire.OutPoint, id change.ClaimID) error {

	chg := change.Change{
		Type:     change.SpendSupport,
		Name:     name,
		OutPoint: op,
		ClaimID:  id,
	}

	return ct.forwardNodeChange(chg)
}

// AppendBlock increases block by one.
func (ct *ClaimTrie) AppendBlock(temporary bool) error {

	ct.height++

	names, err := ct.nodeManager.IncrementHeightTo(ct.height, temporary)
	if err != nil {
		return errors.Wrap(err, "node manager increment")
	}

	expirations, err := ct.temporalRepo.NodesAt(ct.height)
	if err != nil {
		return errors.Wrap(err, "temporal repo get")
	}

	names = removeDuplicates(names) // comes out sorted

	updateNames := make([][]byte, 0, len(names)+len(expirations))
	updateHeights := make([]int32, 0, len(names)+len(expirations))
	updateNames = append(updateNames, names...)
	for range names { // log to the db that we updated a name at this height for rollback purposes
		updateHeights = append(updateHeights, ct.height)
	}
	names = append(names, expirations...)
	names = removeDuplicates(names)

	nhns := ct.makeNameHashNext(names, false, nil)
	for nhn := range nhns {

		ct.merkleTrie.Update(nhn.Name, nhn.Hash, true)
		if nhn.Next <= 0 {
			continue
		}

		newName := normalization.NormalizeIfNecessary(nhn.Name, nhn.Next)
		updateNames = append(updateNames, newName)
		updateHeights = append(updateHeights, nhn.Next)
	}
	if !temporary && len(updateNames) > 0 {
		err = ct.temporalRepo.SetNodesAt(updateNames, updateHeights)
		if err != nil {
			return errors.Wrap(err, "temporal repo set")
		}
	}

	hitFork := ct.updateTrieForHashForkIfNecessary()
	h := ct.MerkleHash()

	if !temporary {
		ct.blockRepo.Set(ct.height, h)
	}

	if hitFork {
		err = ct.merkleTrie.SetRoot(h) // for clearing the memory entirely
	}

	return errors.Wrap(err, "merkle trie clear memory")
}

func (ct *ClaimTrie) updateTrieForHashForkIfNecessary() bool {
	if ct.height != param.ActiveParams.AllClaimsInMerkleForkHeight {
		return false
	}

	node.LogOnce(fmt.Sprintf("Rebuilding all trie nodes for the hash fork at %d...", ct.height))
	ct.runFullTrieRebuild(nil, nil) // I don't think it's safe to allow interrupt during fork
	return true
}

func removeDuplicates(names [][]byte) [][]byte { // this might be too expensive; we'll have to profile it
	sort.Slice(names, func(i, j int) bool { // put names in order so we can skip duplicates
		return bytes.Compare(names[i], names[j]) < 0
	})

	for i := len(names) - 2; i >= 0; i-- {
		if bytes.Equal(names[i], names[i+1]) {
			names = append(names[:i], names[i+1:]...)
		}
	}
	return names
}

// ResetHeight resets the ClaimTrie to a previous known height..
func (ct *ClaimTrie) ResetHeight(height int32) error {

	names := make([][]byte, 0)
	for h := height + 1; h <= ct.height; h++ {
		results, err := ct.temporalRepo.NodesAt(h)
		if err != nil {
			return err
		}
		names = append(names, results...)
	}
	names, err := ct.nodeManager.DecrementHeightTo(names, height)
	if err != nil {
		return err
	}

	passedHashFork := ct.height >= param.ActiveParams.AllClaimsInMerkleForkHeight && height < param.ActiveParams.AllClaimsInMerkleForkHeight
	hash, err := ct.blockRepo.Get(height)
	if err != nil {
		return err
	}

	oldHeight := ct.height
	ct.height = height // keep this before the rebuild

	if passedHashFork {
		names = nil // force them to reconsider all names
	}

	var fullRebuildRequired bool

	err = ct.merkleTrie.SetRoot(hash)
	if err == merkletrie.ErrFullRebuildRequired {
		fullRebuildRequired = true
	} else if err != nil {
		return errors.Wrapf(err, "setRoot")
	}

	if fullRebuildRequired {
		ct.runFullTrieRebuild(names, nil)
	}

	if !ct.MerkleHash().IsEqual(hash) {
		return errors.Errorf("unable to restore the hash at height %d"+
			" (fullTriedRebuilt: %t)", height, fullRebuildRequired)
	}

	return errors.WithStack(ct.blockRepo.Delete(height+1, oldHeight))
}

func (ct *ClaimTrie) runFullTrieRebuild(names [][]byte, interrupt <-chan struct{}) {
	var nhns chan NameHashNext
	if names == nil {
		node.Log("Building the entire claim trie in RAM...")
		ct.claimLogger = newClaimProgressLogger("Processed", node.GetLogger())
		nhns = ct.makeNameHashNext(nil, true, interrupt)
	} else {
		ct.claimLogger = nil
		nhns = ct.makeNameHashNext(names, false, interrupt)
	}

	for nhn := range nhns {
		ct.merkleTrie.Update(nhn.Name, nhn.Hash, false)
		if ct.claimLogger != nil {
			ct.claimLogger.LogName(nhn.Name)
		}
	}
}

// MerkleHash returns the Merkle Hash of the claimTrie.
func (ct *ClaimTrie) MerkleHash() *chainhash.Hash {
	if ct.height >= param.ActiveParams.AllClaimsInMerkleForkHeight {
		return ct.merkleTrie.MerkleHashAllClaims()
	}
	return ct.merkleTrie.MerkleHash()
}

// Height returns the current block height.
func (ct *ClaimTrie) Height() int32 {
	return ct.height
}

// Close persists states.
// Any calls to the ClaimTrie after Close() being called results undefined behaviour.
func (ct *ClaimTrie) Close() {

	for i := len(ct.cleanups) - 1; i >= 0; i-- {
		cleanup := ct.cleanups[i]
		err := cleanup()
		if err != nil { // it would be better to cleanup what we can than exit early
			node.LogOnce("On cleanup: " + err.Error())
		}
	}
	ct.cleanups = nil
}

func (ct *ClaimTrie) forwardNodeChange(chg change.Change) error {

	chg.Height = ct.Height() + 1
	ct.nodeManager.AppendChange(chg)
	return nil
}

func (ct *ClaimTrie) NodeAt(height int32, name []byte) (*node.Node, error) {
	return ct.nodeManager.NodeAt(height, name)
}

func (ct *ClaimTrie) NamesChangedInBlock(height int32) ([]string, error) {
	hits, err := ct.temporalRepo.NodesAt(height)
	r := make([]string, len(hits))
	for i := range hits {
		r[i] = string(hits[i])
	}
	return r, err
}

func (ct *ClaimTrie) FlushToDisk() {
	// maybe the user can fix the file lock shown in the warning before they shut down
	if err := ct.nodeManager.Flush(); err != nil {
		node.Warn("During nodeManager flush: " + err.Error())
	}
	if err := ct.temporalRepo.Flush(); err != nil {
		node.Warn("During temporalRepo flush: " + err.Error())
	}
	if err := ct.merkleTrie.Flush(); err != nil {
		node.Warn("During merkleTrie flush: " + err.Error())
	}
	if err := ct.blockRepo.Flush(); err != nil {
		node.Warn("During blockRepo flush: " + err.Error())
	}
}

type NameHashNext struct {
	Name []byte
	Hash *chainhash.Hash
	Next int32
}

func interruptRequested(interrupted <-chan struct{}) bool {
	select {
	case <-interrupted: // should never block on nil
		return true
	default:
	}

	return false
}

func (ct *ClaimTrie) makeNameHashNext(names [][]byte, all bool, interrupt <-chan struct{}) chan NameHashNext {
	inputs := make(chan []byte, 512)
	outputs := make(chan NameHashNext, 512)

	var wg sync.WaitGroup
	hashComputationWorker := func() {
		for name := range inputs {
			hash, next := ct.nodeManager.Hash(name)
			outputs <- NameHashNext{name, hash, next}
		}
		wg.Done()
	}

	threads := int(0.8 * float32(runtime.GOMAXPROCS(0)))
	if threads < 1 {
		threads = 1
	}
	for threads > 0 {
		threads--
		wg.Add(1)
		go hashComputationWorker()
	}
	go func() {
		if all {
			ct.nodeManager.IterateNames(func(name []byte) bool {
				if interruptRequested(interrupt) {
					return false
				}
				clone := make([]byte, len(name))
				copy(clone, name) // iteration name buffer is reused on future loops
				inputs <- clone
				return true
			})
		} else {
			for _, name := range names {
				if interruptRequested(interrupt) {
					break
				}
				inputs <- name
			}
		}
		close(inputs)
	}()
	go func() {
		wg.Wait()
		close(outputs)
	}()
	return outputs
}
