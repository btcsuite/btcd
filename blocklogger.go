package main

import (
	"sync"
	"time"

	"github.com/btcsuite/btclog"
	"github.com/decred/dcrd/dcrutil"
)

// blockProgressLogger provides periodic logging for other services in order
// to show users progress of certain "actions" involving some or all current
// blocks. Ex: syncing to best chain, indexing all blocks, etc.
type blockProgressLogger struct {
	receivedLogBlocks      int64
	receivedLogTx          int64
	receivedLogVotes       int64
	receivedLogRevocations int64
	receivedLogTickets     int64
	lastBlockLogTime       time.Time

	subsystemLogger btclog.Logger
	progressAction  string
	sync.Mutex
}

// newBlockProgressLogger returns a new block progress logger.
// The progress message is templated as follows:
//  {progressAction} {numProcessed} {blocks|block} in the last {timePeriod}
//  ({numTxs}, height {lastBlockHeight}, {lastBlockTimeStamp})
func newBlockProgressLogger(progressMessage string, logger btclog.Logger) *blockProgressLogger {
	return &blockProgressLogger{
		lastBlockLogTime: time.Now(),
		progressAction:   progressMessage,
		subsystemLogger:  logger,
	}
}

// logBlockHeight logs a new block height as an information message to show
// progress to the user. In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (b *blockProgressLogger) logBlockHeight(block *dcrutil.Block) {
	b.Lock()
	defer b.Unlock()
	b.receivedLogBlocks++
	b.receivedLogTx += int64(len(block.MsgBlock().Transactions))
	b.receivedLogVotes += int64(block.MsgBlock().Header.Voters)
	b.receivedLogRevocations += int64(block.MsgBlock().Header.Revocations)
	b.receivedLogTickets += int64(block.MsgBlock().Header.FreshStake)
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
	ticketStr := "tickets"
	if b.receivedLogTickets == 1 {
		ticketStr = "ticket"
	}
	revocationStr := "revocations"
	if b.receivedLogRevocations == 1 {
		revocationStr = "revocation"
	}
	voteStr := "votes"
	if b.receivedLogVotes == 1 {
		voteStr = "vote"
	}
	b.subsystemLogger.Infof("%s %d %s in the last %s (%d %s, %d %s, %d %s, %d %s, height "+
		"%d, %s)",
		b.progressAction, b.receivedLogBlocks, blockStr, tDuration,
		b.receivedLogTx, txStr, b.receivedLogTickets, ticketStr,
		b.receivedLogVotes, voteStr, b.receivedLogRevocations,
		revocationStr, block.Height(), block.MsgBlock().Header.Timestamp)

	b.receivedLogBlocks = 0
	b.receivedLogTx = 0
	b.receivedLogVotes = 0
	b.receivedLogTickets = 0
	b.receivedLogRevocations = 0
	b.lastBlockLogTime = now
}

func (b *blockProgressLogger) SetLastLogTime(time time.Time) {
	b.lastBlockLogTime = time
}
