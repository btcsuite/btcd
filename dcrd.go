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

	"github.com/decred/dcrd/limits"
)

var (
	cfg             *config
	shutdownChannel = make(chan struct{})
)

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

	// Show version at startup.
	dcrdLog.Infof("Version %s", version())
	// Show dcrd home dir location
	dcrdLog.Debugf("Dcrd home dir: %s", cfg.DcrdHomeDir)

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

	// Perform upgrades to dcrd as new versions require it.
	if err := doUpgrades(); err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}
	defer db.Close()

	if cfg.DropAddrIndex {
		/*
			TODO New address index
			dcrdLog.Info("Deleting entire addrindex.")
			err := db.PurgeAddrIndex()
			if err != nil {
				dcrdLog.Errorf("Unable to delete the addrindex: %v", err)
				return err
			}
			dcrdLog.Info("Successfully deleted addrindex, exiting")
			return nil
		*/
	}

	tmdb, err := loadTicketDB(db, activeNetParams.Params)
	if err != nil {
		dcrdLog.Errorf("%v", err)
		return err
	}
	defer func() {
		err := tmdb.Store(cfg.DataDir, "ticketdb.gob")
		if err != nil {
			dcrdLog.Errorf("Failed to store ticket database: %v", err.Error())
		}
	}()
	defer tmdb.Close()

	// Ensure the databases are sync'd and closed on Ctrl+C.
	addInterruptHandler(func() {
		dcrdLog.Infof("Gracefully shutting down the database...")
		db.Close()
	})

	// Create server and start it.
	server, err := newServer(cfg.Listeners, db, tmdb, activeNetParams.Params)
	if err != nil {
		// TODO(oga) this logging could do with some beautifying.
		dcrdLog.Errorf("Unable to start server on %v: %v",
			cfg.Listeners, err)
		return err
	}
	addInterruptHandler(func() {
		dcrdLog.Infof("Gracefully shutting down the server...")
		server.Stop()
		server.WaitForShutdown()
	})
	server.Start()
	if serverChan != nil {
		serverChan <- server
	}

	// Monitor for graceful server shutdown and signal the main goroutine
	// when done.  This is done in a separate goroutine rather than waiting
	// directly so the main goroutine can be signaled for shutdown by either
	// a graceful shutdown or from the main interrupt handler.  This is
	// necessary since the main goroutine must be kept running long enough
	// for the interrupt handler goroutine to finish.
	go func() {
		server.WaitForShutdown()
		srvrLog.Infof("Server shutdown complete")
		shutdownChannel <- struct{}{}
	}()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-shutdownChannel
	dcrdLog.Info("Shutdown complete")
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
