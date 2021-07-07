package config

import (
	"path/filepath"

	"github.com/lbryio/lbcd/claimtrie/param"
	btcutil "github.com/lbryio/lbcutil"
)

var DefaultConfig = Config{
	Params: param.MainNet,

	RamTrie: true, // as it stands the other trie uses more RAM, more time, and 40GB+ of disk space

	DataDir: filepath.Join(btcutil.AppDataDir("chain", false), "data"),

	BlockRepoPebble: pebbleConfig{
		Path: "blocks_pebble_db",
	},
	NodeRepoPebble: pebbleConfig{
		Path: "node_change_pebble_db",
	},
	TemporalRepoPebble: pebbleConfig{
		Path: "temporal_pebble_db",
	},
	MerkleTrieRepoPebble: pebbleConfig{
		Path: "merkletrie_pebble_db",
	},
}

// Config is the container of all configurations.
type Config struct {
	Params param.ClaimTrieParams

	RamTrie bool

	DataDir string

	BlockRepoPebble      pebbleConfig
	NodeRepoPebble       pebbleConfig
	TemporalRepoPebble   pebbleConfig
	MerkleTrieRepoPebble pebbleConfig

	Interrupt <-chan struct{}
}

type pebbleConfig struct {
	Path string
}
