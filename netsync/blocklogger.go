// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btclog"
)

// blockProgressLogger provides periodic logging for other services in order
// to show users progress of certain "actions" involving some or all current
// blocks. Ex: syncing to best chain, indexing all blocks, etc.
type blockProgressLogger struct {
	receivedLogBlocks int64
	receivedLogTx     int64
	lastBlockLogTime  time.Time

	subsystemLogger btclog.Logger
	progressAction  string
	sync.Mutex
}

// newBlockProgressLogger returns a new block progress logger.
// The progress message is templated as follows:
//
//	{progressAction} {numProcessed} {blocks|block} in the last {timePeriod}
//	({numTxs}, height {lastBlockHeight}, {lastBlockTimeStamp})
func newBlockProgressLogger(progressMessage string, logger btclog.Logger) *blockProgressLogger {
	return &blockProgressLogger{
		lastBlockLogTime: time.Now(),
		progressAction:   progressMessage,
		subsystemLogger:  logger,
	}
}

// LogBlockHeight logs a new block height as an information message to show
// progress to the user. In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (b *blockProgressLogger) LogBlockHeight(block *btcutil.Block, chain *blockchain.BlockChain) {
	b.Lock()
	defer b.Unlock()

	b.receivedLogBlocks++
	b.receivedLogTx += int64(len(block.MsgBlock().Transactions))

	now := time.Now()
	duration := now.Sub(b.lastBlockLogTime)
	if duration < time.Second*10 {
		return
	}

	// Truncate the duration to 10s of milliseconds.
	durationMillis := int64(duration / time.Millisecond)
	tDuration := 10 * time.Millisecond * time.Duration(durationMillis/10)

	// Log information about new block height.
	blockStr := "blocks"
	if b.receivedLogBlocks == 1 {
		blockStr = "block"
	}
	txStr := "transactions"
	if b.receivedLogTx == 1 {
		txStr = "transaction"
	}
	cacheSizeStr := fmt.Sprintf("~%d MiB", chain.CachedStateSize()/1024/1024)
	b.subsystemLogger.Infof("%s %d %s in the last %s (%d %s, height %d, %s, %s cache)",
		b.progressAction, b.receivedLogBlocks, blockStr, tDuration, b.receivedLogTx,
		txStr, block.Height(), block.MsgBlock().Header.Timestamp, cacheSizeStr)

	b.receivedLogBlocks = 0
	b.receivedLogTx = 0
	b.lastBlockLogTime = now
}

func (b *blockProgressLogger) SetLastLogTime(time time.Time) {
	b.lastBlockLogTime = time
}

// peerLogger logs the progress of blocks downloaded from different peers during
// headers-first download.
type peerLogger struct {
	lastPeerLogTime time.Time
	peers           map[string]int

	subsystemLogger btclog.Logger
	sync.Mutex
}

// newPeerLogger returns a new peerLogger with fields initialized.
func newPeerLogger(logger btclog.Logger) *peerLogger {
	return &peerLogger{
		lastPeerLogTime: time.Now(),
		subsystemLogger: logger,
		peers:           make(map[string]int),
	}
}

// LogPeers logs how many blocks have been received from which peers in the last
// 10 seconds.
func (p *peerLogger) LogPeers(peer string) {
	p.Lock()
	defer p.Unlock()

	count, found := p.peers[peer]
	if found {
		count++
		p.peers[peer] = count
	} else {
		p.peers[peer] = 1
	}

	now := time.Now()
	duration := now.Sub(p.lastPeerLogTime)
	if duration < time.Second*10 {
		return
	}
	// Truncate the duration to 10s of milliseconds.
	durationMillis := int64(duration / time.Millisecond)
	tDuration := 10 * time.Millisecond * time.Duration(durationMillis/10)

	type peerInfo struct {
		name  string
		count int
	}

	// Sort by blocks downloaded before printing.
	var sortedPeers []peerInfo
	for k, v := range p.peers {
		sortedPeers = append(sortedPeers, peerInfo{k, v})
	}
	sort.Slice(sortedPeers, func(i, j int) bool {
		return sortedPeers[i].count > sortedPeers[j].count
	})

	totalBlocks := 0
	peerDownloadStr := ""
	for _, sortedPeer := range sortedPeers {
		peerDownloadStr += fmt.Sprintf("%d blocks from %v, ",
			sortedPeer.count, sortedPeer.name)
		totalBlocks += sortedPeer.count
	}

	p.subsystemLogger.Infof("Peer download stats in the last %s. total: %v, %s",
		tDuration, totalBlocks, peerDownloadStr)

	// Reset fields.
	p.lastPeerLogTime = now
	for k := range p.peers {
		delete(p.peers, k)
	}
}
