// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"sort"
	"time"

	"github.com/btcsuite/btcd/chainhash/v2"
	peerpkg "github.com/btcsuite/btcd/peer"
)

const (
	// maxParallelBlockPeers is the number of peers used concurrently for
	// IBD block download once headers are caught up.
	maxParallelBlockPeers = 3

	// maxInFlightPerPeer caps outstanding getdata block hashes per peer so
	// work stays partitioned across the parallel pool.
	maxInFlightPerPeer = 64

	// slowPeerRatio kicks a peer whose score-window throughput is below
	// this fraction of the pool median (when at least two peers have data).
	slowPeerRatio = 0.25
)

// startParallelBlockFetch selects up to maxParallelBlockPeers download peers
// and begins partitioned getdata against the header chain.
func (sm *SyncManager) startParallelBlockFetch() {
	best := sm.chain.BestSnapshot()
	candidates := sm.fetchHigherPeers(best.Height)
	if len(candidates) == 0 {
		log.Warnf("No sync peer candidates available for block fetch")
		return
	}

	sm.ibdMode = true
	sm.lastProgressTime = time.Now()
	sm.lastScoreTime = time.Now()

	// Start with the header sync peer only. Unproven peers often claim
	// NODE_NETWORK but cannot serve history; assigning tip+1/+2 to them
	// stalls IBD until the long stall timer fires. Grow the pool only
	// after peers prove they can deliver (see maybeGrowBlockPeerPool).
	if sm.blockPeers == nil {
		sm.blockPeers = make(map[*peerpkg.Peer]struct{})
	}
	if sm.syncPeer != nil {
		if _, ok := sm.blockPeers[sm.syncPeer]; !ok {
			sm.addBlockPeer(sm.syncPeer)
		}
	} else {
		sm.ensureBlockPeers()
	}

	if len(sm.blockPeers) == 0 {
		log.Warnf("Failed to populate IBD block peer pool")
		return
	}

	sm.pickSyncPeerFromBlockPool()
	log.Infof("Syncing blocks to height %d from %d peers (parallel IBD)",
		sm.maxBlockPeerHeight(), len(sm.blockPeers))
	sm.fillAllBlockRequests()
}

// ensureBlockPeers grows the download pool up to maxParallelBlockPeers from
// sync candidates that advertise blocks we still need.
func (sm *SyncManager) ensureBlockPeers() {
	if sm.blockPeers == nil {
		sm.blockPeers = make(map[*peerpkg.Peer]struct{})
	}

	bestHeight := sm.chain.BestSnapshot().Height

	// Prefer the current sync peer first (often the peer that just finished
	// headers quickly) so it is not left out of a randomly ordered map walk.
	if sm.syncPeer != nil {
		if state := sm.peerStates[sm.syncPeer]; state != nil &&
			state.syncCandidate &&
			sm.syncPeer.LastBlock() > bestHeight {
			if _, exists := sm.blockPeers[sm.syncPeer]; !exists &&
				len(sm.blockPeers) < maxParallelBlockPeers {
				sm.addBlockPeer(sm.syncPeer)
			}
		}
	}

	for peer, state := range sm.peerStates {
		if len(sm.blockPeers) >= maxParallelBlockPeers {
			break
		}
		if !state.syncCandidate {
			continue
		}
		if _, exists := sm.blockPeers[peer]; exists {
			continue
		}
		if peer.LastBlock() <= bestHeight {
			continue
		}

		sm.addBlockPeer(peer)
	}
}

// addBlockPeer registers peer for parallel IBD getdata and resets its score
// window so a fresh join is not immediately treated as slow.
func (sm *SyncManager) addBlockPeer(peer *peerpkg.Peer) {
	state, exists := sm.peerStates[peer]
	if !exists {
		return
	}

	sm.blockPeers[peer] = struct{}{}
	state.bytesFetched = 0
	state.lastBlockProgress = time.Now()
	log.Infof("IBD block peer +%s (%d/%d)",
		peer.Addr(), len(sm.blockPeers), maxParallelBlockPeers)
}

// pickSyncPeerFromBlockPool keeps syncPeer non-nil for header/inv compatibility
// by selecting any active block peer (preferring the least loaded).
func (sm *SyncManager) pickSyncPeerFromBlockPool() {
	peers := sm.blockPeersByLoad()
	if len(peers) == 0 {
		sm.syncPeer = nil
		return
	}
	sm.syncPeer = peers[0]
}

// maxBlockPeerHeight returns the highest LastBlock among block peers.
func (sm *SyncManager) maxBlockPeerHeight() int32 {
	var maxHeight int32
	for peer := range sm.blockPeers {
		if h := peer.LastBlock(); h > maxHeight {
			maxHeight = h
		}
	}
	return maxHeight
}

