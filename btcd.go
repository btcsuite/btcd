// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
)

var (
	cfg             *config
	shutdownChannel = make(chan bool)
)

// btcdMain is the real main function for btcd.  It is necessary to work around
// the fact that deferred functions do not run when os.Exit() is called.
func btcdMain() error {
	// Initialize logging and setup deferred flushing to ensure all
	// outstanding messages are written on shutdown.
	loggers := setLogLevel(defaultLogLevel)
	defer func() {
		for _, logger := range loggers {
			logger.Flush()
		}
	}()

	// Load configuration and parse command line.
	tcfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	cfg = tcfg

	// Change the logging level if needed.
	if cfg.DebugLevel != defaultLogLevel {
		loggers = setLogLevel(cfg.DebugLevel)
	}

	// Show version at startup.
	log.Infof("Version %s", version())

	// Enable http profiling server if requested.
	if cfg.Profile != "" {
		go func() {
			listenAddr := net.JoinHostPort("", cfg.Profile)
			log.Infof("Profile server listening on %s", listenAddr)
			profileRedirect := http.RedirectHandler("/debug/pprof",
				http.StatusSeeOther)
			http.Handle("/", profileRedirect)
			log.Errorf("%v", http.ListenAndServe(listenAddr, nil))
		}()
	}

	// Write cpu profile if requested.
	if cfg.CpuProfile != "" {
		f, err := os.Create(cfg.CpuProfile)
		if err != nil {
			log.Errorf("Unable to create cpu profile: %v", err)
			return err
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Perform upgrades to btcd as new versions require it.
	if err := doUpgrades(); err != nil {
		log.Errorf("%v", err)
		return err
	}

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		log.Errorf("%v", err)
		return err
	}
	defer db.Close()

	// Ensure the database is sync'd and closed on Ctrl+C.
	addInterruptHandler(func() {
		log.Infof("Gracefully shutting down the database...")
		db.RollbackClose()
	})

	// Create server and start it.
	listenAddr := net.JoinHostPort("", cfg.Port)
	server, err := newServer(listenAddr, db, activeNetParams.btcnet)
	if err != nil {
		log.Errorf("Unable to start server on %v: %v", listenAddr, err)
		return err
	}
	server.Start()

	// Monitor for graceful server shutdown and signal the main goroutine
	// when done.  This is done in a separate goroutine rather than waiting
	// directly so the main goroutine can be signaled for shutdown by either
	// a graceful shutdown or from the main interrupt handler.  This is
	// necessary since the main goroutine must be kept running long enough
	// for the interrupt handler goroutine to finish.
	go func() {
		server.WaitForShutdown()
		shutdownChannel <- true
	}()

	// Wait for shutdown signal from either a graceful server stop or from
	// the interrupt handler.
	<-shutdownChannel
	log.Info("Shutdown complete")
	return nil
}

func main() {
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Up some limits.
	if err := setLimits(); err != nil {
		os.Exit(1)
	}

	// Work around defer not working after os.Exit()
	if err := btcdMain(); err != nil {
		os.Exit(1)
	}
}
