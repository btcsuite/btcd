// Copyright (c) 2013-2014 Conformal Systems LLC.
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"errors"
	"fmt"
	"sort"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	database "github.com/decred/dcrd/database2"
	"github.com/decred/dcrutil"
)

// TicketStatus is used to indicate the state of a ticket in the ticket store and
// the ticket database. Non-existing is included because it's possible to have
// a ticket not exist in a sidechain that exists in the mainchain (and thus exists
// in the ticket database), and so you need to indicate this in the ticket store.
// It could also point to a ticket that's been missed and eliminated from the
// ticket db by SSRtx.
type TicketStatus int

// Possible values for TicketStatus
const (
	TiNonexisting = iota
	TiSpent
	TiAvailable
	TiMissed
	TiRevoked
	TiError
)

// TicketPatchData contains contextual information about tickets, namely their
// ticket data and whether or not they are spent.
type TicketPatchData struct {
	td  *stake.TicketData
	ts  TicketStatus
	err error
}

// NewTicketPatchData creates a new TicketPatchData struct.
func NewTicketPatchData(td *stake.TicketData,
	ts TicketStatus,
	err error) *TicketPatchData {
	return &TicketPatchData{td, ts, err}
}

// TicketStore is used to store a patch of the ticket db for use in validating the
// block header and subsequently the block reward. It allows you to observe the
// ticket db from the point-of-view of different points in the chain.
// TicketStore is basically like an extremely inefficient version of the ticket
// database that isn't designed to be able to be easily rolled back, which is fine
// because we're only going to use it in ephermal cases.
type TicketStore map[chainhash.Hash]*TicketPatchData

// GenerateLiveTicketBucket takes ticket patch data and a bucket number as input,
// then recreates a ticket bucket from the patch and the current database state.
func (b *BlockChain) GenerateLiveTicketBucket(ticketStore TicketStore,
	tpdBucketMap map[uint8][]*TicketPatchData, bucket uint8) (stake.SStxMemMap,
	error) {
	bucketTickets := make(stake.SStxMemMap)

	// Check the ticketstore for live tickets and add these to the bucket if
	// their ticket number matches.
	for _, tpd := range tpdBucketMap[bucket] {
		if tpd.ts == TiAvailable {
			bucketTickets[tpd.td.SStxHash] = tpd.td
		}
	}

	// Check the ticket database for live tickets; prune live tickets that
	// have been spent/missed/were otherwise accounted for.
	liveTicketsFromDb, err := b.tmdb.DumpLiveTickets(bucket)
	if err != nil {
		return nil, err
	}

	for hash, td := range liveTicketsFromDb {
		if _, exists := ticketStore[hash]; exists {
			continue
		}
		bucketTickets[hash] = td
	}

	return bucketTickets, nil
}

// GenerateMissedTickets takes ticket patch data as input, then recreates the
// missed tickets bucket from the patch and the current database state.
func (b *BlockChain) GenerateMissedTickets(tixStore TicketStore) (stake.SStxMemMap,
	error) {
	missedTickets := make(stake.SStxMemMap)

	// Check the ticketstore for live tickets and add these to the bucket if
	// their ticket number matches.
	for hash, tpd := range tixStore {
		if tpd.ts == TiMissed {
			missedTickets[hash] = tpd.td
		}
	}

	// Check the ticket database for live tickets; prune live tickets that
	// have been spent/missed/were otherwise accounted for.
	missedTicketsFromDb, err := b.tmdb.DumpMissedTickets()
	if err != nil {
		return nil, err
	}

	for hash, td := range missedTicketsFromDb {
		if _, exists := tixStore[hash]; exists {
			continue
		}
		missedTickets[hash] = td
	}

	return missedTickets, nil
}

