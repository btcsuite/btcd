// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	_ "github.com/conformal/btcdb/ldb"
	_ "github.com/conformal/btcdb/sqlite3"
	"github.com/conformal/btcwire"
	"github.com/conformal/go-flags"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultConfigFilename = "btcd.conf"
	defaultLogLevel       = "info"
	defaultBtcnet         = btcwire.MainNet
	defaultMaxPeers       = 8
	defaultBanDuration    = time.Hour * 24
	defaultVerifyEnabled  = false
)

var (
	defaultConfigFile = filepath.Join(btcdHomeDir(), defaultConfigFilename)
	defaultDbDir      = filepath.Join(btcdHomeDir(), "db")
)

// config defines the configuration options for btcd.
//
// See loadConfig for details on the configuration load process.
type config struct {
	ShowVersion    bool          `short:"V" long:"version" description:"Display version information and exit"`
	ConfigFile     string        `short:"C" long:"configfile" description:"Path to configuration file"`
	DbDir          string        `short:"b" long:"dbdir" description:"Directory to store database"`
	AddPeers       []string      `short:"a" long:"addpeer" description:"Add a peer to connect with at startup"`
	ConnectPeers   []string      `long:"connect" description:"Connect only to the specified peers at startup"`
	SeedPeer       string        `short:"s" long:"seedpeer" description:"Retrieve peer addresses from this peer and then disconnect"`
	DisableListen  bool          `long:"nolisten" description:"Disable listening for incoming connections -- NOTE: Listening is automatically disabled if the --connect option is used or if the --proxy option is used without the --tor option"`
	Port           string        `short:"p" long:"port" description:"Listen for connections on this port (default: 8333, testnet: 18333)"`
	MaxPeers       int           `long:"maxpeers" description:"Max number of inbound and outbound peers"`
	BanDuration    time.Duration `long:"banduration" description:"How long to ban misbehaving peers.  Valid time units are {s, m, h}.  Minimum 1 second"`
	VerifyDisabled bool          `long:"noverify" description:"Disable block/transaction verification -- WARNING: This option can be dangerous and is for development use only"`
	RpcUser        string        `short:"u" long:"rpcuser" description:"Username for RPC connections"`
	RpcPass        string        `short:"P" long:"rpcpass" description:"Password for RPC connections"`
	RpcPort        string        `short:"r" long:"rpcport" description:"Listen for JSON/RPC messages on this port"`
	DisableRpc     bool          `long:"norpc" description:"Disable built-in RPC server -- NOTE: The RPC server is disabled by default if no rpcuser/rpcpass is specified"`
	DisableDNSSeed bool          `long:"nodnsseed" description:"Disable DNS seeding for peers"`
	Proxy          string        `long:"proxy" description:"Connect via SOCKS5 proxy (eg. 127.0.0.1:9050)"`
	ProxyUser      string        `long:"proxyuser" description:"Username for proxy server"`
	ProxyPass      string        `long:"proxypass" description:"Password for proxy server"`
	UseTor         bool          `long:"tor" description:"Specifies the proxy server used is a Tor node"`
	TestNet3       bool          `long:"testnet" description:"Use the test network"`
	RegressionTest bool          `long:"regtest" description:"Use the regression test network"`
	DbType	       string         `long:"dbtype" description:"DB backend to use for Block Chain"`
	DebugLevel     string        `short:"d" long:"debuglevel" description:"Logging level {trace, debug, info, warn, error, critical}"`
}

// btcdHomeDir returns an OS appropriate home directory for btcd.
func btcdHomeDir() string {
	// Search for Windows APPDATA first.  This won't exist on POSIX OSes.
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "btcd")
	}

	// Fall back to standard HOME directory that works for most POSIX OSes.
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".btcd")
	}

	// In the worst case, use the current directory.
	return "."
}

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	switch logLevel {
	case "trace":
		fallthrough
	case "debug":
		fallthrough
	case "info":
		fallthrough
	case "warn":
		fallthrough
	case "error":
		fallthrough
	case "critical":
		return true
	}
	return false
}

// normalizePeerAddress returns addr with the default peer port appended if
// there is not already a port specified.
func normalizePeerAddress(addr string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, activeNetParams.peerPort)
	}
	return addr
}

// removeDuplicateAddresses returns a new slice with all duplicate entries in
// addrs removed.
func removeDuplicateAddresses(addrs []string) []string {
	result := make([]string, 0)
	seen := map[string]bool{}
	for _, val := range addrs {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = true
		}
	}
	return result
}

// normalizeAndRemoveDuplicateAddresses return a new slice with all the passed
// addresses normalized and duplicates removed.
func normalizeAndRemoveDuplicateAddresses(addrs []string) []string {
	for i, addr := range addrs {
		addrs[i] = normalizePeerAddress(addr)
	}
	addrs = removeDuplicateAddresses(addrs)

	return addrs
}

// updateConfigWithActiveParams update the passed config with parameters
// from the active net params if the relevant options in the passed config
// object are the default so options specified by the user on the command line
// are not overridden.
func updateConfigWithActiveParams(cfg *config) {
	if cfg.Port == netParams(defaultBtcnet).listenPort {
		cfg.Port = activeNetParams.listenPort
	}

	if cfg.RpcPort == netParams(defaultBtcnet).rpcPort {
		cfg.RpcPort = activeNetParams.rpcPort
	}
}

// filesExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
//
// The above results in btcd functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DebugLevel:  defaultLogLevel,
		Port:        netParams(defaultBtcnet).listenPort,
		RpcPort:     netParams(defaultBtcnet).rpcPort,
		MaxPeers:    defaultMaxPeers,
		BanDuration: defaultBanDuration,
		ConfigFile:  defaultConfigFile,
		DbDir:       defaultDbDir,
	}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.
	preCfg := cfg
	preParser := flags.NewParser(&preCfg, flags.Default)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			preParser.WriteHelp(os.Stderr)
		}
		return nil, nil, err
	}

	// Show the version and exit if the version flag was specified.
	if preCfg.ShowVersion {
		appName := filepath.Base(os.Args[0])
		appName = strings.TrimSuffix(appName, filepath.Ext(appName))
		fmt.Println(appName, "version", version())
		os.Exit(0)
	}

	// Load additional config from file.
	parser := flags.NewParser(&cfg, flags.Default)
	err = parser.ParseIniFile(preCfg.ConfigFile)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			fmt.Fprintln(os.Stderr, err)
			parser.WriteHelp(os.Stderr)
			return nil, nil, err
		}
		log.Warnf("%v", err)
	}

	// Don't add peers from the config file when in regression test mode.
	if preCfg.RegressionTest && len(cfg.AddPeers) > 0 {
		cfg.AddPeers = nil
	}

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			parser.WriteHelp(os.Stderr)
		}
		return nil, nil, err
	}

	// The two test networks can't be selected simultaneously.
	if cfg.TestNet3 && cfg.RegressionTest {
		str := "%s: The testnet and regtest params can't be used " +
			"together -- choose one of the two"
		err := errors.New(fmt.Sprintf(str, "loadConfig"))
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Choose the active network params based on the testnet and regression
	// test net flags.
	if cfg.TestNet3 {
		activeNetParams = netParams(btcwire.TestNet3)
	} else if cfg.RegressionTest {
		activeNetParams = netParams(btcwire.TestNet)
	}
	updateConfigWithActiveParams(&cfg)

	// Validate debug log level.
	if !validLogLevel(cfg.DebugLevel) {
		str := "%s: The specified debug level is invalid -- parsed [%v]"
		err := errors.New(fmt.Sprintf(str, "loadConfig", cfg.DebugLevel))
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Don't allow ban durations that are too short.
	if cfg.BanDuration < time.Duration(time.Second) {
		str := "%s: The banduration option may not be less than 1s -- parsed [%v]"
		err := errors.New(fmt.Sprintf(str, "loadConfig", cfg.BanDuration))
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// --addPeer and --connect do not mix.
	if len(cfg.AddPeers) > 0 && len(cfg.ConnectPeers) > 0 {
		str := "%s: the --addpeer and --connect options can not be " +
			"mixed"
		err := errors.New(fmt.Sprintf(str, "loadConfig"))
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// --tor requires --proxy to be set.
	if cfg.UseTor && cfg.Proxy == "" {
		str := "%s: the --tor option requires --proxy to be set"
		err := errors.New(fmt.Sprintf(str, "loadConfig"))
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// --proxy without --tor means no listening.
	if cfg.Proxy != "" && !cfg.UseTor {
		cfg.DisableListen = true
	}

	// Connect means no seeding or listening.
	if len(cfg.ConnectPeers) > 0 {
		cfg.DisableDNSSeed = true
		cfg.DisableListen = true
	}

	// The RPC server is disabled if no username or password is provided.
	if cfg.RpcUser == "" || cfg.RpcPass == "" {
		cfg.DisableRpc = true
	}

	// Add default port to all added peer addresses if needed and remove
	// duplicate addresses.
	cfg.AddPeers = normalizeAndRemoveDuplicateAddresses(cfg.AddPeers)
	cfg.ConnectPeers =
		normalizeAndRemoveDuplicateAddresses(cfg.ConnectPeers)

	// determine which database backend to use
	// would be interesting of btcdb had a 'supportedDBs() []string'
	// API to populate this field.
	knownDbs := []string{"leveldb", "sqlite"}
	defaultDb := "sqlite"

	if len(cfg.DbType) == 0 {
		// if db was not specified, use heuristic to see what
		// type the database is (if the specified database is a
		// file then it is sqlite3, if it is a directory assume
		// it is leveldb, if not found default to type _____
		dbPath := filepath.Join(cfg.DbDir, activeNetParams.dbName)

		fi, err := os.Stat(dbPath)
		if err == nil {
			if fi.IsDir() {
				cfg.DbType = "leveldb"
			} else {
				cfg.DbType = "sqlite"
			}
		} else {
			cfg.DbType = defaultDb
		}
	} else {
		// does checking this here really make sense ?
		typeVerified := false
		for _, dbtype := range knownDbs {
			if cfg.DbType == dbtype {
				typeVerified = true
			}
		}
		if typeVerified == false {
			err := fmt.Errorf("Specified database type [%v] not in list of supported databases: %v", cfg.DbType, knownDbs)
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
	}
	return &cfg, remainingArgs, nil
}
