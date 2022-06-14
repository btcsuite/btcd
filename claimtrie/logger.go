// Copyright (c) 2015-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package claimtrie

import (
	"sync"
	"time"

	"github.com/btcsuite/btclog"
)

// nameProgressLogger provides periodic logging for other services in order
// to show users progress of certain "actions" involving some or all current
// claim names. Ex: rebuilding claimtrie.
type nameProgressLogger struct {
	totalLogName    int64
	recentLogName   int64
	lastLogNameTime time.Time

	subsystemLogger btclog.Logger
	progressAction  string
	sync.Mutex
}

// newNameProgressLogger returns a new name progress logger.
// The progress message is templated as follows:
//  {progressAction} {numProcessed} {names|name} in the last {timePeriod} (total {totalProcessed})
func newNameProgressLogger(progressMessage string, logger btclog.Logger) *nameProgressLogger {
	return &nameProgressLogger{
		lastLogNameTime: time.Now(),
		progressAction:  progressMessage,
		subsystemLogger: logger,
	}
}

// LogName logs a new name as an information message to show progress
// to the user. In order to prevent spam, it limits logging to one
// message every 10 seconds with duration and totals included.
func (n *nameProgressLogger) LogName(name []byte) {
	n.Lock()
	defer n.Unlock()

	n.totalLogName++
	n.recentLogName++

	now := time.Now()
	duration := now.Sub(n.lastLogNameTime)
	if duration < time.Second*10 {
		return
	}

	// Truncate the duration to 10s of milliseconds.
	durationMillis := int64(duration / time.Millisecond)
	tDuration := 10 * time.Millisecond * time.Duration(durationMillis/10)

	// Log information about progress.
	nameStr := "names"
	if n.recentLogName == 1 {
		nameStr = "name"
	}
	n.subsystemLogger.Infof("%s %d %s in the last %s (total %d)",
		n.progressAction, n.recentLogName, nameStr, tDuration, n.totalLogName)

	n.recentLogName = 0
	n.lastLogNameTime = now
}

func (n *nameProgressLogger) SetLastLogTime(time time.Time) {
	n.lastLogNameTime = time
}