// connectTickets updates the passed map by removing removing any tickets
// from the ticket pool that have been considered spent or missed in this block
// according to the block header. Then, it connects all the newly mature tickets
// to the passed map.
func (b *BlockChain) connectTickets(tixStore TicketStore, node *blockNode,
	block *dcrutil.Block, view *UtxoViewpoint) error {
	if tixStore == nil {
		return fmt.Errorf("nil ticket store")
	}

	// Nothing to do if tickets haven't yet possibly matured.
	height := node.height
	if height < b.chainParams.StakeEnabledHeight {
		return nil
	}

	parentBlock, err := b.getBlockFromHash(&node.header.PrevBlock)
	if err != nil {
		return err
	}

	revocations := node.header.Revocations

	tM := int64(b.chainParams.TicketMaturity)

	// Skip a number of validation steps before we requiring chain
	// voting.
	if node.height >= b.chainParams.StakeValidationHeight {
		// We need the missed tickets bucket from the original perspective of
		// the node.
		missedTickets, err := b.GenerateMissedTickets(tixStore)
		if err != nil {
			return err
		}

		// TxStore at blockchain HEAD + TxTreeRegular of prevBlock (if
		// validated) for this node.
		parent, err := b.getBlockFromHash(&node.header.PrevBlock)
		if err != nil {
			return err
		}
		regularTxTreeValid := dcrutil.IsFlagSet16(node.header.VoteBits,
			dcrutil.BlockValid)
		thisNodeStakeViewpoint := ViewpointPrevInvalidStake
		if regularTxTreeValid {
			thisNodeStakeViewpoint = ViewpointPrevValidStake
		}
		view.SetStakeViewpoint(thisNodeStakeViewpoint)
		err = view.fetchInputUtxos(b.db, block, parent)
		if err != nil {
			errStr := fmt.Sprintf("fetchInputUtxos failed for incoming "+
				"node %v; error given: %v", node.hash, err)
			return errors.New(errStr)
		}

		// PART 1: Spend/miss winner tickets

		// Iterate through all the SSGen (vote) tx in the block and add them to
		// a map of tickets that were actually used.
		spentTicketsFromBlock := make(map[chainhash.Hash]bool)
		numberOfSSgen := 0
		for _, staketx := range block.STransactions() {
			if is, _ := stake.IsSSGen(staketx); is {
				msgTx := staketx.MsgTx()
				sstxIn := msgTx.TxIn[1] // sstx input
				sstxHash := sstxIn.PreviousOutPoint.Hash

				originUTXO := view.LookupEntry(&sstxHash)
				if originUTXO == nil {
					str := fmt.Sprintf("unable to find input transaction "+
						"%v for transaction %v", sstxHash, staketx.Sha())
					return ruleError(ErrMissingTx, str)
				}

				// Check maturity of ticket; we can only spend the ticket after it
				// hits maturity at height + tM + 1.
				sstxHeight := originUTXO.BlockHeight()
				if (height - sstxHeight) < (tM + 1) {
					blockSha := block.Sha()
					errStr := fmt.Sprintf("Error: A ticket spend as an SSGen in "+
						"block height %v was immature! Block sha %v",
						height,
						blockSha)
					return errors.New(errStr)
				}

				// Fill out the ticket data.
				spentTicketsFromBlock[sstxHash] = true
				numberOfSSgen++
			}
		}

		// Obtain the TicketsPerBlock many tickets that were selected this round,
		// then check these against the tickets that were actually used to make
		// sure that any SSGen actually match the selected tickets. Commit the
		// spent or missed tickets to the ticket store after.
		spentAndMissedTickets := make(TicketStore)
		tixSpent := 0
		tixMissed := 0

		// Sort the entire list of tickets lexicographically by sorting
		// each bucket and then appending it to the list. Start by generating
		// a prefix matched map of tickets to speed up the lookup.
		tpdBucketMap := make(map[uint8][]*TicketPatchData)
		for _, tpd := range tixStore {
			// Bucket does not exist.
			if _, ok := tpdBucketMap[tpd.td.Prefix]; !ok {
				tpdBucketMap[tpd.td.Prefix] = make([]*TicketPatchData, 1)
				tpdBucketMap[tpd.td.Prefix][0] = tpd
			} else {
				// Bucket exists.
				data := tpdBucketMap[tpd.td.Prefix]
				tpdBucketMap[tpd.td.Prefix] = append(data, tpd)
			}
		}
		totalTickets := 0
		var sortedSlice []*stake.TicketData
		for i := 0; i < stake.BucketsSize; i++ {
			ltb, err := b.GenerateLiveTicketBucket(tixStore, tpdBucketMap,
				uint8(i))
			if err != nil {
				h := node.hash
				str := fmt.Sprintf("Failed to generate live ticket bucket "+
					"%v for node %v, height %v! Error: %v",
					i,
					h,
					node.height,
					err.Error())
				return fmt.Errorf(str)
			}
			mapLen := len(ltb)

			tempTdSlice := stake.NewTicketDataSlice(mapLen)
			itr := 0 // Iterator
			for _, td := range ltb {
				tempTdSlice[itr] = td
				itr++
				totalTickets++
			}
			sort.Sort(tempTdSlice)
			sortedSlice = append(sortedSlice, tempTdSlice...)
		}

		// Use the parent block's header to seed a PRNG that picks the
		// lottery winners.
		ticketsPerBlock := int(b.chainParams.TicketsPerBlock)
		pbhB, err := parentBlock.MsgBlock().Header.Bytes()
		if err != nil {
			return err
		}
		prng := stake.NewHash256PRNG(pbhB)
		ts, err := stake.FindTicketIdxs(int64(totalTickets), ticketsPerBlock, prng)
		if err != nil {
			return err
		}

		ticketsToSpendOrMiss := make([]*stake.TicketData, ticketsPerBlock,
			ticketsPerBlock)
		for i, idx := range ts {
			ticketsToSpendOrMiss[i] = sortedSlice[idx]
		}

		// Spend or miss these tickets by checking for their existence in the
		// passed spentTicketsFromBlock map.
		for _, ticket := range ticketsToSpendOrMiss {
			// Move the ticket from active tickets map into the used tickets
			// map if the ticket was spent.
			wasSpent, _ := spentTicketsFromBlock[ticket.SStxHash]

			if wasSpent {
				tpd := NewTicketPatchData(ticket, TiSpent, nil)
				spentAndMissedTickets[ticket.SStxHash] = tpd
				tixSpent++
			} else { // Ticket missed being spent and --> false or nil
				tpd := NewTicketPatchData(ticket, TiMissed, nil)
				spentAndMissedTickets[ticket.SStxHash] = tpd
				tixMissed++
			}
		}

		// This error is thrown if for some reason there exists an SSGen in
		// the block that doesn't spend a ticket from the eligible list of
		// tickets, thus making it invalid.
		if tixSpent != numberOfSSgen {
			errStr := fmt.Sprintf("an invalid number %v "+
				"tickets was spent, but %v many tickets should "+
				"have been spent!", tixSpent, numberOfSSgen)
			return errors.New(errStr)
		}

		if tixMissed != (ticketsPerBlock - numberOfSSgen) {
			errStr := fmt.Sprintf("an invalid number %v "+
				"tickets was missed, but %v many tickets should "+
				"have been missed!", tixMissed,
				ticketsPerBlock-numberOfSSgen)
			return errors.New(errStr)
		}

		if (tixSpent + tixMissed) != int(b.chainParams.TicketsPerBlock) {
			errStr := fmt.Sprintf("an invalid number %v "+
				"tickets was spent and missed, but TicketsPerBlock %v many "+
				"tickets should have been spent!", tixSpent,
				ticketsPerBlock)
			return errors.New(errStr)
		}

		// Calculate all the tickets expiring this block and mark them as missed.
		tpdBucketMap = make(map[uint8][]*TicketPatchData)
		for _, tpd := range tixStore {
			// Bucket does not exist.
			if _, ok := tpdBucketMap[tpd.td.Prefix]; !ok {
				tpdBucketMap[tpd.td.Prefix] = make([]*TicketPatchData, 1)
				tpdBucketMap[tpd.td.Prefix][0] = tpd
			} else {
				// Bucket exists.
				data := tpdBucketMap[tpd.td.Prefix]
				tpdBucketMap[tpd.td.Prefix] = append(data, tpd)
			}
		}
		toExpireHeight := node.height - int64(b.chainParams.TicketExpiry)
		if !(toExpireHeight < int64(b.chainParams.StakeEnabledHeight)) {
			for i := 0; i < stake.BucketsSize; i++ {
				// Generate the live ticket bucket.
				ltb, err := b.GenerateLiveTicketBucket(tixStore,
					tpdBucketMap, uint8(i))
				if err != nil {
					return err
				}

				for _, ticket := range ltb {
					if ticket.BlockHeight == toExpireHeight {
						tpd := NewTicketPatchData(ticket, TiMissed, nil)
						spentAndMissedTickets[ticket.SStxHash] = tpd
					}
				}
			}
		}

		// Merge the ticket store patch containing the spent and missed tickets
		// with the ticket store.
		for hash, tpd := range spentAndMissedTickets {
			tixStore[hash] = tpd
		}

		// At this point our tixStore now contains all the spent and missed tx
		// as per this block.

		// PART 2: Remove tickets that were missed and are now revoked.

		// Iterate through all the SSGen (vote) tx in the block and add them to
		// a map of tickets that were actually used.
		revocationsFromBlock := make(map[chainhash.Hash]struct{})
		numberOfSSRtx := 0
		for _, staketx := range block.STransactions() {
			if is, _ := stake.IsSSRtx(staketx); is {
				msgTx := staketx.MsgTx()
				sstxIn := msgTx.TxIn[0] // sstx input
				sstxHash := sstxIn.PreviousOutPoint.Hash

				// Fill out the ticket data.
				revocationsFromBlock[sstxHash] = struct{}{}
				numberOfSSRtx++
			}
		}

		if numberOfSSRtx != int(revocations) {
			errStr := fmt.Sprintf("an invalid revocations %v was calculated "+
				"the block header indicates %v instead", numberOfSSRtx,
				revocations)
			return errors.New(errStr)
		}

		// Lookup the missed ticket. If we find it in the patch data,
		// modify the patch data so that it doesn't exist.
		// Otherwise, just modify load the missed ticket data from
		// the ticket db and create patch data based on that.
		for hash := range revocationsFromBlock {
			ticketWasMissed := false
			if td, is := missedTickets[hash]; is {
				maturedHeight := td.BlockHeight

				// Check maturity of ticket; we can only spend the ticket after it
				// hits maturity at height + tM + 2.
				if height < maturedHeight+2 {
					blockSha := block.Sha()
					errStr := fmt.Sprintf("Error: A ticket spend as an "+
						"SSRtx in block height %v was immature! Block sha %v",
						height,
						blockSha)
					return errors.New(errStr)
				}

				ticketWasMissed = true
			}

			if !ticketWasMissed {
				errStr := fmt.Sprintf("SSRtx spent missed sstx %v, "+
					"but that missed sstx could not be found!",
					hash)
				return errors.New(errStr)
			}
		}
	}

	// PART 3: Add newly maturing tickets
	// This is the only chunk we need to do for blocks appearing before
	// stake validation height.

	// Calculate block number for where new tickets are maturing from and retrieve
	// this block from db.

	// Get the block that is maturing.
	matureNode, err := b.getNodeAtHeightFromTopNode(node, tM)
	if err != nil {
		return err
	}

	matureBlock, errBlock := b.getBlockFromHash(matureNode.hash)
	if errBlock != nil {
		return errBlock
	}

	// Maturing tickets are from the maturingBlock; fill out the ticket patch data
	// and then push them to the tixStore.
	for _, stx := range matureBlock.STransactions() {
		if is, _ := stake.IsSStx(stx); is {
			// Calculate the prefix for pre-sort.
			sstxHash := *stx.Sha()
			prefix := uint8(sstxHash[0])

			// Fill out the ticket data.
			td := stake.NewTicketData(sstxHash,
				prefix,
				chainhash.Hash{},
				height,
				false, // not missed
				false) // not expired

			tpd := NewTicketPatchData(td,
				TiAvailable,
				nil)
			tixStore[*stx.Sha()] = tpd
		}
	}

	return nil
}

