package main

import (
	"os"

	"github.com/btcsuite/btclog"
)

var log btclog.Logger

func init() {
	backendLogger := btclog.NewBackend(os.Stdout)
	log = backendLogger.Logger("GSBT")
	log.SetLevel(btclog.LevelInfo)
}
