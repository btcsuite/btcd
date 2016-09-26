// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"github.com/decred/dcrd/blockchain/indexers"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/limits"
)

var cfg *config

// winServiceMain is only invoked on Windows.  It detects when dcrd is running
// as a service and reacts accordingly.
var winServiceMain func() (bool, error)

// dcrdMain is the real main function for dcrd.  It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called.  The
// optional serverChan parameter is mainly used by the service code to be
// notified with the server once it is setup so it can gracefully stop it when
// requested from the service control manager.
func dcrdMain(serverChan chan<- *server) error {
	// Load configuration and parse command line.  This function also
	// initializes logging and configures it accordingly.
	tcfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	cfg = tcfg
	defer backendLog.Flush()

	interrupted := interruptListener()
	defer dcrdLog.Info("Shutdown complete")

	// Show version at startup.
	dcrdLog.Infof("Version %s", version())
	// Show dcrd home dir location
	dcrdLog.Infof("Home dir: %s", cfg.HomeDir)

	// Enable http profiling server if requested.
	if cfg.Profile != "" {
		go func() {
			listenAddr := net.JoinHostPort("", cfg.Profile)
			dcrdLog.Infof("Creating profiling server "+
				"listening on %s", listenAddr)
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			err := http.ListenAndServe(listenAddr, nil)
			if err != nil {
				fatalf(err.Error())
			}
		}()
	}

	// Write cpu profile if requested.
	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			dcrdLog.Errorf("Unable to create cpu profile: %v", err.Error())
			return err
		}
		pprof.StartCPUProfile(f)
		defer f.Close()
		defer pprof.StopCPUProfile()
	}

	// Write mem profile if requested.
	if cfg.MemProfile != "" {
		f, err := os.Create(cfg.MemProfile)
		if err != nil {
			dcrdLog.Errorf("Unable to create cpu profile: %v", err)
			return err
		}
		timer := time.NewTimer(time.Minute * 20) // 20 minutes
		go func() {
			<-timer.C
			pprof.WriteHeapProfile(f)
			f.Close()
		}()
	}

	var lifetimeNotifier lifetimeEventServer
	if cfg.LifetimeEvents {
		lifetimeNotifier = newLifetimeEventServer(outgoingPipeMessages)
	}

	if cfg.PipeRx != 0 {
		go serviceControlPipeRx(uintptr(cfg.PipeRx))
	}
	if cfg.PipeTx != 0 {
		go serviceControlPipeTx(uintptr(cfg.PipeTx))
	} else {
		go drainOutgoingPipeMessages()
	}

	if interruptRequested(interrupted) {
		return nil
	}

	// Perform upgrades to dcrd as new versions require it.
	if err := doUpgrades(); err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}

	if interruptRequested(interrupted) {
		return nil
	}

	// Load the block database.
	lifetimeNotifier.notifyStartupEvent(lifetimeEventDBOpen)
	db, err := loadBlockDB()
	if err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}
	defer func() {
		lifetimeNotifier.notifyShutdownEvent(lifetimeEventDBOpen)
		dcrdLog.Infof("Gracefully shutting down the database...")
		db.Close()
	}()

	if interruptRequested(interrupted) {
		return nil
	}

	// Drop indexes and exit if requested.
	//
	// NOTE: The order is important here because dropping the tx index also
	// drops the address index since it relies on it.
	if cfg.DropAddrIndex {
		if err := indexers.DropAddrIndex(db); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropTxIndex {
		if err := indexers.DropTxIndex(db); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropExistsAddrIndex {
		if err := indexers.DropExistsAddrIndex(db); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}

	// The ticket "DB" takes ages to load and serialize back out to a file.
	// Load it asynchronously and if the process is interrupted during the
	// load, discard the result since no cleanup is necessary.
	lifetimeNotifier.notifyStartupEvent(lifetimeEventTicketDB)
	type ticketDBResult struct {
		ticketDB *stake.TicketDB
		err      error
	}
	ticketDBResultChan := make(chan ticketDBResult)
	go func() {
		tmdb, err := loadTicketDB(db, activeNetParams.Params)
		ticketDBResultChan <- ticketDBResult{tmdb, err}
	}()
	var tmdb *stake.TicketDB
	select {
	case <-interrupted:
		return nil
	case r := <-ticketDBResultChan:
		if r.err != nil {
			dcrdLog.Errorf("%v", r.err)
			return r.err
		}
		tmdb = r.ticketDB
	}
	defer func() {
		lifetimeNotifier.notifyShutdownEvent(lifetimeEventTicketDB)
		tmdb.Close()
		err := tmdb.Store(cfg.DataDir, "ticketdb.gob")
		if err != nil {
			dcrdLog.Errorf("Failed to store ticket database: %v", err.Error())
		}
	}()

	// Create server and start it.
	lifetimeNotifier.notifyStartupEvent(lifetimeEventP2PServer)
	server, err := newServer(cfg.Listeners, db, tmdb, activeNetParams.Params)
	if err != nil {
		// TODO(oga) this logging could do with some beautifying.
		dcrdLog.Errorf("Unable to start server on %v: %v",
			cfg.Listeners, err)
		return err
	}
	defer func() {
		lifetimeNotifier.notifyShutdownEvent(lifetimeEventP2PServer)
		dcrdLog.Infof("Gracefully shutting down the server...")
		server.Stop()
		server.WaitForShutdown()
		srvrLog.Infof("Server shutdown complete")
	}()

	server.Start()
	if serverChan != nil {
		serverChan <- server
	}

	if interruptRequested(interrupted) {
		return nil
	}

	lifetimeNotifier.notifyStartupComplete()

	// Wait until the interrupt signal is received from an OS signal or
	// shutdown is requested through the RPC server.
	<-interrupted
	return nil
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Block and transaction processing can cause bursty allocations.  This
	// limits the garbage collector from excessively overallocating during
	// bursts.  This value was arrived at with the help of profiling live
	// usage.
	debug.SetGCPercent(10)

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set limits: %v\n", err)
		os.Exit(1)
	}

	// Call serviceMain on Windows to handle running as a service.  When
	// the return isService flag is true, exit now since we ran as a
	// service.  Otherwise, just fall through to normal operation.
	if runtime.GOOS == "windows" {
		isService, err := winServiceMain()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if isService {
			os.Exit(0)
		}
	}

	// Work around defer not working after os.Exit()
	if err := dcrdMain(nil); err != nil {
		os.Exit(1)
	}
}
