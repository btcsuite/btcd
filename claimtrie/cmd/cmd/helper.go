package cmd

import (
	"path/filepath"
	"time"

	"github.com/lbryio/lbcd/blockchain"
	"github.com/lbryio/lbcd/chaincfg"
	"github.com/lbryio/lbcd/database"
	"github.com/lbryio/lbcd/txscript"

	"github.com/cockroachdb/errors"
)

func loadBlocksDB() (database.DB, error) {

	dbPath := filepath.Join(dataDir, netName, "blocks_ffldb")
	log.Infof("Loading blocks database: %s", dbPath)
	db, err := database.Open("ffldb", dbPath, chainPramas().Net)
	if err != nil {
		return nil, errors.Wrapf(err, "open blocks database")
	}

	return db, nil
}

func loadChain(db database.DB) (*blockchain.BlockChain, error) {
	paramsCopy := chaincfg.MainNetParams

	log.Infof("Loading chain from database")

	startTime := time.Now()
	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: &paramsCopy,
		TimeSource:  blockchain.NewMedianTime(),
		SigCache:    txscript.NewSigCache(1000),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "create blockchain")
	}

	log.Infof("Loaded chain from database (%s)", time.Since(startTime))

	return chain, err

}

func chainPramas() chaincfg.Params {

	// Make a copy so the user won't modify the global instance.
	params := chaincfg.MainNetParams
	switch netName {
	case "mainnet":
		params = chaincfg.MainNetParams
	case "testnet":
		params = chaincfg.TestNet3Params
	case "regtest":
		params = chaincfg.RegressionNetParams
	}
	return params
}
