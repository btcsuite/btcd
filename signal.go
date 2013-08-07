// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"os/signal"
)

// interruptChannel is used to receive SIGINT (Ctrl+C) signals.
var interruptChannel chan os.Signal

// interruptCallbacks is a list of callbacks to invoke when a SIGINT (Ctrl+C) is
// received.
var interruptCallbacks []func()

// addInterruptHandler adds a handler to call when a SIGINT (Ctrl+C) is
// received.
func addInterruptHandler(handler func()) {
	// Create the channel and start the main interrupt handler which invokes
	// all other callbacks and exits if not already done.
	if interruptChannel == nil {
		interruptChannel = make(chan os.Signal, 1)
		signal.Notify(interruptChannel, os.Interrupt)
		go func() {
			<-interruptChannel
			for _, callback := range interruptCallbacks {
				callback()
			}
			os.Exit(0)
		}()
	}
	interruptCallbacks = append(interruptCallbacks, handler)
}