// blockPeersByLoad returns block peers sorted for assignment: peers that have
// already delivered IBD blocks are preferred, then fewest in-flight.
func (sm *SyncManager) blockPeersByLoad() []*peerpkg.Peer {
	peers := make([]*peerpkg.Peer, 0, len(sm.blockPeers))
	for peer := range sm.blockPeers {
		peers = append(peers, peer)
	}

	haveWorking := false
	for _, peer := range peers {
		if state := sm.peerStates[peer]; state != nil && state.blocksDelivered > 0 {
			haveWorking = true
			break
		}
	}

	sort.Slice(peers, func(i, j int) bool {
		si := sm.peerStates[peers[i]]
		sj := sm.peerStates[peers[j]]
		li, lj := 0, 0
		di, dj := uint64(0), uint64(0)
		if si != nil {
			li = len(si.requestedBlocks)
			di = si.blocksDelivered
		}
		if sj != nil {
			lj = len(sj.requestedBlocks)
			dj = sj.blocksDelivered
		}
		if haveWorking {
			wi, wj := di > 0, dj > 0
			if wi != wj {
				return wi
			}
		}
		if li != lj {
			return li < lj
		}
		return peers[i].Addr() < peers[j].Addr()
	})
	return peers
}

// fillAllBlockRequests assigns more getdata work to under-filled block peers
// in small round-robin chunks so the pool stays partitioned.
func (sm *SyncManager) fillAllBlockRequests() {
	if len(sm.blockPeers) == 0 {
		return
	}

	_, bestHeaderHeight := sm.chain.BestHeader()
	bestHeight := sm.chain.BestSnapshot().Height
	if bestHeight >= bestHeaderHeight {
		return
	}

	// Small chunks keep heights interleaved across peers. Large consecutive
	// ranges stall IBD on whichever peer holds tip+1 while later blocks sit
	// as orphans.
	const assignChunk = 1
	workingOnly := false
	for peer := range sm.blockPeers {
		if state := sm.peerStates[peer]; state != nil && state.blocksDelivered > 0 {
			workingOnly = true
			break
		}
	}

	for {
		assignedAny := false
		for _, peer := range sm.blockPeersByLoad() {
			state := sm.peerStates[peer]
			if state == nil {
				continue
			}
			if workingOnly && state.blocksDelivered == 0 {
				continue
			}
			room := maxInFlightPerPeer - len(state.requestedBlocks)
			if room <= 0 {
				continue
			}
			if room > assignChunk {
				room = assignChunk
			}
			if sm.fetchHeaderBlocksLimited(peer, room) > 0 {
				assignedAny = true
			}
		}
		if !assignedAny {
			return
		}
	}
}

// fetchHeaderBlocksLimited requests up to maxNew header-chain blocks from peer.
func (sm *SyncManager) fetchHeaderBlocksLimited(peer *peerpkg.Peer, maxNew int) int {
	if peer == nil || maxNew <= 0 {
		return 0
	}
	gdmsg := sm.buildBlockRequest(peer, maxNew)
	if len(gdmsg.InvList) == 0 {
		return 0
	}
	peer.QueueMessage(gdmsg, nil)
	return len(gdmsg.InvList)
}

// headersCaughtUp reports whether we have no peers advertising headers beyond
// our best header tip (block download may proceed).
func (sm *SyncManager) headersCaughtUp() bool {
	_, bestHeaderHeight := sm.chain.BestHeader()
	return len(sm.fetchHigherPeers(bestHeaderHeight)) == 0
}

// isBlockDownloadPeer reports whether peer is in the parallel IBD pool.
func (sm *SyncManager) isBlockDownloadPeer(peer *peerpkg.Peer) bool {
	_, ok := sm.blockPeers[peer]
	return ok
}

// noteBlockPeerProgress records that peer delivered a connected IBD block.
func (sm *SyncManager) noteBlockPeerProgress(peer *peerpkg.Peer, blockBytes int) {
	state, exists := sm.peerStates[peer]
	if !exists {
		return
	}

	state.bytesFetched += uint64(blockBytes)
	state.blocksDelivered++
	state.lastBlockProgress = time.Now()
	sm.lastProgressTime = time.Now()

	// After a peer proves it can serve blocks, try growing the pool.
	sm.maybeGrowBlockPeerPool()
}

