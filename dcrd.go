// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"github.com/decred/dcrd/blockchain/indexers"
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
	defer func() {
		if logRotator != nil {
			logRotator.Close()
		}
	}()

	// Get a channel that will be closed when a shutdown signal has been
	// triggered either from an OS signal such as SIGINT (Ctrl+C) or from
	// another subsystem such as the RPC server.
	interrupt := interruptListener()
	defer dcrdLog.Info("Shutdown complete")

	// Show version and home dir at startup.
	dcrdLog.Infof("Version %s (Go version %s)", version(), runtime.Version())
	dcrdLog.Infof("Home dir: %s", cfg.HomeDir)
	if cfg.NoFileLogging {
		dcrdLog.Info("File logging disabled")
	}

	// Enable http profiling server if requested.
	if cfg.Profile != "" {
		go func() {
			listenAddr := cfg.Profile
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

	// Return now if an interrupt signal was triggered.
	if interruptRequested(interrupt) {
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
		// Ensure the database is sync'd and closed on shutdown.
		lifetimeNotifier.notifyShutdownEvent(lifetimeEventDBOpen)
		dcrdLog.Infof("Gracefully shutting down the database...")
		db.Close()
	}()

	// Return now if an interrupt signal was triggered.
	if interruptRequested(interrupt) {
		return nil
	}

	// Drop indexes and exit if requested.
	//
	// NOTE: The order is important here because dropping the tx index also
	// drops the address index since it relies on it.
	if cfg.DropAddrIndex {
		if err := indexers.DropAddrIndex(db, interrupt); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropTxIndex {
		if err := indexers.DropTxIndex(db, interrupt); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}
	if cfg.DropExistsAddrIndex {
		if err := indexers.DropExistsAddrIndex(db, interrupt); err != nil {
			dcrdLog.Errorf("%v", err)
			return err
		}

		return nil
	}

	// Create server and start it.
	lifetimeNotifier.notifyStartupEvent(lifetimeEventP2PServer)
	server, err := newServer(cfg.Listeners, db, activeNetParams.Params,
		interrupt)
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

	if interruptRequested(interrupt) {
		return nil
	}

	lifetimeNotifier.notifyStartupComplete()

	// Wait until the interrupt signal is received from an OS signal or
	// shutdown is requested through one of the subsystems such as the RPC
	// server.
	<-interrupt
	return nil
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Block and transaction processing can cause bursty allocations.  This
	// limits the garbage collector from excessively overallocating during
	// bursts.  This value was arrived at with the help of profiling live
	// usage.
	debug.SetGCPercent(20)

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
