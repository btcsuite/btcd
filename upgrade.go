// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
)

var ticketDBName = "ticketdb.gob"
var oldTicketDBName = "ticketdb_old.gob"

// checkForAndMoveOldTicketDb checks for an old copy of the ticket database
// from before ffldb was introduced and renames it in preparation for resyncing
// the blockchain.
func checkForAndMoveOldTicketDb() error {
	ffldbPath := filepath.Join(cfg.DcrdHomeDir, defaultDataDirname,
		activeNetParams.Name, blockDbNamePrefix+"_"+defaultDbType)

	if _, err := os.Stat(ffldbPath); os.IsNotExist(err) {
		// Rename the old ticket database.
		ticketDBPath := filepath.Join(cfg.DcrdHomeDir, defaultDataDirname,
			activeNetParams.Name, ticketDBName)
		if _, err := os.Stat(ticketDBPath); !os.IsNotExist(err) && err != nil {
			return err
		}

		dcrdLog.Warnf("The old ticket database file has been named " +
			oldTicketDBName + ". It can be safely removed if you " +
			"no longer wish to roll back to an old version of the " +
			"software.")
		oldTicketDBPath := filepath.Join(cfg.DcrdHomeDir, defaultDataDirname,
			activeNetParams.Name, oldTicketDBName)
		err = os.Rename(ticketDBPath, oldTicketDBPath)
		if err != nil {
			return err
		}
	}

	return nil
}

// doUpgrades performs upgrades to dcrd as new versions require it.
func doUpgrades() error {
	return checkForAndMoveOldTicketDb()
}