// disconnectTransactions updates the passed map by undoing transaction and
// spend information for all transactions in the passed block.  Only
// transactions in the passed map are updated.
// This function should only ever have to disconnect transactions from the main
// chain, so most of the calls are directly the the tmdb which contains all this
// data in an organized bucket.
func (b *BlockChain) disconnectTickets(tixStore TicketStore, node *blockNode,
	block *dcrutil.Block) error {
	tM := int64(b.chainParams.TicketMaturity)
	height := node.height

	// Nothing to do if tickets haven't yet possibly matured.
	if height < b.chainParams.StakeEnabledHeight {
		return nil
	}

	// PART 1: Remove newly maturing tickets

	// Calculate block number for where new tickets matured from and retrieve
	// this block from db.
	matureNode, err := b.getNodeAtHeightFromTopNode(node, tM)
	if err != nil {
		return err
	}

	matureBlock, errBlock := b.getBlockFromHash(matureNode.hash)
	if errBlock != nil {
		return errBlock
	}

	// Store pointers to empty ticket data in the ticket store and mark them as
	// non-existing.
	for _, stx := range matureBlock.STransactions() {
		if is, _ := stake.IsSStx(stx); is {
			// Leave this pointing to nothing, as the ticket technically does not
			// exist. It may exist when we add blocks later, but we can fill it
			// out then.
			td := &stake.TicketData{}

			tpd := NewTicketPatchData(td,
				TiNonexisting,
				nil)

			tixStore[*stx.Sha()] = tpd
		}
	}

	// PART 2: Unrevoke any SSRtx in this block and restore them as
	// missed tickets.
	for _, stx := range block.STransactions() {
		if is, _ := stake.IsSSRtx(stx); is {
			// Move the revoked ticket to missed tickets. Obtain the
			// revoked ticket data from the ticket database.
			msgTx := stx.MsgTx()
			sstxIn := msgTx.TxIn[0] // sstx input
			sstxHash := sstxIn.PreviousOutPoint.Hash

			td := b.tmdb.GetRevokedTicket(sstxHash)
			if td == nil {
				return fmt.Errorf("Failed to find revoked ticket %v in tmdb",
					sstxHash)
			}

			tpd := NewTicketPatchData(td,
				TiMissed,
				nil)

			tixStore[sstxHash] = tpd
		}
	}

	// PART 3: Unspend or unmiss all tickets spent/missed/expired at this block.

	// Query the stake db for used tickets (spentTicketDb), which includes all of
	// the spent and missed tickets.
	spentTickets, errDump := b.tmdb.DumpSpentTickets(height)
	if errDump != nil {
		return errDump
	}

	// Move all of these tickets into the ticket store as available tickets.
	for hash, td := range spentTickets {
		tpd := NewTicketPatchData(td,
			TiAvailable,
			nil)

		tixStore[hash] = tpd
	}

	return nil
}