// maybeGrowBlockPeerPool adds one unproven candidate when we have a working
// peer and spare pool capacity. Newcomers only receive work while
// workingOnly is false for them — once any peer has delivered, fillAll
// skips undelivered peers, so growth uses probeAssignment instead.
func (sm *SyncManager) maybeGrowBlockPeerPool() {
	if len(sm.blockPeers) >= maxParallelBlockPeers {
		return
	}
	haveWorking := false
	for peer := range sm.blockPeers {
		if state := sm.peerStates[peer]; state != nil && state.blocksDelivered > 0 {
			haveWorking = true
			break
		}
	}
	if !haveWorking {
		return
	}

	bestHeight := sm.chain.BestSnapshot().Height
	for peer, state := range sm.peerStates {
		if len(sm.blockPeers) >= maxParallelBlockPeers {
			return
		}
		if !state.syncCandidate {
			continue
		}
		if _, exists := sm.blockPeers[peer]; exists {
			continue
		}
		if peer.LastBlock() <= bestHeight {
			continue
		}
		sm.addBlockPeer(peer)
		// Give the newcomer a tip-adjacent priority probe so it can prove
		// itself; if it notfounds/stalls it will be kicked quickly.
		sm.probeNewBlockPeer(peer)
		return
	}
}

// probeNewBlockPeer assigns a single missing tip-adjacent block to a newly
// added peer so it can earn blocksDelivered (or get kicked on notfound/stall).
func (sm *SyncManager) probeNewBlockPeer(peer *peerpkg.Peer) {
	state := sm.peerStates[peer]
	if state == nil {
		return
	}
	// Temporarily allow assignment by marking as working for one request:
	// fetchHeaderBlocksLimited respects per-peer caps only; workingOnly
	// skips undelivered peers in fillAll, so call build directly here.
	if sm.fetchHeaderBlocksLimited(peer, 1) > 0 {
		log.Infof("IBD probe getdata → %s", peer.Addr())
	}
}

// requeuePeerBlockRequests returns a peer's outstanding IBD block hashes to
// the front of the assign line: they are cleared from in-flight maps and
// recorded in priorityBlocks so the next fill requests them before scanning
// further ahead of tip (keeps the orphan buffer small).
func (sm *SyncManager) requeuePeerBlockRequests(state *peerSyncState) int {
	if state == nil {
		return 0
	}
	if sm.priorityBlocks == nil {
		sm.priorityBlocks = make(map[chainhash.Hash]struct{})
	}

	n := len(state.requestedBlocks)
	for blockHash := range state.requestedBlocks {
		delete(sm.requestedBlocks, blockHash)
		sm.priorityBlocks[blockHash] = struct{}{}
	}
	state.requestedBlocks = make(map[chainhash.Hash]struct{})
	return n
}

// priorityHashesByHeight returns priority block hashes ordered by ascending
// header height so tip-adjacent gaps are filled first.
func (sm *SyncManager) priorityHashesByHeight() []chainhash.Hash {
	type item struct {
		hash   chainhash.Hash
		height int32
	}
	items := make([]item, 0, len(sm.priorityBlocks))
	for hash := range sm.priorityBlocks {
		height, err := sm.chain.HeaderHeightByHash(hash)
		if err != nil {
			// Unknown to header index; still try soon with height 0.
			items = append(items, item{hash: hash, height: 0})
			continue
		}
		items = append(items, item{hash: hash, height: height})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].height != items[j].height {
			return items[i].height < items[j].height
		}
		return items[i].hash.String() < items[j].hash.String()
	})
	out := make([]chainhash.Hash, len(items))
	for i := range items {
		out[i] = items[i].hash
	}
	return out
}

// freeAheadSlotsForPriority cancels the farthest in-flight IBD requests so
// peers have room to pull priority (requeued) hashes next to tip.
func (sm *SyncManager) freeAheadSlotsForPriority() {
	need := len(sm.priorityBlocks)
	if need == 0 || len(sm.blockPeers) == 0 {
		return
	}

	free := 0
	for peer := range sm.blockPeers {
		state := sm.peerStates[peer]
		if state == nil {
			continue
		}
		if n := maxInFlightPerPeer - len(state.requestedBlocks); n > 0 {
			free += n
		}
	}
	if free >= need {
		return
	}
	toFree := need - free

	type held struct {
		peer   *peerpkg.Peer
		hash   chainhash.Hash
		height int32
	}
	var far []held
	for peer := range sm.blockPeers {
		state := sm.peerStates[peer]
		if state == nil {
			continue
		}
		for hash := range state.requestedBlocks {
			if _, priority := sm.priorityBlocks[hash]; priority {
				continue
			}
			height, err := sm.chain.HeaderHeightByHash(hash)
			if err != nil {
				continue
			}
			far = append(far, held{peer: peer, hash: hash, height: height})
		}
	}
	sort.Slice(far, func(i, j int) bool {
		return far[i].height > far[j].height
	})

	for _, item := range far {
		if toFree <= 0 {
			break
		}
		state := sm.peerStates[item.peer]
		if state == nil {
			continue
		}
		if _, ok := state.requestedBlocks[item.hash]; !ok {
			continue
		}
		delete(state.requestedBlocks, item.hash)
		delete(sm.requestedBlocks, item.hash)
		// Cancelled ahead-of-tip work becomes normal scan fodder later;
		// do not mark priority — tip gaps come first.
		toFree--
	}
}

