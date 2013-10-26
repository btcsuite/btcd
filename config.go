// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	"github.com/conformal/btcwire"
	"github.com/conformal/go-flags"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConfigFilename = "btcd.conf"
	defaultLogLevel       = "info"
	defaultBtcnet         = btcwire.MainNet
	defaultMaxPeers       = 125
	defaultBanDuration    = time.Hour * 24
	defaultVerifyEnabled  = false
	defaultDbType         = "leveldb"
)

var (
	defaultConfigFile = filepath.Join(btcdHomeDir(), defaultConfigFilename)
	defaultDataDir    = filepath.Join(btcdHomeDir(), "data")
	knownDbTypes      = btcdb.SupportedDBs()
)

// config defines the configuration options for btcd.
//
// See loadConfig for details on the configuration load process.
type config struct {
	ShowVersion        bool          `short:"V" long:"version" description:"Display version information and exit"`
	ConfigFile         string        `short:"C" long:"configfile" description:"Path to configuration file"`
	DataDir            string        `short:"b" long:"datadir" description:"Directory to store data"`
	AddPeers           []string      `short:"a" long:"addpeer" description:"Add a peer to connect with at startup"`
	ConnectPeers       []string      `long:"connect" description:"Connect only to the specified peers at startup"`
	DisableListen      bool          `long:"nolisten" description:"Disable listening for incoming connections -- NOTE: Listening is automatically disabled if the --connect option is used or if the --proxy option is used without the --tor option"`
	Port               string        `short:"p" long:"port" description:"Listen for connections on this port (default: 8333, testnet: 18333)"`
	MaxPeers           int           `long:"maxpeers" description:"Max number of inbound and outbound peers"`
	BanDuration        time.Duration `long:"banduration" description:"How long to ban misbehaving peers.  Valid time units are {s, m, h}.  Minimum 1 second"`
	RPCUser            string        `short:"u" long:"rpcuser" description:"Username for RPC connections"`
	RPCPass            string        `short:"P" long:"rpcpass" description:"Password for RPC connections"`
	RPCPort            string        `short:"r" long:"rpcport" description:"Listen for JSON/RPC messages on this port"`
	DisableRPC         bool          `long:"norpc" description:"Disable built-in RPC server -- NOTE: The RPC server is disabled by default if no rpcuser/rpcpass is specified"`
	DisableDNSSeed     bool          `long:"nodnsseed" description:"Disable DNS seeding for peers"`
	Proxy              string        `long:"proxy" description:"Connect via SOCKS5 proxy (eg. 127.0.0.1:9050)"`
	ProxyUser          string        `long:"proxyuser" description:"Username for proxy server"`
	ProxyPass          string        `long:"proxypass" description:"Password for proxy server"`
	UseTor             bool          `long:"tor" description:"Specifies the proxy server used is a Tor node"`
	TestNet3           bool          `long:"testnet" description:"Use the test network"`
	RegressionTest     bool          `long:"regtest" description:"Use the regression test network"`
	DisableCheckpoints bool          `long:"nocheckpoints" description:"Disable built-in checkpoints.  Don't do this unless you know what you're doing."`
	DbType             string        `long:"dbtype" description:"Database backend to use for the Block Chain"`
	Profile            string        `long:"profile" description:"Enable HTTP profiling on given port -- NOTE port must be between 1024 and 65536"`
	DebugLevel         string        `short:"d" long:"debuglevel" description:"Logging level {trace, debug, info, warn, error, critical}"`
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

// cleanAndExpandPath expands environement variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		homeDir := filepath.Dir(btcdHomeDir())
		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but they variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
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

// validDbType returns whether or not dbType is a supported database type.
func validDbType(dbType string) bool {
	for _, knownType := range knownDbTypes {
		if dbType == knownType {
			return true
		}
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

	if cfg.RPCPort == netParams(defaultBtcnet).rpcPort {
		cfg.RPCPort = activeNetParams.rpcPort
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
		RPCPort:     netParams(defaultBtcnet).rpcPort,
		MaxPeers:    defaultMaxPeers,
		BanDuration: defaultBanDuration,
		ConfigFile:  defaultConfigFile,
		DataDir:     defaultDataDir,
		DbType:      defaultDbType,
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
		err := fmt.Errorf(str, "loadConfig")
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
		str := "%s: The specified debug level [%v] is invalid"
		err := fmt.Errorf(str, "loadConfig", cfg.DebugLevel)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Validate database type.
	if !validDbType(cfg.DbType) {
		str := "%s: The specified database type [%v] is invalid -- " +
			"supported types %v"
		err := fmt.Errorf(str, "loadConfig", cfg.DbType, knownDbTypes)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Validate profile port number
	if cfg.Profile != "" {
		profilePort, err := strconv.Atoi(cfg.Profile)
		if err != nil || profilePort < 1024 || profilePort > 65535 {
			str := "%s: The profile port must be between 1024 and 65535"
			err := fmt.Errorf(str, "loadConfig")
			fmt.Fprintln(os.Stderr, err)
			parser.WriteHelp(os.Stderr)
			return nil, nil, err
		}
	}

	// Append the network type to the data directory so it is "namespaced"
	// per network.  In addition to the block database, there are other
	// pieces of data that are saved to disk such as address manager state.
	// All data is specific to a network, so namespacing the data directory
	// means each individual piece of serialized data does not have to
	// worry about changing names per network and such.
	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	cfg.DataDir = filepath.Join(cfg.DataDir, activeNetParams.netName)

	// Don't allow ban durations that are too short.
	if cfg.BanDuration < time.Duration(time.Second) {
		str := "%s: The banduration option may not be less than 1s -- parsed [%v]"
		err := fmt.Errorf(str, "loadConfig", cfg.BanDuration)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// --addPeer and --connect do not mix.
	if len(cfg.AddPeers) > 0 && len(cfg.ConnectPeers) > 0 {
		str := "%s: the --addpeer and --connect options can not be " +
			"mixed"
		err := fmt.Errorf(str, "loadConfig")
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// --tor requires --proxy to be set.
	if cfg.UseTor && cfg.Proxy == "" {
		str := "%s: the --tor option requires --proxy to be set"
		err := fmt.Errorf(str, "loadConfig")
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
	if cfg.RPCUser == "" || cfg.RPCPass == "" {
		cfg.DisableRPC = true
	}

	// Add default port to all added peer addresses if needed and remove
	// duplicate addresses.
	cfg.AddPeers = normalizeAndRemoveDuplicateAddresses(cfg.AddPeers)
	cfg.ConnectPeers = normalizeAndRemoveDuplicateAddresses(cfg.ConnectPeers)

	return &cfg, remainingArgs, nil
}