// fetchTicketStore fetches ticket data from the point of view of the given node.
// For example, a given node might be down a side chain where a ticket hasn't been
// spent from its point of view even though it might have been spent in the main
// chain (or another side chain).  Another scenario is where a ticket exists from
// the point of view of the main chain, but doesn't exist in a side chain that
// branches before the block that contains the ticket on the main chain.
func (b *BlockChain) fetchTicketStore(node *blockNode) (TicketStore, error) {
	tixStore := make(TicketStore)

	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return nil, err
	}

	// If we haven't selected a best chain yet or we are extending the main
	// (best) chain with a new block, just use the ticket database we already
	// have.
	if b.bestNode == nil || (prevNode != nil &&
		prevNode.hash.IsEqual(b.bestNode.hash)) {
		return nil, nil
	}

	// We don't care about nodes before stake enabled height.
	if node.height < b.chainParams.StakeEnabledHeight {
		return nil, nil
	}

	// The requested node is either on a side chain or is a node on the main
	// chain before the end of it.  In either case, we need to undo the
	// transactions and spend information for the blocks which would be
	// disconnected during a reorganize to the point of view of the
	// node just before the requested node.
	detachNodes, attachNodes := b.getReorganizeNodes(node)
	if err != nil {
		return nil, err
	}

	view := NewUtxoViewpoint()
	view.SetBestHash(b.bestNode.hash)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)

	for e := detachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		block, err := b.getBlockFromHash(n.hash)
		if err != nil {
			return nil, err
		}

		parent, err := b.getBlockFromHash(&n.header.PrevBlock)
		if err != nil {
			return nil, err
		}

		// Load all of the spent txos for the block from the spend
		// journal.
		var stxos []spentTxOut
		err = b.db.View(func(dbTx database.Tx) error {
			stxos, err = dbFetchSpendJournalEntry(dbTx, block, parent, view)
			return err
		})
		if err != nil {
			return nil, err
		}
		err = b.disconnectTransactions(view, block, parent, stxos)
		if err != nil {
			return nil, err
		}
		err = b.disconnectTickets(tixStore, n, block)
		if err != nil {
			return nil, err
		}
	}

	// The ticket store is now accurate to either the node where the
	// requested node forks off the main chain (in the case where the
	// requested node is on a side chain), or the requested node itself if
	// the requested node is an old node on the main chain.  Entries in the
	// attachNodes list indicate the requested node is on a side chain, so
	// if there are no nodes to attach, we're done.
	if attachNodes.Len() == 0 {
		return tixStore, nil
	}

	// The requested node is on a side chain, so we need to apply the
	// transactions and spend information from each of the nodes to attach.
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		n := e.Value.(*blockNode)
		block, exists := b.blockCache[*n.hash]
		if !exists {
			return nil, fmt.Errorf("unable to find block %v in "+
				"side chain cache for ticket db patch construction",
				n.hash)
		}

		// The number of blocks below this block but above the root of the fork
		err = b.connectTickets(tixStore, n, block, view)
		if err != nil {
			return nil, err
		}

		parent, err := b.getBlockFromHash(&n.header.PrevBlock)
		if err != nil {
			return nil, err
		}

		var stxos []spentTxOut
		err = b.connectTransactions(view, block, parent, &stxos)
		if err != nil {
			return nil, err
		}

		view.SetBestHash(node.hash)
	}

	return tixStore, nil
}