// kickBlockPeer removes peer from the parallel pool, requeues its in-flight
// block hashes so they are not stranded, optionally disconnects, and refills
// the pool from remaining candidates.
func (sm *SyncManager) kickBlockPeer(peer *peerpkg.Peer, disconnect bool, reason string) {
	state, exists := sm.peerStates[peer]
	if !exists {
		return
	}
	if !sm.isBlockDownloadPeer(peer) {
		return
	}

	requeued := sm.requeuePeerBlockRequests(state)
	delete(sm.blockPeers, peer)
	state.bytesFetched = 0
	// Prevent ensureBlockPeers from immediately re-adding this peer before
	// Disconnect/handleDonePeerMsg finishes removing it.
	state.syncCandidate = false

	log.Infof("IBD block peer -%s (%s); requeued %d in-flight block(s) to front of line; pool %d/%d",
		peer.Addr(), reason, requeued, len(sm.blockPeers), maxParallelBlockPeers)

	if peer == sm.syncPeer {
		sm.syncPeer = nil
		sm.pickSyncPeerFromBlockPool()
	}

	if disconnect {
		peer.Disconnect()
	}

	if len(sm.blockPeers) == 0 {
		// Emergency refill from any candidates when the pool is empty.
		sm.ensureBlockPeers()
		if len(sm.blockPeers) == 0 {
			sm.syncPeer = nil
			sm.startSync()
			return
		}
	}

	sm.freeAheadSlotsForPriority()
	sm.pickSyncPeerFromBlockPool()
	sm.fillAllBlockRequests()
	sm.maybeGrowBlockPeerPool()
}

// handleParallelStallSample kicks stalled or clearly slow block peers and
// replaces them. Header download still uses the single-syncPeer stall path.
func (sm *SyncManager) handleParallelStallSample() {
	if len(sm.blockPeers) == 0 {
		return
	}

	now := time.Now()

	// Stall kicks first: no progress while holding in-flight work.
	// Unproven peers (no blocks delivered) fail fast so they cannot pin
	// tip-adjacent hashes for the full stall duration.
	for _, peer := range sm.blockPeerSnapshot() {
		state := sm.peerStates[peer]
		if state == nil {
			continue
		}
		if len(state.requestedBlocks) == 0 {
			continue
		}
		limit := maxStallDuration
		if state.blocksDelivered == 0 {
			limit = 30 * time.Second
		}
		if now.Sub(state.lastBlockProgress) <= limit {
			continue
		}
		sm.kickBlockPeer(peer, true, "stalled")
	}

	if len(sm.blockPeers) == 0 {
		// Pool emptied; resume normal startSync selection.
		sm.syncPeer = nil
		sm.startSync()
		return
	}

	if now.Sub(sm.lastScoreTime) < stallSampleInterval {
		return
	}
	sm.lastScoreTime = now

	slow := sm.findSlowBlockPeer()
	if slow == nil {
		sm.resetBlockPeerScores()
		return
	}

	sm.kickBlockPeer(slow, true, "slowest parallel peer")
	sm.resetBlockPeerScores()
}

// blockPeerSnapshot copies block peer pointers for safe mutation while ranging.
func (sm *SyncManager) blockPeerSnapshot() []*peerpkg.Peer {
	peers := make([]*peerpkg.Peer, 0, len(sm.blockPeers))
	for peer := range sm.blockPeers {
		peers = append(peers, peer)
	}
	return peers
}

// findSlowBlockPeer returns the slowest peer when it is clearly below the
// pool median; otherwise nil.
func (sm *SyncManager) findSlowBlockPeer() *peerpkg.Peer {
	type scored struct {
		peer  *peerpkg.Peer
		bytes uint64
	}

	scores := make([]scored, 0, len(sm.blockPeers))
	for peer := range sm.blockPeers {
		state := sm.peerStates[peer]
		if state == nil {
			continue
		}
		scores = append(scores, scored{peer: peer, bytes: state.bytesFetched})
	}
	if len(scores) < 2 {
		return nil
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].bytes != scores[j].bytes {
			return scores[i].bytes < scores[j].bytes
		}
		return scores[i].peer.Addr() < scores[j].peer.Addr()
	})

	slowest := scores[0]
	median := scores[len(scores)/2].bytes

	// Need some pool progress before judging stragglers.
	if median == 0 {
		return nil
	}
	if float64(slowest.bytes) >= float64(median)*slowPeerRatio {
		return nil
	}
	return slowest.peer
}

// resetBlockPeerScores clears the scoring window after a ranking pass.
func (sm *SyncManager) resetBlockPeerScores() {
	for peer := range sm.blockPeers {
		if state := sm.peerStates[peer]; state != nil {
			state.bytesFetched = 0
		}
	}
}
