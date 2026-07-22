// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package inbound

import "github.com/btcsuite/btclog"

// log is initialized with no output filters. This means the package will not
// perform any logging until the caller requests it.
var log btclog.Logger

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all package log output.
func DisableLog() {
	log = btclog.Disabled
}

// UseLogger uses the specified logger for package output.
func UseLogger(logger btclog.Logger) {
	log = logger
}
